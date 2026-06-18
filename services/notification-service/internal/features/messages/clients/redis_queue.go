package messageclients

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"cosmicforge/logistics/shared/go/notifications"
)

type RedisQueue struct {
	client           *redis.Client
	requestStream    string
	deliveryStream   string
	deadLetterStream string
	group            string
	consumer         string
}

func NewRedisQueue(client *redis.Client, requestStream string, deliveryStream string, deadLetterStream string) *RedisQueue {
	return &RedisQueue{
		client:           client,
		requestStream:    requestStream,
		deliveryStream:   deliveryStream,
		deadLetterStream: deadLetterStream,
		group:            notifications.ConsumerGroup,
		consumer:         "notification-worker",
	}
}

func (q *RedisQueue) EnqueueDelivery(ctx context.Context, deliveryID string) error {
	return q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.deliveryStream,
		Values: map[string]interface{}{"delivery_id": deliveryID},
	}).Err()
}

func (q *RedisQueue) DeadLetterDelivery(ctx context.Context, deliveryID string, reason string) error {
	return q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.deadLetterStream,
		Values: map[string]interface{}{
			"delivery_id": deliveryID,
			"reason":      reason,
		},
	}).Err()
}

func (q *RedisQueue) StartDeliveryConsumer(ctx context.Context, handler func(context.Context, string) error) {
	q.ensureGroup(ctx, q.deliveryStream)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    q.group,
			Consumer: q.consumer + "-delivery",
			Streams:  []string{q.deliveryStream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			log.Printf("notification_delivery_consumer error=%v", err)
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				deliveryID, _ := message.Values["delivery_id"].(string)
				if deliveryID != "" {
					if err := handler(ctx, deliveryID); err != nil {
						log.Printf("notification_delivery_handler delivery_id=%s error=%v", deliveryID, err)
					}
				}
				_ = q.client.XAck(ctx, q.deliveryStream, q.group, message.ID).Err()
			}
		}
	}
}

func (q *RedisQueue) StartRequestConsumer(ctx context.Context, handler func(context.Context, []byte) error) {
	q.ensureGroup(ctx, q.requestStream)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := q.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    q.group,
			Consumer: q.consumer + "-request",
			Streams:  []string{q.requestStream, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			log.Printf("notification_request_consumer error=%v", err)
			continue
		}
		for _, stream := range streams {
			for _, message := range stream.Messages {
				payload, _ := message.Values["payload"].(string)
				if payload == "" {
					if raw, ok := message.Values["request"].(string); ok {
						payload = raw
					}
				}
				if payload != "" {
					if err := handler(ctx, []byte(payload)); err != nil {
						log.Printf("notification_request_handler error=%v", err)
					}
				}
				_ = q.client.XAck(ctx, q.requestStream, q.group, message.ID).Err()
			}
		}
	}
}

func (q *RedisQueue) ensureGroup(ctx context.Context, stream string) {
	err := q.client.XGroupCreateMkStream(ctx, stream, q.group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		log.Printf("notification_stream_group stream=%s error=%v", stream, err)
	}
}

func DecodeNotificationRequest(payload []byte) (notifications.Request, error) {
	var request notifications.Request
	if err := json.Unmarshal(payload, &request); err != nil {
		return notifications.Request{}, err
	}
	return request, nil
}
