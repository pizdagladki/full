ALTER TABLE products DROP COLUMN IF EXISTS points_price;
DROP INDEX IF EXISTS points_ledger_user_id_reason_ref_id_key;
DROP INDEX IF EXISTS points_ledger_user_id_created_at_idx;
DROP TABLE IF EXISTS points_ledger;
DROP TABLE IF EXISTS points_balance;
