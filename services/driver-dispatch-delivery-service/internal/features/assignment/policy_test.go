package assignment

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestPolicyDefaultsToLegacyBroadcastAndGatesNewStrategies(t *testing.T) {
	p := normalizePolicy(Policy{})
	got, err := p.Select("", "", false)
	if err != nil || got != BroadcastFirstAccept {
		t.Fatalf("default=%s err=%v", got, err)
	}
	if _, err = p.Select(SequentialNearest, "", false); err == nil {
		t.Fatal("disabled sequential strategy allowed")
	}
	if p.OfferWindow != 75*time.Second {
		t.Fatalf("offer window=%s", p.OfferWindow)
	}
}
func TestCandidateRankingIsDeterministicAndRejectsDuplicates(t *testing.T) {
	ranked, err := RankCandidates([]Candidate{{ProviderID: "b", DistanceM: 100}, {ProviderID: "a", DistanceM: 100}, {ProviderID: "c", DistanceM: 50}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"c", "a", "b"}
	for i, c := range ranked {
		if c.ProviderID != want[i] || c.Rank != i+1 {
			t.Fatalf("ranked=%+v", ranked)
		}
	}
	if _, err = RankCandidates([]Candidate{{ProviderID: "a"}, {ProviderID: "a"}}); err == nil {
		t.Fatal("duplicate accepted")
	}
}

type memoryStore struct {
	mu       sync.Mutex
	winner   string
	started  []Candidate
	added    int
	request  StartRequest
	strategy Strategy
}

func (m *memoryStore) Start(_ context.Context, request StartRequest, strategy Strategy, candidates []Candidate, _ time.Duration, _ string, _ bool) (Assignment, error) {
	m.request, m.strategy = request, strategy
	m.started = append([]Candidate(nil), candidates...)
	return Assignment{}, nil
}
func (m *memoryStore) ListProviderOffers(context.Context, string) ([]Offer, error) { return nil, nil }
func (m *memoryStore) Accept(_ context.Context, offer, provider string, d Decision, _ time.Time) (Assignment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.winner != "" && m.winner != provider {
		return Assignment{}, ErrConflict
	}
	m.winner = provider
	return Assignment{MissionID: "mission", Status: "ASSIGNED", Offer: &Offer{ID: offer, ProviderID: provider, Status: "ACCEPTED"}}, nil
}
func (m *memoryStore) OfferAuthorityContext(context.Context, string, string) (OfferAuthorityContext, error) {
	return OfferAuthorityContext{MissionID: "mission", Strategy: SequentialNearest}, nil
}
func (m *memoryStore) Decline(context.Context, string, string, Decision, time.Time, time.Duration) (Assignment, error) {
	return Assignment{}, nil
}
func (m *memoryStore) ExpireOffers(context.Context, time.Time, time.Duration, int) (int, error) {
	return 0, nil
}
func (m *memoryStore) ApplyQueueOption(context.Context, string, string, QueueOption, time.Time) (QueueEntry, error) {
	return QueueEntry{}, nil
}
func (m *memoryStore) ProcessQueue(context.Context, time.Time, time.Duration, time.Duration, int) (int, error) {
	return 0, nil
}
func (m *memoryStore) ListDueQueueMissions(context.Context, time.Time, int) ([]string, error) {
	return nil, nil
}
func (m *memoryStore) ListPendingQueueFinancial(context.Context, int) ([]PendingQueueFinancial, error) {
	return nil, nil
}
func (m *memoryStore) MarkQueueFinancial(context.Context, string, string, string) error { return nil }
func (m *memoryStore) ApplyPartnerQueueOption(context.Context, string, string, int64, QueueOption, time.Time) (QueueEntry, error) {
	return QueueEntry{}, nil
}
func (m *memoryStore) AddCandidates(_ context.Context, _ string, candidates []Candidate) error {
	m.added += len(candidates)
	return nil
}
func (m *memoryStore) ListAcceptedWithoutTrip(context.Context, int) ([]PendingWinner, error) {
	return nil, nil
}

type finderFake struct{ candidates []Candidate }

func (f finderFake) Discover(context.Context, string, bool) ([]Candidate, error) {
	return f.candidates, nil
}
func (f finderFake) Validate(context.Context, string, string) error { return nil }

func TestSequentialStartUsesAuthoritativeDiscovery(t *testing.T) {
	store := &memoryStore{}
	svc := New(store, nil, Policy{SequentialEnabled: true}).WithCandidateFinder(finderFake{[]Candidate{{ProviderID: "provider-b", DistanceM: 200}, {ProviderID: "provider-a", DistanceM: 100}}})
	if _, err := svc.Start(context.Background(), StartRequest{MissionID: "mission", Strategy: SequentialNearest, IdempotencyKey: "key"}, false); err != nil {
		t.Fatal(err)
	}
	if len(store.started) != 2 || store.started[0].ProviderID != "provider-a" {
		t.Fatalf("candidates=%+v", store.started)
	}
}

func TestQueuePriorityIsServerOwned(t *testing.T) {
	store := &memoryStore{}
	svc := New(store, nil, Policy{SequentialEnabled: true, QueueEnabled: true}).WithCandidateFinder(finderFake{[]Candidate{{ProviderID: "provider", DistanceM: 1}}})
	_, err := svc.Start(context.Background(), StartRequest{MissionID: "mission", Strategy: SequentialNearest, Priority: 100, ServiceTier: "standard", IdempotencyKey: "key"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if store.request.Priority != 50 || store.request.PriorityReason != "NORMAL" {
		t.Fatalf("priority=%d reason=%s", store.request.Priority, store.request.PriorityReason)
	}
}

func TestAdminAssistedRequiresReasonAndUsesTargetOffer(t *testing.T) {
	store := &memoryStore{}
	svc := New(store, nil, Policy{AdminEnabled: true}).WithCandidateFinder(finderFake{})
	base := StartRequest{MissionID: "mission", Strategy: AdminAssisted, ProviderID: "provider", ActorRef: "admin:1", ExpectedMissionVersion: 1, IdempotencyKey: "key"}
	if _, err := svc.Start(context.Background(), base, true); err == nil {
		t.Fatal("missing reason accepted")
	}
	base.ReasonCode = "RECOVERY"
	if _, err := svc.Start(context.Background(), base, true); err != nil {
		t.Fatal(err)
	}
	if store.strategy != AdminAssisted || len(store.started) != 1 || store.started[0].ProviderID != "provider" {
		t.Fatalf("strategy=%s candidates=%+v", store.strategy, store.started)
	}
}

func TestConcurrentAcceptHasOneWinner(t *testing.T) {
	store := &memoryStore{}
	svc := New(store, nil, Policy{SequentialEnabled: true}).WithCandidateFinder(finderFake{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, provider := range []string{"a", "b"} {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			_, err := svc.Accept(context.Background(), "offer", p, Decision{IdempotencyKey: "key-" + p, ExpectedVersion: 1})
			results <- err
		}(provider)
	}
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestQueueRequoteRequiresFreshAuthoritativeReference(t *testing.T) {
	svc := New(&memoryStore{}, nil, Policy{})
	_, err := svc.ApplyQueueOption(context.Background(), "mission", "customer", QueueOption{Option: "REQUOTE", IdempotencyKey: "key", QuoteRef: "q", QuoteExpiresAt: ptrTime(time.Now().Add(-time.Minute))})
	if !errors.Is(err, ErrStaleQuote) {
		t.Fatalf("err=%v", err)
	}
}
func ptrTime(v time.Time) *time.Time { return &v }
