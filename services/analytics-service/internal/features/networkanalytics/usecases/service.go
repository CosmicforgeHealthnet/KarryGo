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
	answer.SampleSize, answer.DataAsOf = totals.ReadyCount, totals.DataAsOf
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
	if held >= models.MinimumSample {
		answer.Available, answer.Status = true, networkanalytics.StatusOK
		answer.Score = answer.CompletionRate * (1 - answer.RecoveryInvolvement)
	}
	return answer, nil
}

// SupplySnapshot is the latest supply in a zone, if it is still current.
func (s *Service) SupplySnapshot(ctx context.Context, authority, zone string) (networkanalytics.SupplySnapshot, error) {
	answer := networkanalytics.SupplySnapshot{Signal: networkanalytics.Neutral(networkanalytics.StatusInsufficientData),
		Authority: authority, Zone: zone}
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
