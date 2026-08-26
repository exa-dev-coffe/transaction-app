ALTER TABLE th_user_checkouts
    DROP COLUMN IF EXISTS order_type,
    DROP COLUMN IF EXISTS payment_method,
    DROP COLUMN IF EXISTS payment_status,
    DROP COLUMN IF EXISTS cash_amount,
    DROP COLUMN IF EXISTS cash_change,
    DROP COLUMN IF EXISTS is_cashier,
    DROP COLUMN IF EXISTS qr_string,
    DROP COLUMN IF EXISTS qr_url;
