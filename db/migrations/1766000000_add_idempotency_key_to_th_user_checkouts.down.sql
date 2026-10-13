DROP INDEX IF EXISTS idx_th_user_checkouts_idempotency_key;

ALTER TABLE th_user_checkouts
    DROP COLUMN IF EXISTS idempotency_key;
