// Package adminmedia is the fixed private Media case-access contract. It is
// unrelated to ambient owner download tokens or legacy media_assets IDs.
package adminmedia

import (
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"time"
)

const Caller = "admin-backoffice-service"
const Permission = "media.case.content.read"
const Contract = "media.admin-case-content.v1"
const GrantPath = "/internal/media/v1/admin/case-grants"

type Actor struct {
	Product       string    `json:"product"`
	Staff         string    `json:"staff"`
	Roles         []string  `json:"roles"`
	Permissions   []string  `json:"permissions"`
	Markets       []string  `json:"market_scopes"`
	Zones         []string  `json:"zone_scopes"`
	Organizations []string  `json:"organization_scopes"`
	Tenants       []string  `json:"tenant_scopes"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (a Actor) Delegation() adminpolicy.Actor {
	return adminpolicy.Actor{Product: a.Product, Staff: a.Staff, Roles: a.Roles, Permissions: a.Permissions, Markets: a.Markets, Zones: a.Zones, Organizations: a.Organizations, Tenants: a.Tenants}
}
func (a Actor) Valid(now time.Time) bool {
	if a.Product != "karryam" || !adminread.Opaque(a.Staff) || !a.ExpiresAt.After(now) {
		return false
	}
	for _, list := range [][]string{a.Roles, a.Permissions, a.Markets, a.Zones, a.Organizations, a.Tenants} {
		if len(list) > 128 {
			return false
		}
		seen := map[string]bool{}
		for _, v := range list {
			if !adminread.Opaque(v) || seen[v] {
				return false
			}
			seen[v] = true
		}
	}
	return true
}

type Request struct {
	Scope            adminpolicy.Scope `json:"scope"`
	Purpose          string            `json:"purpose"`
	SourceCapability string            `json:"source_capability"`
	CaseOwner        string            `json:"case_owner"`
	CaseKind         string            `json:"case_kind"`
	CaseReference    string            `json:"case_reference"`
	SubjectType      string            `json:"subject_type"`
	SubjectReference string            `json:"subject_reference"`
	MediaObjectID    string            `json:"media_object_id"`
}

func (q Request) Valid() bool {
	if q.Scope.Product != "karryam" {
		return false
	}
	for _, v := range []string{q.Scope.Market, q.Scope.Zone, q.Scope.Organization, q.Scope.Tenant, q.Purpose, q.SourceCapability, q.CaseOwner, q.CaseKind, q.CaseReference, q.SubjectType, q.SubjectReference, q.MediaObjectID} {
		if !adminread.Opaque(v) {
			return false
		}
	}
	return true
}

type Grant struct {
	ContractVersion string    `json:"contract_version"`
	ID              string    `json:"id"`
	Token           string    `json:"token"`
	MediaObjectID   string    `json:"media_object_id"`
	ExpiresAt       time.Time `json:"expires_at"`
}
type IssueRequest struct {
	Actor   Actor   `json:"actor"`
	Request Request `json:"request"`
}
type ReadRequest struct {
	Actor   Actor   `json:"actor"`
	Request Request `json:"request"`
	Token   string  `json:"token"`
}
type RevokeRequest struct {
	Actor      Actor  `json:"actor"`
	ReasonCode string `json:"reason_code"`
}
