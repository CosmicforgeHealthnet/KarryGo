package inventory

import "strings"

// TenantContext represents exactly one Vendor business Inventory tenant.
//
// There is intentionally no implicit "all businesses" Vendor actor.
type TenantContext struct {
	BusinessID string
}

// Validate rejects unscoped Inventory tenancy.
func (t TenantContext) Validate() error {
	if strings.TrimSpace(t.BusinessID) == "" {
		return ErrInvalidTenant
	}

	return nil
}

// Actor represents an authenticated Inventory application actor after the
// existing Vendor authentication and IAM layers have resolved the caller.
//
// Inventory does not parse JWTs and does not authorize using role names.
type Actor struct {
	ActorRef string
	StaffID  string

	BusinessID string

	CorrelationID string
	CausationID   string
	RequestID     string
}

// Validate requires a tenant and an identifiable actor.
func (a Actor) Validate() error {
	if err := (TenantContext{
		BusinessID: a.BusinessID,
	}).Validate(); err != nil {
		return ErrInvalidActor
	}

	if strings.TrimSpace(a.ActorRef) == "" &&
		strings.TrimSpace(a.StaffID) == "" {
		return ErrInvalidActor
	}

	return nil
}

// Tenant returns the actor's tenant.
func (a Actor) Tenant() TenantContext {
	return TenantContext{
		BusinessID: strings.TrimSpace(a.BusinessID),
	}
}

// Identity returns the stable actor evidence identity.
func (a Actor) Identity() string {
	if value := strings.TrimSpace(a.ActorRef); value != "" {
		return value
	}

	return strings.TrimSpace(a.StaffID)
}
