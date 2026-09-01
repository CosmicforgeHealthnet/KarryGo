package assignment

import (
	"context"
	"testing"
)

type completedStartStore struct{ Store }

func (completedStartStore) ReadStart(_ context.Context, in StartRequest, strategy Strategy) (Assignment, bool, error) {
	return Assignment{MissionID: in.MissionID, Strategy: strategy, Status: "ASSIGNED", TripID: "canonical-trip", Replayed: true}, true, nil
}
func TestStartReplayDoesNotRequireWinningProviderToStillBeAvailable(t *testing.T) {
	service := New(completedStartStore{}, nil, Policy{SequentialEnabled: true})
	result, err := service.Start(context.Background(), StartRequest{MissionID: "mission", IdempotencyKey: "same-operation", Strategy: SequentialNearest}, false)
	if err != nil || !result.Replayed || result.TripID != "canonical-trip" {
		t.Fatal(result, err)
	}
}
