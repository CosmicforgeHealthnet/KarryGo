// Package usecases builds the drivers' fuel index and turns a large enough
// move in it into a proposed tariff change for an approver.
package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/pricing-service/internal/engine"
	"cosmicforge/logistics/services/pricing-service/internal/features/fuel/models"
	policymodels "cosmicforge/logistics/services/pricing-service/internal/features/policies/models"
	policyusecases "cosmicforge/logistics/services/pricing-service/internal/features/policies/usecases"
	"cosmicforge/logistics/services/pricing-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/pricingclient"
)

// SystemOperator is the operator the index proposes changes as. It holds
// EDITOR only, so every proposal still needs a human approver.
const SystemOperator = "system:fuel-index"

// Place is where a report was made.
type Place struct {
	MarketID   string
	MarketCode string
	ZoneID     string
}

// Locator finds the pricing market containing a point; found is false when no
// market covers it.
type Locator interface {
	Locate(ctx context.Context, lat, lng float64) (place Place, found bool, err error)
}

// Policies is what the index needs from policy management.
type Policies interface {
	Resolve(ctx context.Context, kind string, chain policymodels.GeographyChain, asOf time.Time) (policymodels.Resolution, error)
	CreateDraft(ctx context.Context, in policyusecases.DraftInput) (policymodels.Version, error)
	Submit(ctx context.Context, versionID, performedBy, actorCaller, reason, requestID string, expectedBodyHash ...string) (string, error)
}

// Store persists reports, the index and proposals.
type Store interface {
	InsertReport(ctx context.Context, r models.Report) (string, bool, error)
	MarketFuelPairs(ctx context.Context, from, to time.Time) ([][2]string, error)
	Prices(ctx context.Context, marketID, fuelType string, from, to time.Time) ([]int64, error)
	UpsertIndex(ctx context.Context, i models.Index) error
	IndexesForDay(ctx context.Context, day time.Time) ([]models.Index, error)
	ProposalThatProduced(ctx context.Context, versionID string) (models.Proposal, bool, error)
	BlockingProposal(ctx context.Context, marketID, tariffKind, baseVersionID string, cooldownSince time.Time) (bool, error)
	InsertProposal(ctx context.Context, p models.Proposal) error
	MarketChain(ctx context.Context, marketID string) (stateID, countryCode string, err error)
}

// Service runs the fuel index.
type Service struct {
	store      Store
	locator    Locator
	policies   Policies
	windowDays int
	now        func() time.Time
}

// NewService builds the service. windowDays is how many days of reports one
// index looks back over.
func NewService(store Store, locator Locator, policies Policies, windowDays int) *Service {
	if windowDays < 1 {
		windowDays = 3
	}
	return &Service{store: store, locator: locator, policies: policies, windowDays: windowDays,
		now: func() time.Time { return time.Now().UTC() }}
}

// Report records a driver's fuel price.
func (s *Service) Report(ctx context.Context, caller string, in pricingclient.FuelReport) (pricingclient.FuelReportResult, error) {
	in.FuelType = pricingclient.NormalizeFuelType(in.FuelType)
	if err := in.Validate(); err != nil {
		return pricingclient.FuelReportResult{}, fmt.Errorf("%w: %v", models.ErrInvalid, err)
	}
	now := s.now()
	// A phone's clock can be wrong; never count a report as from the future or
	// from long ago.
	reportedAt := in.ReportedAt.UTC()
	if reportedAt.After(now.Add(5*time.Minute)) || reportedAt.Before(now.Add(-48*time.Hour)) {
		reportedAt = now
	}
	place, found, err := s.locator.Locate(ctx, float64(in.LatE6)/1e6, float64(in.LngE6)/1e6)
	if err != nil {
		return pricingclient.FuelReportResult{}, err
	}
	report := models.Report{
		ID: ids.New(ids.PrefixFuelReport), ConsumerService: caller, ProviderRef: strings.TrimSpace(in.ProviderRef),
		FuelType: in.FuelType, PricePerLitreKobo: in.PricePerLitreKobo, LatE6: in.LatE6, LngE6: in.LngE6,
		ReportDay: models.Day(reportedAt), ReportedAt: reportedAt,
	}
	if found {
		report.MarketID, report.MarketCode, report.ZoneID = place.MarketID, place.MarketCode, place.ZoneID
	}
	id, duplicate, err := s.store.InsertReport(ctx, report)
	if err != nil {
		return pricingclient.FuelReportResult{}, err
	}
	return pricingclient.FuelReportResult{ID: id, Duplicate: duplicate, MarketCode: report.MarketCode}, nil
}

