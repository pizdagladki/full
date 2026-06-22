CREATE TABLE user_consents (
    user_id           BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    is_adult          BOOLEAN     NOT NULL,
    consent_recording BOOLEAN     NOT NULL,
    consent_tos       BOOLEAN     NOT NULL,
    accepted_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX user_consents_user_id_key ON user_consents (user_id);
