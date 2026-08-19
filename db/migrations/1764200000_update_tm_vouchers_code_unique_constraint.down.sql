DROP INDEX IF EXISTS tm_vouchers_code_key;
ALTER TABLE tm_vouchers ADD CONSTRAINT tm_vouchers_code_key UNIQUE (code);
