# Local Development

## Prerequisites

- Go 1.26+ (workspace `go.work` already includes this service).
- A dedicated PostgreSQL database for this service (e.g. a
  `postgres:16-alpine` container on its own port, matching the commented
  per-service pattern in `infra/docker-compose.yml`). **Never point this
  service at another service's database.**
- Redis is optional.

## Setup

```bash
cd services/fleet-partner-service
cp .env.example .env    # edit: database URL, secrets, metrics token
```

## Run

```bash
go run ./cmd            # HTTP API on :8113; MIGRATION=true applies migrations
go run ./cmd/worker     # optional dedicated worker process
```

With `FLEET_PARTNER_WORKER_ENABLED=true` (default) the API process runs the
job worker and outbox publisher in-process; disable it when running the
dedicated worker binary.

## Smoke checks

```bash
curl -s http://localhost:8113/health
curl -s http://localhost:8113/ready
curl -s -H "Authorization: Bearer $FLEET_PARTNER_METRICS_AUTH_TOKEN" http://localhost:8113/metrics | head
```

Corporate and internal meta endpoints require a valid access token /
service signature — see [api-conventions.md](api-conventions.md).

## Tests

```bash
go test ./...                          # unit, HTTP, contract, architecture, migration-content
go vet ./... && gofmt -l .             # hygiene
```

### PostgreSQL integration suite (mandatory, never skipped in CI)

The suite needs a **disposable database that belongs only to this service**.
Never point it at another service's database, staging, production, or
anything holding real data — the suite itself refuses names that are not
test-specific (`test` required; `prod`/`production`/`staging`/`live`
rejected).

Create a throwaway container:

```bash
docker run -d --name karrygo-fleet-partner-test-db -e POSTGRES_USER=fleet_partner_test -e POSTGRES_PASSWORD=CHOOSE_A_LOCAL_PASSWORD -e POSTGRES_DB=karrygo_fleet_partner_test -p 5446:5432 postgres:16-alpine
```

Point the suite at it and run every test:

```bash
FLEET_PARTNER_TEST_DATABASE_URL='postgres://fleet_partner_test:CHOOSE_A_LOCAL_PASSWORD@localhost:5446/karrygo_fleet_partner_test?sslmode=disable' go test -count=1 -v ./internal/integration/...
```

The suite applies migrations from zero, truncates the foundation tables
before each test so runs are repeatable, and exercises rollback/reapply.
Without the variable the suite **skips** (local convenience only) — CI must
always set it, and `scripts/ci.sh` fails the build if any integration test
reports `SKIP`.

Dispose of the database when finished:

```bash
docker rm -f karrygo-fleet-partner-test-db
```

### Full CI job locally

`scripts/ci.sh` runs the whole gate — gofmt, vet, build, both binaries, unit
and architecture tests, then the integration suite. If
`FLEET_PARTNER_TEST_DATABASE_URL` is unset it starts and removes its own
throwaway container; if Docker is also unavailable it **fails** rather than
skipping.

```bash
bash services/fleet-partner-service/scripts/ci.sh
```

### HTTP smoke checks against a running instance

```bash
SMOKE_BASE_URL=http://localhost:8113 SMOKE_METRICS_TOKEN=... SMOKE_ACCESS_TOKEN_SECRET=... SMOKE_SERVICE_NAME=admin-backoffice-service SMOKE_SERVICE_SECRET=... go run ./scripts/smoke
```

It checks `/health`, `/ready`, `/metrics` (unauthenticated rejected,
authorized accepted), and both meta routes with and without credentials.

Race checks (requires cgo/gcc):

```bash
CGO_ENABLED=1 go test -race ./internal/jobs ./internal/events ./internal/observability
```

## Builds

```bash
go build ./...                          # everything
go build -o bin/fleet-partner-service ./cmd
go build -o bin/fleet-partner-worker ./cmd/worker
```
