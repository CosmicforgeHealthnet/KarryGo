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

## Section 17C status (implemented)

Planning is implemented. Where reality differed from this document's original
assumptions, the difference is recorded here rather than left for a reader to
discover:

- **The authoritative quote did not exist.** This document assumed each
  logistics service already had a quote to request. Neither did: both exposed a
  public stateless fare estimate with no reference, no expiry and no
  serviceability. Section 17C added a real internal quote contract
  (`shared/go/commercequote`) and an implementation in dispatch and hauling.
  See `docs/PLATFORM_SERVICE_CHANGE_REGISTER.md`.

- **Consolidation is co-location-only.** "Which units, which authority, what
  sequencing" is real, but no logistics authority models a multi-pickup
  mission, so a mission covering two distant pickups would carry a fee for a
  route nobody can drive. Groups consolidate only when their pickups are
  genuinely co-located, share a class and an urgency, carry no separation
  conflict, and fit the class combined.

- **The quoted mission and the execution mission are already separate
  concepts.** A quoted mission is the commercial unit - one authority quote,
  one fee, recording exactly which vendor groups it covers. Nothing in the
  schema assumes one mission is one trip, so 17F may split it into several
  execution missions without destroying the commercial commitment.

- **Custody handover is no longer pending.** This document recorded
  `internal/integrations/handover` as PENDING_EXTERNAL_DEPENDENCY platform-wide.
  That is stale: both dispatch and hauling implement a real two-stage
  pickup/delivery credential (see the 2026-08-05 platform work). Fulfilment
  planning does not depend on it either way - it plans and quotes, it does not
  execute.

- **The fee beneficiary is derived, never stored as vendor-editable data.** The
  "single boolean decision recorded on the fulfilment unit" this document asked
  for is `commerce_fulfilment_missions.fee_beneficiary`, and the database
  refuses a PLATFORM_LOGISTICS mission that names the vendor.

## What the original architecture phase did not do

No fulfilment aggregate, planner, or worker was implemented then. This document
fixed the model so `internal/features/fulfilment` and its integration clients
were built against an agreed shape. Section 17C has since implemented planning;
mission creation, lifecycle and replanning remain later work.
