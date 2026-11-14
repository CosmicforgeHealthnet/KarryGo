// Package tracking owns Section 16 tracking projections. Fleet consumes
// versioned authoritative Dispatch/Hauling events and never becomes an
// independent GPS or provider-location authority.
//
// Layout: models (projection, event and freshness rule), repositories (SQL),
// usecases (idempotent ingestion, gap reconciliation, scoped reads and the
// retention/freshness sweep) and http (/tracking reads and the internal
// POST /tracking/events ingress).
package tracking
