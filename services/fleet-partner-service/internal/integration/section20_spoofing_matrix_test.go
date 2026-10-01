package integration_test

import (
	"context"
	"errors"
	"testing"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/fleetpartner"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/providercompat"
)

// Section 20 / IAM-006 / IAM-007: one execution service must never be able to
// act as another, and Vendor must never be able to act as any of them.
//
// The rule already existed; what did not exist was a test that walks every
// wrong pairing rather than one or two. These run against Fleet's real
// ingestion and a real database, because the check that matters is the one in
// the code path a live submission takes.

// spoofCases enumerates every caller/source pairing that must be refused,
// including the two the specification names explicitly and the ones a reader
// would expect to be covered by them.
func spoofCases() []struct {
	name, caller, source string
} {
	return []struct {
		name, caller, source string
	}{
		{"taxi caller claiming a dispatch job", "driver-taxi-service", fleetpartner.ServiceDispatch},
		{"taxi caller claiming a hauling job", "driver-taxi-service", fleetpartner.ServiceHauling},
		{"dispatch caller claiming a taxi job", "driver-dispatch-delivery-service", fleetpartner.ServiceTaxi},
		{"dispatch caller claiming a hauling job", "driver-dispatch-delivery-service", fleetpartner.ServiceHauling},
		{"hauling caller claiming a dispatch job", "driver-hauling-service", fleetpartner.ServiceDispatch},
		{"hauling caller claiming a taxi job", "driver-hauling-service", fleetpartner.ServiceTaxi},
		{"vendor claiming a dispatch job", "vendor-service", fleetpartner.ServiceDispatch},
		{"vendor claiming a taxi job", "vendor-service", fleetpartner.ServiceTaxi},
		{"vendor claiming a hauling job", "vendor-service", fleetpartner.ServiceHauling},
		{"wallet claiming a dispatch job", "payment-wallet-service", fleetpartner.ServiceDispatch},
		{"fleet claiming its own job", "fleet-partner-service", fleetpartner.ServiceDispatch},
		{"an unknown caller", "attacker-service", fleetpartner.ServiceDispatch},
	}
}

func TestCrossServiceCompletedWorkSpoofingIsRefused(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	ctx := context.Background()

	for _, spoof := range spoofCases() {
		t.Run(spoof.name, func(t *testing.T) {
			orgID, membershipID, providerID := seedManagedMembership(t, pool, spoof.source)
			jobID := ids.New("job")
			request := canonicalRequest(orgID, membershipID, providerID, spoof.source, jobID)

			_, _, _, err := service.RecordCompletedWork(ctx, spoof.caller, asInput(request))
			if err == nil {
				t.Fatal("the spoofed completion was accepted")
			}
			var appErr *apperrors.Error
			if !errors.As(err, &appErr) || appErr.Status != 403 {
				t.Fatalf("err = %v; want a 403 refusal", err)
			}
			// Nothing may be written on a refused submission: a rejected
			// spoof that still leaves a row is a spoof that worked.
			if got := countCompletedWork(t, pool, jobID); got != 0 {
				t.Fatalf("completed-work rows = %d after a refused spoof", got)
			}
		})
	}
}

// The matching caller/source pairings must still work, so the test above is
// proving a boundary rather than a service that refuses everything.
func TestEachExecutionServiceOwnsItsOwnSource(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	ctx := context.Background()

	for caller, source := range map[string]string{
		"driver-taxi-service":              fleetpartner.ServiceTaxi,
		"driver-dispatch-delivery-service": fleetpartner.ServiceDispatch,
		"driver-hauling-service":           fleetpartner.ServiceHauling,
	} {
		orgID, membershipID, providerID := seedManagedMembership(t, pool, source)
		jobID := ids.New("job")
		request := canonicalRequest(orgID, membershipID, providerID, source, jobID)

		if _, _, _, err := service.RecordCompletedWork(ctx, caller, asInput(request)); err != nil {
			t.Fatalf("%s submitting its own %s completion: %v", caller, source, err)
		}
		if got := countCompletedWork(t, pool, jobID); got != 1 {
			t.Fatalf("%s/%s wrote %d rows; want 1", caller, source, got)
		}
	}
}

