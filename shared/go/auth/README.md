# Shared Auth Package

Shared auth helpers live here: OTP generation/verification, refresh-token
hashing, token signing/verification, role claims, and reusable Gin bearer-token
middleware.

Auth remains owned by each user-facing service. This package only prevents the
customer, taxi, dispatch delivery, and hauling services from duplicating security
logic.

Current helpers include:

- `GenerateNumericOTP`, `HashOTP`, and `VerifyOTP`.
- `GenerateOpaqueToken` and `HashRefreshToken`.
- `TokenSigner` for HMAC-signed access tokens with `sub`, `role`, `service`,
  `session_id`, `typ`, `iat`, and `exp` claims.
- `BearerMiddleware` for validating access tokens and enforcing service/role
  boundaries.

Customer-service uses these helpers with `role=customer` and
`service=customer`.
