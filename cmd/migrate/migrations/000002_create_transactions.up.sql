CREATE TYPE transaction_status AS ENUM ('pending', 'completed', 'failed');

CREATE TABLE transactions (
    id                  UUID                    PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key     TEXT                    NOT NULL UNIQUE,
    from_account_id     UUID                    NOT NULL REFERENCES accounts(id),
    to_account_id       UUID                    NOT NULL REFERENCES accounts(id),
    amount              BIGINT                  NOT NULL,
    status              transaction_status      NOT NULL DEFAULT 'pending',
    created_at          TIMESTAMPTZ             NOT NULL DEFAULT NOW(),

    CONSTRAINT amount_positive    CHECK (amount > 0),
    CONSTRAINT different_accounts CHECK (from_account_id != to_account_id)
);

CREATE INDEX idx_transactions_from_account ON transactions(from_account_id, created_at DESC);
CREATE INDEX idx_transactions_to_account   ON transactions(to_account_id, created_at DESC);
CREATE INDEX idx_idempotency_key           ON transactions(idempotency_key);
