package request

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const (
	TaskExpireWindow = "request:expire_window"
	TaskReBroadcast  = "request:rebroadcast"
	TaskSendPush     = "request:send_push"
)

type ExpireWindowPayload struct {
	BroadcastID   string `json:"broadcast_id"`
	BookingID     string `json:"booking_id"`
	AttemptNumber int    `json:"attempt_number"`
}

type ReBroadcastPayload struct {
	BroadcastID   string  `json:"broadcast_id"`
	BookingID     string  `json:"booking_id"`
	AttemptNumber int     `json:"attempt_number"`
	NewRadiusKM   float64 `json:"new_radius_km"`
}

type SendPushPayload struct {
	ProviderID     string  `json:"provider_id"`
	InboxID        string  `json:"inbox_id"`
	BroadcastID    string  `json:"broadcast_id"`
	BookingID      string  `json:"booking_id"`
	FareAmount     int64   `json:"fare_amount"`
	PickupAddress  string  `json:"pickup_address"`
	DropoffAddress string  `json:"dropoff_address"`
	DistanceKM     float64 `json:"distance_km"`
	PackageDesc    string  `json:"package_desc"`
	ReceiverName   string  `json:"receiver_name"`
	ExpiresIn      int     `json:"expires_in"`
}

func NewExpireWindowTask(payload ExpireWindowPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskExpireWindow, data), nil
}

func NewReBroadcastTask(payload ReBroadcastPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskReBroadcast, data), nil
}

func NewSendPushTask(payload SendPushPayload) (*asynq.Task, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskSendPush, data), nil
}
