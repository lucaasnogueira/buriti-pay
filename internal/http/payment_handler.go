package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	"github.com/lucaasnogueira/buriti-pay/internal/service"
)

type PaymentHandler struct {
	service service.PaymentService
}

func NewPaymentHandler(service service.PaymentService) *PaymentHandler {
	return &PaymentHandler{service: service}
}

type CreatePaymentRequest struct {
	FromAccountID uuid.UUID `json:"from_account_id"`
	ToAccountID   uuid.UUID `json:"to_account_id"`
	Amount        int64     `json:"amount"`
}

type PaymentResponse struct {
	ID             uuid.UUID `json:"id"`
	IdempotencyKey string    `json:"idempotency_key"`
	FromAccountID  uuid.UUID `json:"from_account_id"`
	ToAccountID    uuid.UUID `json:"to_account_id"`
	Amount         int64     `json:"amount"`
	Status         string    `json:"status"`
	FailureReason  *string   `json:"failure_reason,omitempty"`
	CreatedAt      string    `json:"created_at"`
	UpdatedAt      string    `json:"updated_at"`
}

func (h *PaymentHandler) CreatePayment(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Idempotency-Key header is required"})
		return
	}

	var req CreatePaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	payment, err := h.service.CreatePaymentAsync(
		r.Context(),
		idempotencyKey,
		req.FromAccountID,
		req.ToAccountID,
		req.Amount,
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrIdempotencyConflict):
			respondJSON(w, http.StatusConflict, map[string]string{"error": "idempotency key reused with different payload"})
		case errors.Is(err, domain.ErrInvalidAmount),
			errors.Is(err, domain.ErrSameAccountTransfer),
			errors.Is(err, domain.ErrIdempotencyKeyRequired):
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		default:
			if err.Error() == "worker queue is full (backpressure)" {
				w.Header().Set("Retry-After", "2")
				respondJSON(w, http.StatusTooManyRequests, map[string]string{"error": "server queue full, retry after 2s"})
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal payment error"})
		}
		return
	}

	statusHTTP := http.StatusOK
	if payment.Status == domain.StatusPending {
		statusHTTP = http.StatusAccepted
	} else if payment.Status == domain.StatusConfirmed {
		statusHTTP = http.StatusOK
	}

	respondJSON(w, statusHTTP, toPaymentResponse(payment))
}

func (h *PaymentHandler) GetPayment(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payment id format"})
		return
	}

	payment, err := h.service.GetPayment(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrPaymentNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "payment not found"})
			return
		}
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query payment"})
		return
	}

	respondJSON(w, http.StatusOK, toPaymentResponse(payment))
}

func toPaymentResponse(p *domain.Payment) PaymentResponse {
	return PaymentResponse{
		ID:             p.ID,
		IdempotencyKey: p.IdempotencyKey,
		FromAccountID:  p.FromAccountID,
		ToAccountID:    p.ToAccountID,
		Amount:         p.Amount,
		Status:         string(p.Status),
		FailureReason:  p.FailureReason,
		CreatedAt:      p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:      p.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
