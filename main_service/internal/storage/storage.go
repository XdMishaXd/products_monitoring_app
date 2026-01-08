package storage

import "errors"

const (
	UniqueViolation = "23505"
)

var (
	ErrProductAlreadyExists = errors.New("Product already exists")
	ErrProductsNotFound     = errors.New("Products not found")
	ErrProductNotFound      = errors.New("Product not found")
	ErrParsingFailed        = errors.New("Failed to parse product")
)