// ComputeIndex recomputes today's index for every market and fuel with
// reports in the window.
func (s *Service) ComputeIndex(ctx context.Context) (int, error) {
	day := models.Day(s.now())
	from := day.AddDate(0, 0, -(s.windowDays - 1))
	pairs, err := s.store.MarketFuelPairs(ctx, from, day)
	if err != nil {
		return 0, err
	}
	for _, pair := range pairs {
		prices, err := s.store.Prices(ctx, pair[0], pair[1], from, day)
		if err != nil {
			return 0, err
		}
		median, used, dropped := models.Median(prices)
		if used == 0 {
			continue
		}
		if err := s.store.UpsertIndex(ctx, models.Index{
			MarketID: pair[0], FuelType: pair[1], Day: day, MedianKobo: median,
			ReportCount: used, DroppedCount: dropped, WindowDays: s.windowDays,
		}); err != nil {
			return 0, err
		}
	}
	return len(pairs), nil
}

// Propose drafts and submits a per-km change for every market and tariff
// whose fuel has moved past the threshold. It returns how many it proposed.
// A failure for one market does not stop the others.
func (s *Service) Propose(ctx context.Context) (int, error) {
	now := s.now()
	indexes, err := s.store.IndexesForDay(ctx, models.Day(now))
	if err != nil {
		return 0, err
	}
	byMarket := map[string]map[string]models.Index{}
	for _, index := range indexes {
		if byMarket[index.MarketID] == nil {
			byMarket[index.MarketID] = map[string]models.Index{}
		}
		byMarket[index.MarketID][index.FuelType] = index
	}
	proposed := 0
	var errs []error
	for marketID, fuels := range byMarket {
		n, err := s.proposeForMarket(ctx, marketID, fuels, now)
		proposed += n
		if err != nil {
			errs = append(errs, fmt.Errorf("market %s: %w", marketID, err))
		}
	}
	return proposed, errors.Join(errs...)
}

func (s *Service) proposeForMarket(ctx context.Context, marketID string, fuels map[string]models.Index, now time.Time) (int, error) {
	stateID, countryCode, err := s.store.MarketChain(ctx, marketID)
	if err != nil {
		return 0, err
	}
	chain := policymodels.GeographyChain{MarketID: marketID, StateID: stateID, CountryCode: countryCode}
	rulesResolution, err := s.policies.Resolve(ctx, engine.KindFuelAdjustment, chain, now)
	if errors.Is(err, policymodels.ErrNoActivePolicy) || errors.Is(err, policymodels.ErrNotFound) {
		return 0, nil // no fuel rules here: the index is informational only
	}
	if err != nil {
		return 0, err
	}
	decoded, err := engine.Decode(engine.KindFuelAdjustment, rulesResolution.Version.SchemaVersion, rulesResolution.Version.Body)
	if err != nil {
		return 0, err
	}
	rules := decoded.(engine.FuelAdjustmentV1)

	proposed := 0
	for tariffKind, fuelType := range rules.FuelFor {
		index, ok := fuels[fuelType]
		if !ok || index.ReportCount < rules.MinReports {
			continue
		}
		done, err := s.proposeTariff(ctx, rules, rulesResolution.Version.ID, chain, tariffKind, index, now)
		if err != nil {
			return proposed, fmt.Errorf("%s: %w", tariffKind, err)
		}
		if done {
			proposed++
		}
	}
	return proposed, nil
}

