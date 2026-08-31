# Taxi service features

Layout mirrors `services/driver-hauling-service`: each feature owns
`{clients,http,models,repositories,usecases}` where it needs them. Handlers
parse DTOs and call usecases; usecases own the workflow; models own domain
rules; repositories own persistence; clients own outbound adapters.

- `provider_auth` — driver phone/OTP login, refresh sessions, phone change.
- `provider_profile` — driver profile and vehicle records.
- `provider_availability` — online/offline, GPS heartbeat, live position index.
- `booking` — the whole ride lifecycle: fares, matching, payments, reviews.
- `notifications` — thin proxy onto notification-service for both apps.
- `identity` — internal, HMAC-only driver lookup for support-dispute-service.

There is deliberately no `matching/` package: matching is part of the booking
lifecycle and lives in `booking/usecases`. The equivalent directory is an empty
scaffold in both hauling and dispatch.
