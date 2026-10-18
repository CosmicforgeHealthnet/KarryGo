# Karryam public API Gateway

Routing-only Go service. It forwards customer/provider/vendor/fleet traffic to
canonical owners, with correlation, timeouts and per-upstream circuit breakers.
No partner database, API key custody, billing, OTP, webhooks or Admin authority.

The Partner Web App backend is services/partner-web-service. Public
/api/v1/partner/v1/* requests are forwarded to its private partner_web origin
with original URLs, authentication, request bodies and status codes. Partner
CORS belongs solely to the partner service (avoid duplicate response headers).

This is a cutover-gated migration. Do not deploy the new binary until the
partner service, internal callbacks and existing routes are certified.
See docs/migrations/partner-web-gateway-split.md.

GATEWAY_UPSTREAM_PARTNER_WEB_URL is required. Never place partner secrets,
database URLs or billing keys in the public gateway environment.
