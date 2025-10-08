CREATE TABLE tm_menus
(
    id           SERIAL PRIMARY KEY,
    name         VARCHAR(100)   NOT NULL,
    description  TEXT,
    price        DECIMAL(10, 2) NOT NULL,
    is_available BOOLEAN   DEFAULT TRUE,
    category_id  INT            REFERENCES tm_categories (id) ON DELETE SET NULL,
    photo        TEXT           NOT NULL,
    created_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by   INT       DEFAULT NULL,
    updated_at   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_by   INT       DEFAULT NULL
);