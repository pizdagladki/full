ALTER TABLE clips
    ADD COLUMN mp4_object_key   TEXT NOT NULL DEFAULT '',
    ADD COLUMN conversion_status TEXT NOT NULL DEFAULT 'none';
