CREATE TABLE clips (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      BIGINT      NOT NULL,
    object_key   TEXT        NOT NULL,
    mode         TEXT        NOT NULL,
    result       TEXT        NOT NULL,
    size_bytes   BIGINT      NOT NULL,
    content_type TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX clips_user_id_created_at_idx ON clips (user_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX clips_object_key_key ON clips (object_key);
