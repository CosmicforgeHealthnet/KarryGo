// Package usecases implements organization onboarding, lifecycle,
// verification, wallet provisioning, activation and closure.
package usecases

import (
	"fmt"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/organizations/models"
)

// Transition names the allowed organization lifecycle moves.
//
//	DRAFT -> CONTACT_VERIFIED -> BUSINESS_SUBMITTED -> UNDER_REVIEW -> ACTIVE
//	UNDER_REVIEW -> CHANGES_REQUIRED -> BUSINESS_SUBMITTED
//	ACTIVE <-> SUSPENDED
//	ACTIVE|SUSPENDED -> CLOSED
//
// There is deliberately no immediate-delete state: closure is a workflow.
var allowedTransitions = map[string]map[string]bool{
	models.OrganizationStatusDraft: {
		models.OrganizationStatusContactVerified: true,
		models.OrganizationStatusArchived:        true,
	},
	models.OrganizationStatusPendingReview: {
		// Sections 3–7 created organizations as PENDING_REVIEW; treat it as a
		// synonym of DRAFT for onboarding so existing rows remain valid.
		models.OrganizationStatusContactVerified:   true,
		models.OrganizationStatusBusinessSubmitted: true,
		models.OrganizationStatusArchived:          true,
	},
	models.OrganizationStatusContactVerified: {
		models.OrganizationStatusBusinessSubmitted: true,
		models.OrganizationStatusArchived:          true,
	},
	models.OrganizationStatusBusinessSubmitted: {
		models.OrganizationStatusUnderReview:     true,
		models.OrganizationStatusChangesRequired: true,
	},
	models.OrganizationStatusUnderReview: {
		models.OrganizationStatusActive:          true,
		models.OrganizationStatusChangesRequired: true,
	},
	models.OrganizationStatusChangesRequired: {
		models.OrganizationStatusBusinessSubmitted: true,
	},
	models.OrganizationStatusActive: {
		models.OrganizationStatusSuspended: true,
		models.OrganizationStatusClosed:    true,
	},
	models.OrganizationStatusSuspended: {
		models.OrganizationStatusActive: true,
		models.OrganizationStatusClosed: true,
	},
	models.OrganizationStatusClosed:   {},
	models.OrganizationStatusArchived: {},
}

// ErrInvalidTransition is returned for a move the state machine forbids.
type ErrInvalidTransition struct {
	From string
	To   string
}

func (e ErrInvalidTransition) Error() string {
	return fmt.Sprintf("organizations: transition %s -> %s is not allowed", e.From, e.To)
}

// CanTransition reports whether a lifecycle move is allowed.
func CanTransition(from string, to string) bool {
	next, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

// RequireTransition validates a lifecycle move.
//
// Every status change must go through a named use case that calls this,
// locks the aggregate, bumps the version, appends status history, writes
// audit and emits an outbox event. No handler updates status directly.
func RequireTransition(from string, to string) error {
	if from == to {
		return ErrInvalidTransition{From: from, To: to}
	}
	if !CanTransition(from, to) {
		return ErrInvalidTransition{From: from, To: to}
	}
	return nil
}

// IsOperational reports whether an organization may run corporate operations.
func IsOperational(status string) bool {
	return status == models.OrganizationStatusActive
}

// AcceptsOnboardingEdits reports whether onboarding fields may still change.
func AcceptsOnboardingEdits(status string) bool {
	switch status {
	case models.OrganizationStatusDraft,
		models.OrganizationStatusPendingReview,
		models.OrganizationStatusContactVerified,
		models.OrganizationStatusChangesRequired:
		return true
	default:
		return false
	}
}
