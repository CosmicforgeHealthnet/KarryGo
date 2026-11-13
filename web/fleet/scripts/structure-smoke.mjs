// Browser smoke test for the organization structure, provider administration
// and workforce template/enrolment screens (spec Sections 7-11, 17, 46).
//
// Runs the production build (`npm run build` first) and answers the API from
// test fixtures through Playwright request interception, so it needs no
// backend. It checks each screen renders what the API returned, writes send
// the version the user saw (If-Match) and an idempotency key where the API
// requires one, and a refused write shows the server's message and request
// reference. The fixtures exist only here; the app never ships sample data.
//
//   node scripts/structure-smoke.mjs
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { chromium } from "@playwright/test";

const port = 3419;
const origin = `http://127.0.0.1:${port}`;
const now = new Date().toISOString();

const branch = {
  id: "brn_1", branchId: "brn_1", organizationId: "org_1", code: "LAG-IKJ", name: "Ikeja hub", type: "HUB",
  address: "1 Allen Avenue", latitude: 6.6, longitude: 3.35, operatingHours: "Mon-Sat 07:00-19:00",
  timezone: "Africa/Lagos", status: "ACTIVE", version: 3, createdAt: now, updatedAt: now, archivedAt: null,
};
const department = {
  id: "dep_1", departmentId: "dep_1", organizationId: "org_1", branchId: "brn_1", parentId: null, costCentreId: null,
  code: "OPS", name: "Operations", status: "ACTIVE", version: 1, createdAt: now, updatedAt: now, archivedAt: null,
};
const permissions = [
  { code: "branches.read", category: "branches", description: "View branches", isActive: true },
  { code: "dispatch.manage", category: "dispatch", description: "Manage dispatch", isActive: true },
];
const customRole = {
  id: "rol_custom", organizationId: "org_1", code: "DISPATCH_LEAD", name: "Dispatch lead", description: "Leads dispatch",
  isSystem: false, status: "ACTIVE", permissions: ["dispatch.manage"], version: 2, createdAt: now, updatedAt: now,
};
const staffMember = {
  id: "stf_1", fullName: "Ada Obi", jobTitle: "Dispatcher", email: "ada@example.com", phone: "+2348000000000",
  status: "ACTIVE", isOwner: false, roles: [{ id: "rol_custom", code: "DISPATCH_LEAD", name: "Dispatch lead" }],
  branchIds: ["brn_1"], version: 4, invitedAt: now, activatedAt: now, suspendedAt: null, createdAt: now,
};
const binding = {
  id: "rb_1", organizationUserId: "stf_1", roleId: "rol_custom", scopeType: "BRANCH", scopeId: "brn_1",
  effectiveFrom: now, expiresAt: null, grantedBy: "owner", version: 1, createdAt: now,
};
const membership = {
  membershipId: "mem_1", serviceCategory: "DISPATCH", providerService: "driver-dispatch-delivery-service",
  providerId: "prv_1", onboardingSource: "ORGANIZATION_SPONSORED", relationshipType: "EMPLOYEE",
  homeBranchId: "brn_1", status: "SUSPENDED", joinedAt: now, version: 7,
};
const template = {
  id: "sht_1", organizationId: "org_1", branchId: "brn_1", name: "Morning dispatch", role: "Rider",
  timezone: "Africa/Lagos", weekdays: [1, 2, 3, 4, 5], startLocalTime: "08:00", endLocalTime: "17:00",
  requiredCapacity: 3, breakRules: {}, status: "ACTIVE", version: 1, createdAt: now, updatedAt: now,
};
const course = {
  id: "crs_1", organizationId: "org_1", code: "SAFE-1", title: "Safe riding", providerReference: "",
  requiredRoles: [], requiredServices: [], validityPeriodSeconds: null, passingScore: 70, status: "ACTIVE",
  version: 1, createdAt: now, updatedAt: now,
};
const enrollment = {
  id: "tre_1", organizationId: "org_1", courseId: "crs_1", courseVersion: 1, membershipId: "mem_1", status: "ASSIGNED",
  assignedBy: "stf_1", assignedAt: now, dueAt: null, completedAt: null, score: null, completionId: null, version: 1,
  createdAt: now, updatedAt: now,
};

const ok = (data) => ({ status: 200, body: { data, meta: { requestId: "req-ok" } } });
const refused = (status, code, message) => ({
  status,
  body: { error: { code, message, retryable: false, details: [], requestId: "req-smoke-1" } },
});

// Every write the screens send, for assertions.
const writes = [];

