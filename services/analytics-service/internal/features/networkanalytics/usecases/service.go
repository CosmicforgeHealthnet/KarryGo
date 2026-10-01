// Package usecases turns network events into read models, and read models
// into advisory signals.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/models"
	"cosmicforge/logistics/services/analytics-service/internal/features/networkanalytics/repositories"
	sharedevents "cosmicforge/logistics/shared/go/events"
	"cosmicforge/logistics/shared/go/networkanalytics"
)

// ErrCallerMismatch: the envelope names a source other than the signed caller.
var ErrCallerMismatch = errors.New("event source does not match the calling service")

// SupplyFreshness is how old a supply snapshot may be and still count. A zone
// that stopped reporting has not stopped having riders or losing them; an
// old number presented as current is worse than no number.
const SupplyFreshness = 15 * time.Minute

// Service is the network analytics usecase layer.
type Service struct {
	repo *repositories.Repository
	now  func() time.Time
}

// NewService builds the service.
func NewService(repo *repositories.Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// Ingest validates, minimizes, classifies and stores one event.
func (s *Service) Ingest(ctx context.Context, caller string, envelope sharedevents.Envelope) (string, error) {
	if err := envelope.Validate(); err != nil {
		return "", err
	}
	if !sameService(caller, envelope.SourceService) {
		return "", ErrCallerMismatch
	}
	var raw map[string]any
	if err := json.Unmarshal(envelope.Payload, &raw); err != nil {
		return "", fmt.Errorf("%w: payload must be a JSON object", sharedevents.ErrInvalidEnvelope)
	}
	// Minimize first, on the producer's own key spelling, then settle on one
	// spelling for the read models.
	payload := networkanalytics.NormalizeKeys(networkanalytics.Minimize(raw))
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	authorityHint, _ := payload["authority"].(string)
	authority, kind, _ := networkanalytics.Classify(envelope.EventName, authorityHint)
	return s.repo.Ingest(ctx, models.Event{
		EventID: envelope.EventID, EventName: strings.TrimSpace(envelope.EventName),
		SourceService: envelope.SourceService, SourceEntityType: envelope.SourceEntityType,
		SourceEntityID: envelope.SourceEntityID, SourceVersion: envelope.SourceVersion,
		OccurredAt: envelope.OccurredAt, CorrelationID: envelope.CorrelationID,
		Payload: payload, PayloadHash: hex.EncodeToString(sum[:]),
		Authority: authority, Kind: kind,
	})
}

// serviceAliases are the two spellings several services sign under.
var serviceAliases = map[string]string{
	"taxi-service": "driver-taxi-service", "dispatch-delivery-service": "driver-dispatch-delivery-service",
	"hauling-service": "driver-hauling-service",
}

func canonical(service string) string {
	service = strings.TrimSpace(service)
	if alias, ok := serviceAliases[service]; ok {
		return alias
	}
	return service
}

func sameService(a, b string) bool { return canonical(a) == canonical(b) }

// BranchReadiness is the advisory readiness read for one branch.
func (s *Service) BranchReadiness(ctx context.Context, branchID string, days int) (networkanalytics.BranchReadiness, error) {
	answer := networkanalytics.BranchReadiness{Signal: networkanalytics.Neutral(networkanalytics.StatusInsufficientData),
		BranchID: branchID}
	totals, err := s.repo.ReadinessTotals(ctx, branchID, s.window(days))
	if errors.Is(err, repositories.ErrNoData) {
		return answer, nil
	}
	if err != nil {
		return answer, err
	}
	answer.SampleSize, answer.DataAsOf, answer.ProfileVersion = totals.ReadyCount, totals.DataAsOf, totals.ReadyCount
	answer.OnTimeRate = ratio(totals.OnTimeCount, totals.ReadyCount)
	answer.AverageLatenessSecs = ratioFloat(totals.LatenessSecondsTotal, totals.LateCount)
	answer.AveragePrepSecs = ratioFloat(totals.PrepSecondsTotal, totals.PrepCount)
	if totals.ReadyCount >= models.MinimumSample {
		answer.Available, answer.Status, answer.Score = true, networkanalytics.StatusOK, answer.OnTimeRate
	}
	return answer, nil
}

// ProviderQuality is the advisory reliability read for one provider.
//
// The score is operational only: how often jobs this provider held were
// completed, discounted by how often they had to be rescued from them. It
// uses no price, fare, rating or earnings -- none of which analytics holds.
func (s *Service) ProviderQuality(ctx context.Context, authority, providerID string, days int) (networkanalytics.ProviderQuality, error) {
	answer := networkanalytics.ProviderQuality{Signal: networkanalytics.Neutral(networkanalytics.StatusInsufficientData),
		Authority: authority, ProviderID: providerID}
	totals, err := s.repo.ProviderTotals(ctx, authority, providerID, s.window(days))
	if errors.Is(err, repositories.ErrNoData) {
		return answer, nil
	}
	if err != nil {
		return answer, err
	}
	held := totals.Completed + totals.Failed + totals.RecoveriesAsOriginal
	answer.SampleSize, answer.DataAsOf = held, totals.DataAsOf
	answer.CompletionRate = ratio(totals.Completed, totals.Completed+totals.Failed)
	answer.RecoveryInvolvement = ratio(totals.RecoveriesAsOriginal, held)
	answer.Declines, answer.SafetyDeclines = totals.Declines, totals.SafetyDeclines
	if held >= models.MinimumSample {
		answer.Available, answer.Status = true, networkanalytics.StatusOK
		answer.Score = answer.CompletionRate * (1 - answer.RecoveryInvolvement)
	}
	return answer, nil
}

// SupplySnapshot is the latest supply in a zone, if it is still current.
func (s *Service) SupplySnapshot(ctx context.Context, authority, zone string) (networkanalytics.SupplySnapshot, error) {
	return s.SupplySnapshotFor(ctx, authority, zone, "")
}

// SupplySnapshotFor is the latest supply in a zone, optionally for one
// product. Per-product facts from the authorities win; the legacy per-zone
// table answers only where no product fact exists.
func (s *Service) SupplySnapshotFor(ctx context.Context, authority, zone, product string) (networkanalytics.SupplySnapshot, error) {
	answer := networkanalytics.SupplySnapshot{Signal: networkanalytics.Neutral(networkanalytics.StatusInsufficientData),
		Authority: authority, Zone: zone, ProductCode: product}
	facts, err := s.repo.ProductSupply(ctx, authority, zone, strings.ToUpper(strings.TrimSpace(product)))
	if err == nil {
		answer.OnlineCount, answer.EligibleCount, answer.OccupiedCount = facts.OnlineCount, facts.EligibleCount, facts.OccupiedCount
		answer.SupplyState, answer.ObservedAt, answer.DataAsOf = facts.State, facts.ObservedAt, facts.ObservedAt
		answer.SampleSize = facts.OnlineCount
		if facts.OccupiedCount != nil {
			answer.AvailableProviders = facts.OnlineCount - *facts.OccupiedCount
			answer.BusyProviders = *facts.OccupiedCount
		}
		// Fresh AND fully known: only then is there a score to give. An
		// unknown count is exactly the case where a guess would mislead.
		if s.now().Sub(facts.ObservedAt) <= SupplyFreshness && facts.EligibleCount != nil && facts.OccupiedCount != nil {
			answer.Available, answer.Status = true, networkanalytics.StatusOK
			answer.Score = ratio(*facts.EligibleCount-min(*facts.EligibleCount, *facts.OccupiedCount), facts.OnlineCount)
		}
		return answer, nil
	}
	if !errors.Is(err, repositories.ErrNoData) {
		return answer, err
	}
	supply, err := s.repo.LatestSupply(ctx, authority, zone)
	if errors.Is(err, repositories.ErrNoData) {
		return answer, nil
	}
	if err != nil {
		return answer, err
	}
	answer.AvailableProviders, answer.BusyProviders = supply.AvailableProviders, supply.BusyProviders
	answer.ObservedAt, answer.DataAsOf = supply.ObservedAt.UTC(), supply.ObservedAt.UTC()
	total := supply.AvailableProviders + supply.BusyProviders
	answer.SampleSize = total
	if s.now().Sub(supply.ObservedAt) <= SupplyFreshness {
		answer.Available, answer.Status = true, networkanalytics.StatusOK
		answer.Score = ratio(supply.AvailableProviders, total)
		if total == 0 {
			answer.Score = 0
		}
	}
	return answer, nil
}

// MissionSuccess, Recovery, Mismatches and Demand back the admin dashboards.
func (s *Service) MissionSuccess(ctx context.Context, authority string, days int) ([]models.MissionSuccessDay, error) {
	return s.repo.MissionSuccess(ctx, authority, s.window(days))
}

func (s *Service) Recovery(ctx context.Context, authority string, days int) ([]models.RecoveryDay, error) {
	return s.repo.Recovery(ctx, authority, s.window(days))
}

func (s *Service) Mismatches(ctx context.Context, authority string, days int) ([]models.MismatchDay, error) {
	return s.repo.Mismatches(ctx, authority, s.window(days))
}

func (s *Service) Declines(ctx context.Context, authority string, days int) ([]models.DeclineDay, error) {
	return s.repo.Declines(ctx, authority, s.window(days))
}

func (s *Service) Demand(ctx context.Context, authority string, days int) ([]models.DemandDay, error) {
	return s.repo.Demand(ctx, authority, s.window(days))
}

func (s *Service) window(days int) models.Window {
	if days <= 0 || days > 366 {
		days = 30
	}
	end := s.now().Truncate(24 * time.Hour)
	return models.Window{Start: end.AddDate(0, 0, -(days - 1)), End: end}
}

func ratio(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func ratioFloat(total, count int64) float64 { return ratio(total, count) }

// DemandForecast is the expected demand for the bucket containing at.
//
// Deterministic and explainable: the mean demand of the same weekday and time
// slot over the previous weeks, from buckets the authorities published. A
// slot the authorities never reported is absent, not zero.
func (s *Service) DemandForecast(ctx context.Context, authority, zone, product string, at time.Time) (networkanalytics.DemandForecast, error) {
	if at.IsZero() {
		at = s.now()
	}
	bucket := networkanalytics.DemandBucketStart(at)
	product = strings.ToUpper(strings.TrimSpace(product))
	answer := networkanalytics.DemandForecast{Signal: networkanalytics.Neutral(networkanalytics.StatusInsufficientData),
		Authority: authority, Zone: zone, ProductCode: product, BucketStart: bucket,
		Method: networkanalytics.DemandForecastMethod}
	slots := []time.Time{bucket}
	for week := 1; week <= networkanalytics.DemandForecastWeeks; week++ {
		slots = append(slots, bucket.AddDate(0, 0, -7*week))
	}
	found, err := s.repo.DemandSlots(ctx, authority, zone, product, slots)
	if err != nil {
		return answer, err
	}
	var total, history, latestVersion int64
	for _, slot := range found {
		if slot.ObservedAt.After(answer.DataAsOf) {
			answer.DataAsOf = slot.ObservedAt
		}
		if slot.SourceVersion > latestVersion {
			latestVersion = slot.SourceVersion
		}
		if slot.BucketStart.Equal(bucket) {
			current := slot.DemandCount
			answer.CurrentCount = &current
			continue
		}
		total += slot.DemandCount
		history++
	}
	answer.SampleSize = history
	answer.ModelVersion = fmt.Sprintf("%s@%d", networkanalytics.DemandForecastMethod, latestVersion)
	if history > 0 {
		answer.Expected = float64(total) / float64(history)
	}
	if history >= networkanalytics.DemandForecastMinimumSlots {
		answer.Available, answer.Status = true, networkanalytics.StatusOK
		answer.Score = networkanalytics.NeutralScore
	}
	return answer, nil
}

// MissionOutcomes backs the per-product lifecycle dashboard.
func (s *Service) MissionOutcomes(ctx context.Context, authority, product string, days int) ([]models.MissionOutcomeDay, error) {
	return s.repo.MissionOutcomes(ctx, authority, strings.ToUpper(strings.TrimSpace(product)), s.window(days))
}
