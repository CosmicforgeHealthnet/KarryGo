# Section 55 organization API clients and outbound webhooks

Release 1 adds an organization-owned integration surface independent of corporate-user JWTs, provider JWTs, internal service HMAC, and the third-party Partner API credential model. All three feature flags default to false; stored configuration and delivery history are retained when a feature is disabled.

## Organization API clients

Corporate management routes live under `/fleet-partner/v1/integrations/api-clients`. The credential-authenticated route is `/fleet-partner/integrations/v1/context` and requires exact scope `integration.context.read`. Basic authentication carries the opaque client ID and one-time-issued secret. Organization scope comes only from the credential; `organizationId` in a body/query or `X-Organization-ID` is rejected.

Secrets are generated with `crypto/rand`, HMAC-SHA-256 hashed with the server-side `FLEET_PARTNER_API_CLIENT_HASH_KEY`, returned only at create/rotation, and never included in list responses, idempotency records, audit values, event payloads, metrics, or logs. Rotation may retain the previous hash for a bounded overlap (maximum configured by `FLEET_PARTNER_WEBHOOK_ROTATION_OVERLAP_MAX`); each version is stored separately and revocation destroys current usability immediately. CIDR restrictions use parsed network membership and Gin's explicitly configured trusted-proxy resolution.

## Webhook subscription and signature contract

Webhook secrets are independent random credentials encrypted with AES-256-GCM under `FLEET_PARTNER_WEBHOOK_SECRET_ENCRYPTION_KEY`. Production requires HTTPS. Create/update and delivery/connect validate scheme, URL credentials, port, DNS answers, redirects, and all resolved addresses; loopback, link-local, unspecified, multicast, metadata, and private ranges are rejected unless an operator has configured a narrow enterprise CIDR allowlist. DNS is resolved again by the dial transport to block rebinding.

Each raw request body is signed with HMAC-SHA-256. Headers are:

- `X-KarryGo-Webhook-Signature`: comma-separated `v1.k<keyVersion>=<hex MAC>` components during bounded rotation overlap.
- `X-KarryGo-Webhook-Timestamp`: Unix seconds.
- `X-KarryGo-Event-Id`: stable source event identity.
- `X-KarryGo-Delivery-Id`: stable delivery-attempt identity.
- `X-KarryGo-Webhook-Id`: subscription identity.
- `Idempotency-Key`: stable delivery identity across automatic retries. Consumers should additionally deduplicate business receipt by event ID, including across a controlled manual replay.

Canonical signed input is `v1\n<subscriptionId>\n<deliveryId>\n<eventId>\n<timestamp>\n<SHA-256(raw body)>`. Consumers must parse the advertised key version, reconstruct the exact raw-body digest, compare decoded MAC bytes in constant time, reject timestamps outside `FLEET_PARTNER_WEBHOOK_REPLAY_WINDOW`, and deduplicate by event ID. This is at-least-once delivery, not exactly once.

## Delivery and recovery

A committed domain transaction writes its normal outbox event. A fan-out coordinator reads only registry entries with `PUBLIC_INTEGRATION` visibility, writes a durable delivery and a shared Section 27 `webhook.delivery` job, then advances a durable checkpoint transactionally. The worker revalidates the destination, signs a minimized envelope, propagates correlation IDs, and stores only bounded/redacted response excerpts.

Timeouts, network failures, 408, 429, 502, 503, and 504 are retryable. `Retry-After` is honored with a 24-hour safety bound; the shared job queue applies its existing exponential backoff/jitter otherwise. Other 4xx responses and unsafe endpoints are permanent. Attempts are bounded; the final failure is dead-lettered and repeated failures disable the subscription. Operator replay creates a new, linked attempt without deleting or rewriting the original history.

The delivery payload recursively rejects credential, authorization, password, bank, signed-URL, debug, and internal-exception fields. Metrics use only bounded result/status/error-class labels—never organization, client, subscription, event, or URL labels.
