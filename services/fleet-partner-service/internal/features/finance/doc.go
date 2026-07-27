// Package finance is the reserved feature boundary for finance projections and settlement references (no local financial ledger, no pricing formulas).
//
// Not implemented in the foundation work package (Sections 3-7). Future
// implementations must follow the feature layout (models, repositories,
// usecases, http, clients), depend only on interfaces of other features,
// enforce tenant scoping in repositories, and respect the platform
// source-of-truth boundaries in docs/platform-boundaries.md.
package finance
