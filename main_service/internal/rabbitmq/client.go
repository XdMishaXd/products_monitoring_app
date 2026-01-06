package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

type RabbitMQClient struct {
	conn    *amqp.Connection
	Channel *amqp.Channel
}

func New(url string) (*RabbitMQClient, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq dial: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}

	return &RabbitMQClient{
		conn:    conn,
		Channel: ch,
	}, nil
}

func (c *RabbitMQClient) Close() error {
	if err := c.Channel.Close(); err != nil {
		return err
	}
	return c.conn.Close()
}
