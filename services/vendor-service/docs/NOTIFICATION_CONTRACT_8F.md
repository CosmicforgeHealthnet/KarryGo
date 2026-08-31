# Section 8F Vendor to Notification contract

- Caller: `vendor-service`; callee: `notification-service`.
- Authentication: service-auth HMAC signature v2 using `VENDOR_NOTIFICATION_SECRET`.
- Route: `POST {VENDOR_NOTIFICATION_URL}/api/v1/notifications/send`.
- Production channels: `email`, `push`, and `in_app`. SMS is not supported by the current Notification contract and Vendor suppresses it as `CHANNEL_UNAVAILABLE`; there is **no production SMS send**.
- Request identity: `vendor-service:crm.campaign.delivery:{deliveryRef}`. It is minted once with the delivery and reused unchanged on every timeout/retry.
- Recipient: `{type:"customer", id:"<opaque customer-service reference>"}`. Vendor sends no email address, telephone number, device token, or other destination PII.
- Safe references: `campaign_ref`, `run_ref`, `delivery_ref`, and random `attribution_ref` are included in `data`.
- Content: an approved `template_key`, or bounded plain-text title/body; no HTML or arbitrary template language.
- Idempotency: Notification Service has a durable unique `idempotency_key`; an ambiguous timeout retries the same key, so it remains one logical notification.
- Response: `message_id` and Notification's current `status`. Vendor records a successful response as `HANDED_OFF`; this does not mean delivered.
- Delivery events: Vendor accepts future HMAC v2 events only from `notification-service` at `POST /internal/vendor/v1/crm/campaign-delivery-events`. Requests require a single-use signed nonce. `event_id` is durably deduplicated, conflicting reuse is rejected, source versions are monotonic, and late events cannot regress `DELIVERED`.
- Producer status: the current Notification Service exposes no callback/event producer. Vendor's receiver is ready, but production delivery receipts remain dependent on that producer contract.

