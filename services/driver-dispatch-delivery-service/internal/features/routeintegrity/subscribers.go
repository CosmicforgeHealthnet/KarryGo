package routeintegrity

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

func StartSubscribers(ctx context.Context, client *redis.Client, service *Service) {
	if client == nil || service == nil || !service.policy.Enabled {
		return
	}
	go subscribe(ctx, client, "trip.started", func(raw []byte) {
		var event struct {
			TripID     string `json:"trip_id"`
			ProviderID string `json:"provider_id"`
		}
		if json.Unmarshal(raw, &event) == nil && event.TripID != "" && event.ProviderID != "" {
			if _, err := service.EnsureRoute(ctx, event.TripID, event.ProviderID, "TRIP_STARTED"); err != nil && ctx.Err() == nil {
				log.Printf("route integrity start route trip_id=%s: %v", event.TripID, err)
			}
		}
	})
	go subscribe(ctx, client, "provider.location_updated", func(raw []byte) {
		var event struct {
			ProviderID string    `json:"provider_id"`
			Lat        float64   `json:"lat"`
			Lng        float64   `json:"lng"`
			Heading    float64   `json:"heading"`
			Speed      float64   `json:"speed"`
			Accuracy   float64   `json:"accuracy"`
			UpdatedAt  time.Time `json:"updated_at"`
		}
		if json.Unmarshal(raw, &event) == nil && event.ProviderID != "" {
			if err := service.IngestLocation(ctx, LocationFix{ProviderID: event.ProviderID, Latitude: event.Lat, Longitude: event.Lng, Heading: event.Heading, Speed: event.Speed, Accuracy: event.Accuracy, ClientRecordedAt: event.UpdatedAt, ReceivedAt: time.Now().UTC()}); err != nil && ctx.Err() == nil {
				log.Printf("route integrity location evaluation failed: %v", err)
			}
		}
	})
	go service.RunStaleWorker(ctx, 30*time.Second)
}
func subscribe(ctx context.Context, client *redis.Client, topic string, handle func([]byte)) {
	pub := client.Subscribe(ctx, topic)
	defer pub.Close()
	ch := pub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-ch:
			if !ok {
				return
			}
			handle([]byte(message.Payload))
		}
	}
}
