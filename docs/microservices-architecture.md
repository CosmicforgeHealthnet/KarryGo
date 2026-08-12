# Cosmicforge Logistics Microservice Architecture

This structure keeps the monorepo but separates backend ownership into distinct
services. Booking and matching are not standalone services. They live inside the
operational service that owns the business flow.

Section 7 contract correction: `verification-compliance-service` and `vendor-service` exchange the exact Vendor VerificationStatus vocabulary `NOT_STARTED`, `IN_PROGRESS`, `PENDING_REVIEW`, `VERIFIED`, `REJECTED`, `EXPIRED`, `SUSPENDED`. The signed callback uses direct-service root path `/internal/vendor/v1/verification-events`; the public `/api/v1/vendor` prefix does not apply to that internal route.

## Core Business Services

| Service | Owns |
|---|---|
| `customer-service` | Customer auth entry, customer profile, saved locations, preferences, customer-facing request history views |
| `taxi-service` | Taxi provider profiles, car records, taxi ride bookings, taxi matching, taxi trip lifecycle |
| `dispatch-delivery-service` | Dispatch rider profiles, bike records, package delivery bookings, rider matching, proof of delivery |
| `hauling-service` | Truck provider profiles, truck records, haulage bookings, truck matching, cargo workflow |
| `vendor-service` | Vendor staff auth/IAM, business/brand/branch graph, onboarding checklist and authoritative verification/bank/media projections |

## Shared Platform Services

| Service | Owns |
|---|---|
| `api-gateway` | Client entry point, request routing, edge health checks |
| `payment-wallet-service` | Wallets, payments, refunds, provider earnings, withdrawals, fleet settlement |
| `notification-service` | Push, email, websocket and in-app notifications; SMS is not currently implemented |
| `support-dispute-service` | Complaints, disputes, evidence, issue resolution |
| `verification-compliance-service` | Formal verification cases/decisions, including source-versioned Vendor KYB/KYC dimensions |
| `media-file-service` | Profile photos, document uploads, proof images, signatures |
| `admin-backoffice-service` | Admin dashboards, moderation, user actions, operational monitoring |
| `analytics-service` | Reports, metrics, revenue dashboards, service performance |

## Auth Boundary

There is no standalone identity service in this layout. Auth entry lives inside
each user-facing service, while common auth logic should be implemented in
`shared/go/auth`.

This keeps customer, taxi, dispatch delivery, and hauling services independent
without duplicating token, OTP, role, and session logic.

For customer auth, `customer-service` owns the customer records, refresh
sessions, and OTP challenges. In local development it uses its own
`customer-postgres` database and `customer-redis` instance, configured through
`CUSTOMER_DATABASE_URL` and `CUSTOMER_REDIS_ADDR`. Other user-facing services
can later follow the same pattern with their own storage and the shared auth
helpers.

## Vendor onboarding authority boundary

`vendor-service` owns onboarding progress, representative metadata, document
references, requirements, activation policy and safe projections. Formal
verification decisions remain in `verification-compliance-service`; full bank
records and resolution remain in `payment-wallet-service`; uploaded bytes and
object safety/deletion truth remain in `media-file-service`. Verification sends
signed, nonce-protected callbacks and Vendor also reconciles by authoritative
client reference. Vendor activation never treats transport success as approval.

## Booking And Matching Boundary

Booking and matching stay inside the relevant operational service:

| Business Area | Booking Lives In | Matching Lives In |
|---|---|---|
| Taxi rides | `taxi-service` | `taxi-service` |
| Package delivery | `dispatch-delivery-service` | `dispatch-delivery-service` |
| Truck hauling | `hauling-service` | `hauling-service` |

## Shared Standards

Every service should use:

- `shared/go/apperrors` for the Cosmicforge Logistics error model.
- `shared/go/httpx` for request IDs, recovery, and error responses.
- `shared/go/events` for event names.
- `shared/go/validation` for shared validation helpers.
- `shared/go/pagination` for list pagination defaults.

All HTTP API errors should keep this response shape:

```json
{
  "success": false,
  "error": {
    "code": "validation_failed",
    "message": "Check your details.",
    "request_id": "req-id",
    "fields": []
  }
}
```

## Local Development

Run the scaffolded services with:

```bash
docker compose -f infra/docker-compose.yml up --build
```

Each service exposes:

- `GET /health`
- `GET /ready`
- `GET <service-api-base>/meta`

Example:

```bash
curl http://localhost:8102/api/v1/taxi/meta
```