function answer(method, path) {
  const routes = [
    ["GET", /^\/organizations\/current$/, () => ok({ organizationId: "org_1", legalName: "Smoke Logistics Ltd", tradingName: "Smoke", status: "ACTIVE", registrationNumber: "RC1", organizationType: "LIMITED", countryCode: "NG", timezone: "Africa/Lagos", defaultCurrency: "NGN", version: 1 })],
    ["GET", /^\/branches$/, () => ok([branch])],
    ["POST", /^\/branches$/, () => refused(409, "branch_code_taken", "A branch with this code already exists.")],
    ["PATCH", /^\/branches\/brn_1$/, () => ok({ ...branch, version: 4 })],
    ["GET", /^\/departments/, () => ok([department])],
    ["GET", /^\/organization-settings$/, () => ok({ organizationId: "org_1", assignmentTimeoutSeconds: 120, operatingMode: "MANAGED_FLEET", approvalThresholds: { expense: 5000000 }, providerVisibility: "BRANCH", financialVisibilityPolicy: { jobGross: false, providerShare: true }, version: 5, updatedAt: now })],
    ["PATCH", /^\/organization-settings$/, () => ok({ organizationId: "org_1", assignmentTimeoutSeconds: 120, operatingMode: "MANAGED_FLEET", approvalThresholds: { expense: 5000000 }, providerVisibility: "BRANCH", financialVisibilityPolicy: {}, version: 6, updatedAt: now })],
    ["GET", /^\/roles$/, () => ok([customRole])],
    ["GET", /^\/roles\/rol_custom$/, () => ok(customRole)],
    ["PATCH", /^\/roles\/rol_custom$/, () => refused(409, "role_is_system_managed", "System roles are managed by the platform.")],
    ["GET", /^\/permissions$/, () => ok(permissions)],
    ["GET", /^\/staff$/, () => ok([staffMember])],
    ["GET", /^\/staff\/invitations$/, () => ok([])],
    ["GET", /^\/staff\/stf_1$/, () => ok(staffMember)],
    ["GET", /^\/staff\/stf_1\/role-bindings$/, () => ok([binding])],
    ["PUT", /^\/staff\/stf_1\/roles$/, () => refused(403, "permission_escalation_forbidden", "You cannot change your own roles.")],
    ["GET", /^\/providers$/, () => ok([membership])],
    ["GET", /^\/providers\/mem_1$/, () => ok(membership)],
    ["POST", /^\/providers\/mem_1\/reactivate$/, () => ok({ membershipId: "mem_1", status: "ACTIVE" })],
    ["GET", /^\/providers\/mem_1\/capabilities$/, () => ok([{ id: "cap_1", serviceType: "DISPATCH", vehicleClasses: ["MOTORCYCLE"], operatingZones: ["LAGOS_MAINLAND"], qualifications: ["Defensive riding"], validFrom: now, validUntil: null, status: "ACTIVE" }])],
    ["GET", /^\/providers\/mem_1\/policies$/, () => ok({ membershipId: "mem_1", assignmentMode: "OFFER", providerDeclineRequired: true, visibilityPolicy: "ORGANIZATION", shiftRestrictions: {}, zoneRestrictions: {}, workloadRestrictions: {}, version: 1, updatedAt: now })],
    ["GET", /^\/providers\/mem_1\/declines$/, () => ok([{ assignmentId: "asg_9", workOrigin: "MARKETPLACE", reasonCode: "UNSAFE_VEHICLE", safetyRelated: true, declinedAt: now }])],
    ["GET", /^\/shift-templates/, () => ok([template])],
    ["POST", /^\/shift-templates\/sht_1\/generate$/, () => ok({ templateId: "sht_1", created: [], alreadyExist: ["2026-03-02"], skippedDays: ["2026-03-29"] })],
    ["GET", /^\/shifts/, () => ok([])],
    ["GET", /^\/training\/courses$/, () => ok([course])],
    ["GET", /^\/training\/enrollments/, () => ok([enrollment])],
  ];
  for (const [m, re, fn] of routes) if (m === method && re.test(path)) return fn();
  return method === "GET" ? ok([]) : ok({});
}

