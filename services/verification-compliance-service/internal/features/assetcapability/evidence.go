package assetcapability

import (
	"context"
	"fmt"
	"strings"

	"cosmicforge/logistics/shared/go/mediaclient"
)

// EvidenceStore is the narrow slice of media-file-service this feature needs.
//
// It is an interface rather than the concrete client so the binding rules can
// be tested without a Media deployment, and so a deployment with no Media
// credentials gets a nil store and an honest refusal rather than a client that
// silently approves everything.
type EvidenceStore interface {
	GetByID(ctx context.Context, id string) (mediaclient.MediaAsset, error)
}

// AcceptedEvidencePurposes are the media purposes an inspection or capability
// case may cite.
//
// The specification names vehicle_inspection and capability_evidence as if they
// existed. They do not: media-file-service defines exactly four purposes
// (profile_photo, document_file, proof_image, signature) and a purpose it does
// not know cannot be uploaded in the first place. So the two that actually fit
// documentary and photographic evidence are accepted, and a profile photo or a
// delivery signature is refused -- not because the bytes would be unreadable,
// but because citing one as proof of a vehicle's condition means somebody
// pointed the reviewer at the wrong object.
func AcceptedEvidencePurposes() []string {
	return []string{"document_file", "proof_image"}
}

// mediaActiveStatus is the only status a citable media asset may hold.
const mediaActiveStatus = "active"

// EvidenceBinding is the context an evidence reference must actually belong to.
type EvidenceBinding struct {
	Authority  Authority
	ProviderID string
	AssetID    string
}

// Owns reports whether a media asset is bound to this context.
//
// Media owns assets by (owner_service, owner_id). An evidence object for a
// vehicle is legitimately owned either by the driver service that uploaded it
// or by Verification itself when a reviewer attached it, and its owner_id must
// be the asset or the provider the case is about. Anything else is a reference
// to somebody else's file, and accepting it would let one provider's inspection
// photograph certify another provider's vehicle.
func (b EvidenceBinding) Owns(asset mediaclient.MediaAsset) bool {
	owner := strings.TrimSpace(asset.OwnerService)
	if owner != b.Authority.OwningService() && owner != ServiceName {
		return false
	}
	ownerID := strings.TrimSpace(asset.OwnerID)
	return ownerID != "" && (ownerID == b.AssetID || ownerID == b.ProviderID)
}

// EvidenceValidator checks that cited media actually exists and belongs where
// the citation claims.
type EvidenceValidator struct {
	store    EvidenceStore
	purposes map[string]bool
}

// NewEvidenceValidator builds a validator over a media store. A nil store makes
// every citation fail closed.
func NewEvidenceValidator(store EvidenceStore) *EvidenceValidator {
	purposes := map[string]bool{}
	for _, purpose := range AcceptedEvidencePurposes() {
		purposes[purpose] = true
	}
	return &EvidenceValidator{store: store, purposes: purposes}
}

// Validate checks every reference, returning the first stable reason a
// reference is unacceptable.
//
// An empty list is accepted: not every capability decision rests on an uploaded
// object, and requiring one would push reviewers into attaching a placeholder
// file, which is worse evidence than none.
func (v *EvidenceValidator) Validate(ctx context.Context, binding EvidenceBinding, refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	if v == nil || v.store == nil {
		return fmt.Errorf("%w: %s", ErrEvidence, ReasonEvidenceNotFound)
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return fmt.Errorf("%w: %s", ErrEvidence, ReasonEvidenceNotFound)
		}
		if seen[ref] {
			continue
		}
		seen[ref] = true
		asset, err := v.store.GetByID(ctx, ref)
		if err != nil {
			return fmt.Errorf("%w: %s (%s)", ErrEvidence, ReasonEvidenceNotFound, ref)
		}
		if !strings.EqualFold(strings.TrimSpace(asset.Status), mediaActiveStatus) {
			return fmt.Errorf("%w: %s (%s)", ErrEvidence, ReasonEvidenceNotActive, ref)
		}
		if !binding.Owns(asset) {
			return fmt.Errorf("%w: %s (%s)", ErrEvidence, ReasonEvidenceWrongOwner, ref)
		}
		if !v.purposes[strings.TrimSpace(asset.Purpose)] {
			return fmt.Errorf("%w: %s (%s)", ErrEvidence, ReasonEvidenceWrongPurpose, ref)
		}
	}
	return nil
}
