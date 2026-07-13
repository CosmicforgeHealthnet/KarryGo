# API Gateway

Routes requests from customer, provider, and admin apps to the correct Cosmicforge Logistics
service. This should become the only public backend entry point for client apps.


## Internal Callbacks

`/api/v1/internal/callbacks/*` is the additive HTTP callback path for dispatch and payment-wallet lifecycle events. These routes require `X-Internal-Service-Key` and do not accept partner API keys or dashboard session JWTs.

Dispatch callback examples:

```bash
curl -X POST "$BASE_URL/api/v1/internal/callbacks/trip-started" \
  -H "Content-Type: application/json" \
  -H "X-Internal-Service-Key: $PARTNER_INTERNAL_SERVICE_KEY" \
  -d '{"event":"trip.started","booking_id":"dispatch-booking-id","provider_id":"11111111-2222-3333-4444-555555555555","rider_name":"Taiwo Adeleke","rider_phone":"+2348033445566","rider_rating":4.85,"vehicle_color":"Red","vehicle_model":"Honda CB125","plate_number":"LAG-234-XY","occurred_at":"2026-07-05T10:08:00Z"}'

curl -X POST "$BASE_URL/api/v1/internal/callbacks/provider-arrived" \
  -H "Content-Type: application/json" \
  -H "X-Internal-Service-Key: $PARTNER_INTERNAL_SERVICE_KEY" \
  -d '{"event":"trip.provider_arrived","booking_id":"dispatch-booking-id","provider_id":"11111111-2222-3333-4444-555555555555","occurred_at":"2026-07-05T10:06:00Z"}'
```

After `trip-started`, `GET /api/v1/partner/v1/deliveries/:id` returns the saved rider and vehicle snapshot plus a `tracking_url` when `rider_id` is present.

Payment-wallet charge result example:

```bash
curl -X POST "$BASE_URL/api/v1/internal/callbacks/charge-result" \
  -H "Content-Type: application/json" \
  -H "X-Internal-Service-Key: $PARTNER_INTERNAL_SERVICE_KEY" \
  -d '{"source_reference":"invoice-id","status":"success","reference":"paystack_ref","amount_kobo":500000}'
```

Redis Pub/Sub subscribers remain enabled during the transition. Do not remove them until dispatch HTTP callbacks, customer-service callbacks, api-gateway callbacks, payment-wallet job/refund/charge endpoints, and the end-to-end lifecycle have all been confirmed live without duplicate partner webhooks.