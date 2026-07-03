CREATE TABLE king_clips (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT      NOT NULL,
    hill_type    TEXT        NOT NULL,
    object_key   TEXT        NOT NULL,
    blink_ts_ms  BIGINT      NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX king_clips_object_key_key ON king_clips (object_key);
CREATE INDEX king_clips_hill_type_created_at_idx ON king_clips (hill_type, created_at DESC, id DESC);
CREATE INDEX king_clips_expires_at_idx ON king_clips (expires_at);
