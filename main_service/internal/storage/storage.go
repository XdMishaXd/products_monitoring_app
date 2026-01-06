package storage

import "errors"

const (
	UniqueViolation = "23505"
)

var (
	ErrUserAlreadyTracksProduct = errors.New("This product is already tracking")
	ErrProductsNotFound         = errors.New("products not found")
)
