package models

import "time"

type Marketplace string

const (
	Etsy       Marketplace = "etsy"
	Ebay       Marketplace = "ebay"
	Aliexpress Marketplace = "aliexpress"
)

type Product struct {
	ID          int64       `json:"id"`
	Title       string      `json:"title"`
	Marketplace Marketplace `json:"marketplace"`
	Price       float32     `json:"price"`
	Currency    string      `json:"currency"`
	InStock     bool        `json:"in_stock"`
	Created_at  time.Time   `json:"created_at"`
	Updated_at  time.Time   `json:"updated_at"`
}

type ProductForProducer struct {
	ID          int64       `json:"id"`
	URL         string      `json:"url"`
	Marketplace Marketplace `json:"marketplace"`
}

type ParsedProduct struct {
	ID         int64   `json:"id"`
	Price      float32 `json:"price"`
	CurrencyID int     `json:"currency_id"`
	InStock    bool    `json:"in_stock"`
	Err        error   `json:"err"`
}

var CurrencyIDs = map[string]int{
	"USD": 1,
	"EUR": 2,
	"GBP": 3,
	"JPY": 4,
	"RUB": 5,
	"MDL": 6,
}
