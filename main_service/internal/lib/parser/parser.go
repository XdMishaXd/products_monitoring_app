package parser

import (
	"context"
	"encoding/json"
	"fmt"

	"main_service/internal/models"
)

type PostgresStorage interface {
	UpdateParsedData(
		ctx context.Context,
		productID int64,
		price float32,
		inStock bool,
	) error
}

type Consumer interface {
	Consume(ctx context.Context, handler func(ctx context.Context, body []byte) error) error
}

type Parser struct {
	postgres         PostgresStorage
	rabbitmqConsumer Consumer
}

func New(pg PostgresStorage, c Consumer) *Parser {
	return &Parser{
		postgres:         pg,
		rabbitmqConsumer: c,
	}
}

func (s *Parser) Run(ctx context.Context) error {
	return s.rabbitmqConsumer.Consume(ctx, s.handleMessage)
}

func (p *Parser) handleMessage(ctx context.Context, body []byte) error {
	var msg models.ParsedProduct

	if err := json.Unmarshal(body, &msg); err != nil {
		return fmt.Errorf("invalid message format: %w", err)
	}

	return p.postgres.UpdateParsedData(
		ctx,
		msg.ID,
		msg.Price,
		msg.In_stock,
	)
}
