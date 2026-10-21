# Karryam Partner Web service

Backend for the existing web/partner-web application and external partners.
Preserves /api/v1/partner/v1 authentication, onboarding, API keys, deliveries,
webhooks, billing, and /api/v1/internal/callbacks/* for direct internal callers.

Relocated from the legacy services/api-gateway module, keeping its original
database schema migrations 001-010 and durable partner data/identities.
Partner service is private on karrygo-net; only the public partner API prefix
is proxied through the new routing-only API Gateway.

For migration compatibility, existing API_GATEWAY_* environment variable names
and the wallet HMAC caller name api-gateway remain unchanged. Renaming this
outbound identity requires prior enrollment with the wallet owner.

Never start old gateway and new partner service workers simultaneously.
See docs/migrations/partner-web-gateway-split.md for cutover and rollback.
