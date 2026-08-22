package inventory

import "context"

// Dependencies are the generic Inventory application ports.
type Dependencies struct {
	Resolver InventoryItemResolver

	Evidence InventoryEvidenceRecorder
	Events   InventoryEventPublisher

	Transactions TransactionBoundary

	Clock Clock
	Flags Flags
}

// Service is the Vendor-wide generic Inventory application boundary.
type Service struct {
	resolver InventoryItemResolver

	evidence InventoryEvidenceRecorder
	events   InventoryEventPublisher

	transactions TransactionBoundary

	clock Clock
	flags Flags
}

// NewService creates the generic Inventory application boundary.
func NewService(
	deps Dependencies,
) *Service {
	clock := deps.Clock

	if clock == nil {
		clock = systemClock{}
	}

	return &Service{
		resolver:     deps.Resolver,
		evidence:     deps.Evidence,
		events:       deps.Events,
		transactions: deps.Transactions,
		clock:        clock,
		flags:        deps.Flags,
	}
}

// Clock returns the configured Inventory clock.
func (s *Service) Clock() Clock {
	if s == nil || s.clock == nil {
		return systemClock{}
	}

	return s.clock
}

// Flags returns the configured Inventory feature controls.
func (s *Service) Flags() Flags {
	if s == nil {
		return DefaultFlags()
	}

	return s.flags
}

// RequireRead validates the generic Inventory read boundary.
//
// Existing authentication and permission evaluation happen before this
// application boundary.
func (s *Service) RequireRead(
	_ context.Context,
	actor Actor,
	scope BranchScope,
) error {
	if s == nil || !s.flags.Enabled {
		return ErrFeatureDisabled
	}

	if err := actor.Validate(); err != nil {
		return err
	}

	if err := scope.Validate(); err != nil {
		return err
	}

	if actor.BusinessID != scope.BusinessID {
		return ErrScopeViolation
	}

	return nil
}

// RequireWrite validates the generic Inventory write boundary.
func (s *Service) RequireWrite(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
) error {
	if err := s.RequireRead(
		ctx,
		actor,
		scope,
	); err != nil {
		return err
	}

	if !s.flags.WritesEnabled {
		return ErrWritesDisabled
	}

	return nil
}

// RequireBranch validates one exact target branch.
func (s *Service) RequireBranch(
	ctx context.Context,
	actor Actor,
	scope BranchScope,
	targetBusinessID string,
	targetBranchID string,
	write bool,
) error {
	if write {
		if err := s.RequireWrite(
			ctx,
			actor,
			scope,
		); err != nil {
			return err
		}
	} else {
		if err := s.RequireRead(
			ctx,
			actor,
			scope,
		); err != nil {
			return err
		}
	}

	if !scope.Covers(
		targetBusinessID,
		targetBranchID,
	) {
		return ErrScopeViolation
	}

	return nil
}
