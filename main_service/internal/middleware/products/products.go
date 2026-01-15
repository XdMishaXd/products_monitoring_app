package products

import (
	"context"
	"errors"
	"log/slog"
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
	GetProductsForParsing(ctx context.Context, limit int) ([]models.ProductForProducer, error)
}

type RabbitMQProducer interface {
	PublishJSON(ctx context.Context, product models.ProductForProducer) error
}

type ProductOperator struct {
	log              *slog.Logger
	checkInterval    time.Duration
	redis            RedisStorage
	postgres         PostgresStorage
	rabbitmqProducer RabbitMQProducer
	batchSize        int
}

func New(
	log *slog.Logger,
	p PostgresStorage,
	r RedisStorage,
	rabbit RabbitMQProducer,
	checkInterval time.Duration,
	batchSize int,
) *ProductOperator {
	return &ProductOperator{
		log:              log,
		checkInterval:    checkInterval,
		redis:            r,
		postgres:         p,
		rabbitmqProducer: rabbit,
		batchSize:        batchSize,
	}
}

// * SaveProduct сохраняет продукт и сразу отправляет на парсинг
func (p *ProductOperator) SaveProduct(
	ctx context.Context,
	url, title string,
	userID int64,
	marketplace models.Marketplace,
) (int64, error) {
	productID, err := p.postgres.SaveProduct(ctx, userID, url, title, marketplace)
	if err != nil {
		return 0, err
	}

	product := models.ProductForProducer{
		ID:          productID,
		URL:         url,
		Marketplace: marketplace,
	}

	if err := p.rabbitmqProducer.PublishJSON(ctx, product); err != nil {
		p.log.Error("failed to publish product for parsing",
			slog.Int64("product_id", productID),
			slog.String("error", err.Error()),
		)
		// Не возвращаем ошибку, т.к. продукт уже сохранен
		// Он будет распарсен при следующем периодическом цикле
	}

	return productID, nil
}

// * ProductByID возвращает продукт с кэшированием в Redis
func (p *ProductOperator) ProductByID(ctx context.Context, productID int64) (models.Product, error) {
	product, err := p.redis.Product(ctx, productID)
	switch {
	case err == nil:
		return product, nil

	case !errors.Is(err, storage.ErrProductsNotFound):
		return models.Product{}, err
	}

	product, err = p.postgres.ProductByID(ctx, productID)
	if err != nil {
		return models.Product{}, err
	}

	_ = p.redis.SaveProduct(ctx, product)

	return product, nil
}

// * RunPeriodicParsing запускает периодический парсинг продуктов
func (p *ProductOperator) RunPeriodicParsing(ctx context.Context) error {
	p.log.Info("starting periodic parsing",
		slog.Duration("interval", p.checkInterval),
		slog.Int("batch_size", p.batchSize),
	)

	// Первый запуск сразу
	p.parseProducts(ctx)

	ticker := time.NewTicker(p.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.log.Info("periodic parsing stopped")
			return ctx.Err()
		case <-ticker.C:
			p.parseProducts(ctx)
		}
	}
}

// * parseProducts получает продукты для парсинга и отправляет их в очередь
func (p *ProductOperator) parseProducts(ctx context.Context) {
	p.log.Info("starting parsing cycle")
	start := time.Now()

	products, err := p.postgres.GetProductsForParsing(ctx, p.batchSize)
	if err != nil {
		p.log.Error("failed to get products for parsing",
			slog.String("error", err.Error()),
		)

		return
	}

	p.log.Info("products fetched for parsing",
		slog.Int("count", len(products)),
		slog.Duration("fetch_duration", time.Since(start)),
	)

	if len(products) == 0 {
		p.log.Debug("no products to parse")
		return
	}

	var successCount, errorCount int

	for _, product := range products {
		productMsg := models.ProductForProducer{
			ID:          product.ID,
			URL:         product.URL,
			Marketplace: product.Marketplace,
		}

		if err := p.rabbitmqProducer.PublishJSON(ctx, productMsg); err != nil {
			p.log.Error("failed to publish product",
				slog.Int64("product_id", product.ID),
				slog.String("error", err.Error()),
			)

			errorCount++
			continue
		}

		successCount++
	}

	p.log.Info("parsing cycle completed",
		slog.Int("total", len(products)),
		slog.Int("published", successCount),
		slog.Int("errors", errorCount),
		slog.Duration("total_duration", time.Since(start)),
	)
}