const server = spawn(
  process.execPath,
  ["node_modules/next/dist/bin/next", "start", "--hostname", "127.0.0.1", "--port", String(port)],
  { env: { ...process.env, FLEET_API_BASE_URL: "http://127.0.0.1:9" }, stdio: ["ignore", "pipe", "pipe"] },
);
let output = "";
server.stdout.on("data", (b) => (output += b));
server.stderr.on("data", (b) => (output += b));
let browser;
const pageErrors = [];
try {
  for (let i = 0; i < 120; i++) {
    try {
      if ((await fetch(origin + "/sign-in")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 500));
  }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  await context.addCookies([{ name: "karrygo_has_session", value: "1", url: origin }]);
  const expires = new Date(Date.now() + 3600_000).toISOString();
  await context.addInitScript((state) => {
    localStorage.setItem("auth-storage", JSON.stringify({ state, version: 0 }));
  }, {
    accessToken: "smoke-token", sessionId: "ses_1", accessExpiresAt: expires, refreshExpiresAt: expires,
    identityId: "idn_1", organizations: [], selectedOrganization: null,
  });
  await context.route(/^https?:\/\/(?!127\.0\.0\.1)/, (route) => route.abort());
  await context.route(`${origin}/api/**`, async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(/^\/api/, "");
    if (request.method() !== "GET") {
      writes.push({ method: request.method(), path, headers: request.headers(), body: request.postData() });
    }
    const { status, body } = answer(request.method(), path);
    await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
  });
  const page = await context.newPage();
  page.setDefaultTimeout(30000);
  page.on("pageerror", (e) => pageErrors.push(String(e)));
  const write = (method, path) => writes.find((w) => w.method === method && w.path === path);

  // Branches: list, refused create shows the reference, edit sends If-Match.
  await page.goto(origin + "/settings/branches");
  await page.getByText("Ikeja hub").waitFor();
  await page.getByRole("button", { name: "Add branch" }).click();
  await page.getByPlaceholder("LAG-IKJ").fill("LAG-IKJ");
  await page.getByPlaceholder("Ikeja hub").fill("Duplicate");
  await page.getByRole("button", { name: "Save branch" }).click();
  await page.getByText("A branch with this code already exists.").waitFor();
  await page.getByText("Reference: branch_code_taken · req-smoke-1").waitFor();
  await page.getByTitle("Edit").first().click();
  await page.getByRole("button", { name: "Save branch" }).click();
  await page.getByText("Branch Ikeja hub updated.").waitFor();
  assert.equal(write("PATCH", "/branches/brn_1")?.headers["if-match"], "3");

  // Departments.
  await page.goto(origin + "/settings/departments");
  await page.getByText("Operations").first().waitFor();

  // Organization settings: read-only facts and the version on save.
  await page.goto(origin + "/settings/policies");
  await page.getByText("Managed Fleet").waitFor();
  await page.getByRole("button", { name: "Save settings" }).click();
  await page.getByText("Organization settings saved.").waitFor();
  const settingsWrite = write("PATCH", "/organization-settings");
  assert.equal(settingsWrite?.headers["if-match"], "5");
  assert.deepEqual(JSON.parse(settingsWrite.body).approvalThresholds, { expense: 5000000 });

  // Role editor: real role, a refused save shows the server's message.
  await page.goto(origin + "/settings/roles/rol_custom");
  await page.getByRole("heading", { name: "Dispatch lead" }).waitFor();
  await page.getByRole("button", { name: "Edit Role" }).click();
  await page.getByRole("switch", { name: "branches.read" }).click();
  await page.getByRole("button", { name: "Save Changes" }).click();
  await page.getByText("System roles are managed by the platform.").waitFor();
  const roleWrite = write("PATCH", "/roles/rol_custom");
  assert.equal(roleWrite?.headers["if-match"], "2");
  assert.deepEqual(JSON.parse(roleWrite.body).permissions, ["branches.read", "dispatch.manage"]);

  // Role assignments come from the staff role bindings.
  await page.goto(origin + "/settings/roles/assignments");
  await page.getByText("Ada Obi").waitFor();

  // Staff detail: scoped role bindings; changing your own roles is refused.
  await page.goto(origin + "/settings/staff/stf_1");
  await page.getByRole("heading", { name: "Roles and scope" }).waitFor();
  await page.getByRole("button", { name: "Edit roles" }).click();
  await page.getByRole("button", { name: "Save roles" }).click();
  await page.getByText("You cannot change your own roles.").waitFor();
  const rolesWrite = write("PUT", "/staff/stf_1/roles");
  assert.deepEqual(JSON.parse(rolesWrite.body).bindings[0], { roleId: "rol_custom", scopeType: "BRANCH", scopeId: "brn_1", expiresAt: null });

  // Provider overview: reactivate a suspended provider with its version.
  await page.goto(origin + "/providers/mem_1");
  await page.getByRole("button", { name: "Reactivate" }).click();
  await page.getByText("Provider reactivated.").waitFor();
  assert.equal(write("POST", "/providers/mem_1/reactivate")?.headers["if-match"], "7");

  // Work setup and declines.
  await page.goto(origin + "/providers/mem_1/work-setup");
  await page.getByText("The provider can always decline company work").waitFor();
  await page.getByText("MOTORCYCLE").waitFor();
  await page.goto(origin + "/providers/mem_1/declines");
  await page.getByText("Safety flag").waitFor();

  // Shift templates: generation reports DST-skipped days.
  await page.goto(origin + "/workforce/shifts");
  await page.getByText("Morning dispatch").waitFor();
  await page.getByRole("button", { name: "Generate shifts" }).first().click();
  await page.getByRole("button", { name: "Generate shifts" }).first().click();
  await page.getByText(/daylight-saving change\): 2026-03-29/).waitFor();
  assert.ok(write("POST", "/shift-templates/sht_1/generate")?.headers["idempotency-key"], "generate sends an idempotency key");

  // Training enrolments.
  await page.goto(origin + "/workforce/training");
  await page.getByRole("heading", { name: "Enrolments" }).waitFor();
  await page.getByRole("button", { name: "Record result" }).waitFor();

  assert.deepEqual(pageErrors, [], "no page errors");
  console.log(JSON.stringify({ status: "passed", writes: writes.length }));
} catch (e) {
  console.error(output.slice(-3000));
  console.error(pageErrors);
  throw e;
} finally {
  await browser?.close();
  server.kill("SIGTERM");
}
