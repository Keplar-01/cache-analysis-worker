package kafka

import (
	"context"
	"encoding/json"
	"log"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/diploma/worker-cache-interpreter/internal/model"
)

type MessageHandler interface {
	HandleStartEvent(ctx context.Context, event model.StartEvent) error
}

type Consumer struct {
	reader  *kafkago.Reader
	handler MessageHandler
}

func NewConsumer(brokers []string, handler MessageHandler) *Consumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: brokers,
		Topic:   TopicStartCache,
		GroupID: "worker-cache-group",
	})

	return &Consumer{
		reader:  reader,
		handler: handler,
	}
}

func (c *Consumer) Listen(ctx context.Context) {
	log.Println("[kafka] consumer started, topic:", TopicStartCache)

	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("[kafka] consumer context cancelled, stopping")
				return
			}
			log.Printf("[kafka] fetch error: %v", err)
			continue
		}

		var event model.StartEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("[kafka] unmarshal error: %v", err)
			if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
				log.Printf("[kafka] commit error: %v", commitErr)
			}
			continue
		}

		log.Printf("[kafka] received start_cache event: task_id=%s", event.TaskID)

		if err := c.handler.HandleStartEvent(ctx, event); err != nil {
			log.Printf("[kafka] handle error for task %s: %v", event.TaskID, err)
		}

		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("[kafka] commit error: %v", err)
		}
	}
}

func (c *Consumer) Close() error {
	return c.reader.Close()
}
