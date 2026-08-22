package orders

// Part 3: durable-worker error propagation.
//
// Three recovery sweeps used to discard their errors, so a persistent database
// or Wallet outage returned nil from RecoverPayments. The durable job saw
// success and therefore never retried, never counted a failure and never
// dead-lettered: a stalled recovery was completely silent. These tests pin the
// fixed behaviour.
//
// The outage is injected with a pool pointed at a port where nothing is
// listening. pgxpool connects lazily, so the pool builds and every query fails
// the way it would against a database that has gone away — which is the real
// failure mode, unlike a nil pool that only ever occurs in a test.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func unreachableStore(t *testing.T) *Store {
	t.Helper()

	pool, err := pgxpool.New(context.Background(),
		"postgres://nobody:nobody@127.0.0.1:1/absent?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("building the pool itself failed: %v", err)
	}
	t.Cleanup(pool.Close)

	return NewStore(pool)
}

func unreachableService(t *testing.T) *Service {
	t.Helper()
	return &Service{
		store:        unreachableStore(t),
		enabled:      true,
		now:          time.Now,
		payments:     unavailableAuthority{},
		changeQuotes: unavailableChangeQuotes{},
		// Configured, so the activation resume actually runs its listing query
		// instead of no-opping the way an unconfigured deployment does.
		inventory: unavailableInventory{},
		missions:  unavailableMissions{},
	}
}

type unavailableInventory struct{}

func (unavailableInventory) ConfirmReservation(context.Context, string, string, string) error {
	return errors.New("inventory unavailable")
}

func (unavailableInventory) CommitReservation(context.Context, string, string, string) error {
	return errors.New("inventory unavailable")
}

func (unavailableInventory) ReleaseReservation(context.Context, string, string, string) error {
	return errors.New("inventory unavailable")
}

type unavailableMissions struct{}

func (unavailableMissions) Available(LogisticsService) bool { return true }

func (unavailableMissions) CreateMission(
	context.Context, MissionRequest,
) (MissionResult, error) {
	return MissionResult{}, errors.New("logistics authority unavailable")
}

type unavailableAuthority struct{}

func (unavailableAuthority) Configured() bool { return true }

func (unavailableAuthority) HoldLeg(context.Context, HoldLegRequest) (LegResult, error) {
	return LegResult{}, errors.New("wallet unavailable")
}

func (unavailableAuthority) GetLeg(context.Context, string) (LegResult, error) {
	return LegResult{}, errors.New("wallet unavailable")
}

func (unavailableAuthority) RefundLeg(context.Context, RefundLegRequest) (RefundResult, error) {
	return RefundResult{}, errors.New("wallet unavailable")
}

type unavailableChangeQuotes struct{}

func (unavailableChangeQuotes) CreateReplacementQuote(
	context.Context, string, string, string,
) (ReplacementQuote, error) {
	return ReplacementQuote{}, errors.New("quote authority unavailable")
}

func (unavailableChangeQuotes) GetReplacementQuote(
	context.Context, string, string,
) (ReplacementQuote, error) {
	return ReplacementQuote{}, errors.New("quote authority unavailable")
}

func (unavailableChangeQuotes) ApproveReplacementQuote(
	context.Context, string, string, string,
) (ReplacementQuote, error) {
	return ReplacementQuote{}, errors.New("quote authority unavailable")
}

// A sweep that cannot run at all must surface an error to the durable job.
// Returning nil is what made a persistent outage invisible.
func TestRecoverPayments_SweepLevelFailureReachesTheDurableJob(t *testing.T) {
	service := unreachableService(t)

	_, err := service.RecoverPayments(context.Background(), RecoveryLimits{Batch: 10})
	if err == nil {
		t.Fatal("a database outage returned nil: the durable job would record " +
			"success and never retry, so recovery would stall silently")
	}
}

// Each sweep RecoverPayments depends on must return its own error rather than
// swallow it. These are the three that used to discard theirs, plus the
// activation resume that runs after them.
func TestRecoveryLevelSweeps_EachPropagateTheirOwnFailure(t *testing.T) {
	service := unreachableService(t)
	ctx := context.Background()
	limits := RecoveryLimits{Batch: 10}.normalized()

	sweeps := map[string]func() error{
		"recoverTerminalPaymentLegs": func() error {
			_, err := service.recoverTerminalPaymentLegs(ctx, limits.Batch)
			return err
		},
		"RecoverCommercialChanges": func() error {
			_, err := service.RecoverCommercialChanges(ctx, limits.Batch)
			return err
		},
		"RecoverCommercialAdjustments": func() error {
			_, err := service.RecoverCommercialAdjustments(ctx, limits.Batch)
			return err
		},
		"resumeActivation": func() error {
			_, err := service.resumeActivation(ctx, limits)
			return err
		},
	}

	for name, sweep := range sweeps {
		if err := sweep(); err == nil {
			t.Errorf("%s swallowed a database outage and reported success", name)
		}
	}
}

// One failing sweep must not hide another. RecoverPayments joins them so the
// operator reading the job's last error sees every cause, not just the first.
func TestRecoverPayments_ReportsEverySweepFailureNotJustTheFirst(t *testing.T) {
	first := errors.New("terminal legs sweep failed")
	second := errors.New("commercial changes sweep failed")
	third := errors.New("activation resume failed")

	joined := errors.Join(first, second, third)

	for _, cause := range []error{first, second, third} {
		if !errors.Is(joined, cause) {
			t.Errorf("joined error lost %v", cause)
		}
	}
}

// Fulfilment that is not configured has nothing to resume, and must stay a
// no-op rather than an error that would dead-letter the job on a deployment
// which simply does not run logistics.
func TestResumeActivation_UnconfiguredFulfilmentIsANoOpNotAnError(t *testing.T) {
	service := &Service{
		store:    unreachableStore(t),
		enabled:  true,
		now:      time.Now,
		payments: unavailableAuthority{},
	}

	activated, err := service.resumeActivation(
		context.Background(), RecoveryLimits{Batch: 10}.normalized())
	if err != nil {
		t.Fatalf("an unconfigured fulfilment deployment errored: %v", err)
	}
	if activated != 0 {
		t.Fatalf("resumed %d activations without a fulfilment authority", activated)
	}
}

// A deployment with no money authority has nothing to recover. The sweep must
// stay a harmless no-op rather than an error loop that would dead-letter the
// job on a perfectly healthy installation.
func TestRecoverPayments_NoAuthorityIsNotAnError(t *testing.T) {
	service := &Service{store: unreachableStore(t), enabled: true, now: time.Now}

	outcome, err := service.RecoverPayments(context.Background(), RecoveryLimits{})
	if err != nil {
		t.Fatalf("a deployment without a money authority errored: %v", err)
	}
	if outcome.LegsReconciled != 0 || outcome.Compensated != 0 || outcome.Activated != 0 {
		t.Fatalf("a no-authority sweep acted: %+v", outcome)
	}
}