// AUTH-01 / IAM-003: the organization on the request is a claim, not a fact.
// A real execution service submitting a real provider's completion cannot
// attribute it to an organization that provider does not belong to.
func TestClientSuppliedOrganizationCannotBeSpoofed(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	ctx := context.Background()

	_, victimMembership, victimProvider := seedManagedMembership(t, pool, fleetpartner.ServiceTaxi)
	attackerOrg, _, _ := seedManagedMembership(t, pool, fleetpartner.ServiceTaxi)

	jobID := ids.New("job")
	request := canonicalRequest(attackerOrg, victimMembership, victimProvider,
		fleetpartner.ServiceTaxi, jobID)

	_, _, _, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(request))
	if err == nil {
		t.Fatal("a completion was attributed to an organization the provider does not belong to")
	}
	if got := countCompletedWork(t, pool, jobID); got != 0 {
		t.Fatalf("completed-work rows = %d after a refused organization spoof", got)
	}
}

// AUTH-02 / IAM-004: the membership on the request is a lookup hint, not
// authority. One provider's completion cannot be booked against another
// provider's membership.
func TestClientSuppliedMembershipCannotBeSpoofed(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	ctx := context.Background()

	orgID, _, attackerProvider := seedManagedMembership(t, pool, fleetpartner.ServiceTaxi)
	_, victimMembership, _ := seedManagedMembership(t, pool, fleetpartner.ServiceTaxi)

	jobID := ids.New("job")
	request := canonicalRequest(orgID, victimMembership, attackerProvider,
		fleetpartner.ServiceTaxi, jobID)

	_, _, _, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(request))
	if err == nil {
		t.Fatal("a completion was booked against another provider's membership")
	}
	if got := countCompletedWork(t, pool, jobID); got != 0 {
		t.Fatalf("completed-work rows = %d after a refused membership spoof", got)
	}
}

// AUTH-05 / IAM-012 / IAM-013: a membership suspended while the provider was
// already working does NOT erase the work.
//
// This is the policy boundary the section asks to be explicit about, and it
// is easy to get backwards. Refusing the completion here would look like
// enforcement and would actually mean: the provider finished the delivery,
// the customer received the goods, and the platform has no record of who did
// it or what they are owed. The organization's decision belongs at
// acquisition — no new offer, no new assignment, no new execution, which is
// where providercompat's membership gate sits and what
// TestSuspendedExpiredAndTerminatedAreRefusedAndDistinct covers — while work
// already in hand is recorded against the membership it was performed under.
func TestSuspensionDuringWorkStillRecordsTheCompletion(t *testing.T) {
	pool := testPool(t)
	service := newCompletedWorkService(t, pool, sectionsFlags())
	ctx := context.Background()

	orgID, membershipID, providerID := seedManagedMembership(t, pool, fleetpartner.ServiceTaxi)
	jobID := ids.New("job")
	request := canonicalRequest(orgID, membershipID, providerID, fleetpartner.ServiceTaxi, jobID)

	// The organization suspends the provider while the trip is in progress.
	if _, err := pool.Exec(ctx, `
		UPDATE fleet_partner_organization_provider_memberships
		   SET status='SUSPENDED', suspended_at=now() WHERE id=$1`, membershipID); err != nil {
		t.Fatalf("suspend membership: %v", err)
	}

	record, _, _, err := service.RecordCompletedWork(ctx, "driver-taxi-service", asInput(request))
	if err != nil {
		t.Fatalf("a completed delivery was lost because the provider was suspended mid-trip: %v", err)
	}
	if got := countCompletedWork(t, pool, jobID); got != 1 {
		t.Fatalf("completed-work rows = %d; want the completion recorded exactly once", got)
	}

	// IAM-013: the record keeps the membership the work was performed under,
	// so the money can still be attributed after the relationship changed.
	if record.MembershipID != membershipID || record.OrganizationID != orgID {
		t.Fatalf("the completion lost its corporate context: membership=%q organization=%q",
			record.MembershipID, record.OrganizationID)
	}

	// The suspension is still real: it blocks NEW work. That gate lives in
	// the shared corporate-context check every execution service consults
	// before it assigns or executes anything, not in this ingestion path.
	if providercompat.MembershipSuspended.AllowsNewManagedWork() {
		t.Fatal("a suspended membership would be allowed to start new work")
	}
	if !providercompat.MembershipSuspended.AllowsActiveWorkCompletion() {
		t.Fatal("the documented policy no longer allows finishing work already in custody")
	}
}
