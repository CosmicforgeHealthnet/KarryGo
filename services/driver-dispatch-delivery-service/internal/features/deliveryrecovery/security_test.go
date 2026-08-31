package deliveryrecovery

import (
	"context"
	"cosmicforge/logistics/shared/go/apperrors"
	"testing"
)

func TestEmergencyRequiresActorReasonAndExpectedVersionBeforeMutation(t *testing.T) {
	s := New(nil, Policy{Enabled: true, ReplacementEnabled: true})
	valid := EmergencyRecoveryCommand{TripID: "trip-a", ActorRef: "admin-backoffice-service:operator-a", ReasonCode: "CUSTODY_SAFETY", ExpectedTripVersion: 1, IdempotencyKey: "operation-a"}
	for _, field := range []string{"actor", "reason", "version", "idempotency"} {
		c := valid
		switch field {
		case "actor":
			c.ActorRef = ""
		case "reason":
			c.ReasonCode = ""
		case "version":
			c.ExpectedTripVersion = 0
		case "idempotency":
			c.IdempotencyKey = ""
		}
		if _, err := s.RecordEmergencyRecovery(context.Background(), c); apperrors.From(err).Status != 400 {
			t.Fatalf("missing %s accepted", field)
		}
	}
}
