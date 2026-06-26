ALTER TABLE purchases ADD COLUMN stripe_event_id TEXT;
CREATE UNIQUE INDEX purchases_stripe_event_id_key ON purchases (stripe_event_id) WHERE stripe_event_id IS NOT NULL;
