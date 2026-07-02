CREATE TABLE points_balance (
    user_id    BIGINT      PRIMARY KEY,
    balance    BIGINT      NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE points_ledger (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    BIGINT      NOT NULL,
    delta      BIGINT      NOT NULL,
    reason     TEXT        NOT NULL,
    ref_id     TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX points_ledger_user_id_created_at_idx ON points_ledger (user_id, created_at);

CREATE UNIQUE INDEX points_ledger_user_id_reason_ref_id_key ON points_ledger (user_id, reason, ref_id) WHERE ref_id IS NOT NULL;

ALTER TABLE products ADD COLUMN points_price INTEGER;
