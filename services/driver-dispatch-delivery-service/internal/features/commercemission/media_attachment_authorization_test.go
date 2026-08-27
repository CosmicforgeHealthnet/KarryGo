package commercemission

import (
	"context"
	"errors"
	"testing"
)

type rejectingMediaAuthorizer struct {
	calls int
	err   error
}

func (a *rejectingMediaAuthorizer) AuthorizeEvidence(_ context.Context, _ string, _, _ []string) error {
	a.calls++
	return a.err
}

func TestOperationalEvidenceMediaAuthorizationFailsClosed(t *testing.T) {
	authority := &rejectingMediaAuthorizer{err: errors.New("cross-mission media")}
	service := &OperationalService{media: authority}
	err := service.authorizeEvidence(context.Background(), "mission-a", EvidenceCommand{
		ParcelRefs: []string{"parcel-a"}, MediaRefs: []string{"media-b"},
	})
	if err == nil || authority.calls != 1 {
		t.Fatalf("cross-mission evidence was not rejected: err=%v calls=%d", err, authority.calls)
	}

	service.media = nil
	err = service.authorizeEvidence(context.Background(), "mission-a", EvidenceCommand{MediaRefs: []string{"media-a"}})
	if err == nil {
		t.Fatal("configured media evidence was accepted without an authority")
	}
}

func TestProviderTransferUsesSameMediaAuthorizationBoundary(t *testing.T) {
	authority := &rejectingMediaAuthorizer{}
	service := &OperationalService{media: authority}
	command := EvidenceCommand{ParcelRefs: []string{"parcel-a"}, MediaRefs: []string{"transfer-proof"}}
	if err := service.authorizeEvidence(context.Background(), "mission-a", command); err != nil {
		t.Fatalf("valid transfer evidence rejected: %v", err)
	}
	if authority.calls != 1 {
		t.Fatalf("media authority calls=%d want 1", authority.calls)
	}
}
