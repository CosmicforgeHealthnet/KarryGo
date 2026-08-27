// Package finance_projection is the reserved feature boundary for vendor
// operational finance READ MODELS ONLY (payable summaries, settlement
// status, reconciliation views built from payment-wallet-service's
// authoritative allocations). This package never records money movement or
// competes with payment-wallet-service's ledger — see CLAUDE.md,
// "Payment/Wallet Rules" and docs/architecture/05_PAYMENT_ALLOCATION_MODEL.md.
// The package is a bearer-authenticated Vendor facade over Wallet's HMAC
// authority. It exposes safe projections and commands, never ledger entries.
package finance_projection
