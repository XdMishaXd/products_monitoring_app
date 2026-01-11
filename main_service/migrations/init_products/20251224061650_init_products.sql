-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS products (
	id BIGSERIAL PRIMARY KEY,
	user_id BIGINT NOT NULL,
	url TEXT NOT NULL,
	marketplace TEXT NOT NULL,
	currency_id INT DEFAULT -1,
	title TEXT NOT NULL,
	price REAL DEFAULT -1,
	parsing_error TEXT DEFAULT NULL,
	in_stock BOOLEAN DEFAULT FALSE,
	last_checked TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

	CONSTRAINT fk_products_user
    FOREIGN KEY (user_id)
    REFERENCES users(id)
    ON DELETE CASCADE

	CONSTRAINT fk_products_currency
    FOREIGN KEY (user_id)
    REFERENCES currencies(id)
    ON DELETE CASCADE
);

CREATE UNIQUE INDEX uniq_products_user_url
ON products (user_id, url);

CREATE INDEX idx_products_user_id
  ON products(user_id);

CREATE INDEX idx_products_in_stock
	ON products(in_stock)
	WHERE in_stock = true;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE products IF EXISTS;
-- +goose StatementEnd
