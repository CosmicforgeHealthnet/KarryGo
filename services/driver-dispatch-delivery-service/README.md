# driver-dispatch-delivery-service

KarryGo microservice for dispatch delivery rider flows. Dispatch rider auth is owned inside this service at `internal/features/auth`; no separate auth microservice is used.

## Service

| Property | Value |
| --- | --- |
| Service name | `driver-dispatch-delivery-service` |
| Module | `cosmicforge/logistics/services/dispatch-delivery-service` |
| Port | `8103` |
| Database | `dispatch_delivery_service` |
| Local Postgres port | `5435` |
| Local Redis port | `6382` |
| Redis prefix | `dispatch_rider_auth:` |

## Scope

| Area | Location |
| --- | --- |
| Auth | `internal/features/auth` |
| Rider profile | `internal/features/riders` |
| Bike management | `internal/features/bikes` |
| Delivery lifecycle | `internal/features/deliveries` |
| Matching | `internal/features/matching` |
| Proof of delivery | `internal/features/proof_of_delivery` |

## Endpoints

| Method | Path | Auth |
| --- | --- | --- |
| `GET` | `/health` | None |
| `GET` | `/ready` | None |
| `POST` | `/api/v1/auth/start` | None |
| `POST` | `/api/v1/auth/verify` | None |
| `POST` | `/api/v1/auth/refresh` | None |
| `POST` | `/api/v1/auth/logout` | Bearer access token |

Successful responses use:

```json
{
  "success": true,
  "data": {}
}
```

Errors use:

```json
{
  "success": false,
  "error": {
    "code": "validation_failed",
    "message": "Check your details.",
    "request_id": "request-id",
    "fields": []
  }
}
```

## Environment

Copy `.env.example` when running locally and provide strong local-only secrets.

```env
APP_ENV=development
SERVICE_NAME=driver-dispatch-delivery-service
GIN_MODE=debug
HTTP_ADDR=:8103
DISPATCH_DELIVERY_DATABASE_URL=postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5435/dispatch_delivery_service?sslmode=disable
DISPATCH_DELIVERY_REDIS_ADDR=localhost:6382
DISPATCH_DELIVERY_REDIS_PASSWORD=
DISPATCH_DELIVERY_REDIS_DB=0
DISPATCH_RIDER_ACCESS_TOKEN_SECRET=replace-with-strong-random-secret-min-32-chars
DISPATCH_RIDER_REFRESH_TOKEN_SECRET=replace-with-strong-random-secret-min-32-chars
DISPATCH_RIDER_OTP_SECRET=replace-with-strong-random-secret-min-32-chars
DISPATCH_RIDER_JWT_ACCESS_TTL_MINUTES=15
DISPATCH_RIDER_JWT_REFRESH_TTL_DAYS=30
DISPATCH_RIDER_OTP_TTL_MINUTES=10
DISPATCH_RIDER_OTP_MAX_ATTEMPTS=3
DISPATCH_RIDER_OTP_LOCKOUT_MINUTES=30
DISPATCH_RIDER_OTP_RATE_LIMIT_MAX=3
DISPATCH_RIDER_OTP_RATE_LIMIT_WINDOW_MINUTES=10
DISPATCH_RIDER_DEBUG_OTP=true
```

The service reads `HTTP_ADDR`, `DISPATCH_DELIVERY_DATABASE_URL`, and `DISPATCH_DELIVERY_REDIS_ADDR`. Docker Compose sets container DNS values; local shell runs usually use `localhost` with ports `5435` and `6382`.

## Docker

From the repository root:

```powershell
docker compose -f infra/docker-compose.yml config --quiet
docker compose -f infra/docker-compose.yml up --build -d driver-dispatch-delivery-service
docker compose -f infra/docker-compose.yml logs -f driver-dispatch-delivery-service
docker compose -f infra/docker-compose.yml down
```

Compose starts:

- `driver-dispatch-delivery-service`
- `driver-dispatch-delivery-postgres`
- `driver-dispatch-delivery-redis`

## Local Development

From the repository root:

```powershell
go work sync
```

From the service folder:

```powershell
cd services/driver-dispatch-delivery-service
go mod tidy
gofmt -w .
go vet ./...
go test ./...
go test -v ./...
```

This workstation may not have a local Go toolchain. In that case, run the same commands through the repo's Docker Go image workflow.

