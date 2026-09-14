package custodychain

import (
	"strings"
	"time"
)

// Direction is the self-describing metadata a credential view carries.
//
// CFC-008 exists because holder and verifier were previously only knowable by
// reading the route a credential came back from. That works until a route is
// re-mounted, proxied, or reused by a second flow — at which point the client
// on the other end is inferring a security property from a URL. Carrying the
// direction in the payload makes it a fact the authority states.
//
// The JSON names are snake_case to match the credential and mission contracts
// these fields are embedded into.
type Direction struct {
	// Purpose is which handover this credential authorizes.
	Purpose CredentialPurpose `json:"credential_purpose"`

	// Holder is the party the credential was issued to.
	Holder PartyRole `json:"holder_role"`

	// Verifier is the party entitled to submit it for checking.
	Verifier PartyRole `json:"verifier_role"`

	// Authority is the execution service that owns this credential's
	// lifecycle — the only service that can generate, expire or consume it.
	Authority string `json:"execution_authority"`

	// MissionID scopes the credential. A credential without a mission is not
	// scoped to anything and could in principle be presented against other
	// work, so it is part of the description rather than an optional extra.
	MissionID string `json:"mission_id,omitempty"`

	// ExpiresAt is when the credential stops being usable. Carried here so a
	// client can tell an expired credential from a wrong one without a second
	// call, which is what CFC-011 needs to surface a recoverable action.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// DirectionFor builds the description for one credential.
//
// Holder and verifier come from the canonical matrix, never from the caller:
// an authority cannot describe its Commerce pickup as customer-held by
// filling in a struct differently from its neighbour.
func DirectionFor(purpose CredentialPurpose, authority, missionID string, expiresAt *time.Time) (Direction, bool) {
	descriptor, ok := DescriptorFor(purpose)
	if !ok {
		return Direction{}, false
	}
	return Direction{
		Purpose:   descriptor.Purpose,
		Holder:    descriptor.Holder,
		Verifier:  descriptor.Verifier,
		Authority: strings.ToUpper(strings.TrimSpace(authority)),
		MissionID: strings.TrimSpace(missionID),
		ExpiresAt: expiresAt,
	}, true
}

// Consistent reports whether a received Direction still agrees with the
// canonical matrix.
//
// A consumer calls this on anything it did not build itself. A payload whose
// holder and verifier have been swapped — by a misconfigured authority, a
// proxy, or an attacker — is refused rather than acted on, which is the
// difference between carrying the direction and trusting it.
func (d Direction) Consistent() bool {
	descriptor, ok := DescriptorFor(d.Purpose)
	if !ok {
		return false
	}
	return d.Holder == descriptor.Holder && d.Verifier == descriptor.Verifier
}

// Expired reports whether the credential has lapsed. A Direction with no
// expiry is never expired: absence means the authority did not state one, not
// that it lapsed at the zero time.
func (d Direction) Expired(now time.Time) bool {
	return d.ExpiresAt != nil && now.After(*d.ExpiresAt)
}
