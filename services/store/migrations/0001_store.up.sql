CREATE TABLE products (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind        TEXT        NOT NULL CHECK (kind IN ('distraction', 'edit')),
    tier        SMALLINT             CHECK (tier IS NULL OR tier BETWEEN 1 AND 3),
    name        TEXT        NOT NULL,
    price_cents INTEGER     NOT NULL,
    is_free     BOOLEAN     NOT NULL DEFAULT false,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE purchases (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT      NOT NULL,
    product_id   BIGINT      NOT NULL REFERENCES products (id),
    provider     TEXT        NOT NULL,
    provider_ref TEXT,
    amount_cents INTEGER     NOT NULL,
    status       TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX purchases_user_id_created_at_idx ON purchases (user_id, created_at);

CREATE TABLE inventory (
    user_id    BIGINT      NOT NULL,
    product_id BIGINT      NOT NULL REFERENCES products (id),
    quantity   INTEGER     NOT NULL DEFAULT 0 CHECK (quantity >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX inventory_user_id_product_id_key ON inventory (user_id, product_id);
