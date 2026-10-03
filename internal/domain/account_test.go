package domain_test

import (
	"testing"

	"github.com/lucaasnogueira/buriti-pay/internal/domain"
)

func TestNewAccount(t *testing.T) {
	t.Run("successfully create account with positive balance", func(t *testing.T) {
		owner := "Alice"
		initialBalance := int64(10000) // 100.00 BRL/USD in cents

		acc, err := domain.NewAccount(owner, initialBalance)
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		if acc.Owner != owner {
			t.Errorf("expected owner %s, got %s", owner, acc.Owner)
		}
		if acc.Balance != initialBalance {
			t.Errorf("expected balance %d, got %d", initialBalance, acc.Balance)
		}
		if acc.Version != 0 {
			t.Errorf("expected version 0, got %d", acc.Version)
		}
		if acc.ID.String() == "" {
			t.Errorf("expected non-empty UUID")
		}
	})

	t.Run("fail when owner is empty", func(t *testing.T) {
		_, err := domain.NewAccount("", 100)
		if err != domain.ErrOwnerRequired {
			t.Fatalf("expected ErrOwnerRequired, got %v", err)
		}
	})

	t.Run("fail when initial balance is negative", func(t *testing.T) {
		_, err := domain.NewAccount("Bob", -10)
		if err == nil {
			t.Fatalf("expected error for negative balance, got nil")
		}
	})
}
