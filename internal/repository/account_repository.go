package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucaasnogueira/buriti-pay/internal/domain"
)

type AccountRepository interface {
	Create(ctx context.Context, account *domain.Account) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	Ping(ctx context.Context) error
	Close()
}

type PostgresAccountRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	config.MaxConns = 50
	config.MinConns = 5
	config.MaxConnLifetime = 1 * time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	var pool *pgxpool.Pool
	for attempts := 1; attempts <= 5; attempts++ {
		pool, err = pgxpool.NewWithConfig(ctx, config)
		if err == nil {
			if pingErr := pool.Ping(ctx); pingErr == nil {
				return pool, nil
			}
		}
		time.Sleep(time.Duration(attempts) * time.Second)
	}

	return nil, fmt.Errorf("failed to connect to postgres after 5 attempts: %w", err)
}

func NewPostgresAccountRepository(pool *pgxpool.Pool) *PostgresAccountRepository {
	return &PostgresAccountRepository{pool: pool}
}

func (r *PostgresAccountRepository) Ping(ctx context.Context) error {
	return r.pool.Ping(ctx)
}

func (r *PostgresAccountRepository) Close() {
	r.pool.Close()
}

func (r *PostgresAccountRepository) Create(ctx context.Context, account *domain.Account) error {
	query := `
		INSERT INTO accounts (id, owner, balance, version, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.pool.Exec(ctx, query, account.ID, account.Owner, account.Balance, account.Version, account.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert account: %w", err)
	}
	return nil
}

func (r *PostgresAccountRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	query := `
		SELECT id, owner, balance, version, created_at
		FROM accounts
		WHERE id = $1
	`
	var acc domain.Account
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&acc.ID,
		&acc.Owner,
		&acc.Balance,
		&acc.Version,
		&acc.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("failed to get account by id: %w", err)
	}
	return &acc, nil
}
