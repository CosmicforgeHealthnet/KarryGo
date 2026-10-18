package webhooks

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// EventProcessor is the subset of Service the subscriber needs (aids testing).
type EventProcessor interface {
	ProcessDispatchEvent(ctx context.Context, topic string, raw []byte) error
}

// StartSubscribers subscribes to dispatch lifecycle topics and forwards each
// message to the processor. It returns immediately; one goroutine per topic runs
// until ctx is cancelled. Malformed payloads never crash the subscriber.
func StartSubscribers(ctx context.Context, client *redis.Client, processor EventProcessor) {
	if client == nil || processor == nil {
		log.Printf("webhooks subscribers: not started (redis client or processor is nil)")
		return
	}
	for _, topic := range DispatchTopics() {
		go runSubscriber(ctx, client, topic, processor)
		log.Printf("webhooks subscribers: started %s subscriber", topic)
	}
}

func runSubscriber(ctx context.Context, client *redis.Client, topic string, processor EventProcessor) {
	sub := client.Subscribe(ctx, topic)
	defer sub.Close()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			msgCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if err := processor.ProcessDispatchEvent(msgCtx, topic, []byte(msg.Payload)); err != nil {
				log.Printf("webhooks subscriber %s: processing error: %v", topic, err)
			}
			cancel()
		}
	}
}
