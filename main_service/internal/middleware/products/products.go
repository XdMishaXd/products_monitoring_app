package products

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"main_service/internal/models"
	"main_service/internal/storage"
)

type RedisStorage interface {
	SaveProduct(ctx context.Context, product models.Product) error
	Product(ctx context.Context, productID int64) (models.Product, error)
}

type PostgresStorage interface {
	SaveProduct(ctx context.Context, userID int64, productURL, title string, marketplace models.Marketplace) (int64, error)
	ProductByID(ctx context.Context, productID int64) (models.Product, error)
}

type RabbitMQProducer interface {
	PublishJSON(ctx context.Context, msg any) error
}

type ProductOperator struct {
	CheckInterval    time.Duration
	Redis            RedisStorage
	Postgres         PostgresStorage
	RabbitMQProducer RabbitMQProducer
}

func New(p PostgresStorage, r RedisStorage, rabbit RabbitMQProducer, checkInterval time.Duration) *ProductOperator {
	return &ProductOperator{
		CheckInterval:    checkInterval,
		Redis:            r,
		Postgres:         p,
		RabbitMQProducer: rabbit,
	}
}

func (p *ProductOperator) SaveProduct(
	ctx context.Context,
	url, title string,
	userID int64,
	marketplace models.Marketplace,
) (int64, error) {
	productID, err := p.Postgres.SaveProduct(ctx, userID, url, title, marketplace)
	if err != nil {
		return 0, err
	}

	product := models.ProductForProducer{
		ID:          productID,
		URL:         url,
		Marketplace: marketplace,
	}

	data, err := json.Marshal(product)
	if err != nil {
		return 0, fmt.Errorf("failed to serialize product: %w", err)
	}

	err = p.RabbitMQProducer.PublishJSON(ctx, data)
	if err != nil {
		return 0, err
	}

	return productID, nil
}

func (p *ProductOperator) ProductByID(ctx context.Context, productID int64) (models.Product, error) {
	product, err := p.Redis.Product(ctx, productID)
	switch {
	case err == nil:
		return product, nil

	case !errors.Is(err, storage.ErrProductsNotFound):
		return models.Product{}, err
	}

	product, err = p.Postgres.ProductByID(ctx, productID)
	if err != nil {
		return models.Product{}, err
	}

	_ = p.Redis.SaveProduct(ctx, product)

	return product, nil
}
