package request

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"github.com/hibiken/asynq"
)

type Worker struct {
	service *Service
}

func NewWorker(service *Service) *Worker {
	return &Worker{service: service}
}

func (w *Worker) HandleExpireWindow(ctx context.Context, task *asynq.Task) error {
	var payload ExpireWindowPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	broadcast, ok, err := w.service.repository.GetBroadcastByID(ctx, payload.BroadcastID)
	if err != nil || !ok || broadcast.Status != BroadcastStatusBroadcasting {
		return err
	}
	if payload.AttemptNumber != broadcast.AttemptNumber {
		return nil
	}
	if broadcast.ExpiresAt.After(w.service.now()) {
		return nil
	}
	if err := w.service.repository.MarkPendingInboxExpired(ctx, broadcast.ID); err != nil {
		return err
	}
	if w.service.redis != nil {
		_ = w.service.redis.Del(ctx, RequestBroadcastingKey(broadcast.BookingID)).Err()
	}
	if broadcast.AttemptNumber >= w.service.config.MaxAttempts {
		if err := w.service.repository.MarkBroadcastNoProviderFound(ctx, broadcast.ID); err != nil {
			return err
		}
		if w.service.events != nil {
			return w.service.events.PublishNoProviderFound(ctx, NoProviderFoundEvent{
				Event: TopicNoProviderFound, BookingID: broadcast.BookingID, BroadcastID: broadcast.ID,
				Attempts: broadcast.AttemptNumber, OccurredAt: w.service.now(),
			})
		}
		return nil
	}
	if w.service.tasks == nil {
		return nil
	}
	next, err := NewReBroadcastTask(ReBroadcastPayload{
		BroadcastID: broadcast.ID, BookingID: broadcast.BookingID, AttemptNumber: broadcast.AttemptNumber + 1,
		NewRadiusKM: broadcast.BroadcastRadiusKM + w.service.config.RadiusIncrementKM,
	})
	if err != nil {
		return err
	}
	_, err = w.service.tasks.Enqueue(next, asynq.Queue("default"))
	return err
}

func (w *Worker) HandleReBroadcast(ctx context.Context, task *asynq.Task) error {
	var payload ReBroadcastPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	broadcast, ok, err := w.service.repository.GetBroadcastByID(ctx, payload.BroadcastID)
	if err != nil || !ok || broadcast.Status != BroadcastStatusBroadcasting {
		return err
	}
	if payload.AttemptNumber != broadcast.AttemptNumber+1 {
		return nil
	}
	var event BookingDispatchCreatedEvent
	if err := json.Unmarshal(broadcast.BookingPayload, &event); err != nil {
		return err
	}
	now := w.service.now()
	broadcast.BroadcastAt = now
	broadcast.ExpiresAt = now.Add(w.service.config.BroadcastWindow)
	radius := payload.NewRadiusKM
	if radius <= broadcast.BroadcastRadiusKM {
		radius = broadcast.BroadcastRadiusKM + w.service.config.RadiusIncrementKM
	}
	if err := w.service.broadcastToNearby(ctx, &broadcast, event, payload.AttemptNumber, radius); err != nil {
		return err
	}
	return w.service.scheduleExpire(broadcast)
}

func (w *Worker) HandleSendPush(ctx context.Context, task *asynq.Task) error {
	var payload SendPushPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	err := w.service.notifications.SendRequestBroadcast(ctx, payload.ProviderID, RequestPushPayload{
		InboxID: payload.InboxID, BroadcastID: payload.BroadcastID, BookingID: payload.BookingID,
		FareAmount: payload.FareAmount, PickupAddress: payload.PickupAddress, DropoffAddress: payload.DropoffAddress,
		PackageDesc: payload.PackageDesc, ReceiverName: payload.ReceiverName, ExpiresIn: payload.ExpiresIn, Type: "new_request",
	})
	if errors.Is(err, ErrNoFCMToken) {
		log.Printf("request send push skipped provider_id=%s inbox_id=%s: no FCM token", payload.ProviderID, payload.InboxID)
		return nil
	}
	if err != nil {
		return err
	}
	return w.service.repository.MarkFCMSent(ctx, payload.InboxID, w.service.now())
}

func RegisterWorkerHandlers(mux *asynq.ServeMux, worker *Worker) {
	mux.HandleFunc(TaskSendPush, worker.HandleSendPush)
	mux.HandleFunc(TaskExpireWindow, worker.HandleExpireWindow)
	mux.HandleFunc(TaskReBroadcast, worker.HandleReBroadcast)
}
