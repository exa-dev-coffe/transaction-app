ALTER TABLE tm_vouchers DROP CONSTRAINT IF EXISTS tm_vouchers_code_key;
CREATE UNIQUE INDEX tm_vouchers_code_key ON tm_vouchers (code) WHERE deleted_at IS NULL;
