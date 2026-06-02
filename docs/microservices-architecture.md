# KarryGo Microservice Architecture

This structure keeps the monorepo but separates backend ownership into distinct
services. Booking and matching are not standalone services. They live inside the
operational service that owns the business flow.

## Core Business Services

| Service | Owns |
|---|---|
| `customer-service` | Customer auth entry, customer profile, saved locations, preferences, customer-facing request history views |
| `taxi-service` | Taxi provider profiles, car records, taxi ride bookings, taxi matching, taxi trip lifecycle |
| `dispatch-delivery-service` | Dispatch rider profiles, bike records, package delivery bookings, rider matching, proof of delivery |
| `hauling-service` | Truck provider profiles, truck records, haulage bookings, truck matching, cargo workflow |

## Shared Platform Services

| Service | Owns |
|---|---|
| `api-gateway` | Client entry point, request routing, edge health checks |
| `payment-wallet-service` | Wallets, payments, refunds, provider earnings, withdrawals, fleet settlement |
| `notification-service` | Push, SMS, email, in-app notifications, retry handling |
| `support-dispute-service` | Complaints, disputes, evidence, issue resolution |
| `verification-compliance-service` | ID checks, licenses, vehicle documents, provider verification |
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

## Booking And Matching Boundary

Booking and matching stay inside the relevant operational service:

| Business Area | Booking Lives In | Matching Lives In |
|---|---|---|
| Taxi rides | `taxi-service` | `taxi-service` |
| Package delivery | `dispatch-delivery-service` | `dispatch-delivery-service` |
| Truck hauling | `hauling-service` | `hauling-service` |

## Shared Standards

Every service should use:

- `shared/go/apperrors` for the KarryGo error model.
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
