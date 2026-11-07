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



### Source-specific read policies

The resource inventory has more read permissions than the 128-rule bound on one signed policy. Keep that bound and use a separate signed policy per source when enabling the complete inventory.

For each configured `ADMIN_<SOURCE_PREFIX>_RESOURCE_READ_ENABLED=true`, optionally set both `ADMIN_<SOURCE_PREFIX>_ACCESS_POLICY_FILE` and `ADMIN_<SOURCE_PREFIX>_ACCESS_POLICY_KEYS_FILE`. The prefix is the existing fixed source prefix, for example `CUSTOMER`, `HAULING`, `PAYMENT_WALLET`, or `FLEET_PARTNER`. Each policy needs an appropriate source owner/contract rule and should use its own policy ID so revisions can advance independently. The Bridge and source must receive the same signed document and checksum.

With neither override set, existing deployments continue using `ADMIN_ACCESS_POLICY_FILE` and `ADMIN_ACCESS_POLICY_KEYS_FILE`. A partial override, bad signature, wrong owner or unreadable explicit policy fails configuration; it never falls back to the shared policy. An explicitly blocked policy remains blocked. Accepted policy revisions are pinned in the existing Bridge database before their clients are configured.

This only selects signed read policy. It creates no source context or staff grant, changes no MFA rule, enables no command, and does not remove any deployment hold.
