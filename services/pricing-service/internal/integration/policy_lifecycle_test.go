package integration

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/pricing-service/internal/database"
	"cosmicforge/logistics/services/pricing-service/internal/engine"
	approvalmodels "cosmicforge/logistics/services/pricing-service/internal/features/approvals/models"
	approvalrepos "cosmicforge/logistics/services/pricing-service/internal/features/approvals/repositories"
	approvalusecases "cosmicforge/logistics/services/pricing-service/internal/features/approvals/usecases"
	geomodels "cosmicforge/logistics/services/pricing-service/internal/features/geography/models"
	georepos "cosmicforge/logistics/services/pricing-service/internal/features/geography/repositories"
	policymodels "cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	policyrepos "cosmicforge/logistics/services/pricing-service/internal/features/policies/repositories"
	policyusecases "cosmicforge/logistics/services/pricing-service/internal/features/policies/usecases"
	rolloutmodels "cosmicforge/logistics/services/pricing-service/internal/features/rollouts/models"
	rolloutrepos "cosmicforge/logistics/services/pricing-service/internal/features/rollouts/repositories"
	"cosmicforge/logistics/services/pricing-service/internal/platform/audit"
	"cosmicforge/logistics/services/pricing-service/internal/platform/ids"
	"cosmicforge/logistics/services/pricing-service/internal/platform/operators"
)

// policyHarness wires the real stores the way cmd/ does, so these tests exercise
// the production composition rather than a test-only arrangement.
type policyHarness struct {
	pool      *pgxpool.Pool
	policies  *policyusecases.Service
	geo       *georepos.Store
	rollouts  *rolloutrepos.Store
	approvals *approvalusecases.Service
	editor    string
	approver  string
}

func newPolicyHarness(t *testing.T) policyHarness {
	t.Helper()
	pool := testPool(t)
	ctx := context.Background()

	editor := unique("editor")
	approver := unique("approver")
	for id, roles := range map[string]string{
		editor:   "{EDITOR}",
		approver: "{APPROVER,EDITOR}",
	} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO pricing_operators (operator_id, roles) VALUES ($1, $2::text[])`, id, roles); err != nil {
			t.Fatalf("seed operator %s: %v", id, err)
		}
	}

	service := policyusecases.NewService(policyusecases.Options{
		Pool:  pool,
		Store: func(q database.Querier) policyusecases.Store { return policyrepos.NewStore(q) },
		Approvals: func(q database.Querier) policyusecases.Approvals {
			return approvalusecases.NewService(approvalrepos.NewStore(q))
		},
		Ops:             operators.NewStore(pool),
		Audit:           audit.NewPostgresRecorder(),
		FourEyeRequired: true,
	})

	return policyHarness{
		pool:      pool,
		policies:  service,
		geo:       georepos.NewStore(pool),
		rollouts:  rolloutrepos.NewStore(pool),
		approvals: approvalusecases.NewService(approvalrepos.NewStore(pool)),
		editor:    editor,
		approver:  approver,
	}
}

// place is a test's own geography. Every test gets a unique STATE, MARKET and
// ZONE, because a policy lineage is unique per (kind, level, reference): two
// tests sharing a reference would see each other's versions and activations.
//
// The country is shared and deliberately carries no policy, so it also serves as
// proof that resolution skips a candidate level with nothing active.
type place struct {
	country  string
	stateID  string
	marketID string
	zoneID   string
}

func (h policyHarness) newPlace(t *testing.T) place {
	t.Helper()
	ctx := context.Background()

	const country = "ZZ" // a reserved code, never a real market
	if err := h.geo.UpsertCountry(ctx, geomodels.Country{Code: country, Name: "Test Country", Currency: "NGN"}); err != nil {
		t.Fatalf("seed country: %v", err)
	}

	stateID := ids.New(ids.PrefixState)
	if _, err := h.pool.Exec(ctx,
		`INSERT INTO pricing_states (id, country_code, code, name) VALUES ($1, $2, $3, $4)`,
		stateID, country, unique("ST"), "Test State"); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	market, err := h.geo.UpsertMarket(ctx, geomodels.Market{
		ID: ids.New(ids.PrefixMarket), CountryCode: country, StateID: stateID,
		Code: unique("MKT"), Name: "Test Metro",
	})
	if err != nil {
		t.Fatalf("seed market: %v", err)
	}
	zone, err := h.geo.UpsertZone(ctx, geomodels.Zone{
		ID: ids.New(ids.PrefixZone), MarketID: market.ID, Code: "ISLAND", Name: "Island",
	})
	if err != nil {
		t.Fatalf("seed zone: %v", err)
	}

	return place{country: country, stateID: stateID, marketID: market.ID, zoneID: zone.ID}
}

// chain returns the full geography chain for this place.
func (p place) chain() policymodels.GeographyChain {
	return policymodels.GeographyChain{
		ZoneID: p.zoneID, MarketID: p.marketID, StateID: p.stateID, CountryCode: p.country,
	}
}

// seedBody loads a V1 seed body, optionally patching one top-level field so a
// test can create a genuinely different second version.
func seedBody(t *testing.T, name string, patch map[string]any) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "seeds", "v1", name))
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	var file struct {
		Body map[string]any `json:"body"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse seed: %v", err)
	}
	for key, value := range patch {
		file.Body[key] = value
	}
	body, err := json.Marshal(file.Body)
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}
	return body
}

