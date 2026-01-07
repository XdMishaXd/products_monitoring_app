package storage

import "errors"

const (
	UniqueViolation = "23505"
)

var (
	ErrProductAlreadyExists = errors.New("Product already exists")
	ErrProductsNotFound     = errors.New("products not found")
)
