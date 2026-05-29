package jobs

import (
	"context"
	"log"

	"github.com/hibiken/asynq"

	"karrygo/backend/internal/platform/redisx"
)

const (
	TypeExpireStaleBookings = "booking:expire_stale"
	TypeReleasePayouts      = "payout:release"
	TypeSendNotification    = "notification:send"
)

type Worker struct {
	server *asynq.Server
	mux    *asynq.ServeMux
}

func NewWorker(cfg redisx.Config, mux *asynq.ServeMux) *Worker {
	return &Worker{
		server: asynq.NewServer(redisOptions(cfg), asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		}),
		mux: mux,
	}
}

func NewHandlerMux() *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeExpireStaleBookings, logOnlyHandler(TypeExpireStaleBookings))
	mux.HandleFunc(TypeReleasePayouts, logOnlyHandler(TypeReleasePayouts))
	mux.HandleFunc(TypeSendNotification, logOnlyHandler(TypeSendNotification))
	return mux
}

func (w *Worker) Run() error {
	return w.server.Run(w.mux)
}

func (w *Worker) Shutdown() {
	w.server.Shutdown()
}

func logOnlyHandler(taskType string) asynq.HandlerFunc {
	return func(ctx context.Context, task *asynq.Task) error {
		log.Printf("job=%s payload=%s", taskType, string(task.Payload()))
		return nil
	}
}
