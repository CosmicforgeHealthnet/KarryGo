package assignment

import (
	"errors"
	"sort"
	"strings"
	"time"
)

const PolicyVersion = "dispatch_assignment_v1"

type Policy struct {
	SequentialEnabled         bool
	QueueEnabled              bool
	OrganizationEnabled       bool
	AdminEnabled              bool
	OfferWindow               time.Duration
	DefaultQueueTTL           time.Duration
	QueueRetryInterval        time.Duration
	InitialRadiusKM           float64
	MaxRadiusKM               float64
	ReconciliationLease       time.Duration
	ReconciliationMaxAttempts int
	ReconciliationBackoffBase time.Duration
	ReconciliationBackoffMax  time.Duration
	RolloutServiceTiers       string
}

func (p Policy) SequentialSelected(serviceTier string) bool {
	configured := false
	for _, value := range strings.Split(p.RolloutServiceTiers, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		configured = true
		if strings.EqualFold(value, serviceTier) {
			return true
		}
	}
	// Empty preserves the established global-flag behavior. Once configured,
	// this is a restrictive server-side selector and clients cannot opt in.
	return !configured
}

func (p Policy) Select(requested Strategy, organizationID string, adminAuthorized bool) (Strategy, error) {
	if requested == "" || requested == BroadcastFirstAccept {
		return BroadcastFirstAccept, nil
	}
	switch requested {
	case SequentialNearest:
		if p.SequentialEnabled {
			return requested, nil
		}
	case OrganizationDispatch:
		if p.OrganizationEnabled && strings.TrimSpace(organizationID) != "" {
			return requested, nil
		}
	case AdminAssisted:
		if p.AdminEnabled && adminAuthorized {
			return requested, nil
		}
	default:
		return "", errors.New("invalid assignment strategy")
	}
	return "", errors.New("assignment strategy is disabled")
}

func RankCandidates(input []Candidate) ([]Candidate, error) {
	seen := map[string]bool{}
	result := append([]Candidate(nil), input...)
	for _, c := range result {
		if strings.TrimSpace(c.ProviderID) == "" || c.DistanceM < 0 || seen[c.ProviderID] {
			return nil, errors.New("invalid assignment candidate")
		}
		seen[c.ProviderID] = true
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].DistanceM == result[j].DistanceM {
			return result[i].ProviderID < result[j].ProviderID
		}
		return result[i].DistanceM < result[j].DistanceM
	})
	for i := range result {
		result[i].Rank = i + 1
	}
	return result, nil
}

func normalizePolicy(p Policy) Policy {
	if p.OfferWindow < 60*time.Second || p.OfferWindow > 90*time.Second {
		p.OfferWindow = 75 * time.Second
	}
	if p.DefaultQueueTTL <= 0 {
		p.DefaultQueueTTL = 30 * time.Minute
	}
	if p.QueueRetryInterval <= 0 {
		p.QueueRetryInterval = 90 * time.Second
	}
	if p.InitialRadiusKM <= 0 {
		p.InitialRadiusKM = 10
	}
	if p.MaxRadiusKM < p.InitialRadiusKM {
		p.MaxRadiusKM = 50
	}
	if p.ReconciliationLease <= 0 {
		p.ReconciliationLease = time.Minute
	}
	if p.ReconciliationMaxAttempts <= 0 {
		p.ReconciliationMaxAttempts = 10
	}
	if p.ReconciliationBackoffBase <= 0 {
		p.ReconciliationBackoffBase = 5 * time.Second
	}
	if p.ReconciliationBackoffMax < p.ReconciliationBackoffBase {
		p.ReconciliationBackoffMax = 15 * time.Minute
	}
	return p
}

func (p Policy) EffectivePriority(in StartRequest) (int, string, string) {
	if strings.EqualFold(in.ServiceTier, "express") {
		return 80, "EXPRESS", "dispatch_queue_priority_v1"
	}
	if in.ScheduledFor != nil && time.Until(*in.ScheduledFor) <= 30*time.Minute {
		return 70, "TIME_SENSITIVE", "dispatch_queue_priority_v1"
	}
	return 50, "NORMAL", "dispatch_queue_priority_v1"
}
