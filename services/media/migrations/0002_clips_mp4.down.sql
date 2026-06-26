ALTER TABLE clips
    DROP COLUMN IF EXISTS mp4_object_key,
    DROP COLUMN IF EXISTS conversion_status;
