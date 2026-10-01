package supportusecases

import (
	"context"
	"errors"
	"testing"

	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
)

type complaintMediaValidator struct {
	bindingID string
	usable    bool
	err       error
}

func (v *complaintMediaValidator) Validate(_ context.Context, objectID, bindingType, bindingID string) (supportclients.EvidenceVerdict, error) {
	v.bindingID = bindingID
	if objectID != "media-one" || bindingType != "COMPLAINT" {
		return supportclients.EvidenceVerdict{}, errors.New("wrong media binding")
	}
	return supportclients.EvidenceVerdict{Usable: v.usable}, v.err
}

func TestCustomerEvidenceRequiresMediaVerdictForOwnedComplaint(t *testing.T) {
	repo := newMemRepo()
	validator := &complaintMediaValidator{}
	svc := NewSupportService(repo, Options{Media: validator})
	c := seedComplaint(t, svc)
	in := AddEvidenceInput{ComplaintID: c.ID, UploaderType: supportmodels.ComplainantCustomer, UploaderID: "owner", MediaAssetID: "media-one"}
	if _, err := svc.AddEvidence(context.Background(), in); err == nil {
		t.Fatal("unusable MediaRef was accepted")
	}
	if validator.bindingID != c.ID {
		t.Fatalf("validated against %q, want owned complaint", validator.bindingID)
	}
	in.MediaURL = "https://untrusted.example/evidence"
	validator.usable = true
	if _, err := svc.AddEvidence(context.Background(), in); err == nil {
		t.Fatal("caller URL was accepted")
	}
	in.MediaURL = ""
	in.UploaderID = "someone-else"
	if _, err := svc.AddEvidence(context.Background(), in); err == nil {
		t.Fatal("cross-customer evidence accepted")
	}
	in.UploaderID = "owner"
	validator.err = errors.New("media unavailable")
	if _, err := svc.AddEvidence(context.Background(), in); err == nil {
		t.Fatal("media outage was treated as approval")
	}
	validator.err = nil
	if _, err := svc.AddEvidence(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	if len(repo.evidence[c.ID]) != 1 {
		t.Fatalf("stored %d evidence rows, want one", len(repo.evidence[c.ID]))
	}
}
