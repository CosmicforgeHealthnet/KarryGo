// Staff management end to end against the disposable API: the owner sees
// themself in the staff directory, invites a colleague, resends and cancels
// the invitation, opens their own access detail, and reads the real roles and
// permission catalogue. Screenshots go to .codex-cache/fleet-staff-screens.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { chromium } from "@playwright/test";

const api = process.env.FLEET_AUTH_E2E_API;
assert(api?.startsWith("http://127.0.0.1:"), "Disposable local API required");
const origin = "http://127.0.0.1:3417";
const out = "../../.codex-cache/fleet-staff-screens";
await mkdir(out, { recursive: true });
const server = spawn(
  process.execPath,
  ["node_modules/next/dist/bin/next", process.env.FLEET_PERF_NEXT || "start", "--hostname", "127.0.0.1", "--port", "3417"],
  { env: { ...process.env, FLEET_API_BASE_URL: api }, stdio: ["ignore", "pipe", "pipe"] },
);
let output = "";
server.stdout.on("data", (b) => (output += b));
server.stderr.on("data", (b) => (output += b));
let browser;
const failures = [];
try {
  for (let i = 0; i < 180; i++) {
    try {
      if ((await fetch(origin + "/sign-in")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 1000));
  }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  page.setDefaultTimeout(60000);
  await page.route(/^https?:\/\/(?!127\.0\.0\.1)/, (route) => route.abort());
  page.on("response", (res) => {
    if (res.url().includes("/api/") && res.status() >= 500) failures.push(`${res.status()} ${res.url()}`);
  });
  const shot = async (name) => {
    await page.waitForLoadState("networkidle");
    await page.screenshot({ path: `${out}/${name}.png`, fullPage: true });
  };
  const otp = async () => {
    const code = (await (await fetch(api + "/__test/otp")).json()).code;
    const inputs = page.locator('input[inputmode="numeric"]');
    for (let i = 0; i < 6; i++) await inputs.nth(i).fill(code[i]);
    await page.getByRole("button", { name: "Verify", exact: true }).click();
  };

  await page.goto(origin + "/sign-up");
  await page.waitForLoadState("networkidle");
  await page.getByLabel("Work email").fill("staff-owner-e2e@example.com");
  await page.getByLabel("Phone number", { exact: true }).fill("08012345681");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  await otp();
  await page.waitForURL("**/setup/create-company");
  await page.getByLabel("Legal company name").fill("Staff Logistics Limited");
  await page.getByLabel("Trading name (optional)", { exact: true }).fill("Staff Freight");
  await page.getByLabel("Registration number").fill("RC-STAFF-E2E");
  await page.getByLabel("Company type").selectOption("PRIVATE_LIMITED");
  await page.getByLabel("Your full name").fill("Ada Owner");
  await page.getByRole("button", { name: "Save & continue", exact: true }).click();
  await page.waitForURL("**/setup/headquarters");

  // A submitted company under review (the state a new customer is in) can
  // manage its staff. Sign in again so the session reflects the status.
  const res = await fetch(api + "/__test/organization-status", {
    method: "POST",
    body: JSON.stringify({ tradingName: "Staff Freight", status: "UNDER_REVIEW" }),
  });
  assert.equal(res.status, 204);
  await page.context().clearCookies();
  await page.evaluate(() => localStorage.clear());
  await fetch(api + "/__test/cooldown", { method: "POST" });
  await page.goto(origin + "/sign-in");
  await page.waitForLoadState("networkidle");
  await page.getByLabel("Email address or phone number").fill("staff-owner-e2e@example.com");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  await otp();
  await page.waitForURL((url) => url.pathname === "/");

  // The directory lists the owner with their real contact and role.
  await page.goto(origin + "/settings/staff");
  await page.getByRole("heading", { name: "Staff Directory" }).waitFor();
  const ownerRow = page.locator("tr", { hasText: "Ada Owner" });
  await ownerRow.waitFor();
  assert.match(await ownerRow.innerText(), /staff-owner-e2e@example\.com/);
  assert.match(await ownerRow.innerText(), /Owner/);
  await shot("directory-owner-only");

  // Invite a colleague.
  await page.getByRole("button", { name: "Invite Staff" }).click();
  await page.waitForURL("**/settings/staff/invite");
  await page.getByLabel("Role *").locator("option").nth(0).waitFor({ state: "attached" });
  await page.getByRole("button", { name: "Send Invitation" }).click();
  await page.getByText("Name is required").waitFor();
  await page.getByLabel("Full Name *").fill("Bola Dispatcher");
  await page.getByLabel("Email Address *").fill("bola@example.com");
  await page.getByLabel("Phone Number *").fill("08012345682");
  const roleOptions = await page.getByLabel("Role *").locator("option").allInnerTexts();
  assert(roleOptions.length > 0 && !roleOptions.some((r) => /^owner$/i.test(r.trim())), `invitable roles: ${roleOptions}`);
  await shot("invite-form");
  await page.getByRole("button", { name: "Send Invitation" }).click();
  const sent = await Promise.race([
    page.waitForURL((url) => url.pathname === "/settings/staff").then(() => "sent"),
    page.getByRole("alert").filter({ hasText: /\w/ }).waitFor().then(async () => "alert: " + (await page.getByRole("alert").filter({ hasText: /\w/ }).innerText())),
  ]);
  assert.equal(sent, "sent");
  const invitedRow = page.locator("tr", { hasText: "bola@example.com" });
  await invitedRow.waitFor();
  assert.match(await invitedRow.innerText(), /Invited/);
  assert.match(await invitedRow.innerText(), /Pending/);
  await shot("directory-with-invitation");

  // Resend, then cancel.
  await invitedRow.getByRole("button", { name: "Resend" }).click();
  await page.getByText("Invitation resent to bola@example.com.").waitFor();
  await invitedRow.getByRole("button", { name: "Cancel" }).click();
  await page.getByText("Invitation to bola@example.com cancelled.").waitFor();
  await invitedRow.waitFor({ state: "detached" });

  // Owner detail: real permissions; the owner cannot be suspended.
  await ownerRow.click();
  await page.waitForURL("**/settings/staff/**");
  await page.getByRole("heading", { name: "Effective Permissions" }).waitFor();
  assert.equal(await page.getByRole("button", { name: "Suspend" }).count(), 0);
  assert((await page.locator("span[title]").count()) > 0, "owner has permissions");
  await shot("owner-detail");

  // Roles and permission catalogue from the server.
  await page.goto(origin + "/settings/roles");
  await page.getByRole("heading", { name: "Roles" }).waitFor();
  await page.locator("tbody tr").first().waitFor();
  assert((await page.locator("tbody tr").count()) >= 3, "system roles listed");
  await shot("roles");
  await page.goto(origin + "/settings/permissions");
  await page.getByRole("heading", { name: "Permission Catalogue" }).waitFor();
  await page.getByText(/\d+ permissions$/).first().waitFor();
  await shot("permissions");

  // The sidebar exposes the Team section.
  await page.goto(origin + "/");
  await page.getByRole("link", { name: "Staff" }).first().waitFor();

  assert.deepEqual(failures, [], "no server errors");
  console.log(JSON.stringify({ status: "passed", out }));
} catch (e) {
  console.error(output.slice(-3000));
  throw e;
} finally {
  await browser?.close();
  server.kill("SIGTERM");
}
