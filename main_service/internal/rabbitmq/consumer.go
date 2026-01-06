package rabbitmq

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

type HandlerFunc func(ctx context.Context, body []byte) error

type Consumer struct {
	ch             *amqp.Channel
	log            *slog.Logger
	queue          amqp.Queue
	workerPoolSize int
}

func NewConsumer(ch *amqp.Channel, log *slog.Logger, queueName string, poolSize int) (*Consumer, error) {
	const op = "rabbitmq.NewConsumer"

	q, err := ch.QueueDeclare(
		queueName, true, false, false, false, nil,
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &Consumer{
		ch:             ch,
		log:            log,
		queue:          q,
		workerPoolSize: poolSize,
	}, nil
}

func (c *Consumer) Consume(
	ctx context.Context,
	handler func(context.Context, []byte) error,
) error {
	const op = "rabbitmq.Consume"

	if err := c.ch.Qos(
		c.workerPoolSize,
		0,
		false,
	); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	msgs, err := c.ch.Consume(
		c.queue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		var wg sync.WaitGroup
		semaphore := make(chan struct{}, 10) // * макс. 10 параллельных обработок

		for {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case msg, ok := <-msgs:
				if !ok {
					wg.Wait()
					return
				}

				wg.Add(1)
				semaphore <- struct{}{}

				go func(m amqp.Delivery) {
					defer wg.Done()
					defer func() { <-semaphore }()

					if err := handler(ctx, m.Body); err != nil {
						err := m.Nack(false, true)
						if err != nil {
							c.log.Error(
								"nack failed",
								slog.String("op", op),
								slog.Any("error", err),
							)
						}
					} else {
						err := m.Ack(false)
						if err != nil {
							c.log.Error(
								"ack failed",
								slog.String("op", op),
								slog.Any("error", err),
							)
						}
					}
				}(msg)
			}
		}
	}()

	return nil
}
