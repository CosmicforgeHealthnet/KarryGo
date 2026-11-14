import { test } from "node:test";
import assert from "node:assert/strict";
import {
  dropStepUpGrant,
  readStepUpGrant,
  saveStepUpGrant,
  stepUpRefusal,
  stepUpVerifyQuery,
} from "./stepUp.ts";

function memoryStorage() {
  const values = new Map();
  return {
    getItem: (key) => (values.has(key) ? values.get(key) : null),
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
  };
}

function refusal(status, code, details) {
  return { response: { status, data: { error: { code, message: "x", details } } } };
}

test("a step-up refusal names the action class to verify for", () => {
  assert.deepEqual(
    stepUpRefusal(refusal(403, "step_up_required", [{ field: "actionClass", message: "iam.access.manage" }])),
    { actionClass: "iam.access.manage", code: "step_up_required" },
  );
  assert.deepEqual(
    stepUpRefusal(refusal(403, "step_up_expired", [{ field: "actionClass", message: "payroll.finalize" }])),
    { actionClass: "payroll.finalize", code: "step_up_expired" },
  );
});

test("other errors are not step-up refusals", () => {
  assert.equal(stepUpRefusal(refusal(403, "forbidden", [])), null);
  assert.equal(stepUpRefusal(refusal(401, "step_up_required", [{ field: "actionClass", message: "x" }])), null);
  assert.equal(stepUpRefusal(refusal(403, "step_up_required", [])), null);
  assert.equal(stepUpRefusal(new Error("network")), null);
  assert.equal(stepUpRefusal(undefined), null);
});

test("grants are kept per action class until they expire", () => {
  const storage = memoryStorage();
  const now = Date.parse("2026-11-14T10:00:00Z");
  saveStepUpGrant(storage, "iam.access.manage", "chl_1", "2026-11-14T10:05:00Z");
  assert.equal(readStepUpGrant(storage, "iam.access.manage", now), "chl_1");
  assert.equal(readStepUpGrant(storage, "payroll.finalize", now), null);
  assert.equal(readStepUpGrant(storage, "iam.access.manage", now + 6 * 60_000), null);
  dropStepUpGrant(storage, "iam.access.manage");
  assert.equal(readStepUpGrant(storage, "iam.access.manage", now), null);
});

test("unusable grants are ignored", () => {
  const storage = memoryStorage();
  saveStepUpGrant(storage, "iam.access.manage", "chl_1", "not-a-date");
  assert.equal(readStepUpGrant(storage, "iam.access.manage", 0), null);
  storage.setItem("fleet:step-up-grants", "{broken");
  assert.equal(readStepUpGrant(storage, "iam.access.manage", 0), null);
});

test("the verification query marks the step-up intent and class", () => {
  const query = new URLSearchParams(stepUpVerifyQuery("organization.closure", { challengeId: "chl_9", maskedDestination: "o***@example.com", expiresInSeconds: 600 }));
  assert.equal(query.get("intent"), "step-up");
  assert.equal(query.get("actionClass"), "organization.closure");
  assert.equal(query.get("challengeId"), "chl_9");
  assert.equal(query.get("expiresIn"), "600");
});
