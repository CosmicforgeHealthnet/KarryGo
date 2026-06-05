# Auth Feature

This feature handles customer sign-in and session management for
`customer-service`.

It owns:

- Starting a phone OTP login.
- Verifying an OTP.
- Creating and rotating refresh sessions.
- Returning the authenticated customer through `/me`.
- Logging out by revoking the current refresh session.

It does not own customer profile data. Customer records live in the `profile`
feature and are used here only when login needs to create or load a customer.

## Folder Structure

```text
auth/
  http/          # HTTP routes, handlers, request DTOs, response helper
  usecases/      # auth business workflows
  models/        # auth-specific data structures and pure validation helpers
  repositories/  # Redis and Postgres persistence for auth data
  clients/       # outbound OTP delivery clients
```

## How The Files Connect

```text
cmd/main.go
  builds repositories and clients
  creates AuthService
  registers auth HTTP routes

auth/http
  receives HTTP requests
  validates request JSON shape
  calls auth/usecases
  returns the standard Cosmicforge Logistics response envelope

auth/usecases
  runs the auth workflow
  calls shared phone-number normalization
  calls shared token and OTP helpers
  reads/writes auth repositories
  reads/writes customer profile repository
  calls OTP sender client

auth/repositories
  stores OTP challenges in customer Redis
  stores refresh sessions in customer Postgres

auth/models
  defines OTP challenge, refresh session, refresh token parsing

auth/clients
  sends or logs OTP messages
```

## HTTP Files

| File | Purpose |
|---|---|
| `http/routes.go` | Registers auth endpoints under `/api/v1/customer`, including protected `/me`. |
| `http/handler.go` | Converts HTTP requests into auth usecase inputs and sends responses. |
| `http/dto.go` | Defines request payload structs for start, verify, refresh, and logout. |
| `http/response.go` | Defines the success response helper for `{ "success": true, "data": ... }`. |
| `http/handler_test.go` | Tests the HTTP flow: start OTP, verify OTP, refresh, logout, rate limit, wrong OTP, and `/me`. |

Registered routes:

```text
POST /api/v1/customer/auth/start
POST /api/v1/customer/auth/verify
POST /api/v1/customer/auth/refresh
POST /api/v1/customer/auth/logout
GET  /api/v1/customer/me
```

## Usecase Files

| File | Purpose |
|---|---|
| `usecases/auth_service.go` | Main auth workflow. Coordinates phone normalization, OTP generation, OTP verification, customer creation/loading, refresh session creation, token signing, refresh rotation, logout, and `/me`. |
| `usecases/auth_service_test.go` | Unit tests for auth workflow behavior without real Redis or Postgres. |

Important dependencies used by `AuthService`:

- `profile/repositories.CustomerRepository` for customer creation/loading.
- `auth/repositories.OTPChallengeRepository` for OTP challenge storage.
- `auth/repositories.RefreshSessionRepository` for refresh sessions.
- `auth/clients.OTPSender` for OTP delivery.
- `shared/go/phonenumber` for Nigerian phone-number normalization.
- `shared/go/auth` for OTP hashing, refresh-token hashing, and access-token signing.

## Model Files

| File | Purpose |
|---|---|
| `models/otp_challenge.go` | Defines `OTPChallenge` and verifies challenge ID, expiry, attempt count, and OTP hash. |
| `models/refresh_session.go` | Defines `RefreshSession` and checks whether a session is active. |
| `models/refresh_token.go` | Parses refresh tokens and extracts the session ID. |

Models should stay free of HTTP, Redis, and Postgres code.

## Repository Files

| File | Purpose |
|---|---|
| `repositories/redis_otp_challenge_repository.go` | Stores OTP challenges in customer Redis, rate-limits OTP starts, tracks failed attempts, and deletes used challenges. |
| `repositories/postgres_session_repository.go` | Creates, loads, and revokes refresh sessions in customer Postgres. |

Repositories should contain persistence details only. They should not decide the
auth workflow.

## Client Files

| File | Purpose |
|---|---|
| `clients/otp_sender.go` | Defines the `OTPSender` interface used by the auth usecase. |
| `clients/logging_otp_sender.go` | Local-development OTP sender that logs generated OTPs instead of sending SMS. |

A real SMS or notification integration should implement `OTPSender` and replace
the logging sender during service wiring.

## Main Flows

### Start OTP

```text
HTTP handler
  -> AuthService.StartAuth
  -> shared phonenumber normalizes the phone number
  -> shared auth generates and hashes OTP
  -> RedisOTPChallengeRepository stores challenge and applies rate limit
  -> OTPSender sends/logs OTP
```

### Verify OTP

```text
HTTP handler
  -> AuthService.VerifyAuth
  -> shared phonenumber normalizes the phone number
  -> RedisOTPChallengeRepository loads challenge
  -> auth model verifies challenge and OTP
  -> profile repository creates/loads customer
  -> PostgresRefreshSessionRepository creates refresh session
  -> shared auth signs access token
```

### Refresh Session

```text
HTTP handler
  -> AuthService.Refresh
  -> auth model parses refresh token session ID
  -> PostgresRefreshSessionRepository loads old session
  -> shared auth verifies refresh-token hash
  -> old session is revoked
  -> new session and access token are created
```

### Logout

```text
HTTP handler
  -> AuthService.Logout
  -> auth model parses refresh token session ID
  -> PostgresRefreshSessionRepository loads session
  -> shared auth verifies refresh-token hash
  -> session is revoked
```

### Get Current Customer

```text
Bearer middleware
  -> validates access token
  -> HTTP handler reads token claims
  -> AuthService.Me
  -> profile repository loads customer
```

## Rules For Future Changes

- Put HTTP-only code in `http`.
- Put auth workflow decisions in `usecases`.
- Put auth data structures in `models`.
- Put Redis/Postgres persistence in `repositories`.
- Put outgoing delivery integrations in `clients`.
- Keep reusable cross-service helpers in `shared/go`, not in this feature.
- Do not move customer profile fields into auth; auth can use profile, but profile remains its own feature.