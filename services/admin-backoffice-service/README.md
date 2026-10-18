# Admin/Backoffice Service

Private Karryam HQ integration boundary. The Admin UI remains in the separate
Centralized Admin repository. This service owns authentication, scoped policy,
replay claims, operation metadata and audit; domain owners retain all decisions.

TLS 1.3 and client certificates are mandatory. Only
`/internal/karryam-admin/v1` is registered, without browser CORS.
`ADMIN_REMOTE_ENABLED` defaults to false. No domain command is executable;
valid submissions are durably REJECTED and return HTTP 503.

See [audit](../../docs/admin-bridge/audit.md),
[runbook](../../docs/admin-bridge/runbook.md), [OpenAPI](docs/openapi/openapi.json),
[capability matrix](docs/openapi/KARRYGO_ADMIN_CAPABILITY_MATRIX.json) and
[signing profile](docs/openapi/signing-profile.md).

Use `--migrate` with the dedicated migration role, then start with the runtime
role. Migration is transactional and serialized. `--probe` checks container-only
loopback readiness on 8116. Neither migrations nor runtime touch domain tables.

Set `ADMIN_BRIDGE_TEST_DATABASE_URL` to a disposable DB whose name includes
`test`, then run `go test ./...`. Tests create/remove isolated schemas. Without
that variable DB tests explicitly skip; they are not certification evidence.
The dedicated CI workflow requires zero test skips.