func (h policyHarness) draft(t *testing.T, kind, level, ref string, body []byte, reason string) policymodels.Version {
	t.Helper()
	version, err := h.policies.CreateDraft(context.Background(), policyusecases.DraftInput{
		Kind: kind, GeographyLevel: level, GeographyRef: ref, SchemaVersion: 1,
		Body: body, PerformedBy: h.editor, ActorCaller: "pricing-ops-cli",
		ChangeReason: reason, RequestID: unique("req"),
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	return version
}

func (h policyHarness) approveAndActivate(t *testing.T, version policymodels.Version, from time.Time) {
	t.Helper()
	ctx := context.Background()
	if _, err := h.policies.Submit(ctx, version.ID, h.editor, "pricing-ops-cli", "ready for review", unique("req")); err != nil {
		t.Fatalf("submit: %v", err)
	}
	if err := h.policies.Approve(ctx, version.ID, h.approver, "pricing-ops-cli", "checked against the golden corpus", unique("req")); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err := h.policies.Activate(ctx, policyusecases.ActivateInput{
		VersionID: version.ID, EffectiveFrom: from, PerformedBy: h.approver,
		ActorCaller: "pricing-ops-cli", Reason: "scheduled activation", RequestID: unique("req"),
	}); err != nil {
		t.Fatalf("activate: %v", err)
	}
}

// TestPolicyReachesProductionOnlyThroughApproval walks the whole intended path
// and proves each shortcut is closed.
func TestPolicyReachesProductionOnlyThroughApproval(t *testing.T) {
	h := newPolicyHarness(t)
	ctx := context.Background()
	where := h.newPlace(t)

	body := seedBody(t, "taxi_tariff_ng.json", nil)
	version := h.draft(t, engine.KindTaxiTariff, policymodels.LevelState, where.stateID, body, "V1 baseline")

	t.Run("a draft cannot be activated", func(t *testing.T) {
		_, err := h.policies.Activate(ctx, policyusecases.ActivateInput{
			VersionID: version.ID, PerformedBy: h.approver, ActorCaller: "cli", Reason: "skip the queue",
		})
		if !errors.Is(err, policymodels.ErrNotApproved) {
			t.Fatalf("error = %v, want ErrNotApproved", err)
		}
	})

	t.Run("an editor cannot approve", func(t *testing.T) {
		if _, err := h.policies.Submit(ctx, version.ID, h.editor, "cli", "ready", unique("req")); err != nil {
			t.Fatalf("submit: %v", err)
		}
		err := h.policies.Approve(ctx, version.ID, h.editor, "cli", "approving my own work", unique("req"))
		if err == nil {
			t.Fatal("an operator without the APPROVER role must not approve")
		}
		if !errors.Is(err, operators.ErrMissingRole) && !errors.Is(err, approvalmodels.ErrSelfApproval) {
			t.Fatalf("error = %v, want a missing-role or self-approval refusal", err)
		}
	})

	t.Run("the author cannot approve even with the role", func(t *testing.T) {
		// Give the editor the APPROVER role, so only the four-eye rule stands
		// between them and approving their own change.
		if _, err := h.pool.Exec(ctx,
			`UPDATE pricing_operators SET roles = '{EDITOR,APPROVER}' WHERE operator_id = $1`, h.editor); err != nil {
			t.Fatalf("grant role: %v", err)
		}
		err := h.policies.Approve(ctx, version.ID, h.editor, "cli", "still my own work", unique("req"))
		if !errors.Is(err, approvalmodels.ErrSelfApproval) {
			t.Fatalf("error = %v, want ErrSelfApproval: the requester of a change may never decide it", err)
		}
	})

	t.Run("a second person can approve, and then it activates", func(t *testing.T) {
		if err := h.policies.Approve(ctx, version.ID, h.approver, "cli", "verified", unique("req")); err != nil {
			t.Fatalf("approve: %v", err)
		}
		if _, err := h.policies.Activate(ctx, policyusecases.ActivateInput{
			VersionID: version.ID, PerformedBy: h.approver, ActorCaller: "cli", Reason: "go live",
		}); err != nil {
			t.Fatalf("activate: %v", err)
		}

		resolution, err := h.policies.Resolve(ctx, engine.KindTaxiTariff, where.chain(), time.Now().UTC())
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if resolution.Version.ID != version.ID {
			t.Fatalf("resolved %s, want %s", resolution.Version.ID, version.ID)
		}
	})

	t.Run("an approved version's body is frozen", func(t *testing.T) {
		_, err := h.policies.UpdateDraft(ctx, version.ID, body, h.editor, "cli", "sneak a change in", unique("req"))
		if !errors.Is(err, policymodels.ErrNotDraft) {
			t.Fatalf("error = %v, want ErrNotDraft", err)
		}
	})
}

// TestTheMostSpecificGeographyWins proves a zone rate beats a market rate, which
// beats a national one.
func TestTheMostSpecificGeographyWins(t *testing.T) {
	h := newPolicyHarness(t)
	ctx := context.Background()
	where := h.newPlace(t)

	// Three editions of the same kind at three levels, with different base fares
	// so the resolved one is identifiable from the price.
	stateVersion := h.draft(t, engine.KindTaxiTariff, policymodels.LevelState, where.stateID,
		taxiBodyWithBase(t, 50_000), "state baseline")
	h.approveAndActivate(t, stateVersion, time.Time{})

	marketVersion := h.draft(t, engine.KindTaxiTariff, policymodels.LevelMarket, where.marketID,
		taxiBodyWithBase(t, 60_000), "market override")
	h.approveAndActivate(t, marketVersion, time.Time{})

	zoneVersion := h.draft(t, engine.KindTaxiTariff, policymodels.LevelZone, where.zoneID,
		taxiBodyWithBase(t, 70_000), "zone override")
	h.approveAndActivate(t, zoneVersion, time.Time{})

	cases := []struct {
		name  string
		chain policymodels.GeographyChain
		want  string
	}{
		{"zone beats market and state", where.chain(), zoneVersion.ID},
		{"market beats state", policymodels.GeographyChain{MarketID: where.marketID, StateID: where.stateID, CountryCode: where.country}, marketVersion.ID},
		{"state alone", policymodels.GeographyChain{StateID: where.stateID, CountryCode: where.country}, stateVersion.ID},
		{"an unmapped zone falls back to its market", policymodels.GeographyChain{ZoneID: "zn_unknown", MarketID: where.marketID, StateID: where.stateID, CountryCode: where.country}, marketVersion.ID},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolution, err := h.policies.Resolve(ctx, engine.KindTaxiTariff, tc.chain, time.Now().UTC())
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if resolution.Version.ID != tc.want {
				t.Fatalf("resolved %s, want %s", resolution.Version.ID, tc.want)
			}
		})
	}

	t.Run("no policy at all is refused, not defaulted", func(t *testing.T) {
		_, err := h.policies.Resolve(ctx, engine.KindDispatchTariff, where.chain(), time.Now().UTC())
		if !errors.Is(err, policymodels.ErrNoActivePolicy) {
			t.Fatalf("error = %v, want ErrNoActivePolicy: pricing must refuse rather than invent a rate", err)
		}
	})
}

