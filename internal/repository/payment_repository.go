package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
)

var (
	ErrUniqueViolation = errors.New("unique constraint violation")
)

type PaymentRepository interface {
	Create(ctx context.Context, payment *domain.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)
	ExecuteTransfer(ctx context.Context, paymentID uuid.UUID) (*domain.Payment, error)
}

type PostgresPaymentRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresPaymentRepository(pool *pgxpool.Pool) *PostgresPaymentRepository {
	return &PostgresPaymentRepository{pool: pool}
}

func (r *PostgresPaymentRepository) Create(ctx context.Context, payment *domain.Payment) error {
	query := `
		INSERT INTO payments (id, idempotency_key, from_account_id, to_account_id, amount, status, failure_reason, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := r.pool.Exec(ctx, query,
		payment.ID,
		payment.IdempotencyKey,
		payment.FromAccountID,
		payment.ToAccountID,
		payment.Amount,
		payment.Status,
		payment.FailureReason,
		payment.CreatedAt,
		payment.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrUniqueViolation
		}
		return fmt.Errorf("failed to insert payment: %w", err)
	}
	return nil
}

func (r *PostgresPaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	query := `
		SELECT id, idempotency_key, from_account_id, to_account_id, amount, status, failure_reason, created_at, updated_at
		FROM payments
		WHERE id = $1
	`
	var p domain.Payment
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID,
		&p.IdempotencyKey,
		&p.FromAccountID,
		&p.ToAccountID,
		&p.Amount,
		&p.Status,
		&p.FailureReason,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to query payment by id: %w", err)
	}
	return &p, nil
}

func (r *PostgresPaymentRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	query := `
		SELECT id, idempotency_key, from_account_id, to_account_id, amount, status, failure_reason, created_at, updated_at
		FROM payments
		WHERE idempotency_key = $1
	`
	var p domain.Payment
	err := r.pool.QueryRow(ctx, query, key).Scan(
		&p.ID,
		&p.IdempotencyKey,
		&p.FromAccountID,
		&p.ToAccountID,
		&p.Amount,
		&p.Status,
		&p.FailureReason,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to query payment by idempotency key: %w", err)
	}
	return &p, nil
}

// ExecuteTransfer executes the funds movement in a single atomic transaction.
// Guarantees:
// 1. Debit and credit happen in the same TX.
// 2. Ledger entries are written (+delta, -delta) with zero sum.
// 3. Outbox event is recorded.
// 4. Accounts are locked in deterministic order (UUID ascending) to prevent deadlocks.
func (r *PostgresPaymentRepository) ExecuteTransfer(ctx context.Context, paymentID uuid.UUID) (*domain.Payment, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Lock and load the payment row
	var p domain.Payment
	queryPayment := `
		SELECT id, idempotency_key, from_account_id, to_account_id, amount, status, failure_reason, created_at, updated_at
		FROM payments
		WHERE id = $1
		FOR UPDATE
	`
	err = tx.QueryRow(ctx, queryPayment, paymentID).Scan(
		&p.ID,
		&p.IdempotencyKey,
		&p.FromAccountID,
		&p.ToAccountID,
		&p.Amount,
		&p.Status,
		&p.FailureReason,
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrPaymentNotFound
		}
		return nil, fmt.Errorf("failed to select payment for update: %w", err)
	}

	if p.Status == domain.StatusConfirmed || p.Status == domain.StatusFailed {
		// Already in terminal state
		return &p, nil
	}

	// 2. Deterministic locking order (ascending UUID) to prevent PostgreSQL deadlocks
	firstID, secondID := p.FromAccountID, p.ToAccountID
	if firstID.String() > secondID.String() {
		firstID, secondID = secondID, firstID
	}

	lockQuery := `
		SELECT id, balance, version
		FROM accounts
		WHERE id IN ($1, $2)
		ORDER BY id
		FOR UPDATE
	`
	rows, err := tx.Query(ctx, lockQuery, firstID, secondID)
	if err != nil {
		return nil, fmt.Errorf("failed to lock accounts: %w", err)
	}
	defer rows.Close()

	accounts := make(map[uuid.UUID]*domain.Account)
	for rows.Next() {
		var a domain.Account
		if err := rows.Scan(&a.ID, &a.Balance, &a.Version); err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}
		accounts[a.ID] = &a
	}

	fromAcc, fromExists := accounts[p.FromAccountID]
	toAcc, toExists := accounts[p.ToAccountID]

	if !fromExists || !toExists {
		failReason := "one or both accounts not found"
		_ = p.TransitionTo(domain.StatusFailed, &failReason)
		_ = r.recordPaymentFailure(ctx, tx, &p, failReason)
		_ = tx.Commit(ctx)
		return &p, nil
	}

	// 3. Verify balance sufficiency
	if fromAcc.Balance < p.Amount {
		failReason := domain.ErrInsufficientBalance.Error()
		_ = p.TransitionTo(domain.StatusFailed, &failReason)
		_ = r.recordPaymentFailure(ctx, tx, &p, failReason)
		_ = tx.Commit(ctx)
		return &p, nil
	}

	// 4. Execute balance updates with optimistic version fencing (WHERE version = $version)
	updateFromQuery := `
		UPDATE accounts
		SET balance = balance - $1, version = version + 1
		WHERE id = $2 AND version = $3
	`
	cmdTag, err := tx.Exec(ctx, updateFromQuery, p.Amount, fromAcc.ID, fromAcc.Version)
	if err != nil || cmdTag.RowsAffected() == 0 {
		return nil, fmt.Errorf("concurrency violation on source account %s", fromAcc.ID)
	}

	updateToQuery := `
		UPDATE accounts
		SET balance = balance + $1, version = version + 1
		WHERE id = $2 AND version = $3
	`
	cmdTag, err = tx.Exec(ctx, updateToQuery, p.Amount, toAcc.ID, toAcc.Version)
	if err != nil || cmdTag.RowsAffected() == 0 {
		return nil, fmt.Errorf("concurrency violation on destination account %s", toAcc.ID)
	}

	// 5. Insert double-entry ledger records: Debit (-delta) & Credit (+delta)
	ledgerQuery := `
		INSERT INTO ledger_entries (payment_id, account_id, delta, created_at)
		VALUES ($1, $2, $3, now()), ($1, $4, $5, now())
	`
	_, err = tx.Exec(ctx, ledgerQuery, p.ID, fromAcc.ID, -p.Amount, toAcc.ID, p.Amount)
	if err != nil {
		return nil, fmt.Errorf("failed to insert ledger entries: %w", err)
	}

	// 6. Transition payment status to CONFIRMED
	if err := p.TransitionTo(domain.StatusConfirmed, nil); err != nil {
		return nil, fmt.Errorf("invalid transition to confirmed: %w", err)
	}
	updatePaymentQuery := `
		UPDATE payments
		SET status = $1, failure_reason = NULL, updated_at = now()
		WHERE id = $2
	`
	_, err = tx.Exec(ctx, updatePaymentQuery, p.Status, p.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update payment to confirmed: %w", err)
	}

	// 7. Write event to Outbox table
	confirmedPayload, _ := json.Marshal(domain.PaymentConfirmedPayload{
		PaymentID:     p.ID,
		FromAccountID: p.FromAccountID,
		ToAccountID:   p.ToAccountID,
		Amount:        p.Amount,
		Status:        string(p.Status),
	})
	outboxQuery := `
		INSERT INTO outbox (aggregate_id, event_type, payload, created_at)
		VALUES ($1, 'payment.confirmed', $2, now())
	`
	_, err = tx.Exec(ctx, outboxQuery, p.ID, confirmedPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to insert outbox event: %w", err)
	}

	// 8. Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transfer transaction: %w", err)
	}

	return &p, nil
}

func (r *PostgresPaymentRepository) recordPaymentFailure(
	ctx context.Context,
	tx pgx.Tx,
	p *domain.Payment,
	reason string,
) error {
	updateQuery := `
		UPDATE payments
		SET status = $1, failure_reason = $2, updated_at = now()
		WHERE id = $3
	`
	if _, err := tx.Exec(ctx, updateQuery, p.Status, reason, p.ID); err != nil {
		return err
	}

	failedPayload, _ := json.Marshal(domain.PaymentFailedPayload{
		PaymentID:     p.ID,
		FromAccountID: p.FromAccountID,
		ToAccountID:   p.ToAccountID,
		Amount:        p.Amount,
		Status:        string(p.Status),
		Reason:        reason,
	})
	outboxQuery := `
		INSERT INTO outbox (aggregate_id, event_type, payload, created_at)
		VALUES ($1, 'payment.failed', $2, now())
	`
	_, err := tx.Exec(ctx, outboxQuery, p.ID, failedPayload)
	return err
}
