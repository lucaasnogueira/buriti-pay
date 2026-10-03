package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
	"github.com/lucaasnogueira/buriti-pay/internal/repository"
)

type AccountHandler struct {
	repo repository.AccountRepository
}

func NewAccountHandler(repo repository.AccountRepository) *AccountHandler {
	return &AccountHandler{repo: repo}
}

type CreateAccountRequest struct {
	Owner          string `json:"owner"`
	InitialBalance int64  `json:"initial_balance"`
}

type AccountResponse struct {
	ID        uuid.UUID `json:"id"`
	Owner     string    `json:"owner"`
	Balance   int64     `json:"balance"`
	Version   int64     `json:"version"`
	CreatedAt string    `json:"created_at"`
}

func (h *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	acc, err := domain.NewAccount(req.Owner, req.InitialBalance)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := h.repo.Create(r.Context(), acc); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create account"})
		return
	}

	respondJSON(w, http.StatusCreated, toAccountResponse(acc))
}

func (h *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid account id format"})
		return
	}

	acc, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrAccountNotFound) {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "account not found"})
			return
		}
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch account"})
		return
	}

	respondJSON(w, http.StatusOK, toAccountResponse(acc))
}

func toAccountResponse(acc *domain.Account) AccountResponse {
	return AccountResponse{
		ID:        acc.ID,
		Owner:     acc.Owner,
		Balance:   acc.Balance,
		Version:   acc.Version,
		CreatedAt: acc.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
