package security

import (
	"cosmicforge/logistics/services/admin-backoffice-service/internal/config"
	"time"
)

type Risk string

const (
	R0 Risk = "R0"
	R1 Risk = "R1"
	R2 Risk = "R2"
	R3 Risk = "R3"
	R4 Risk = "R4"
)

type Scope struct {
	Market       string `json:"market"`
	Organization string `json:"organization"`
}
type Policy struct {
	Permission string
	Risk       Risk
	Version    string
}

// Approval records are operator-provisioned immutable decisions, never body claims.
type Approval struct {
	ID, Operator, Approver, Action, Digest, PolicyVersion, IncidentReference string
	Scope                                                                    Scope
	Expires                                                                  time.Time
	Revoked                                                                  bool
}

func Authorize(c Claims, p Policy, scope Scope, trust config.Trust) error {
	if p.Version != trust.PolicyVersion || !Opaque(scope.Market) || !Opaque(scope.Organization) || !config.Contains(c.Permissions, p.Permission) || !config.Contains(c.Markets, scope.Market) || !config.Contains(c.Organizations, scope.Organization) {
		return ErrDenied
	}
	switch p.Risk {
	case R0, R1, R2:
		return nil
	case R3, R4:
		if config.Contains(trust.StrongACR, c.ACR) {
			return nil
		}
	}
	return ErrDenied
}
func CheckApproval(c Claims, p Policy, a Approval, action, digest string, scope Scope, now time.Time) error {
	if p.Risk != R3 && p.Risk != R4 {
		return ErrDenied
	}
	if !Opaque(a.ID) || !Opaque(a.Approver) || a.Revoked || !a.Expires.After(now) || a.Operator != c.Subject || a.Approver == c.Subject || a.Action != action || a.Digest != digest || a.Scope != scope || a.PolicyVersion != p.Version || (p.Risk == R4 && !Opaque(a.IncidentReference)) {
		return ErrDenied
	}
	return nil
}
