package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
)

// Resource types the approval framework can version-bind.
//
// These are the subjects a controlled operation acts on. Anything not listed
// cannot be version-bound, and the framework says so rather than quietly
// skipping the check — an approval that claims to be bound to a version it
// never read would be worse than one that admits it is unbound.
const (
	ResourceOrganization       = "ORGANIZATION"
	ResourceProviderMembership = "PROVIDER_MEMBERSHIP"
	ResourceAssignment         = "ASSIGNMENT"
	ResourceFinanceOperation   = "FINANCE_OPERATION"
	ResourceEligibilityPolicy  = "ELIGIBILITY_POLICY"
)

// ErrSubjectVersionUnsupported means this resource type carries no version.
var ErrSubjectVersionUnsupported = errors.New("approvals: subject version not supported")

// SubjectVersions reads the current version of a controlled subject.
type SubjectVersions struct {
	db database.Querier
}

// NewSubjectVersions wires the reader.
func NewSubjectVersions(db database.Querier) *SubjectVersions {
	return &SubjectVersions{db: db}
}

// SubjectVersion returns the subject's current version inside the tenant.
//
// A subject that does not exist in this tenant returns version 0 with no
// error. That is deliberate: an approval bound to expected version N will then
// mismatch and invalidate, which is the correct outcome for a subject that was
// removed — while returning an error here would surface a different, less
// specific failure to the caller.
func (s *SubjectVersions) SubjectVersion(ctx context.Context, q database.Querier,
	organizationID string, resourceType string, resourceID string) (int64, error) {
	if q == nil {
		q = s.db
	}

	var statement string
	switch resourceType {
	case ResourceOrganization:
		statement = `SELECT version FROM fleet_partner_organizations
			WHERE id = $2 AND id = $1`
	case ResourceProviderMembership:
		statement = `SELECT version FROM fleet_partner_organization_provider_memberships
			WHERE organization_id = $1 AND id = $2`
	case ResourceAssignment:
		statement = `SELECT version FROM fleet_partner_provider_assignments
			WHERE organization_id = $1 AND id = $2`
	case ResourceFinanceOperation:
		statement = `SELECT version FROM fleet_partner_finance_operations
			WHERE organization_id = $1 AND id = $2`
	default:
		return 0, ErrSubjectVersionUnsupported
	}

	var version int64
	err := q.QueryRow(ctx, statement, organizationID, resourceID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("approvals: subject version: %w", err)
	}
	return version, nil
}
