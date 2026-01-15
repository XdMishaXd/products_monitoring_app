-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS products (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL,
	url TEXT NOT NULL,
	marketplace TEXT NOT NULL,
	currency_id INT DEFAULT NULL,
	title TEXT NOT NULL,
	price REAL DEFAULT -1,
	parsing_error TEXT DEFAULT NULL,
	in_stock BOOLEAN DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

	CONSTRAINT fk_products_user
    FOREIGN KEY (user_id)
    REFERENCES users(id)
    ON DELETE CASCADE,

	CONSTRAINT fk_products_currency
    FOREIGN KEY (currency_id)
    REFERENCES currencies(id)
    ON DELETE CASCADE
);

-- Уникальный индекс для предотвращения дубликатов
CREATE UNIQUE INDEX uniq_products_user_url 
	ON products (user_id, url);

-- Индекс для быстрого поиска продуктов пользователя
CREATE INDEX idx_products_user_id 
	ON products(user_id);

-- Частичный индекс для товаров в наличии
CREATE INDEX idx_products_in_stock
	ON products(in_stock)
	WHERE in_stock = true;

-- Индекс для быстрого поиска продуктов для парсинга
CREATE INDEX idx_products_updated_at 
	ON products(updated_at ASC);

-- Составной индекс для оптимизации запросов с фильтрацией по user_id и сортировкой по created_at
CREATE INDEX idx_products_user_created 
	ON products(user_id, created_at DESC);

-- Индекс для фильтрации по маркетплейсу
CREATE INDEX idx_products_marketplace 
	ON products(marketplace);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_products_marketplace;
DROP INDEX IF EXISTS idx_products_user_created;
DROP INDEX IF EXISTS idx_products_updated_at;
DROP INDEX IF EXISTS idx_products_in_stock;
DROP INDEX IF EXISTS idx_products_user_id;
DROP INDEX IF EXISTS uniq_products_user_url;
DROP TABLE IF EXISTS products;
-- +goose StatementEnd
