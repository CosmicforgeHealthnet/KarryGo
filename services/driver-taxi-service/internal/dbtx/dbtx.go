// Package dbtx defines the one Executor interface every repository that
// needs to run inside a caller-supplied transaction depends on.
//
// Two different features' repositories sharing literally the same pgx.Tx
// requires their own Executor types to be structurally identical AND the
// same underlying named type -- Go's interface-satisfaction rule checks
// exact type identity of method parameters, not mere structural
// compatibility, so two independently-declared (even if identical-shaped)
// interfaces do NOT let one repository's method satisfy another package's
// interface expecting its own Executor. A single shared type, referenced by
// alias from each feature package, is what actually makes this work.
//
// The motivating case: provider_availability's
// OperationalStateRepository.Transition documents that its executor
// parameter exists so "a future caller (the booking lifecycle in phase B)"
// can pass a pgx.Tx and have the operational-state change commit atomically
// with the booking transition that triggered it (addendum requirement).
// Phase B's lifecycle orchestrator (internal/features/booking/usecases)
// is that caller.
package dbtx

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Executor is satisfied by both *pgxpool.Pool and pgx.Tx.
type Executor interface {
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}
