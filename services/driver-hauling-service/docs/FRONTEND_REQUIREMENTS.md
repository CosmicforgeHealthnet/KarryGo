# Truck Provider frontend requirements

## Section 30 — Commerce hauling metadata

No Truck Provider application code is changed by Section 30. The existing
provider booking endpoints now return an optional `commerce` object for a
Vendor Commerce mission. A later frontend change may render:

- ordered pickup stops and their readiness windows;
- parcel and manifest references plus carton/package counts;
- dimensions, weight, fragile, keep-upright, stack-restricted, hot, chilled,
  and frozen handling requirements;
- expected loading duration;
- required Hauling vehicle class;
- delivery location and instructions already authorized by the booking stage.

The provider UI must not display or infer Vendor price, margin, commission,
settlement, customer Commerce payment method, or customer Commerce charge.
Those values are intentionally absent from the Commerce operational projection.

The authoritative Hauling handover status is `AVAILABLE`. A credential is
returned only by its issue/resend command: `code` is the short numeric OTP and
`qr_token` is an independent opaque bearer token. Generic credential GETs are
status-only and cannot redisplay either secret. Either credential consumes the
same challenge, and clients must treat it as short-lived and single-use.
