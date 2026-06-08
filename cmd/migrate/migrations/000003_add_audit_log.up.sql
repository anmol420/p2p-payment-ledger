CREATE TYPE audit_event_type AS ENUM ('debit', 'credit');

CREATE TABLE audit_log(
    id             UUID             PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id     UUID             NOT NULL REFERENCES accounts (id) ON DELETE RESTRICT,
    transaction_id UUID             NOT NULL REFERENCES transactions (id) ON DELETE RESTRICT,
    event_type     audit_event_type NOT NULL,
    amount         BIGINT           NOT NULL CHECK (amount > 0),
    balance_before BIGINT           NOT NULL CHECK ( balance_before >= 0 ),
    balance_after  BIGINT           NOT NULL CHECK ( balance_after >= 0 ),
    created_at     TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_log_account_time ON audit_log(account_id, created_at DESC, id DESC);

CREATE INDEX idx_audit_log_transaction ON audit_log(transaction_id);
