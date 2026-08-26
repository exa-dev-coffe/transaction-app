CREATE TABLE tr_promotion_usages
(
    id              SERIAL PRIMARY KEY,
    transaction_id  BIGINT         NOT NULL,
    promotion_id    BIGINT         NOT NULL,
    menu_id         BIGINT         NOT NULL,
    user_id         BIGINT         NOT NULL,
    qty             INT            NOT NULL,
    discount_amount DECIMAL(10, 2) NOT NULL,
    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_tr_promotion_usages_user_promo ON tr_promotion_usages (user_id, promotion_id);
CREATE INDEX idx_tr_promotion_usages_transaction ON tr_promotion_usages (transaction_id);
