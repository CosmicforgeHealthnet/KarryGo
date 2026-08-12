// Package models defines the generic, cross-domain legal hold: a durable
// preservation flag over a resource that is not itself a document.
// Document-level legal hold already exists (POST
// /documents/:documentId/retention, internal/features/documents) and
// propagates to media-file-service, which is the only service that can
// actually refuse to destroy file bytes; this package deliberately does not
// duplicate that path. DOCUMENT is not a valid scope type here for exactly
// that reason — a second, disconnected "legal hold" flag on a document would
// be one direct media call away from being ignored, same risk the documents
// feature's own SetRetention comment already calls out.
package models

import (
	"strings"
	"time"
)

// Scope types a hold may apply to.
const (
	ScopeOrganization = "ORGANIZATION"
	ScopeCustomer     = "CUSTOMER"
	ScopeProvider     = "PROVIDER"
	ScopeTrip         = "TRIP"
	ScopeAssignment   = "ASSIGNMENT"
	ScopeIncident     = "INCIDENT"
	ScopeSupportCase  = "SUPPORT_CASE"
	ScopeInvoice      = "INVOICE"
	ScopeExport       = "EXPORT"
	ScopeAuditRange   = "AUDIT_RANGE"
)

// ValidScopeTypes is the allowlist enforced both here and by the migration's
// CHECK constraint; a caller-supplied scope type not on this list is
// rejected before ever reaching SQL.
var ValidScopeTypes = map[string]bool{
	ScopeOrganization: true, ScopeCustomer: true, ScopeProvider: true,
	ScopeTrip: true, ScopeAssignment: true, ScopeIncident: true,
	ScopeSupportCase: true, ScopeInvoice: true, ScopeExport: true,
	ScopeAuditRange: true,
}

// Hold statuses.
const (
	StatusActive   = "ACTIVE"
	StatusReleased = "RELEASED"
)

// Hold is one legal hold record.
type Hold struct {
	ID                 string
	OrganizationID     string
	ScopeType          string
	ScopeReference     string
	Reason             string
	Status             string
	CreatedByActorType string
	CreatedByActorID   string
	ApprovedByActorID  string
	StepUpReference    string
	ReleasedByActorID  string
	ReleaseReason      string
	CreatedAt          time.Time
	ReleasedAt         *time.Time
	Version            int64
}

// Active reports whether the hold currently blocks deletion/anonymization.
func (h Hold) Active() bool { return h.Status == StatusActive }

// NormalizeScopeType uppercases and trims a caller-supplied scope type.
func NormalizeScopeType(raw string) string {
	return strings.ToUpper(strings.TrimSpace(raw))
}
