package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"main_service/internal/models"
	"main_service/internal/storage"

	"github.com/redis/go-redis/v9"
)

type RedisRepo struct {
	client     *redis.Client
	DefaultTTL time.Duration
}

func New(ctx context.Context, address string, db int, defautTTL time.Duration) (*RedisRepo, error) {
	const op = "storage.redis.New"

	rdb := redis.NewClient(&redis.Options{
		Addr: address,
		// Password: password,
		DB: db,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &RedisRepo{
		client:     rdb,
		DefaultTTL: defautTTL,
	}, nil
}

func (r *RedisRepo) SaveProduct(ctx context.Context, product models.Product) error {
	const op = "storage.redis.SaveProduct"

	data, err := json.Marshal(product)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	key := fmt.Sprintf("product:%d", product.ID)

	if err := r.client.Set(
		ctx,
		key,
		data,
		r.DefaultTTL,
	).Err(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (r *RedisRepo) Product(ctx context.Context, productID int64) (models.Product, error) {
	const op = "storage.redis.Product"

	var product models.Product

	key := fmt.Sprintf("product:%d", productID)

	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return product, storage.ErrProductsNotFound
		}
		return product, fmt.Errorf("%s: %w", op, err)
	}

	if err := json.Unmarshal(data, &product); err != nil {
		return product, fmt.Errorf("%s: %w", op, err)
	}

	return product, nil
}

// Close закрывает соединение с базой данных.
func (r *RedisRepo) Close() {
	r.client.Close()
}
