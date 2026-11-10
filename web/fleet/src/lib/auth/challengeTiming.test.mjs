import { test } from "node:test";
import assert from "node:assert/strict";
import { otpDeadline, remainingSeconds } from "./challengeTiming.ts";

test("backend absolute deadline remains correct after mobile browser backgrounding", () => {
  const initial = Date.parse("2026-11-10T18:00:00Z");
  const deadline = otpDeadline("2026-11-10T18:10:00Z", 600, initial);
  assert.equal(remainingSeconds(deadline, initial + 68_000), 532);
  assert.equal(remainingSeconds(deadline, initial + 11 * 60_000), 0);
});
test("older backend still has safe relative fallback", () => {
  const now = 1000000;
  assert.equal(otpDeadline(undefined, 60, now), now + 60000);
  assert.equal(otpDeadline("not-a-timestamp", 60, now), now + 60000);
});
