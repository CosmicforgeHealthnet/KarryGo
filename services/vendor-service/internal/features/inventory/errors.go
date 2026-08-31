package inventory

import "errors"

var (
	ErrInvalidActor      = errors.New("inventory actor is invalid")
	ErrInvalidTenant     = errors.New("inventory tenant is invalid")
	ErrInvalidScope      = errors.New("inventory scope is invalid")
	ErrScopeViolation    = errors.New("inventory scope violation")
	ErrInvalidSource     = errors.New("inventory source reference is invalid")
	ErrFeatureDisabled   = errors.New("inventory feature is disabled")
	ErrWritesDisabled    = errors.New("inventory writes are disabled")
	ErrDependencyMissing = errors.New("inventory dependency is missing")
)
