CREATE TABLE cheat_reports (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    reporter_id BIGINT      NOT NULL,
    reported_id BIGINT      NOT NULL,
    match_id    TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT cheat_reports_reporter_id_match_id_key UNIQUE (reporter_id, match_id)
);

CREATE INDEX cheat_reports_reported_id_created_at_idx ON cheat_reports (reported_id, created_at);

CREATE TABLE bug_reports (
    id          BIGINT      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT      NOT NULL,
    device      TEXT        NOT NULL,
    description TEXT,
    object_key  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
