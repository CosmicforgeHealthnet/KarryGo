// Package operators is pricing-service's view of the humans who may change
// prices.
//
// Pricing has no login of its own: administration arrives as a signed request
// from admin-backoffice-service carrying a performed_by identifier. Pricing
// records BOTH the calling service and that person, and checks the person's
// roles here. Until admin-backoffice-service has real staff authentication, the
// separation-of-duties guarantee is only as strong as that caller's claim, which
// is why the caller allowlist for admin routes is kept deliberately short.
package operators

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cosmicforge/logistics/services/pricing-service/internal/database"
)

// Roles. They are separated so that the ability to PROPOSE a price change never
// implies the ability to APPROVE one.
const (
	RoleEditor          = "EDITOR"           // draft, clone, submit, simulate
	RoleApprover        = "APPROVER"         // approve or reject a policy version
	RoleIndexOperator   = "INDEX_OPERATOR"   // upload cost-data batches
	RoleIndexApprover   = "INDEX_APPROVER"   // accept a batch or an override
	RoleRolloutApprover = "ROLLOUT_APPROVER" // move a market to central pricing
	RoleViewer          = "VIEWER"           // read quotes and reports
)

// ErrUnknownOperator is returned when performed_by is not a registered, active
// operator. An unknown actor is refused rather than treated as unprivileged, so
// a typo cannot silently become an anonymous change.
var ErrUnknownOperator = errors.New("operators: unknown or inactive operator")

// ErrMissingRole is returned when a known operator lacks the required role.
var ErrMissingRole = errors.New("operators: operator lacks the required role")

// Operator is a person who may act on pricing configuration.
type Operator struct {
	ID     string
	Roles  []string
	Active bool
}

// Has reports whether o holds role.
func (o Operator) Has(role string) bool {
	for _, held := range o.Roles {
		if held == role {
			return true
		}
	}
	return false
}

// Store reads operators from Postgres.
type Store struct{ db database.Querier }

// NewStore returns a store over db.
func NewStore(db database.Querier) *Store { return &Store{db: db} }

// Get returns the operator with id.
func (s *Store) Get(ctx context.Context, id string) (Operator, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Operator{}, fmt.Errorf("%w: performed_by is required", ErrUnknownOperator)
	}
	var operator Operator
	err := s.db.QueryRow(ctx,
		`SELECT operator_id, roles, active FROM pricing_operators WHERE operator_id = $1`, id,
	).Scan(&operator.ID, &operator.Roles, &operator.Active)
	if err != nil {
		return Operator{}, fmt.Errorf("%w: %s", ErrUnknownOperator, id)
	}
	if !operator.Active {
		return Operator{}, fmt.Errorf("%w: %s is inactive", ErrUnknownOperator, id)
	}
	return operator, nil
}

// Require returns the operator only if they hold role.
func (s *Store) Require(ctx context.Context, id string, role string) (Operator, error) {
	operator, err := s.Get(ctx, id)
	if err != nil {
		return Operator{}, err
	}
	if !operator.Has(role) {
		return Operator{}, fmt.Errorf("%w: %s needs %s", ErrMissingRole, id, role)
	}
	return operator, nil
}
