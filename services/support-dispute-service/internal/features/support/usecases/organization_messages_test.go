package supportusecases

import (
	"context"
	"errors"
	"testing"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/apperrors"
)

func appCode(err error) string {
	var ae *apperrors.Error
	if errors.As(err, &ae) {
		return string(ae.Code)
	}
	return ""
}

// TestOrganizationMessages covers the Fleet support-case message contract:
// organization ownership, validation, exactly-once per client message id, a
// conflicting reuse refused, read-back, and INTERNAL messages hidden from
// complainant routes while platform support still sees them.
func TestOrganizationMessages(t *testing.T) {
	ctx := context.Background()
	repo := newMemRepo()
	svc := NewSupportService(repo, Options{})
	c, err := svc.CreateOrganizationCase(ctx, OrganizationCaseInput{OrganizationReference: "org_a", IdempotencyKey: "case-1", Subject: "Late pickups", Description: "Three late pickups"})
	if err != nil {
		t.Fatal(err)
	}
	const clientID = "6f1c2c1e-3b2a-4f7e-9a51-1d2b3c4d5e6f"
	base := OrganizationMessageInput{OrganizationReference: "org_a", CaseID: c.ID, ClientMessageID: clientID, SenderReference: "usr_1", Text: "Please call the branch.", FileIDs: []string{"fil_1"}}

	for name, in := range map[string]OrganizationMessageInput{
		"bad client id":   func() OrganizationMessageInput { v := base; v.ClientMessageID = "not-a-uuid"; return v }(),
		"empty text":      func() OrganizationMessageInput { v := base; v.Text = "  "; return v }(),
		"bad visibility":  func() OrganizationMessageInput { v := base; v.Visibility = "PUBLIC"; return v }(),
		"duplicate files": func() OrganizationMessageInput { v := base; v.FileIDs = []string{"fil_1", "fil_1"}; return v }(),
		"missing sender":  func() OrganizationMessageInput { v := base; v.SenderReference = ""; return v }(),
		"missing org":     func() OrganizationMessageInput { v := base; v.OrganizationReference = ""; return v }(),
		"too many files": func() OrganizationMessageInput {
			v := base
			v.FileIDs = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}
			return v
		}(),
	} {
		if _, _, err := svc.CreateOrganizationMessage(ctx, in); appCode(err) != string(apperrors.CodeValidationFailed) {
			t.Fatalf("%s: err = %v, want validation", name, err)
		}
	}
	other := base
	other.OrganizationReference = "org_b"
	if _, _, err := svc.CreateOrganizationMessage(ctx, other); appCode(err) != string(apperrors.CodeNotFound) {
		t.Fatalf("another organization's case = %v, want not found", err)
	}

	first, created, err := svc.CreateOrganizationMessage(ctx, base)
	if err != nil || !created || first.Visibility != supportmodels.VisibilityShared || first.SenderType != supportmodels.SenderTypeOrganizationUser {
		t.Fatalf("first = %+v created=%v err=%v", first, created, err)
	}
	again, created, err := svc.CreateOrganizationMessage(ctx, base)
	if err != nil || created || again.ID != first.ID {
		t.Fatalf("replay = %+v created=%v err=%v, want the stored message", again, created, err)
	}
	changed := base
	changed.Text = "A different body"
	if _, _, err := svc.CreateOrganizationMessage(ctx, changed); appCode(err) != string(apperrors.CodeConflict) {
		t.Fatalf("reused client id with a different body = %v, want conflict", err)
	}
	if got, err := svc.GetOrganizationMessage(ctx, "org_a", c.ID, clientID); err != nil || got.ID != first.ID {
		t.Fatalf("read-back = %+v, %v", got, err)
	}
	if _, err := svc.GetOrganizationMessage(ctx, "org_b", c.ID, clientID); appCode(err) != string(apperrors.CodeNotFound) {
		t.Fatalf("read-back from another organization = %v, want not found", err)
	}
	if _, err := svc.GetOrganizationMessage(ctx, "org_a", c.ID, "0d6c9f1e-1111-4f7e-9a51-1d2b3c4d5e6f"); appCode(err) != string(apperrors.CodeNotFound) {
		t.Fatalf("unknown client id = %v, want not found", err)
	}

	internal := base
	internal.ClientMessageID = "7a2d3c1e-3b2a-4f7e-9a51-1d2b3c4d5e6f"
	internal.Visibility = "internal"
	internal.FileIDs = nil
	if m, _, err := svc.CreateOrganizationMessage(ctx, internal); err != nil || m.Visibility != supportmodels.VisibilityInternal {
		t.Fatalf("internal message = %+v, %v", m, err)
	}
	all, err := svc.ListChatMessages(ctx, c.ID, supportmodels.SenderTypeAdmin, "admin", 50, 0)
	if err != nil || len(all) != 2 {
		t.Fatalf("support sees %d messages (%v), want 2", len(all), err)
	}
	if visible, _ := repo.ListChatMessages(ctx, c.ID, false, 50, 0); len(visible) != 1 {
		t.Fatalf("complainant view has %d messages, want 1 (INTERNAL hidden)", len(visible))
	}
}
