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
)

type mockAccountRepo struct {
	accounts map[uuid.UUID]*domain.Account
}

func newMockAccountRepo() *mockAccountRepo {
	return &mockAccountRepo{accounts: make(map[uuid.UUID]*domain.Account)}
}

func (m *mockAccountRepo) Create(ctx context.Context, account *domain.Account) error {
	m.accounts[account.ID] = account
	return nil
}

func (m *mockAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	acc, ok := m.accounts[id]
	if !ok {
		return nil, domain.ErrAccountNotFound
	}
	return acc, nil
}

func (m *mockAccountRepo) Ping(ctx context.Context) error {
	return nil
}

func (m *mockAccountRepo) Close() {}

func TestAccountHandler_CreateAccount(t *testing.T) {
	repo := newMockAccountRepo()
	handler := apphttp.NewAccountHandler(repo)

	t.Run("successful account creation", func(t *testing.T) {
		body := `{"owner":"Alice","initial_balance":5000}`
		req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()

		handler.CreateAccount(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected status 201, got %d. Body: %s", rec.Code, rec.Body.String())
		}

		var resp apphttp.AccountResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		if resp.Owner != "Alice" || resp.Balance != 5000 {
			t.Errorf("unexpected response content: %+v", resp)
		}
	})

	t.Run("bad request on invalid json", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/accounts", bytes.NewBufferString(`{invalid`))
		rec := httptest.NewRecorder()

		handler.CreateAccount(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rec.Code)
		}
	})
}
