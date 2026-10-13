ALTER TABLE th_user_checkouts
    ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(256) DEFAULT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_th_user_checkouts_idempotency_key
    ON th_user_checkouts (idempotency_key)
    WHERE idempotency_key IS NOT NULL;
