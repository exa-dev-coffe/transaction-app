ALTER TABLE th_user_checkouts
    DROP COLUMN IF EXISTS voucher_id,
    DROP COLUMN IF EXISTS discount_amount;

DROP TABLE IF EXISTS tr_voucher_usages;
DROP TABLE IF EXISTS tm_vouchers;
