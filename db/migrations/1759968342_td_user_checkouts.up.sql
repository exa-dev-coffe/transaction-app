CREATE TABLE td_user_checkouts
(
    id          SERIAL PRIMARY KEY,
    ref_id      INT            NOT NULL,
    menu_id     INT            NOT NULL,
    qty         INT            NOT NULL,
    price       DECIMAL(10, 2) NOT NULL,
    total_price DECIMAL(10, 2) NOT NULL,
    rating      INT       DEFAULT NULL,
    notes       TEXT      DEFAULT NULL,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by  INT       DEFAULT NULL,
    updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_by  INT       DEFAULT NULL

);

ALTER TABLE td_user_checkouts
    ADD CONSTRAINT FK_TD_USER_CHECKOUTS_TH_USER_CHECKOUTS FOREIGN KEY (ref_id) REFERENCES th_user_checkouts (id) ON DELETE CASCADE;
