package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"

	"main_service/internal/models"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	ch    *amqp.Channel
	queue amqp.Queue
}

func NewProducer(ch *amqp.Channel, queueName string) (*Producer, error) {
	const op = "rabbitmq.NewProducer"

	q, err := ch.QueueDeclare(
		queueName, true, false, false, false, nil,
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Producer{
		ch:    ch,
		queue: q,
	}, nil
}

func (p *Producer) PublishJSON(
	ctx context.Context,
	product models.ProductForProducer,
) error {
	body, err := json.Marshal(product)
	if err != nil {
		return err
	}

	return p.ch.PublishWithContext(
		ctx,
		"",
		p.queue.Name,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}
