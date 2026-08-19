CREATE TABLE tm_vouchers
(
    id             SERIAL PRIMARY KEY,
    code           VARCHAR(50) UNIQUE NOT NULL,
    discount_type  VARCHAR(20)        NOT NULL,
    discount_value DECIMAL(10, 2)     NOT NULL,
    max_discount   DECIMAL(10, 2) DEFAULT 0,
    min_purchase   DECIMAL(10, 2) DEFAULT 0,
    quota          INT            DEFAULT -1,
    is_active      BOOLEAN        DEFAULT TRUE,
    expired_at     TIMESTAMP          NOT NULL,
    created_at     TIMESTAMP      DEFAULT CURRENT_TIMESTAMP,
    created_by     INT            DEFAULT NULL,
    updated_at     TIMESTAMP      DEFAULT CURRENT_TIMESTAMP,
    updated_by     INT            DEFAULT NULL
);

CREATE TABLE tr_voucher_usages
(
    id              SERIAL PRIMARY KEY,
    user_id         INT NOT NULL,
    voucher_id      INT NOT NULL REFERENCES tm_vouchers (id) ON DELETE CASCADE,
    checkout_id     INT NOT NULL,
    discount_amount DECIMAL(10, 2) NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE th_user_checkouts
    ADD COLUMN voucher_id INT REFERENCES tm_vouchers (id) ON DELETE SET NULL,
    ADD COLUMN discount_amount DECIMAL(10, 2) DEFAULT 0;
