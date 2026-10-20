# Operations Feature

`models.go` defines commands, operation records and safe state transitions.
`internal/persistence` stores orchestration metadata; `internal/bridge` enforces
authentication and scoped policy. No domain mutation adapter is registered.
Valid submissions are durably REJECTED, never reported as completed domain work.
UNKNOWN_OUTCOME cannot transition to execution or be automatically repeated.

```text
http/          HTTP routes, handlers, and request/response DTOs
usecases/      Business workflows
models/        Feature-owned data structures
repositories/  Database or Redis persistence
clients/       Outbound integrations
```
