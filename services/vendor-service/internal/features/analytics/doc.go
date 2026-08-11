// Package analytics is the reserved feature boundary for vendor operational
// reports/projections computed inside vendor-service for the vendor's own
// dashboards (sales, top items, order volume). This is distinct from
// platform-wide analytics-service (internal/integrations/analytics), which
// vendor-service feeds via outbox events rather than replacing. Not
// implemented in this architecture phase.
package analytics
