import { test } from "node:test";
import assert from "node:assert/strict";
import { challengeFailure } from "./challengeErrors.ts";

test("an earlier OTP replaced while its countdown is still running is not expired", () => {
  assert.equal(challengeFailure("otp_challenge_superseded", "unauthorized"), "replaced");
});
test("invalid, expired, already-used and unknown challenge have distinct UI states", () => {
  assert.equal(challengeFailure("otp_invalid", "unauthorized"), "invalid");
  assert.equal(challengeFailure("otp_challenge_expired", "unauthorized"), "expired");
  assert.equal(challengeFailure("otp_challenge_consumed", "unauthorized"), "consumed");
  assert.equal(challengeFailure("otp_challenge_not_found", "unauthorized"), "missing");
  assert.equal(challengeFailure("otp_attempt_limit_exceeded", "unauthorized"), "locked");
});
test("transport failures and account outcomes are not mislabeled as expired", () => {
  assert.equal(challengeFailure(undefined, "network"), "network");
  assert.equal(challengeFailure(undefined, "unavailable"), "access_denied");
  assert.equal(challengeFailure("identity_pair_conflict", "unauthorized"), "contact_mismatch");
  assert.equal(challengeFailure("identity_suspended", "configuration"), "account_unavailable");
});
