package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	apphttp "github.com/lucaasnogueira/buriti-pay/internal/http"
	"github.com/lucaasnogueira/buriti-pay/internal/worker"
)

type mockPaymentService struct {
	payments      map[string]*domain.Payment
	simulateQueueFull bool
}

func newMockPaymentService() *mockPaymentService {
	return &mockPaymentService{
		payments: make(map[string]*domain.Payment),
	}
}

func (m *mockPaymentService) CreatePaymentAsync(
	ctx context.Context,
	idempotencyKey string,
	fromID, toID uuid.UUID,
	amount int64,
) (*domain.Payment, error) {
	if m.simulateQueueFull {
		return nil, worker.ErrQueueFull
	}

	if existing, ok := m.payments[idempotencyKey]; ok {
		if !existing.MatchesPayload(fromID, toID, amount) {
			return nil, domain.ErrIdempotencyConflict
		}
		return existing, nil
	}

	p, err := domain.NewPayment(idempotencyKey, fromID, toID, amount)
	if err != nil {
		return nil, err
	}
	m.payments[idempotencyKey] = p
	return p, nil
}

func (m *mockPaymentService) Process(ctx context.Context, job worker.Job) error {
	return nil
}

func (m *mockPaymentService) GetPayment(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	for _, p := range m.payments {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, domain.ErrPaymentNotFound
}

func TestPaymentHandler_AsyncIngestionAndBackpressure(t *testing.T) {
	svc := newMockPaymentService()
	handler := apphttp.NewPaymentHandler(svc)

	fromID := uuid.New()
	toID := uuid.New()

	t.Run("require Idempotency-Key header", func(t *testing.T) {
		body := `{"from_account_id":"` + fromID.String() + `","to_account_id":"` + toID.String() + `","amount":1500}`
		req := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()

		handler.CreatePayment(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 when Idempotency-Key is missing, got %d", rec.Code)
		}
	})

	t.Run("successful async payment ingestion returns 202 Accepted with PENDING", func(t *testing.T) {
		body := `{"from_account_id":"` + fromID.String() + `","to_account_id":"` + toID.String() + `","amount":1500}`
		req := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBufferString(body))
		req.Header.Set("Idempotency-Key", "key-async-1")
		rec := httptest.NewRecorder()

		handler.CreatePayment(rec, req)

		if rec.Code != http.StatusAccepted {
			t.Fatalf("expected 202 Accepted, got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var resp apphttp.PaymentResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Status != "PENDING" || resp.Amount != 1500 {
			t.Errorf("unexpected payment response: %+v", resp)
		}
	})

	t.Run("conflict 409 when reusing idempotency key with different payload", func(t *testing.T) {
		body := `{"from_account_id":"` + fromID.String() + `","to_account_id":"` + toID.String() + `","amount":9999}`
		req := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBufferString(body))
		req.Header.Set("Idempotency-Key", "key-async-1") // reused key with 9999 instead of 1500
		rec := httptest.NewRecorder()

		handler.CreatePayment(rec, req)

		if rec.Code != http.StatusConflict {
			t.Errorf("expected 409 Conflict, got %d", rec.Code)
		}
	})

	t.Run("backpressure 429 Too Many Requests with Retry-After header when queue is full", func(t *testing.T) {
		svc.simulateQueueFull = true
		defer func() { svc.simulateQueueFull = false }()

		body := `{"from_account_id":"` + fromID.String() + `","to_account_id":"` + toID.String() + `","amount":2000}`
		req := httptest.NewRequest(http.MethodPost, "/payments", bytes.NewBufferString(body))
		req.Header.Set("Idempotency-Key", "key-saturated")
		rec := httptest.NewRecorder()

		handler.CreatePayment(rec, req)

		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429 Too Many Requests, got %d", rec.Code)
		}
		if retryAfter := rec.Header().Get("Retry-After"); retryAfter == "" {
			t.Errorf("expected Retry-After header on 429 response")
		}
	})
}
