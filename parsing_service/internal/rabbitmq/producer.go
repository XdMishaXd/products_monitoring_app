package rabbitmq

import (
	"context"
	"encoding/json"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Producer struct {
	ch        *amqp.Channel
	queueName string
}

func NewProducer(ch *amqp.Channel, queueName string) *Producer {
	return &Producer{
		ch:        ch,
		queueName: queueName,
	}
}

func (p *Producer) PublishJSON(
	ctx context.Context,
	msg any,
) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return p.ch.PublishWithContext(
		ctx,
		"",
		p.queueName,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}
