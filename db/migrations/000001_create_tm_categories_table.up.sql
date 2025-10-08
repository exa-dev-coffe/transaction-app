CREATE TABLE tm_categories
(
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(100),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by INT       DEFAULT NULL,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_by INT       DEFAULT NULL
);