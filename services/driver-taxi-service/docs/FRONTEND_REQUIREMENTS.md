# Taxi Goods future frontend requirements

Section 29 changed backend code only. No Taxi Provider or Customer frontend was
modified.

A later provider implementation must:

- show `GOODS_DELIVERY` approval separately from passenger eligibility and the
  legacy direct-parcel preference;
- provide an independent Goods availability switch without changing passenger
  availability;
- display verified cargo limits and handling capabilities, clearly separating
  provider declarations from operations verification;
- render Goods job cards with pickup/dropoff, load, parcel/manifest references,
  handling, timing, and evidence requirements, but no Vendor financial split;
- display the provider-held pickup OTP/opaque QR for the Vendor/sender to verify;
- accept or scan the receiver-held delivery credential without exposing it in
  advance;
- upload evidence through Media File Service and submit only the returned Media
  reference to Taxi;
- keep passenger trip-start/completion and direct-customer `ride_purpose=parcel`
  screens separate from Taxi Goods custody controls.

All screens must respect the server-side `TAXI_GOODS_DELIVERY_ENABLED` kill
switch and must not expose Taxi Goods when it is disabled.
