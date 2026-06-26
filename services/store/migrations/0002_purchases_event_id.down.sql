DROP INDEX IF EXISTS purchases_stripe_event_id_key;
ALTER TABLE purchases DROP COLUMN IF EXISTS stripe_event_id;
