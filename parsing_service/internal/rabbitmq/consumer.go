package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"parsing_service/internal/models"

	amqp "github.com/rabbitmq/amqp091-go"
)

type HandlerFunc func(ctx context.Context, product models.Product) error

type Consumer struct {
	ch             *amqp.Channel
	log            *slog.Logger
	queueName      string
	workerPoolSize int
	wg             sync.WaitGroup
}

func NewConsumer(ch *amqp.Channel, log *slog.Logger, queueName string, poolSize int) *Consumer {
	return &Consumer{
		ch:             ch,
		log:            log,
		queueName:      queueName,
		workerPoolSize: poolSize,
	}
}

func (c *Consumer) Consume(
	ctx context.Context,
	handler HandlerFunc,
) error {
	const op = "rabbitmq.Consumer.Consume"

	if err := c.ch.Qos(
		c.workerPoolSize, // prefetch count
		0,                // prefetch size
		false,            // global
	); err != nil {
		return fmt.Errorf("%s: failed to set QoS: %w", op, err)
	}

	_, err := c.ch.QueueDeclare(
		c.queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("%s: failed to declare queue: %w", op, err)
	}

	msgs, err := c.ch.Consume(
		c.queueName,
		"",    // consumer tag (auto-generated)
		false, // auto-ack (manual acknowledgment)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("%s: failed to register consumer: %w", op, err)
	}

	c.log.Info("consumer started",
		slog.String("queue", c.queueName),
		slog.Int("workers", c.workerPoolSize),
	)

	for i := 0; i < c.workerPoolSize; i++ {
		c.wg.Add(1)
		go c.worker(ctx, i, msgs, handler)
	}

	<-ctx.Done()

	c.log.Info("waiting for workers to finish...")
	c.wg.Wait()
	c.log.Info("all workers finished")

	return nil
}

// * worker обрабатывает сообщения из канала
func (c *Consumer) worker(
	ctx context.Context,
	workerID int,
	msgs <-chan amqp.Delivery,
	handler HandlerFunc,
) {
	defer c.wg.Done()

	c.log.Info("worker started", slog.Int("worker_id", workerID))

	for {
		select {
		case <-ctx.Done():
			c.log.Info("worker stopped", slog.Int("worker_id", workerID))
			return

		case msg, ok := <-msgs:
			if !ok {
				c.log.Info("messages channel closed", slog.Int("worker_id", workerID))
				return
			}

			c.processMessage(ctx, workerID, msg, handler)
		}
	}
}

// * processMessage обрабатывает одно сообщение
func (c *Consumer) processMessage(
	ctx context.Context,
	workerID int,
	msg amqp.Delivery,
	handler HandlerFunc,
) {
	start := time.Now()

	var product models.Product
	if err := json.Unmarshal(msg.Body, &product); err != nil {
		c.log.Error("failed to unmarshal message",
			slog.Int("worker_id", workerID),
			slog.String("error", err.Error()),
		)

		// Отклоняем невалидное сообщение без requeue
		if nackErr := msg.Nack(false, false); nackErr != nil {
			c.log.Error("failed to nack message",
				slog.Int("worker_id", workerID),
				slog.String("error", nackErr.Error()),
			)
		}
		return
	}

	c.log.Debug("processing message",
		slog.Int("worker_id", workerID),
		slog.Int64("product_id", product.ID),
	)

	err := handler(ctx, product)
	duration := time.Since(start)

	if err != nil {
		c.log.Error("handler error",
			slog.Int("worker_id", workerID),
			slog.Int64("product_id", product.ID),
			slog.String("error", err.Error()),
			slog.Duration("duration", duration),
		)

		if nackErr := msg.Nack(false, true); nackErr != nil {
			c.log.Error("failed to nack message",
				slog.Int("worker_id", workerID),
				slog.String("error", nackErr.Error()),
			)
		}
		return
	}

	c.log.Debug("message processed successfully",
		slog.Int("worker_id", workerID),
		slog.Int64("product_id", product.ID),
		slog.Duration("duration", duration),
	)

	if ackErr := msg.Ack(false); ackErr != nil {
		c.log.Error("failed to ack message",
			slog.Int("worker_id", workerID),
			slog.String("error", ackErr.Error()),
		)
	}
}
