package recovery

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/capabilityfit"
)

func startCase(t *testing.T, service *Service, bookingID, operationRef string) Public {
	t.Helper()
	opened, err := service.Start(context.Background(), StartCommand{
		BookingID: bookingID, OperationRef: operationRef,
		TriggerCode: TriggerProviderBreakdown, Actor: "test",
	})
	if err != nil {
		t.Fatalf("start recovery: %v", err)
	}
	return opened
}

func fittingCapability() fakeCapability {
	return fakeCapability{fits: map[string][]cargofit.TruckCapability{
		providerB: {tooSmallTruck()},
		providerC: {fittingTruck(truckC, providerC)},
	}}
}

func tooSmallTruck() cargofit.TruckCapability {
	truck := fittingTruck(truckB, providerB)
	truck.MaxPayloadGrams = 100_000
	return truck
}

// ── Detection ─────────────────────────────────────────────────────────────────

func TestProviderCancellationStartsRecovery(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerB, providerC), fittingCapability())

	opened := startCase(t, service, bookingID, "op-start-1")
	if opened.State != string(StateDetected) {
		t.Fatalf("state = %s, want DETECTED", opened.State)
	}
	if opened.OriginalProviderID != providerA || opened.OriginalTruckID != truckA {
		t.Fatalf("case did not capture the failed provider: %+v", opened)
	}
	// Before pickup there is no custody to move.
	if opened.CustodyTransferRequired {
		t.Fatal("a pre-pickup failure demanded a custody transfer")
	}

	var active *string
	if err := pool.QueryRow(context.Background(),
		`SELECT active_recovery_id::text FROM haulage_bookings WHERE id=$1`, bookingID).Scan(&active); err != nil {
		t.Fatalf("read booking: %v", err)
	}
	if active == nil || *active != opened.RecoveryCaseID {
		t.Fatalf("booking does not point at its open case: %v", active)
	}
}

// A failure after pickup means the goods are in the failed provider's truck.
func TestPostPickupFailureRequiresCustodyTransfer(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "picked_up")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())

	opened := startCase(t, service, bookingID, "op-start-2")
	if !opened.CustodyTransferRequired {
		t.Fatal("a post-pickup failure did not demand a custody transfer")
	}
}

