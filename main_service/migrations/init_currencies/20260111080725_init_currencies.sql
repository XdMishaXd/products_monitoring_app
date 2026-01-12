-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS currencies (
	id SERIAL PRIMARY KEY,
	code VARCHAR(3) UNIQUE NOT NULL
);

INSERT INTO currencies (code) VALUES
('USD'), ('EUR'), ('GBP'), ('JPY'),
('CNY'), ('RUB'), ('MDL');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS currencies;
-- +goose StatementEnd