// TestScheduledActivationAndHistoricalResolution is the historical-truth
// requirement: a quote issued under edition 1 must still resolve edition 1 after
// edition 2 goes live.
func TestScheduledActivationAndHistoricalResolution(t *testing.T) {
	h := newPolicyHarness(t)
	ctx := context.Background()
	where := h.newPlace(t)

	first := h.draft(t, engine.KindTaxiTariff, policymodels.LevelState, where.stateID,
		taxiBodyWithBase(t, 50_000), "edition 1")
	h.approveAndActivate(t, first, time.Time{})

	// Edition 2 is scheduled for later today rather than applied immediately.
	switchOver := time.Now().UTC().Add(2 * time.Hour)
	second := h.draft(t, engine.KindTaxiTariff, policymodels.LevelState, where.stateID,
		taxiBodyWithBase(t, 65_000), "edition 2: fuel cost review")
	h.approveAndActivate(t, second, switchOver)

	chain := where.chain()

	t.Run("before the switch-over, edition 1 is live", func(t *testing.T) {
		resolution, err := h.policies.Resolve(ctx, engine.KindTaxiTariff, chain, time.Now().UTC())
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if resolution.Version.ID != first.ID {
			t.Fatalf("resolved %s, want edition 1 (%s)", resolution.Version.ID, first.ID)
		}
	})

	t.Run("after the switch-over, edition 2 is live", func(t *testing.T) {
		resolution, err := h.policies.Resolve(ctx, engine.KindTaxiTariff, chain, switchOver.Add(time.Minute))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if resolution.Version.ID != second.ID {
			t.Fatalf("resolved %s, want edition 2 (%s)", resolution.Version.ID, second.ID)
		}
	})

	t.Run("a trip quoted yesterday still reconciles on yesterday's edition", func(t *testing.T) {
		// This is the whole point of as-of resolution: reconciliation passes the
		// original quote's timestamp, so the price is recomputed on the rules the
		// customer was actually shown.
		resolution, err := h.policies.Resolve(ctx, engine.KindTaxiTariff, chain, switchOver.Add(-time.Hour))
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if resolution.Version.ID != first.ID {
			t.Fatalf("historical resolve returned %s, want edition 1", resolution.Version.ID)
		}
	})

	t.Run("back-dating an activation is refused", func(t *testing.T) {
		third := h.draft(t, engine.KindTaxiTariff, policymodels.LevelState, where.stateID,
			taxiBodyWithBase(t, 80_000), "edition 3")
		if _, err := h.policies.Submit(ctx, third.ID, h.editor, "cli", "ready", unique("req")); err != nil {
			t.Fatalf("submit: %v", err)
		}
		if err := h.policies.Approve(ctx, third.ID, h.approver, "cli", "ok", unique("req")); err != nil {
			t.Fatalf("approve: %v", err)
		}
		_, err := h.policies.Activate(ctx, policyusecases.ActivateInput{
			VersionID: third.ID, EffectiveFrom: time.Now().UTC().Add(-48 * time.Hour),
			PerformedBy: h.approver, ActorCaller: "cli", Reason: "retroactive change",
		})
		if err == nil {
			t.Fatal("back-dating must be refused: it would restate quotes already issued")
		}
		if !strings.Contains(err.Error(), "in the past") {
			t.Fatalf("error should explain why, got: %v", err)
		}
	})
}

