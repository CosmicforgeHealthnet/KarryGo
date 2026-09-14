package managedsettlement

import (
	"context"
	"strings"
)

// Identity is the stable financial identity of one completed job's settlement.
//
// It exists because a completion is settled from two places that must never
// disagree: the synchronous path that runs when the job completes, and the
// reconciliation worker that picks up whatever that path could not finish. If
// each derived its own identity, the worker would ask Wallet about a job
// Wallet has never heard of under that name, conclude nothing had settled, and
// settle it a second time.
//
// So both paths build the identity from the same two facts, and nothing else:
// which execution service owns the work, and that service's own id for the
// job. Neither changes after completion, which is what makes the identity
// stable across a restart, a retry or a version bump.
type Identity struct {
	SourceService string
	SourceJobID   string
}

// IdentityFor builds the identity, normalising the service name so a caller
// spelling it "taxi" and one spelling it "TAXI" address the same job.
func IdentityFor(sourceService, sourceJobID string) Identity {
	return Identity{
		SourceService: strings.ToUpper(strings.TrimSpace(sourceService)),
		SourceJobID:   strings.TrimSpace(sourceJobID),
	}
}

// Valid reports whether the identity names a job at all.
func (i Identity) Valid() bool { return i.SourceService != "" && i.SourceJobID != "" }

// Key is the identity as a single comparable string, for logs and for tests
// that assert two code paths produced the same one.
func (i Identity) Key() string { return i.SourceService + ":" + i.SourceJobID }

// Lookup asks Wallet about the job this identity names.
func (c Client) Lookup(ctx context.Context, identity Identity) (Outcome, error) {
	return c.ForJob(ctx, identity.SourceService, identity.SourceJobID)
}
