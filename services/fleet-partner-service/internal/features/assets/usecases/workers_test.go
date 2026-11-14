package usecases

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
)

func TestSection18WorkerRejectsTenantMismatch(t *testing.T) {
	s := &Service{}
	err := s.handleMaintenance(context.Background(), jobs.Job{OrganizationID: "org-a", Payload: []byte(`{"organizationId":"org-b"}`)})
	if !errors.Is(err, jobs.ErrTenantMismatch) {
		t.Fatalf("expected tenant mismatch, got %v", err)
	}
}
