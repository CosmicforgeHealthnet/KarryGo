package models

import "testing"

func TestLeadTransitionMatrix(t *testing.T) {
	allowed := map[[2]string]bool{
		{LeadNew, LeadContacted}:          true,
		{LeadNew, LeadQualified}:          true,
		{LeadNew, LeadDisqualified}:       true,
		{LeadContacted, LeadQualified}:    true,
		{LeadContacted, LeadDisqualified}: true,
		{LeadQualified, LeadContacted}:    true,
		{LeadQualified, LeadDisqualified}: true,
	}
	all := []string{LeadNew, LeadContacted, LeadQualified, LeadDisqualified, LeadConverted}
	for _, from := range all {
		for _, to := range all {
			if got := CanTransitionLead(from, to); got != allowed[[2]string{from, to}] {
				t.Errorf("CanTransitionLead(%s,%s)=%v", from, to, got)
			}
		}
	}
	// CONVERTED is never a status move; only conversion reaches it.
	for _, from := range all {
		if CanTransitionLead(from, LeadConverted) {
			t.Errorf("%s -> CONVERTED must not be a status transition", from)
		}
	}
	for _, s := range []string{LeadNew, LeadContacted, LeadQualified} {
		if !LeadConvertible(s) || TerminalLead(s) {
			t.Errorf("%s must be open and convertible", s)
		}
	}
	for _, s := range []string{LeadDisqualified, LeadConverted} {
		if LeadConvertible(s) || !TerminalLead(s) {
			t.Errorf("%s must be terminal", s)
		}
	}
}

func TestLeadScoreAndSource(t *testing.T) {
	for _, v := range []int{-1, 101} {
		value := v
		if ValidateLeadScore(&value) == nil {
			t.Errorf("score %d accepted", v)
		}
	}
	zero, hundred := 0, 100
	if ValidateLeadScore(&zero) != nil || ValidateLeadScore(&hundred) != nil || ValidateLeadScore(nil) != nil {
		t.Error("valid scores refused")
	}
	if ValidLeadSource("TELEPATHY") || !ValidLeadSource(LeadSourceReferral) {
		t.Error("lead source vocabulary wrong")
	}
}