// TestRollbackReactivatesAnEarlierVersionWithoutRewritingHistory proves a
// rollback is additive.
func TestRollbackReactivatesAnEarlierVersionWithoutRewritingHistory(t *testing.T) {
	h := newPolicyHarness(t)
	ctx := context.Background()
	where := h.newPlace(t)

	good := h.draft(t, engine.KindTaxiTariff, policymodels.LevelState, where.stateID, taxiBodyWithBase(t, 50_000), "edition 1")
	h.approveAndActivate(t, good, time.Time{})

	bad := h.draft(t, engine.KindTaxiTariff, policymodels.LevelState, where.stateID, taxiBodyWithBase(t, 900_000), "edition 2: mistake")
	h.approveAndActivate(t, bad, time.Time{})

	resolution, err := h.policies.Resolve(ctx, engine.KindTaxiTariff, where.chain(), time.Now().UTC())
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolution.Version.ID != bad.ID {
		t.Fatalf("expected the mistake to be live, got %s", resolution.Version.ID)
	}

	// Roll back by activating the earlier, already-approved edition again.
	rollback, err := h.policies.Activate(ctx, policyusecases.ActivateInput{
		VersionID: good.ID, PerformedBy: h.approver, ActorCaller: "cli",
		Reason: "edition 2 priced Lagos at nine thousand naira base; reverting", Rollback: true,
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rollback.Action != policymodels.ActionRollback {
		t.Fatalf("activation action = %s, want ROLLBACK", rollback.Action)
	}

	resolution, err = h.policies.Resolve(ctx, engine.KindTaxiTariff, where.chain(), time.Now().UTC())
	if err != nil {
		t.Fatalf("resolve after rollback: %v", err)
	}
	if resolution.Version.ID != good.ID {
		t.Fatalf("after rollback resolved %s, want %s", resolution.Version.ID, good.ID)
	}

	// The mistake is still on record, and so is every activation: nothing was
	// deleted or edited to undo it.
	var activationCount int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM pricing_policy_activations WHERE policy_id = $1`, resolution.Policy.ID,
	).Scan(&activationCount); err != nil {
		t.Fatalf("count activations: %v", err)
	}
	if activationCount != 3 {
		t.Fatalf("activation count = %d, want 3 (activate, mistake, rollback all preserved)", activationCount)
	}
	if _, err := h.policies.GetVersion(ctx, bad.ID); err != nil {
		t.Fatalf("the rolled-back version must remain readable: %v", err)
	}
}

// TestFreezingAMarketPinsItsPolicySet proves a freeze holds prices still even
// while new editions go live elsewhere.
func TestFreezingAMarketPinsItsPolicySet(t *testing.T) {
	h := newPolicyHarness(t)
	ctx := context.Background()
	marketCode := unique("FRZ")

	if err := h.rollouts.RecordFreeze(ctx, rolloutmodels.Freeze{
		ID: ids.New(ids.PrefixFreeze), MarketCode: marketCode, Action: rolloutmodels.ActionFreeze,
		PerformedBy: h.approver, Reason: "investigating a fare spike",
	}); err != nil {
		t.Fatalf("freeze: %v", err)
	}

	state, err := h.rollouts.FreezeStateFor(ctx, marketCode)
	if err != nil {
		t.Fatalf("freeze state: %v", err)
	}
	if !state.Frozen {
		t.Fatal("the market should be frozen")
	}
	if state.Since.IsZero() {
		t.Fatal("a freeze must record when it began, because that time becomes the effective as-of for resolution")
	}

	if err := h.rollouts.RecordFreeze(ctx, rolloutmodels.Freeze{
		ID: ids.New(ids.PrefixFreeze), MarketCode: marketCode, Action: rolloutmodels.ActionUnfreeze,
		PerformedBy: h.approver, Reason: "root cause fixed",
	}); err != nil {
		t.Fatalf("unfreeze: %v", err)
	}
	state, err = h.rollouts.FreezeStateFor(ctx, marketCode)
	if err != nil {
		t.Fatalf("freeze state: %v", err)
	}
	if state.Frozen {
		t.Fatal("the market should be unfrozen")
	}

	// Both events remain on record.
	var events int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM pricing_market_freezes WHERE market_code = $1`, marketCode).Scan(&events); err != nil {
		t.Fatalf("count freezes: %v", err)
	}
	if events != 2 {
		t.Fatalf("freeze events = %d, want 2", events)
	}
}

// TestAMarketIsLegacyUntilSomebodySwitchesIt proves the safe default, and that
// both keys are needed.
func TestAMarketIsLegacyUntilSomebodySwitchesIt(t *testing.T) {
	h := newPolicyHarness(t)
	ctx := context.Background()
	marketCode := unique("ROL")

	mode, err := h.rollouts.ModeFor(ctx, "TAXI", rolloutmodels.CapabilityQuote, marketCode)
	if err != nil {
		t.Fatalf("mode: %v", err)
	}
	if mode != rolloutmodels.ModeLegacy {
		t.Fatalf("a market nobody switched is %q, want LEGACY: pricing must not become authoritative by default", mode)
	}

	for _, step := range []string{rolloutmodels.ModeShadow, rolloutmodels.ModeCentral, rolloutmodels.ModeShadow} {
		if err := h.rollouts.RecordRollout(ctx, rolloutmodels.Rollout{
			ID: ids.New(ids.PrefixRollout), Scope: "TAXI", Capability: rolloutmodels.CapabilityQuote,
			MarketCode: marketCode, Mode: step, PerformedBy: h.approver, Reason: "rollout step",
		}); err != nil {
			t.Fatalf("record rollout %s: %v", step, err)
		}
		got, err := h.rollouts.ModeFor(ctx, "TAXI", rolloutmodels.CapabilityQuote, marketCode)
		if err != nil {
			t.Fatalf("mode: %v", err)
		}
		if got != step {
			t.Fatalf("mode = %q, want %q", got, step)
		}
	}

	// Capabilities switch independently: quoting centrally does not oblige the
	// platform to reconcile centrally on the same day.
	reconcileMode, err := h.rollouts.ModeFor(ctx, "TAXI", rolloutmodels.CapabilityReconcile, marketCode)
	if err != nil {
		t.Fatalf("mode: %v", err)
	}
	if reconcileMode != rolloutmodels.ModeLegacy {
		t.Fatalf("reconcile mode = %q, want LEGACY (capabilities are switched separately)", reconcileMode)
	}
}

// TestTheAuditTrailRecordsEveryPolicyStep proves an investigator can reconstruct
// who did what.
func TestTheAuditTrailRecordsEveryPolicyStep(t *testing.T) {
	h := newPolicyHarness(t)
	ctx := context.Background()
	where := h.newPlace(t)

	version := h.draft(t, engine.KindTaxiTariff, policymodels.LevelState, where.stateID, taxiBodyWithBase(t, 50_000), "V1 baseline")
	h.approveAndActivate(t, version, time.Time{})

	rows, err := h.pool.Query(ctx, `
		SELECT action, performed_by, reason FROM pricing_audit_log
		WHERE subject_id = $1 ORDER BY recorded_at`, version.ID)
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	defer rows.Close()

	var actions []string
	for rows.Next() {
		var action, performedBy, reason string
		if err := rows.Scan(&action, &performedBy, &reason); err != nil {
			t.Fatalf("scan audit: %v", err)
		}
		if performedBy == "" || reason == "" {
			t.Errorf("audit row %s has no actor or reason", action)
		}
		actions = append(actions, action)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("audit rows: %v", err)
	}

	for _, want := range []string{
		"POLICY_VERSION_DRAFTED", "POLICY_VERSION_SUBMITTED",
		"POLICY_VERSION_APPROVED", "POLICY_VERSION_ACTIVATED",
	} {
		found := false
		for _, action := range actions {
			if action == want {
				found = true
			}
		}
		if !found {
			t.Errorf("audit trail is missing %s; got %v", want, actions)
		}
	}

	// And the version's own lifecycle history is separately readable.
	var events int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM pricing_policy_version_events WHERE version_id = $1`, version.ID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events < 4 {
		t.Errorf("version lifecycle events = %d, want at least 4", events)
	}
}

// taxiBodyWithBase returns the V1 taxi body with the economy base fare changed,
// so tests can tell resolved versions apart by price.
func taxiBodyWithBase(t *testing.T, baseKobo int64) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "seeds", "v1", "taxi_tariff_ng.json"))
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	var file struct {
		Body map[string]any `json:"body"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parse seed: %v", err)
	}
	classes := file.Body["classes"].(map[string]any)
	economy := classes["economy"].(map[string]any)
	economy["base_kobo"] = baseKobo
	body, err := json.Marshal(file.Body)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return body
}
