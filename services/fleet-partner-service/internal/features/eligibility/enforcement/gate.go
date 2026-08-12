// Package enforcement is the narrow port domain use cases depend on to gate a
// sensitive operation.
//
// It exists so that the organizations, providers, marketplace and finance
// features can enforce eligibility without importing the eligibility use-case
// package's whole dependency graph, and so the gate can be substituted in a
// test without substituting the rule.
//
// The port has no "skip" and no "warn" mode. Every method either returns nil,
// meaning a current unexpired ELIGIBLE decision authorizes the operation, or
// returns an error. A nil Gate is not permission to proceed — see Require.
package enforcement

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/eligibility/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// Subject describes what is being gated.
type Subject struct {
	OrganizationID string
	OperationType  string
	SubjectType    string
	SubjectID      string

	BranchID            string
	MembershipID        string
	ServiceCategory     string
	ExpectedVehicleType string
	AssetReference      string
	ExcludeAssignmentID string

	ActorType     string
	ActorID       string
	CorrelationID string
	CausationID   string
}

// Gate authorizes a sensitive operation from a durable eligibility decision.
type Gate interface {
	// Authorize returns the authorizing decision, or an error naming why the
	// operation is blocked.
	Authorize(ctx context.Context, subject Subject) (*models.Decision, error)
}

// Require enforces a gate, treating a missing gate as a refusal.
//
// This is the function every domain calls. The nil check is the point of it: a
// service wired without an eligibility gate has no way to establish that an
// operation is permitted, and a control that disappears when its dependency is
// missing is not a control. Deployments therefore fail loudly at the first
// gated operation rather than quietly running ungated.
func Require(ctx context.Context, gate Gate, subject Subject) (*models.Decision, error) {
	if gate == nil {
		return nil, errorcodes.Unavailable(errorcodes.EligibilityUnknown,
			"Eligibility could not be confirmed right now. Please try again.")
	}
	return gate.Authorize(ctx, subject)
}

// Company builds the subject for an organization-level gate.
func Company(organizationID string, operationType string) Subject {
	return Subject{
		OrganizationID: organizationID,
		OperationType:  operationType,
		SubjectType:    models.SubjectCompany,
		SubjectID:      organizationID,
	}
}

// Provider builds the subject for a managed-provider gate.
func Provider(organizationID string, membershipID string, operationType string) Subject {
	return Subject{
		OrganizationID: organizationID,
		OperationType:  operationType,
		SubjectType:    models.SubjectProvider,
		SubjectID:      membershipID,
		MembershipID:   membershipID,
	}
}

// Vehicle builds the subject for a vehicle gate.
//
// membershipID is required rather than optional: the provider a vehicle must
// belong to is read from fleet's own membership record, never taken from the
// request, so a caller cannot name whichever provider makes ownership pass.
func Vehicle(organizationID string, vehicleID string, membershipID string,
	operationType string) Subject {
	return Subject{
		OrganizationID: organizationID,
		OperationType:  operationType,
		SubjectType:    models.SubjectVehicle,
		SubjectID:      vehicleID,
		MembershipID:   membershipID,
		AssetReference: vehicleID,
	}
}
