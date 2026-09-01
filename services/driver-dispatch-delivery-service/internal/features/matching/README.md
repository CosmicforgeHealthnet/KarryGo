# Matching Feature

This folder is reserved for this feature's backend implementation.

Legacy broadcast matching is implemented by `internal/features/request`.
Additive policy selection and the durable queue are implemented by
`internal/features/assignment`; multi-vendor route and manifest planning is in
`internal/features/missionplanning`. These packages do not create another
Booking, Request, Trip, availability, Fleet, or Wallet authority.

```text
http/          HTTP routes, handlers, and request/response DTOs
usecases/      Business workflows
models/        Feature-owned data structures
repositories/  Database or Redis persistence
clients/       Outbound integrations
```
