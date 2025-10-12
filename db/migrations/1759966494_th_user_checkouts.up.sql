CREATE TABLE th_user_checkouts
(
    id           SERIAL PRIMARY KEY,
    user_id      INT NOT NULL,
    order_status INT            DEFAULT 0,
    total_price  DECIMAL(10, 2) DEFAULT 0,
    table_id     INT NOT NULL,
    order_for    VARCHAR,
    created_at   TIMESTAMP      DEFAULT CURRENT_TIMESTAMP,
    created_by   INT            DEFAULT NULL,
    updated_at   TIMESTAMP      DEFAULT CURRENT_TIMESTAMP,
    updated_by   INT            DEFAULT NULL
);