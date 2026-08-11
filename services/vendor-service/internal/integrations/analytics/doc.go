// Package analytics is the reserved integration boundary for
// analytics-service.
//
// Current state: analytics-service is a pure scaffold today (no routes, no
// migrations). Planned contract once it exists: vendor-service publishes
// read-model-relevant domain events (order placed/completed, fulfilment
// outcome, vendor revenue snapshots) onto its own outbox
// (internal/events), and analytics-service consumes them into its own
// projections — analytics-service never queries vendor-service's database
// directly, and vendor-service never computes analytics-service's reports
// itself.
package analytics
