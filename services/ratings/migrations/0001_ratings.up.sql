CREATE TABLE ratings (
    user_id    BIGINT      PRIMARY KEY,
    elo        INTEGER     NOT NULL DEFAULT 1000,
    level      SMALLINT    NOT NULL DEFAULT 4,
    games_played INTEGER   NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE match_results (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    winner_id       BIGINT      NOT NULL,
    loser_id        BIGINT      NOT NULL,
    mode            TEXT        NOT NULL,
    winner_elo_delta INTEGER    NOT NULL,
    loser_elo_delta  INTEGER    NOT NULL,
    duration_ms     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX match_results_winner_id_created_at_idx ON match_results (winner_id, created_at);
CREATE INDEX match_results_loser_id_created_at_idx  ON match_results (loser_id,  created_at);
