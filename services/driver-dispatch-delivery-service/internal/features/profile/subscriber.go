package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
)

const (
	TopicVerificationStatusUpdated = "verification.status.updated"
	TopicTripCompleted             = "trip.completed"
	TopicCustomerRatingSubmitted   = "customer.rating.submitted"
)

type VerificationStatusUpdatedEvent struct {
	Event              string             `json:"event"`
	ProviderID         string             `json:"provider_id"`
	VerificationStatus VerificationStatus `json:"verification_status"`
	Status             VerificationStatus `json:"status,omitempty"`
	CreatedAt          time.Time          `json:"created_at"`
}

type TripCompletedEvent struct {
	Event      string    `json:"event"`
	ProviderID string    `json:"provider_id"`
	TripID     string    `json:"trip_id,omitempty"`
	DeliveryID string    `json:"delivery_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type CustomerRatingSubmittedEvent struct {
	Event             string    `json:"event"`
	ProviderID        string    `json:"provider_id"`
	BookingID         string    `json:"booking_id"`
	RatedByCustomerID string    `json:"rated_by_customer_id"`
	Score             int       `json:"score"`
	Comment           *string   `json:"comment,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func StartSubscribers(ctx context.Context, client *redis.Client, repo Repository) {
	if client == nil || repo == nil {
		log.Printf("profile subscribers: not started because redis client or repository is nil")
		return
	}

	subscribe(ctx, client, authclients.TopicSessionCreated, func(ctx context.Context, payload []byte) error {
		return HandleSessionCreatedPayload(ctx, repo, payload)
	})
	subscribe(ctx, client, TopicVerificationStatusUpdated, func(ctx context.Context, payload []byte) error {
		return HandleVerificationStatusUpdatedPayload(ctx, repo, payload)
	})
	subscribe(ctx, client, TopicTripCompleted, func(ctx context.Context, payload []byte) error {
		return HandleTripCompletedPayload(ctx, repo, payload)
	})
	subscribe(ctx, client, TopicCustomerRatingSubmitted, func(ctx context.Context, payload []byte) error {
		return HandleCustomerRatingSubmittedPayload(ctx, repo, payload)
	})
}

// SubscribeSessionCreated is kept for compatibility with earlier callers.
func SubscribeSessionCreated(ctx context.Context, client *redis.Client, repo Repository) {
	subscribe(ctx, client, authclients.TopicSessionCreated, func(ctx context.Context, payload []byte) error {
		return HandleSessionCreatedPayload(ctx, repo, payload)
	})
}

func subscribe(ctx context.Context, client *redis.Client, topic string, handle func(context.Context, []byte) error) {
	if client == nil {
		log.Printf("profile subscriber %s: redis client is nil", topic)
		return
	}
	go func() {
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
				msgCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if err := handle(msgCtx, []byte(msg.Payload)); err != nil {
					log.Printf("profile subscriber %s: %v", topic, err)
				}
				cancel()
			}
		}
	}()
}

func HandleSessionCreatedPayload(ctx context.Context, repo Repository, payload []byte) error {
	var event authclients.SessionCreatedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal session_created: %w", err)
	}
	if err := validateUUID("provider_id", event.ProviderID); err != nil {
		return err
	}
	if strings.TrimSpace(event.PhoneNumber) == "" {
		return fmt.Errorf("phone_number is required")
	}
	_, err := repo.EnsureProvider(ctx, event.ProviderID, event.PhoneNumber)
	return err
}

func HandleVerificationStatusUpdatedPayload(ctx context.Context, repo Repository, payload []byte) error {
	var event VerificationStatusUpdatedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal verification_status_updated: %w", err)
	}
	if err := validateUUID("provider_id", event.ProviderID); err != nil {
		return err
	}
	status := event.VerificationStatus
	if status == "" {
		status = event.Status
	}
	if !isValidVerificationStatus(status) {
		return fmt.Errorf("verification_status is invalid")
	}
	return repo.UpdateVerificationStatus(ctx, event.ProviderID, status)
}

func HandleTripCompletedPayload(ctx context.Context, repo Repository, payload []byte) error {
	var event TripCompletedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal trip_completed: %w", err)
	}
	if err := validateUUID("provider_id", event.ProviderID); err != nil {
		return err
	}
	return repo.IncrementTotalTrips(ctx, event.ProviderID)
}

func HandleCustomerRatingSubmittedPayload(ctx context.Context, repo Repository, payload []byte) error {
	var event CustomerRatingSubmittedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal customer_rating_submitted: %w", err)
	}
	if err := validateUUID("provider_id", event.ProviderID); err != nil {
		return err
	}
	if err := validateUUID("booking_id", event.BookingID); err != nil {
		return err
	}
	if err := validateUUID("rated_by_customer_id", event.RatedByCustomerID); err != nil {
		return err
	}
	if event.Score < 1 || event.Score > 5 {
		return fmt.Errorf("score must be between 1 and 5")
	}
	_, err := repo.InsertRatingAndRecalculate(ctx, RatingInput{
		ProviderID:        event.ProviderID,
		BookingID:         event.BookingID,
		RatedByCustomerID: event.RatedByCustomerID,
		Score:             event.Score,
		Comment:           event.Comment,
	})
	return err
}

func validateUUID(field string, value string) error {
	if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
		return fmt.Errorf("%s must be a valid UUID", field)
	}
	return nil
}

func isValidVerificationStatus(status VerificationStatus) bool {
	switch status {
	case StatusUnverified, StatusPendingReview, StatusVerified, StatusSuspended, StatusRejected:
		return true
	default:
		return false
	}
}