## Migrations

Forward migrations:

- `migrations/001_dispatch_rider_auth.sql`
- `migrations/002_dispatch_rider_auth_alter.sql`

Rollback migrations:

- `migrations/002_dispatch_rider_auth_alter_down.sql`
- `migrations/001_dispatch_rider_auth_down.sql`

Apply manually with `psql` from the repository root:

```powershell
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5435/dispatch_delivery_service?sslmode=disable" -f services/driver-dispatch-delivery-service/migrations/001_dispatch_rider_auth.sql
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5435/dispatch_delivery_service?sslmode=disable" -f services/driver-dispatch-delivery-service/migrations/002_dispatch_rider_auth_alter.sql
```

Rollback in reverse order:

```powershell
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5435/dispatch_delivery_service?sslmode=disable" -f services/driver-dispatch-delivery-service/migrations/002_dispatch_rider_auth_alter_down.sql
psql "postgres://cosmicforge_logistics:cosmicforge_logistics@localhost:5435/dispatch_delivery_service?sslmode=disable" -f services/driver-dispatch-delivery-service/migrations/001_dispatch_rider_auth_down.sql
```

Docker Compose mounts the forward migrations into the service Postgres container for first-time database initialization.

## Auth Flow

Start OTP:

```powershell
$body = @{
  phone_number = "+2348012345678"
} | ConvertTo-Json

Invoke-RestMethod `
  -Uri "http://localhost:8103/api/v1/auth/start" `
  -Method POST `
  -ContentType "application/json" `
  -Body $body
```

Read the OTP from development logs only:

```powershell
docker compose -f infra/docker-compose.yml logs driver-dispatch-delivery-service --tail=100
```

Verify OTP:

```powershell
$verifyBody = @{
  phone_number = "+2348012345678"
  otp_code = "PASTE_OTP_HERE"
  device_id = "device-abc-123"
  device_type = "android"
} | ConvertTo-Json

$verifyResponse = Invoke-RestMethod `
  -Uri "http://localhost:8103/api/v1/auth/verify" `
  -Method POST `
  -ContentType "application/json" `
  -Body $verifyBody

$accessToken = $verifyResponse.data.access_token
$refreshToken = $verifyResponse.data.refresh_token
```

Refresh:

```powershell
$refreshBody = @{
  refresh_token = $refreshToken
} | ConvertTo-Json

$refreshResponse = Invoke-RestMethod `
  -Uri "http://localhost:8103/api/v1/auth/refresh" `
  -Method POST `
  -ContentType "application/json" `
  -Body $refreshBody
```

Logout:

```powershell
$newAccessToken = $refreshResponse.data.access_token
$newRefreshToken = $refreshResponse.data.refresh_token

Invoke-RestMethod `
  -Uri "http://localhost:8103/api/v1/auth/logout" `
  -Method POST `
  -Headers @{ Authorization = "Bearer $newAccessToken" } `
  -ContentType "application/json" `
  -Body "{}"
```

After refresh rotation, the old refresh token returns `401 unauthorized`. After logout, the latest refresh token also returns `401 unauthorized`.

## Health Checks

```powershell
curl.exe http://localhost:8103/health
curl.exe http://localhost:8103/ready
```

Expected health response:

```json
{
  "success": true,
  "data": {
    "service": "driver-dispatch-delivery-service",
    "status": "ok"
  }
}
```

Expected ready response:

```json
{
  "success": true,
  "data": {
    "service": "driver-dispatch-delivery-service",
    "status": "ready"
  }
}
```

## Security Notes

- OTP codes are never returned in HTTP responses.
- OTP codes are stored only as HMAC-SHA256 hashes.
- OTP logging is allowed only when `DISPATCH_RIDER_DEBUG_OTP=true` in development.
- Refresh tokens are stored only as SHA-256 hashes.
- Access, refresh, and OTP secrets are required environment variables.
- SQL uses pgx parameterized queries.
- This service does not configure service-level CORS.
- Do not log access tokens, refresh tokens, or plain OTP codes outside debug OTP development logs.

## Known TODOs

- JWT access tokens do not currently include `jti`; access-token blacklist support is deferred until `jti` exists.
- `provider.profile.suspended` subscriber is present as a placeholder and still needs its payload handling implementation.
