package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron     *cron.Cron
	queue    *Queue
	stopOnce sync.Once
}

func NewScheduler(queue *Queue) *Scheduler {
	return &Scheduler{
		cron:  cron.New(cron.WithSeconds()),
		queue: queue,
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	if _, err := s.cron.AddFunc("0 */5 * * * *", func() {
		_, _ = s.queue.Enqueue(context.Background(), TypeExpireStaleBookings, map[string]string{
			"source": "cron",
		}, asynq.Queue("low"), asynq.MaxRetry(3), asynq.Timeout(2*time.Minute))
	}); err != nil {
		return err
	}

	if _, err := s.cron.AddFunc("0 0 2 * * *", func() {
		_, _ = s.queue.Enqueue(context.Background(), TypeReleasePayouts, map[string]string{
			"source": "cron",
		}, asynq.Queue("default"), asynq.MaxRetry(5), asynq.Timeout(10*time.Minute))
	}); err != nil {
		return err
	}

	s.cron.Start()

	go func() {
		<-ctx.Done()
		s.Stop()
	}()

	return nil
}

func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		stopCtx := s.cron.Stop()
		<-stopCtx.Done()
	})
}
