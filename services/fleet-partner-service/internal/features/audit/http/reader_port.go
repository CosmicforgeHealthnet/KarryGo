package http

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/audit/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
)

// AuditReader is the read-only audit projection this handler serves
// (FLEET-5.1a: the handler depends on an interface, not the concrete
// repository). repositories.Repository satisfies it.
type AuditReader interface {
	Get(ctx context.Context, organizationID, id string) (repositories.Record, error)
	List(ctx context.Context, filter repositories.ListFilter) ([]repositories.Record, string, error)
	VerifyChain(ctx context.Context, partition string) (audit.VerifyChainResult, error)
}
