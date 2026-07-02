CREATE TABLE king_reigns (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    hill_type   TEXT        NOT NULL CHECK (hill_type IN ('daily','monthly')),
    user_id     BIGINT      NOT NULL,
    clip_id     TEXT,
    blink_ts_ms INTEGER     NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX king_reigns_hill_type_current_key ON king_reigns (hill_type) WHERE ended_at IS NULL;

CREATE TABLE hill_ranks (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT      NOT NULL,
    day          DATE        NOT NULL,
    current_rank SMALLINT    NOT NULL DEFAULT 0,
    best_hold_ms INTEGER     NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, day)
);

CREATE INDEX hill_ranks_day_current_rank_idx ON hill_ranks (day, current_rank);
