package assignment

import (
	"context"
	"errors"
	"testing"
	"time"
)

type replacementMemory struct {
	memoryStore
	scope      ReplacementContext
	offers     int
	candidates []Candidate
}

func (m *replacementMemory) ReadReplacement(context.Context, string) (ReplacementContext, error) {
	return m.scope, nil
}
func (m *replacementMemory) OfferReplacement(_ context.Context, _ ReplacementContext, c []Candidate, _ time.Time, _ time.Duration) (bool, error) {
	m.offers++
	m.candidates = c
	return false, nil
}

type fleetMembership struct{}

func (fleetMembership) ValidateMembership(_ context.Context, org, provider string) error {
	if org == "org" && provider == "member" {
		return nil
	}
	return errors.New("not a member")
}
func TestReplacementReadbackBeforeDiscovery(t *testing.T) {
	for _, ready := range []bool{false, true} {
		r := &replacementMemory{scope: ReplacementContext{Ready: ready, LiveOffer: !ready}}
		s := New(r, nil, Policy{SequentialEnabled: true})
		got, err := s.ReconcileReplacement(context.Background(), "incident")
		if err != nil || got != ready || r.offers != 0 {
			t.Fatal(got, err, r.offers)
		}
	}
}
func TestReplacementReusesRankingAndFleetMembershipWithoutTrip(t *testing.T) {
	r := &replacementMemory{scope: ReplacementContext{MissionID: "mission", OldProviderID: "old", Strategy: OrganizationDispatch, OrganizationID: "org"}}
	s := New(r, nil, Policy{OrganizationEnabled: true}).WithCandidateFinder(finderFake{[]Candidate{{ProviderID: "outsider", DistanceM: 1}, {ProviderID: "old", DistanceM: 2}, {ProviderID: "member", DistanceM: 100}}}).WithOrganizationAuthority(fleetMembership{})
	if _, err := s.ReconcileReplacement(context.Background(), "incident"); err != nil {
		t.Fatal(err)
	}
	if r.offers != 1 || len(r.candidates) != 1 || r.candidates[0].ProviderID != "member" {
		t.Fatal(r.candidates)
	}
	r.scope.Strategy = AdminAssisted
	r.scope.OrganizationID = ""
	if _, err := s.ReconcileReplacement(context.Background(), "incident"); err == nil {
		t.Fatal("admin replacement bypass")
	}
}
