package models

import "errors"

// Sentinel errors shared by the repositories and use cases.
var (
	ErrNotFound               = errors.New("fleetpartner: not found")
	ErrIdempotencyConflict    = errors.New("fleetpartner: idempotency conflict")
	ErrDuplicateCompletedWork = errors.New("fleetpartner: duplicate completed work")
)
