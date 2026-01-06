package models

type Marketplace string

const (
	Etsy       Marketplace = "etsy"
	Ebay       Marketplace = "ebay"
	Aliexpress Marketplace = "aliexpress"
)

type Product struct {
	ID          int64       `json:"id"`
	URL         string      `json:"url"`
	Marketplace Marketplace `json:"marketplace"`
}

type ParsedProduct struct {
	ID      int64   `json:"id"`
	Price   float32 `json:"price"`
	InStock bool    `json:"in_stock"`
	Err     error   `json:"err"`
}
