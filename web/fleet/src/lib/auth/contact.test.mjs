import { test } from "node:test";
import assert from "node:assert/strict";
import { normalizePhone, emailIssue, maskEmail } from "./contact.ts";

test("Nigerian national formats normalize to one +234 number", () => {
  for (const input of ["08012345678", "0801 234 5678", "(0801) 234-5678", "8012345678", "2348012345678", "+2348012345678", "+234 801 234 5678", "002348012345678", "+2340801 234 5678"]) {
    assert.deepEqual(normalizePhone(input, "NG"), { ok: true, e164: "+2348012345678" }, input);
  }
});

test("country code is never duplicated or invented", () => {
  assert.deepEqual(normalizePhone("2348012345678", "NG"), { ok: true, e164: "+2348012345678" });
  // A bare national number must not become "+801..." (the old bug).
  assert.notEqual(normalizePhone("8012345678", "NG").e164, "+8012345678");
});

test("the selected country decides how a national number is read", () => {
  assert.deepEqual(normalizePhone("0244123456", "GH"), { ok: true, e164: "+233244123456" });
  assert.deepEqual(normalizePhone("07911 123456", "GB"), { ok: true, e164: "+447911123456" });
  assert.deepEqual(normalizePhone("(415) 555-2671", "US"), { ok: true, e164: "+14155552671" });
});

test("an international number is used as typed, whatever country is selected", () => {
  assert.deepEqual(normalizePhone("+447911123456", "NG"), { ok: true, e164: "+447911123456" });
  assert.deepEqual(normalizePhone("+33612345678", "NG"), { ok: true, e164: "+33612345678" });
  // A country outside the list still works if it is a plausible E.164 number.
  assert.deepEqual(normalizePhone("+6421234567", "NG"), { ok: true, e164: "+6421234567" });
});

test("invalid and missing phones are reported", () => {
  assert.deepEqual(normalizePhone("", "NG"), { ok: false, issue: "missing" });
  assert.deepEqual(normalizePhone("   ", "NG"), { ok: false, issue: "missing" });
  for (const bad of ["0801234", "080123456789", "+12", "+234801", "abc", "0801-ABC-5678", "+0123456789"]) {
    assert.equal(normalizePhone(bad, "NG").ok, false, bad);
  }
});

test("email validation matches the server", () => {
  assert.equal(emailIssue(""), "missing");
  assert.equal(emailIssue("  Owner@Example.COM "), null);
  for (const bad of ["no-at-sign", "a@b", "a@@b.com", "spaces in@example.com"]) assert.equal(emailIssue(bad), "invalid", bad);
});

test("maskEmail hides the local part", () => {
  assert.equal(maskEmail("Owner@Example.com"), "o***@example.com");
});