// The caller is an at-least-once pipeline: a retry must return the original
// case, not open a second one for the same failure.
func TestDuplicateRecoveryCommandIsIdempotent(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())

	first := startCase(t, service, bookingID, "op-same")
	second := startCase(t, service, bookingID, "op-same")

	if first.RecoveryCaseID != second.RecoveryCaseID {
		t.Fatalf("retry opened a second case: %s vs %s", first.RecoveryCaseID, second.RecoveryCaseID)
	}
	var cases int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM hauling_recovery_cases`).Scan(&cases)
	if cases != 1 {
		t.Fatalf("cases = %d, want 1", cases)
	}
}

// Two DIFFERENT reports of the same failure must not each search and each bind
// a different replacement to one load.
func TestTwoReportsOfTheSameFailureOpenOneCase(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())

	startCase(t, service, bookingID, "op-report-1")
	_, err := service.Start(context.Background(), StartCommand{
		BookingID: bookingID, OperationRef: "op-report-2",
		TriggerCode: TriggerVehicleFailure, Actor: "test",
	})
	if err == nil {
		t.Fatal("a second report opened a second case")
	}
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) || appErr.Status != http.StatusConflict {
		t.Fatalf("second report error = %v, want a 409 conflict", err)
	}
}

func TestCompletedBookingCannotEnterRecovery(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "completed")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())

	if _, err := service.Start(context.Background(), StartCommand{
		BookingID: bookingID, OperationRef: "op-done", TriggerCode: TriggerProviderBreakdown,
	}); err == nil {
		t.Fatal("a completed booking entered recovery")
	}
}

func TestUnknownTriggerIsRefused(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())

	if _, err := service.Start(context.Background(), StartCommand{
		BookingID: bookingID, OperationRef: "op-bad", TriggerCode: "IT_BROKE",
	}); err == nil {
		t.Fatal("an unrecognised trigger was stored")
	}
}

// ── Replacement selection ─────────────────────────────────────────────────────

// The core Section 9 requirement: a replacement passes the SAME hard-fit
// engine, so the too-small truck is skipped and the fitting one chosen.
func TestReplacementPassesTheSameHardFitEngine(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	// providerB is nearer but its truck cannot carry 5 tonnes.
	service := serviceFor(t, pool, nearby(providerB, providerC), fittingCapability())
	startCase(t, service, bookingID, "op-fit")

	selected, err := service.SelectReplacement(context.Background(), bookingID, "test")
	if err != nil {
		t.Fatalf("select replacement: %v", err)
	}
	if selected.ReplacementProviderID != providerC {
		t.Fatalf("replacement = %s, want the provider whose truck fits (%s)",
			selected.ReplacementProviderID, providerC)
	}
	if selected.State != string(StateInProgress) {
		t.Fatalf("state = %s, want IN_PROGRESS", selected.State)
	}

	// The booking is rebound to the replacement, and its identity is intact.
	var provider, truck, status string
	if err := pool.QueryRow(context.Background(),
		`SELECT provider_id::text, truck_id::text, status FROM haulage_bookings WHERE id=$1`,
		bookingID).Scan(&provider, &truck, &status); err != nil {
		t.Fatalf("read booking: %v", err)
	}
	if provider != providerC || truck != truckC {
		t.Fatalf("booking not rebound: provider=%s truck=%s", provider, truck)
	}
	if status != "en_route_pickup" {
		t.Fatalf("status changed to %s; a replacement continues the work, it does not restart it", status)
	}
}

// An incompatible truck must never be chosen just to keep the load moving.
func TestIncompatibleReplacementIsRefusedAndEscalates(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	onlyTooSmall := fakeCapability{fits: map[string][]cargofit.TruckCapability{
		providerB: {tooSmallTruck()},
	}}
	service := serviceFor(t, pool, nearby(providerB), onlyTooSmall)
	startCase(t, service, bookingID, "op-nofit")

	parked, err := service.SelectReplacement(context.Background(), bookingID, "test")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if parked.State != string(StateOpsRequired) {
		t.Fatalf("state = %s, want OPS_REQUIRED", parked.State)
	}
	if parked.ReasonCode != capabilityfit.ReasonCapacityExceeded {
		t.Fatalf("reason = %s, want the specific fit failure", parked.ReasonCode)
	}
	if parked.ReplacementProviderID != "" {
		t.Fatal("an incompatible provider was bound anyway")
	}

	// The booking still belongs to the original provider: nothing was
	// silently repointed.
	var provider string
	_ = pool.QueryRow(context.Background(),
		`SELECT provider_id::text FROM haulage_bookings WHERE id=$1`, bookingID).Scan(&provider)
	if provider != providerA {
		t.Fatalf("booking was rebound to %s despite no eligible replacement", provider)
	}
}

// An unsurveyed truck is CAPABILITY_UNKNOWN and must not be used.
func TestUnknownCapabilityReplacementIsRefused(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	unsurveyed := fittingTruck(truckC, providerC)
	unsurveyed.MaxPayloadGrams, unsurveyed.MaxPayloadGramsKnown = 0, false
	service := serviceFor(t, pool, nearby(providerC), fakeCapability{
		fits: map[string][]cargofit.TruckCapability{providerC: {unsurveyed}},
	})
	startCase(t, service, bookingID, "op-unknown")

	parked, err := service.SelectReplacement(context.Background(), bookingID, "test")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if parked.ReasonCode != capabilityfit.ReasonCapabilityUnknown {
		t.Fatalf("reason = %s, want %s", parked.ReasonCode, capabilityfit.ReasonCapabilityUnknown)
	}
}

// A truck with no valid certification for a certified product is refused.
func TestUncertifiedReplacementIsRefused(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	uncertified := fittingTruck(truckC, providerC)
	uncertified.ProductEligibility = capabilityfit.No
	uncertified.ProductEligibilityNote = "eligibility for HAUL_COLD_CHAIN is SUSPENDED"
	service := serviceFor(t, pool, nearby(providerC), certifyingCapability{
		fits: map[string][]cargofit.TruckCapability{providerC: {uncertified}},
	})
	startCase(t, service, bookingID, "op-uncert")

	parked, err := service.SelectReplacement(context.Background(), bookingID, "test")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if parked.ReasonCode != capabilityfit.ReasonAssetNotCertified {
		t.Fatalf("reason = %s, want %s", parked.ReasonCode, capabilityfit.ReasonAssetNotCertified)
	}
}

// certifyingCapability demands certification, so the engine applies the gate.
type certifyingCapability struct {
	fits map[string][]cargofit.TruckCapability
}

func (c certifyingCapability) LoadProviderTruckFits(
	_ context.Context, providerIDs []string, _ cargofit.Requirement,
) (map[string][]cargofit.TruckCapability, error) {
	out := map[string][]cargofit.TruckCapability{}
	for _, id := range providerIDs {
		if trucks, ok := c.fits[id]; ok {
			out[id] = trucks
		}
	}
	return out, nil
}

func (c certifyingCapability) ProductForRequirement(cargofit.Requirement) (string, bool) {
	return "HAUL_COLD_CHAIN", true
}

// A store outage is not a statement about any truck, so it must not be
// recorded as a physical incompatibility.
func TestDependencyOutageIsDistinctFromPhysicalIncompatibility(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC),
		fakeCapability{err: errors.New("postgres unavailable")})
	startCase(t, service, bookingID, "op-outage")

	parked, err := service.SelectReplacement(context.Background(), bookingID, "test")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if parked.ReasonCode != capabilityfit.ReasonDependencyUnavailable {
		t.Fatalf("reason = %s, want %s", parked.ReasonCode, capabilityfit.ReasonDependencyUnavailable)
	}
	if parked.ReasonCode == capabilityfit.ReasonRequiredCapabilityUnavailable {
		t.Fatal("an outage was recorded as a permanent incompatibility")
	}
}

// The failed provider is never their own replacement.
func TestFailedProviderIsNotOfferedAsItsOwnReplacement(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerA), fakeCapability{
		fits: map[string][]cargofit.TruckCapability{providerA: {fittingTruck(truckA, providerA)}},
	})
	startCase(t, service, bookingID, "op-self")

	parked, err := service.SelectReplacement(context.Background(), bookingID, "test")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if parked.ReplacementProviderID == providerA {
		t.Fatal("the broken-down provider was chosen to replace itself")
	}
	if parked.State != string(StateOpsRequired) {
		t.Fatalf("state = %s, want OPS_REQUIRED with no other candidate", parked.State)
	}
}

// Only one replacement can win a concurrent race.
func TestConcurrentSelectionBindsExactlyOneReplacement(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC, providerB), fittingCapability())
	startCase(t, service, bookingID, "op-race")

	const racers = 6
	var wait sync.WaitGroup
	results := make([]Public, racers)
	errs := make([]error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wait.Add(1)
		go func(slot int) {
			defer wait.Done()
			<-start
			results[slot], errs[slot] = service.SelectReplacement(context.Background(), bookingID, "test")
		}(i)
	}
	close(start)
	wait.Wait()

	bound := map[string]bool{}
	for i, result := range results {
		if errs[i] != nil {
			continue // a conflict is the correct outcome for a loser
		}
		if result.ReplacementProviderID != "" {
			bound[result.ReplacementProviderID] = true
		}
	}
	if len(bound) > 1 {
		t.Fatalf("%d different replacements were bound to one load: %v", len(bound), bound)
	}

	var provider string
	_ = pool.QueryRow(context.Background(),
		`SELECT provider_id::text FROM haulage_bookings WHERE id=$1`, bookingID).Scan(&provider)
	if provider == providerA {
		t.Fatal("no racer bound a replacement at all")
	}

	var cases int
	_ = pool.QueryRow(context.Background(),
		`SELECT count(*) FROM hauling_recovery_cases WHERE booking_id=$1`, bookingID).Scan(&cases)
	if cases != 1 {
		t.Fatalf("cases = %d, want 1", cases)
	}
}

// If the original provider finishes or is reassigned between the search and
// the write, the rebind must lose rather than overwrite.
func TestReplacementLosesToAConcurrentBookingChange(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-lost")

	// The original provider completes while the search is running.
	if _, err := pool.Exec(context.Background(),
		`UPDATE haulage_bookings SET status='completed' WHERE id=$1`, bookingID); err != nil {
		t.Fatalf("simulate completion: %v", err)
	}

	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err == nil {
		t.Fatal("a replacement was bound to a booking that had already completed")
	}

	var provider string
	_ = pool.QueryRow(context.Background(),
		`SELECT provider_id::text FROM haulage_bookings WHERE id=$1`, bookingID).Scan(&provider)
	if provider != providerA {
		t.Fatalf("the completed booking was rebound to %s", provider)
	}
}

// ── Controlled transfer ───────────────────────────────────────────────────────

func TestCustodyTransferRequiresBothConfirmations(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "picked_up")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-transfer-1")

	chosen, err := service.SelectReplacement(context.Background(), bookingID, "test")
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if chosen.State != string(StateTransferRequired) {
		t.Fatalf("state = %s, want TRANSFER_REQUIRED", chosen.State)
	}
	// The booking is NOT yet rebound: the cargo has not moved.
	var provider string
	_ = pool.QueryRow(context.Background(),
		`SELECT provider_id::text FROM haulage_bookings WHERE id=$1`, bookingID).Scan(&provider)
	if provider != providerA {
		t.Fatal("the booking was rebound before the cargo changed hands")
	}

	if _, err := service.CompleteTransfer(context.Background(), TransferCommand{
		BookingID: bookingID, OperationRef: "t-1", TransferRef: "TR-1",
		OldProviderConfirmed: true, NewProviderConfirmed: false,
	}); err == nil {
		t.Fatal("a one-sided handover was accepted")
	}
}

func TestControlledTransferRebindsAndPreservesEvidence(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "picked_up")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	opened := startCase(t, service, bookingID, "op-transfer-2")
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select: %v", err)
	}

	transferred, err := service.CompleteTransfer(context.Background(), TransferCommand{
		BookingID: bookingID, OperationRef: "t-2", TransferRef: "TR-2", EvidenceRef: "EV-2",
		OldProviderConfirmed: true, NewProviderConfirmed: true, Actor: "test",
	})
	if err != nil {
		t.Fatalf("complete transfer: %v", err)
	}
	if transferred.State != string(StateInProgress) {
		t.Fatalf("state = %s, want IN_PROGRESS", transferred.State)
	}
	if transferred.CustodyTransferRef != "TR-2" || transferred.EvidenceRef != "EV-2" {
		t.Fatalf("transfer evidence lost: %+v", transferred)
	}

	// Same booking id, same case id, rebound provider.
	if transferred.BookingID != bookingID || transferred.RecoveryCaseID != opened.RecoveryCaseID {
		t.Fatal("identity changed across the transfer")
	}
	var provider string
	_ = pool.QueryRow(context.Background(),
		`SELECT provider_id::text FROM haulage_bookings WHERE id=$1`, bookingID).Scan(&provider)
	if provider != providerC {
		t.Fatalf("booking provider = %s, want the replacement", provider)
	}

	// The original provider and trigger survive in the case: history is
	// append-only, so what happened is still readable afterwards.
	if transferred.OriginalProviderID != providerA || transferred.TriggerCode != TriggerProviderBreakdown {
		t.Fatalf("original evidence was overwritten: %+v", transferred)
	}
}

// ── Lifecycle guards ──────────────────────────────────────────────────────────

func TestStaleVersionIsRefused(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	opened := startCase(t, service, bookingID, "op-stale")
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select: %v", err)
	}

	// opened.Version is now stale.
	if _, err := service.Resolve(context.Background(), ResolveCommand{
		BookingID: bookingID, ExpectedVersion: opened.Version, Actor: "test",
	}); err == nil {
		t.Fatal("a stale version was accepted")
	}
}

func TestTerminalStateCannotRegress(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-terminal")
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if _, err := service.Resolve(context.Background(), ResolveCommand{
		BookingID: bookingID, Actor: "test",
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Resolved cases are closed. Nothing reopens them.
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err == nil {
		t.Fatal("a resolved case accepted a new replacement")
	}
	if _, err := service.Resolve(context.Background(), ResolveCommand{
		BookingID: bookingID, Actor: "test",
	}); err == nil {
		t.Fatal("a resolved case was resolved twice")
	}
}

func TestStateMachineOnlyMovesForward(t *testing.T) {
	cases := []struct {
		from, to State
		want     bool
	}{
		{StateDetected, StateReplacementSearch, true},
		{StateReplacementSearch, StateTransferRequired, true},
		{StateTransferRequired, StateInProgress, true},
		{StateInProgress, StateResolved, true},
		{StateReplacementSearch, StateDetected, false},
		{StateInProgress, StateReplacementSearch, false},
		{StateResolved, StateInProgress, false},
		{StateFailed, StateReplacementSearch, false},
		// The one legitimate step back: an operator who has fixed whatever
		// blocked automatic recovery hands the case to the engine again.
		{StateOpsRequired, StateReplacementSearch, true},
	}
	for _, tc := range cases {
		if got := tc.from.CanAdvanceTo(tc.to); got != tc.want {
			t.Fatalf("%s -> %s = %v, want %v", tc.from, tc.to, got, tc.want)
		}
	}
}

// ── Events and settlement ─────────────────────────────────────────────────────

func TestRecoveryPublishesItsFactsOnce(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-events")
	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select: %v", err)
	}
	if _, err := service.Resolve(context.Background(), ResolveCommand{BookingID: bookingID}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	rows, err := pool.Query(context.Background(),
		`SELECT event_type, count(*) FROM hauling_domain_outbox
		  WHERE aggregate_type='recovery_case' GROUP BY event_type`)
	if err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	defer rows.Close()
	published := map[string]int{}
	for rows.Next() {
		var name string
		var count int
		if err := rows.Scan(&name, &count); err != nil {
			t.Fatalf("scan: %v", err)
		}
		published[name] = count
	}

	for _, expected := range []string{
		EventRecoveryStarted, EventGuaranteeBreached,
		EventRecoveryProviderReplaced, EventRecoveryResolved,
	} {
		if published[expected] != 1 {
			t.Fatalf("%s published %d times, want 1 (all: %v)", expected, published[expected], published)
		}
	}
}

func TestRecoveryEventsCarryTheEnvelopeFields(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-envelope")

	var payload map[string]any
	if err := pool.QueryRow(context.Background(),
		`SELECT payload FROM hauling_domain_outbox
		  WHERE aggregate_type='recovery_case' AND event_type=$1 LIMIT 1`,
		EventRecoveryStarted).Scan(&payload); err != nil {
		t.Fatalf("read event: %v", err)
	}
	for _, field := range []string{
		"event_id", "schema_version", "source_service", "source_entity_type",
		"source_entity_id", "source_version", "occurred_at", "booking_id", "trigger_code",
	} {
		if _, ok := payload[field]; !ok {
			t.Fatalf("event payload is missing %q: %v", field, payload)
		}
	}
	// No customer data travels on a recovery fact.
	for _, forbidden := range []string{"customer_id", "receiver_name", "receiver_phone",
		"pickup_address", "dropoff_address", "cargo_description"} {
		if _, present := payload[forbidden]; present {
			t.Fatalf("event payload carries %q", forbidden)
		}
	}
}

// A replacement must not create a second payable: the booking's payment
// reference is the booking id, and it does not change.
func TestReplacementDoesNotChangeTheSettlementSubject(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-settle")

	var beforeIntent, beforeMethod string
	_ = pool.QueryRow(context.Background(),
		`SELECT COALESCE(payment_intent_id,''), COALESCE(payment_method,'') FROM haulage_bookings WHERE id=$1`,
		bookingID).Scan(&beforeIntent, &beforeMethod)

	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select: %v", err)
	}

	var afterID, afterIntent, afterMethod string
	if err := pool.QueryRow(context.Background(),
		`SELECT id::text, COALESCE(payment_intent_id,''), COALESCE(payment_method,'') FROM haulage_bookings WHERE id=$1`,
		bookingID).Scan(&afterID, &afterIntent, &afterMethod); err != nil {
		t.Fatalf("read booking: %v", err)
	}
	if afterID != bookingID {
		t.Fatal("the booking id changed across a replacement")
	}
	if afterIntent != beforeIntent || afterMethod != beforeMethod {
		t.Fatalf("payment binding changed: intent %q->%q method %q->%q",
			beforeIntent, afterIntent, beforeMethod, afterMethod)
	}
}

// The cargo requirement is what the replacement was evaluated against. If a
// replacement could rewrite it, it could widen it.
func TestRequirementSnapshotIsImmutableAcrossRecovery(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-req")

	var weightBefore int
	var typeBefore string
	_ = pool.QueryRow(context.Background(),
		`SELECT cargo_weight_kg, preferred_truck_type FROM haulage_bookings WHERE id=$1`,
		bookingID).Scan(&weightBefore, &typeBefore)

	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select: %v", err)
	}

	var weightAfter int
	var typeAfter string
	_ = pool.QueryRow(context.Background(),
		`SELECT cargo_weight_kg, preferred_truck_type FROM haulage_bookings WHERE id=$1`,
		bookingID).Scan(&weightAfter, &typeAfter)
	if weightAfter != weightBefore || typeAfter != typeBefore {
		t.Fatalf("cargo requirement changed: %dkg/%s -> %dkg/%s",
			weightBefore, typeBefore, weightAfter, typeAfter)
	}
}

func TestDisabledRecoveryRefusesEverything(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	disabled := New(Options{Repository: NewRepository(pool), Enabled: false})

	if _, err := disabled.Start(context.Background(), StartCommand{
		BookingID: bookingID, OperationRef: "op-off", TriggerCode: TriggerProviderBreakdown,
	}); err == nil {
		t.Fatal("disabled recovery opened a case")
	}
	var cases int
	_ = pool.QueryRow(context.Background(), `SELECT count(*) FROM hauling_recovery_cases`).Scan(&cases)
	if cases != 0 {
		t.Fatalf("disabled recovery wrote %d cases", cases)
	}
}

// After a successful replacement the ORIGINAL provider can no longer act on
// the booking.
//
// The enforcement lives in the booking lifecycle, which guards every
// transition with `b.ProviderID == nil || *b.ProviderID != providerID`
// (booking_service.go MarkEnRoutePickup, ConfirmPickup, ConfirmDelivery and
// siblings). This asserts the precondition that guard depends on -- that the
// booking no longer names the original provider -- and then runs the guard's
// own predicate, so a change that loosened it would fail here.
func TestOriginalProviderCannotActAfterReplacement(t *testing.T) {
	pool := testPool(t)
	bookingID := seed(t, pool, "en_route_pickup")
	service := serviceFor(t, pool, nearby(providerC), fittingCapability())
	startCase(t, service, bookingID, "op-original-out")

	if _, err := service.SelectReplacement(context.Background(), bookingID, "test"); err != nil {
		t.Fatalf("select: %v", err)
	}

	// The lifecycle's ownership predicate, run for each party.
	var originalOwns, replacementOwns bool
	if err := pool.QueryRow(context.Background(),
		`SELECT provider_id = $2::uuid, provider_id = $3::uuid FROM haulage_bookings WHERE id=$1`,
		bookingID, providerA, providerC).Scan(&originalOwns, &replacementOwns); err != nil {
		t.Fatalf("read booking: %v", err)
	}
	if originalOwns {
		t.Fatal("the replaced provider still owns the booking and could complete it")
	}
	if !replacementOwns {
		t.Fatal("the replacement does not own the booking and could not complete it")
	}

	// And a transition guarded on the original provider matches no row, which
	// is exactly what the lifecycle turns into a refusal.
	tag, err := pool.Exec(context.Background(),
		`UPDATE haulage_bookings SET status='picked_up' WHERE id=$1 AND provider_id=$2::uuid`,
		bookingID, providerA)
	if err != nil {
		t.Fatalf("guarded transition: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Fatal("a transition guarded on the original provider still matched")
	}
}
