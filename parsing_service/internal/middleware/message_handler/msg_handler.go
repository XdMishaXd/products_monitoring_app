package msgHandler

import (
	"context"
	"log/slog"
	"time"

	"parsing_service/internal/models"
	"parsing_service/internal/parsers"
	"parsing_service/internal/rabbitmq"
)

type MessageHandler struct {
	log        *slog.Logger
	producer   *rabbitmq.Producer
	ebayParser *parsers.EbayParser
	// TODO: etsyParser       *parsers.EtsyParser
	// TODO: aliexpressParser *parsers.AliexpressParser
	maxRetries int
}

func New(
	log *slog.Logger,
	producer *rabbitmq.Producer,
	ebayParser *parsers.EbayParser,
	// etsyParser *parsers.EtsyParser,
	// aliexpressParser *parsers.AliexpressParser,
	maxReties int,
) *MessageHandler {
	return &MessageHandler{
		log:        log,
		producer:   producer,
		ebayParser: ebayParser,
		// etsyParser:       etsyParser,
		// aliexpressParser: aliexpressParser,
		maxRetries: maxReties,
	}
}

// * Handle обрабатывает одно сообщение из очереди (выполняется в отдельной горутине воркера)
func (h *MessageHandler) Handle(ctx context.Context, product models.Product) error {
	h.log.Info("processing product",
		slog.Int64("product_id", product.ID),
		slog.String("marketplace", string(product.Marketplace)),
		slog.String("url", product.URL),
	)

	parseCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var info *models.ParsedProduct
	var err error

	switch product.Marketplace {
	case "ebay":
		info, err = h.ebayParser.ParseWithRetry(parseCtx, product, h.maxRetries)
	// case "etsy":
	// 	info, err = h.etsyParser.ParseWithRetry(parseCtx, product.URL, h.maxRetries)
	// case "aliexpress":
	// 	info, err = h.aliexpressParser.ParseWithRetry(parseCtx, product.URL, h.maxRetries)
	default:
		h.log.Error("unknown marketplace",
			slog.String("marketplace", string(product.Marketplace)),
			slog.Int64("product_id", product.ID),
		)
		return nil // Не возвращаем ошибку, чтобы не перезапускать обработку
	}

	if err != nil {
		h.log.Error("failed to parse product",
			slog.Int64("product_id", product.ID),
			slog.String("marketplace", string(product.Marketplace)),
			slog.String("err", err.Error()),
		)

		resultMsg := &models.ParsedProduct{
			ID:  product.ID,
			Err: err,
		}

		if sendErr := h.producer.PublishJSON(ctx, resultMsg); sendErr != nil {
			h.log.Error("failed to send error result",
				slog.Int64("product_id", product.ID),
				slog.String("err", sendErr.Error()),
			)
		}

		return nil
	}

	h.log.Info("product parsed successfully",
		slog.Int64("product_id", product.ID),
		slog.Float64("price", float64(info.Price)),
		slog.Bool("in_stock", info.InStock),
	)

	resultMsg := &models.ParsedProduct{
		ID:      product.ID,
		Price:   info.Price,
		InStock: info.InStock,
		Err:     nil,
	}

	if err := h.producer.PublishJSON(ctx, resultMsg); err != nil {
		h.log.Error("failed to send parse result",
			slog.Int64("product_id", product.ID),
			slog.String("err", err.Error()),
		)
		return err
	}

	h.log.Info("parse result sent successfully",
		slog.Int64("product_id", product.ID),
	)

	return nil
}