func (s *Service) proposeTariff(ctx context.Context, rules engine.FuelAdjustmentV1, rulesVersionID string,
	chain policymodels.GeographyChain, tariffKind string, index models.Index, now time.Time) (bool, error) {
	live, err := s.policies.Resolve(ctx, tariffKind, chain, now)
	if errors.Is(err, policymodels.ErrNoActivePolicy) || errors.Is(err, policymodels.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// The live tariff was set at the baseline, unless the index itself
	// produced it -- then it was set at the fuel price of that proposal.
	baseline := rules.BaselineKoboPerLitre[index.FuelType]
	if earlier, ok, err := s.store.ProposalThatProduced(ctx, live.Version.ID); err != nil {
		return false, err
	} else if ok {
		baseline = earlier.FuelPriceKobo
	}
	change := engine.FuelChangeBPS(index.MedianKobo, baseline)
	if abs(change) < rules.ThresholdBPS {
		return false, nil
	}
	blocked, err := s.store.BlockingProposal(ctx, chain.MarketID, tariffKind, live.Version.ID,
		now.AddDate(0, 0, -rules.CooldownDays))
	if err != nil || blocked {
		return false, err
	}

	share := rules.FuelShareBPS[tariffKind]
	body, changes, err := engine.AdjustPerKm(tariffKind, live.Version.SchemaVersion, live.Version.Body, share, change)
	if err != nil {
		return false, err
	}
	reason := proposalReason(index, baseline, change, share, changes)
	requestID := "fuel-" + chain.MarketID + "-" + tariffKind + "-" + index.Day.Format("2006-01-02")
	// Drafted at the market itself, so a country-wide tariff is overridden
	// only where fuel actually moved.
	draft, err := s.policies.CreateDraft(ctx, policyusecases.DraftInput{
		Kind: tariffKind, GeographyLevel: policymodels.LevelMarket, GeographyRef: chain.MarketID,
		SchemaVersion: live.Version.SchemaVersion, Body: body,
		PerformedBy: SystemOperator, ActorCaller: "pricing-service", ChangeReason: reason,
		CloneFrom: live.Version.ID, RequestID: requestID,
	})
	if err != nil {
		return false, err
	}
	approvalID, err := s.policies.Submit(ctx, draft.ID, SystemOperator, "pricing-service", reason, requestID, draft.BodyHash)
	if err != nil {
		return false, err
	}
	return true, s.store.InsertProposal(ctx, models.Proposal{
		ID: ids.New(ids.PrefixFuelProposal), MarketID: chain.MarketID, TariffKind: tariffKind, FuelType: index.FuelType,
		IndexDay: index.Day, FuelPriceKobo: index.MedianKobo, BaselineKobo: baseline, ChangeBPS: change, ShareBPS: share,
		ReportCount: index.ReportCount, RulesVersionID: rulesVersionID, BaseVersionID: live.Version.ID,
		ProposedVersionID: draft.ID, ApprovalRequestID: approvalID,
	})
}

// proposalReason is what the approver reads: what moved, by how much, from how
// many reports, and what it does to the per-km rate.
func proposalReason(index models.Index, baseline, change, share int64, changes []engine.PerKmChange) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Fuel index: %s %s/L from %d drivers' reports (%d-day window), against %s/L when these rates were set (%+.1f%%). ",
		strings.ToLower(index.FuelType), naira(index.MedianKobo), index.ReportCount, index.WindowDays, naira(baseline), float64(change)/100)
	fmt.Fprintf(&b, "Fuel is %.0f%% of the per-km rate, so per-km moves %+.1f%%:", float64(share)/100, float64(share*change)/1_000_000)
	for _, c := range changes {
		label := c.Class
		if label == "" {
			label = "per km"
		}
		fmt.Fprintf(&b, " %s %s -> %s;", label, naira(c.OldKobo), naira(c.NewKobo))
	}
	b.WriteString(" Proposed automatically; approve and activate to apply.")
	return b.String()
}

func naira(kobo int64) string {
	whole, frac := kobo/100, kobo%100
	digits := fmt.Sprintf("%d", whole)
	var out []byte
	for i := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, digits[i])
	}
	if frac == 0 {
		return "NGN " + string(out)
	}
	return fmt.Sprintf("NGN %s.%02d", out, frac)
}

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
