# 06 — Commerce Fulfilment Model

## Aggregates

```text
Parent Commerce Order        (one customer checkout; internal/features/orders)
  └── Vendor Order            (one per vendor in that checkout; independently
       │                       accepted/rejected/substituted by its vendor)
       └── Fulfilment Group    (how a vendor order's items are grouped for
            │                   handover — a single-vendor order is typically
            │                   one group; a large order may split)
            └── Fulfilment Unit  (one execution request: platform delivery via
                 │                dispatch/taxi/hauling, OR vendor-owned delivery
                 │                via internal/features/delivery)
                 └── Parcel(s)    (the physical handover unit(s) carried by that
                                   fulfilment unit's execution)
```

`internal/features/fulfilment` owns the Fulfilment Group/Unit/Parcel/Plan concepts and the planning decision of which execution authority serves each unit. It does not own the Parent/Vendor Order concepts (`internal/features/orders` does) and does not own execution itself (the relevant `internal/integrations/*` package or `internal/features/delivery` does).

## Planning responsibilities (per the specification, restated precisely)

Vendor Service, via `internal/features/fulfilment`:

- determines delivery requirements per fulfilment unit (weight, vehicle class, special handling);
- determines the requested vehicle class (motorcycle → dispatch, car → taxi's future GOODS_DELIVERY, van/truck → hauling, or none → vendor-owned);
- calculates a fulfilment plan (which units, which authority, what sequencing for a multi-vendor checkout);
- requests a quote from the chosen authority (via `internal/integrations/dispatch|taxi|hauling`, using each service's own pricing — never a re-derived fare);
- requests creation of a mission once the vendor side is ready;
- retains only an opaque mission reference from the owning service — never a mirrored copy of that service's trip state;
- consumes lifecycle events (via the owning service's existing notification/event surface, or polling its read-back endpoint) to advance the fulfilment unit's own status;
- replans when required — a rejected/unmatched mission, a vendor-side cancellation, or a failed pickup triggers re-planning (a new quote/mission attempt, or falling back to vendor-owned delivery if enabled and eligible), gated by `COMMERCE_FULFILMENT_WORKERS_ENABLED` for the background replanning worker.

## What Vendor Service explicitly does not do here

It never directly assigns a platform provider, never writes to a driver service's `providers`/`bookings`/`trips` tables, and never invents its own competing matching algorithm for platform-executed missions. For vendor-owned delivery (`internal/features/delivery`), Vendor Service *is* the execution authority — but only for that vendor's own riders, never for platform marketplace providers.

## Custody handover dependency

Every fulfilment unit executed by a platform authority (dispatch/hauling, and taxi's future GOODS_DELIVERY) that involves a vendor pickup or a customer delivery handover depends on `internal/integrations/handover` for custody confirmation. That capability is `PENDING_EXTERNAL_DEPENDENCY` platform-wide (see [07](07_EXISTING_OTP_QR_INTEGRATION.md)) — fulfilment planning and mission creation are not blocked by this, but final activation of confirmed-handover fulfilment units is, until the owning logistics service builds the real credential.

## Multi-vendor pickup and receiver handover rules (carried forward verbatim from the specification, since these are binding product requirements, not architecture choices this document is free to alter)

- Each vendor pickup handover is verified independently.
- One pickup challenge applies to one vendor handover session, not one per individual parcel — every parcel in that pickup stays bound to the pickup manifest.
- One provider delivering one mission to one receiver may use one final receiver handover credential for that exact mission/manifest.
- A split-provider mission requires a separate receiver handover credential per provider.
- Provider reassignment invalidates the previous provider's credential; a receiver change invalidates the previous receiver's credential; a resend invalidates the prior credential.

## What this phase does not do

No fulfilment aggregate, planner, or worker is implemented. This document fixes the model so `internal/features/fulfilment` and its integration clients are built against an agreed shape once implementation begins.
