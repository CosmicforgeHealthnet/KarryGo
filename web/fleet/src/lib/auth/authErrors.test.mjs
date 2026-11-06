import { test } from "node:test";
import assert from "node:assert/strict";
import { classifyAuthError } from "./authErrors.ts";

const http = (status, error) => ({ response: { status, data: { error } } });

test("untrusted_origin is a configuration problem, not a validation error", () => {
  // The exact body the staging backend returned.
  const r = classifyAuthError(http(403, { code: "untrusted_origin", message: "Trusted browser origin required." }));
  assert.equal(r.kind, "configuration");
  assert.equal(r.code, "untrusted_origin");
  assert.doesNotMatch(r.message, /origin/i, "must not leak security internals");
});

test("validation errors name the field the server blamed", () => {
  const v = (field, message) => http(422, { code: "validation_failed", requestId: "r1", details: [{ field, message }] });
  assert.equal(classifyAuthError(v("phone", "Enter a valid phone number with its country code.")).kind, "invalid_phone");
  assert.equal(classifyAuthError(v("phone", "Phone number is required.")).kind, "missing_phone");
  assert.equal(classifyAuthError(v("email", "Enter a valid email address.")).kind, "invalid_email");
  assert.equal(classifyAuthError(v("email", "Email address is required.")).kind, "missing_email");
  assert.equal(classifyAuthError(v("email", "x")).requestId, "r1");
  assert.equal(classifyAuthError(http(422, { code: "validation_failed" })).kind, "invalid_request");
});

test("a message containing 'required' is not treated as a validation error", () => {
  // The old code matched the word "required" in any message.
  assert.equal(classifyAuthError(http(403, { code: "untrusted_origin", message: "Trusted browser origin required." })).kind, "configuration");
  assert.equal(classifyAuthError(http(500, { code: "internal_error", message: "Configuration required." })).kind, "server_error");
});

test("status-driven kinds", () => {
  assert.equal(classifyAuthError(http(401, { code: "unauthorized" })).kind, "unauthorized");
  assert.equal(classifyAuthError(http(429, { code: "authentication_rate_limited" })).kind, "rate_limited");
  assert.equal(classifyAuthError(http(429, { code: "otp_resend_too_soon" })).kind, "rate_limited");
  assert.equal(classifyAuthError(http(503, { code: "otp_delivery_unavailable" })).kind, "delivery_failed");
  assert.equal(classifyAuthError(http(503, { code: "service_unavailable" })).kind, "unavailable");
  assert.equal(classifyAuthError(http(502, undefined)).kind, "unavailable");
  assert.equal(classifyAuthError(http(500, { code: "internal_error" })).kind, "server_error");
});

test("no response at all is a network error and is retryable", () => {
  const r = classifyAuthError({ code: "ERR_NETWORK", message: "Network Error", request: {} });
  assert.equal(r.kind, "network");
  assert.equal(r.retryable, true);
  assert.equal(classifyAuthError({ code: "ECONNABORTED" }).kind, "network");
});

test("a rejection with no request id in the body falls back to the response header", () => {
  // The origin check answers before the error envelope adds a request id.
  const body = { code: "untrusted_origin", message: "Trusted browser origin required." };
  const plain = classifyAuthError({ response: { status: 403, data: { error: body }, headers: { "x-request-id": "req-123" } } });
  assert.equal(plain.requestId, "req-123");

  // Axios hands back an AxiosHeaders object with a get() accessor.
  const headers = { get: (name) => (name === "x-correlation-id" ? "corr-9" : undefined) };
  const viaGet = classifyAuthError({ response: { status: 403, data: { error: body }, headers } });
  assert.equal(viaGet.requestId, "corr-9");

  const bodyWins = classifyAuthError({
    response: { status: 422, data: { error: { code: "validation_failed", requestId: "body-1" } }, headers: { "x-request-id": "hdr-1" } },
  });
  assert.equal(bodyWins.requestId, "body-1");
});
