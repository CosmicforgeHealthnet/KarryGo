# Operational evidence, delivery exceptions, returns and provider transfer

Dispatch extends its existing Commerce mission; it does not create a parallel
delivery state machine. The canonical chain remains mission → ordered pickup
stop → Vendor-issued Parcel → normal Dispatch trip.

## Evidence and manifest policy

Pickup completion must confirm the stop's exact Parcel references, aggregate
package count and manifest reference/version. Successful handover is the
manifest freeze boundary. Stop and Parcel manifest bindings cannot then be
edited, while later damage or discrepancy is recorded as append-only evidence.

Evidence types are closed to `PICKUP`, `DELIVERY`, `DAMAGE`, `DISCREPANCY`,
`FAILED_ATTEMPT`, `RETURN` and `PROVIDER_TRANSFER`. Pickup, delivery, return and
transfer evidence can only be produced by their dedicated authoritative
workflow. Evidence metadata contains UTC-aware capture time, optional bounded
coordinates and opaque Media File Service references. It contains no media
bytes, base64 payloads or local paths. Evidence reads are private HMAC-v2
service routes; provider routes never expose another mission's evidence.

## Exceptions and returns

The provider exception command accepts only the documented reason/recovery
matrix. Package mismatch and prohibited/restricted items fail closed before
custody. An alternative receiver is unavailable until a separate authoritative
authorization policy is connected; a provider-supplied flag cannot authorize
one. Any additional customer fee requires an existing commercial approval
reference, and this service never charges it.

Prepared Food permits return-to-vendor, Food disposal policy or support review.
Shop/retail may use Shop inspection. Prepared Food can never enter the generic
Shop inspection/restock path. Return start and completion append distinct
evidence, preserve the Parcel identity and custody history, and do not decide a
refund or provider earning.

## Breakdown and provider transfer

Vehicle breakdown, provider illness and safety incident enter
`REPLACEMENT_REQUIRED` and freeze normal trip proof/completion. Matching reuses
the existing verified provider/vehicle/capacity policy. Before custody,
replacement uses the normal mission recovery. After custody, the replacement
is only a candidate until transfer completes.

Transfer is an exact two-party handover. The challenge binds the canonical
mission, transfer reference, old provider, replacement provider, all currently
held Parcels, package count, immutable manifest fingerprint and coordinates.
The replacement provider confirms the same scope and supplies an opaque media
reference. Only then does one transaction close old custody, open replacement
custody, change the current carrier, invalidate the old trip challenge and
regenerate the receiver challenge. Raw codes are never copied.

Wallet remains the sole settlement authority. The old trip cannot complete
while replacement/transfer is pending, so it cannot receive a duplicate full
completion earning; the provider that completes the canonical mission follows
the existing settlement path.
