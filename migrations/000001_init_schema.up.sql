-- Accounts table
CREATE TABLE IF NOT EXISTS accounts (
    id          UUID PRIMARY KEY,
    owner       TEXT        NOT NULL,
    balance     BIGINT      NOT NULL CHECK (balance >= 0),  -- stored in cents
    version     BIGINT      NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Payment status enum
DO $$ BEGIN
    CREATE TYPE payment_status AS ENUM ('PENDING', 'PROCESSING', 'CONFIRMED', 'FAILED');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Payments table
CREATE TABLE IF NOT EXISTS payments (
    id               UUID PRIMARY KEY,
    idempotency_key  TEXT           NOT NULL UNIQUE,
    from_account_id  UUID           NOT NULL REFERENCES accounts(id),
    to_account_id    UUID           NOT NULL REFERENCES accounts(id),
    amount           BIGINT         NOT NULL CHECK (amount > 0),
    status           payment_status NOT NULL DEFAULT 'PENDING',
    failure_reason   TEXT,
    created_at       TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ    NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);

-- Double-entry ledger audit trail
CREATE TABLE IF NOT EXISTS ledger_entries (
    id          BIGSERIAL PRIMARY KEY,
    payment_id  UUID        NOT NULL REFERENCES payments(id),
    account_id  UUID        NOT NULL REFERENCES accounts(id),
    delta       BIGINT      NOT NULL,  -- negative = debit, positive = credit
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ledger_payment ON ledger_entries(payment_id);

-- Transactional Outbox
CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL PRIMARY KEY,
    aggregate_id UUID        NOT NULL,
    event_type   TEXT        NOT NULL,
    payload      JSONB       NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending ON outbox(id)
    WHERE published_at IS NULL;
