package custodyops

import (
	"context"
	"fmt"
	"strings"

	"cosmicforge/logistics/shared/go/mediaclient"
)

// MediaFileAuthorizer validates an opaque Media File Service reference. It
// never reads or stores the underlying bytes.
type MediaFileAuthorizer struct {
	Client        *mediaclient.Client
	AllowedOwners map[string]struct{}
}

func (a MediaFileAuthorizer) ValidateCustodyMedia(ctx context.Context, command EvidenceCommand) error {
	if a.Client == nil {
		return ErrMediaRejected
	}
	asset, err := a.Client.GetByID(ctx, command.MediaRef)
	if err != nil || strings.ToLower(asset.Status) != "active" {
		return ErrMediaRejected
	}
	if _, ok := a.AllowedOwners[asset.OwnerService]; !ok {
		return ErrMediaRejected
	}
	mission, _ := asset.Metadata["mission_ref"].(string)
	tenant, _ := asset.Metadata["tenant_ref"].(string)
	intent, _ := asset.Metadata["upload_intent"].(string)
	if strings.TrimSpace(mission) != command.MissionRef || strings.TrimSpace(tenant) != command.TenantRef || !allowedIntent(intent, command.EvidenceType) {
		return ErrMediaRejected
	}
	if asset.OwnerID != command.MissionRef && asset.OwnerID != command.TenantRef {
		return fmt.Errorf("%w: owner mismatch", ErrMediaRejected)
	}
	return nil
}

func allowedIntent(intent string, evidenceType EvidenceType) bool {
	intent = strings.ToUpper(strings.TrimSpace(intent))
	return intent == "CUSTODY_EVIDENCE" || intent == string(evidenceType)+"_EVIDENCE"
}
