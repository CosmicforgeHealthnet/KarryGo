import { test } from "node:test";
import assert from "node:assert/strict";
import {
  belongsInSetup,
  canOperate,
  isLifecycleRefusal,
  organizationAccess,
  organizationDestination,
} from "./organizationDestination.ts";

test("an identity without a company creates one", () => {
  assert.equal(organizationDestination(null), "/setup/create-company");
});

test("companies still onboarding continue setup", () => {
  for (const status of ["DRAFT", "PENDING_REVIEW", "CONTACT_VERIFIED"]) {
    assert.equal(organizationDestination(status), "/setup", status);
    assert.equal(organizationAccess(status), "onboarding", status);
  }
});

test("PENDING_REVIEW is the legacy pre-submission draft, not a submitted company", () => {
  assert.equal(organizationAccess("PENDING_REVIEW"), "onboarding");
  assert.equal(belongsInSetup("PENDING_REVIEW"), true);
});

test("changes requested returns to the correction flow", () => {
  assert.equal(organizationAccess("CHANGES_REQUIRED"), "corrections");
  assert.equal(organizationDestination("CHANGES_REQUIRED"), "/setup");
});

test("submitted and under-review companies enter the dashboard, restricted", () => {
  for (const status of ["BUSINESS_SUBMITTED", "UNDER_REVIEW"]) {
    assert.equal(organizationDestination(status), "/", status);
    assert.equal(organizationAccess(status), "pending", status);
    assert.equal(canOperate(status), false, status);
    assert.equal(belongsInSetup(status), false, status);
  }
});

test("only an active company operates", () => {
  assert.equal(organizationDestination("ACTIVE"), "/");
  assert.equal(canOperate("ACTIVE"), true);
  assert.equal(canOperate("active"), true, "status case is normalised");
});

test("suspended, closed and archived companies stay restricted", () => {
  for (const status of ["SUSPENDED", "CLOSED", "ARCHIVED"]) {
    assert.equal(organizationAccess(status), "restricted", status);
    assert.equal(organizationDestination(status), "/", status);
    assert.equal(canOperate(status), false, status);
  }
});

test("unknown or missing statuses fail safe: no setup loop, no operations", () => {
  for (const status of ["APPROVED", "REJECTED", "SOMETHING_NEW", "", undefined]) {
    assert.equal(organizationAccess(status), "unknown", String(status));
    assert.equal(canOperate(status), false, String(status));
    assert.equal(belongsInSetup(status), false, String(status));
  }
});

test("lifecycle refusals are recognised by code, not by any 403", () => {
  const refusal = (code) => ({ response: { status: 403, data: { error: { code } } } });
  assert.equal(isLifecycleRefusal(refusal("organization_verification_pending")), true);
  assert.equal(isLifecycleRefusal(refusal("organization_changes_required")), true);
  assert.equal(isLifecycleRefusal(refusal("forbidden")), false);
  assert.equal(isLifecycleRefusal({ response: { status: 409, data: { error: { code: "organization_verification_pending" } } } }), false);
  assert.equal(isLifecycleRefusal(new Error("network")), false);
});
