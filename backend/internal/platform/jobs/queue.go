package jobs

import (
	"context"
	"encoding/json"

	"github.com/hibiken/asynq"

	"karrygo/backend/internal/platform/apperrors"
	"karrygo/backend/internal/platform/redisx"
)

type Queue struct {
	client *asynq.Client
}

func NewQueue(cfg redisx.Config) *Queue {
	return &Queue{client: asynq.NewClient(redisOptions(cfg))}
}

func (q *Queue) Enqueue(ctx context.Context, taskType string, payload interface{}, opts ...asynq.Option) (*asynq.TaskInfo, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, apperrors.Internal("Job payload could not be prepared.", err)
	}

	info, err := q.client.EnqueueContext(ctx, asynq.NewTask(taskType, body), opts...)
	if err != nil {
		return nil, apperrors.Unavailable("Job queue is temporarily unavailable.", err)
	}

	return info, nil
}

func (q *Queue) Close() error {
	return q.client.Close()
}

func redisOptions(cfg redisx.Config) asynq.RedisClientOpt {
	return asynq.RedisClientOpt{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	}
}
