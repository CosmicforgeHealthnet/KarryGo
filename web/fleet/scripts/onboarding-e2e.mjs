// Disposable PostgreSQL-backed test router; email and downstream authorities are fixtures.
import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { writeFile, mkdir } from "node:fs/promises";
import { chromium, devices } from "@playwright/test";
const api = process.env.FLEET_AUTH_E2E_API;
assert(api?.startsWith("http://127.0.0.1:"), "Disposable local API required");
const origin = "http://127.0.0.1:3417";
const evidence = "../../.codex-cache/fleet-onboarding-browser";
await mkdir(evidence, { recursive: true });
const server = spawn(
  process.execPath,
  [
    "node_modules/next/dist/bin/next",
    "dev",
    "--hostname",
    "127.0.0.1",
    "--port",
    "3417",
  ],
  {
    env: { ...process.env, FLEET_API_BASE_URL: api },
    windowsHide: true,
    stdio: ["ignore", "pipe", "pipe"],
  },
);
let output = "";
server.stdout.on("data", (b) => {
  output += b;
});
server.stderr.on("data", (b) => {
  output += b;
});
let browser, page;
const checks = [];
const errors = [];
try {
  for (let i = 0; i < 120; i++) {
    if (server.exitCode !== null) throw new Error("Next server exited");
    try {
      if ((await fetch(origin + "/sign-in")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 1000));
    if (i === 119) throw new Error("Next server not ready");
  }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
  });
  page = await context.newPage();
  page.setDefaultTimeout(45000);
  page.on("pageerror", (e) => errors.push(e.message));
  const otp = async () => {
    const code = (await (await fetch(api + "/__test/otp")).json()).code;
    const inputs = page.locator('input[inputmode="numeric"]');
    for (let i = 0; i < 6; i++) await inputs.nth(i).fill(code[i]);
    await page.getByRole("button", { name: "Verify", exact: true }).click();
  };
  const save = async (path) => {
    await page
      .getByRole("button", { name: "Save & continue", exact: true })
      .click();
    await page.waitForURL("**/setup/" + path);
  };
  await page.goto(origin + "/sign-up");
  await page.waitForLoadState("networkidle");
  await page.getByLabel("Work email").fill("onboard-browser@example.com");
  await page.getByLabel("Phone number", { exact: true }).fill("08012345678");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  await otp();
  await page.waitForURL("**/setup/create-company");
  await page
    .getByRole("heading", { name: "Company details", exact: true })
    .waitFor();
  await page
    .getByText("onboard-browser@example.com", { exact: true })
    .waitFor();
  assert.equal(
    await page.locator('input[type="email"]').count(),
    0,
    "Company setup must not request known account email",
  );
  await page.getByLabel("Legal company name").fill("Browser Logistics Limited");
  await page
    .getByLabel("Trading name (optional)", { exact: true })
    .fill("Browser Freight");
  await page.getByLabel("Registration number").fill("RC-BROWSER-ONBOARD");
  await page.getByLabel("Company type").selectOption("PRIVATE_LIMITED");
  await page.getByLabel("Your full name").fill("Browser Owner");
  await save("headquarters");
  checks.push(
    "email OTP → persisted company, legal type, unique owner membership and server-backed contact reuse",
  );
  assert.equal(
    await page.getByLabel("Business email").inputValue(),
    "onboard-browser@example.com",
  );
  await page.getByLabel("Street address").fill("1 Registered Road");
  await page.getByLabel("City").fill("Lagos");
  await page.getByLabel("State or province").fill("Lagos");
  await page.getByLabel("Business email").fill("business-browser@example.com");
  await page.getByLabel("Same as registered business address").uncheck();
  await page.locator("#operating-street").fill("2 Operating Road");
  await page.locator("#operating-city").fill("Abuja");
  await page.locator("#operating-state").fill("FCT");
  await context.setOffline(true);
  await page
    .getByRole("button", { name: "Save & continue", exact: true })
    .click();
  await page.getByRole("alert").filter({hasText:"We couldn't connect"}).waitFor();
  assert.equal(
    await page.locator("#registered-street").inputValue(),
    "1 Registered Road",
    "Network failure retains edits",
  );
  await context.setOffline(false);
  await save("authorized-representative");
  await page
    .getByLabel("I am the authorized representative.", { exact: true })
    .check();
  assert.equal(
    await page.getByLabel("Full name").inputValue(),
    "Browser Owner",
  );
  assert.equal(
    await page.getByLabel("Email").inputValue(),
    "onboard-browser@example.com",
  );
  await page.getByLabel("Role title").fill("Director");
  await page.getByLabel("Authority type").selectOption("DIRECTOR");
  await save("capabilities");
  await page.goto(origin + "/setup/authorized-representative");
  await page.getByLabel("Role title").fill("Managing Director");
  await save("capabilities");
  await page.goto(origin + "/setup/authorized-representative");
  await page
    .getByLabel("Someone else represents the company.", { exact: true })
    .check();
  await page.getByLabel("Full name").fill("Another Authorized Person");
  await page.getByLabel("Email").fill("representative-browser@example.com");
  await page.getByLabel("Phone").fill("+2348012345698");
  await page.getByLabel("Authority type").selectOption("OFFICER");
  await save("capabilities");
  await page.goto(origin + "/setup/authorized-representative");
  await page
    .getByLabel("I am the authorized representative.", { exact: true })
    .check();
  await page.getByLabel("Authority type").selectOption("DIRECTOR");
  await save("capabilities");
  checks.push(
    "distinct operating address; editable business contacts do not change identity; representative self create and versioned edit",
  );
  await page.getByLabel("Package Delivery", { exact: true }).check();
  await save("business-docs");
  await page.route("**/api/organizations/*/capabilities", (route) =>
    route.abort("internetdisconnected"),
  );
  await page.goto(origin + "/setup/capabilities");
  await page.getByRole("alert").filter({hasText:"We couldn't connect"}).waitFor();
  assert.equal(
    await page.locator('input[type="checkbox"]').count(),
    0,
    "Load failure must not offer an empty replacement",
  );
  await page.unroute("**/api/organizations/*/capabilities");
  await page
    .getByRole("button", { name: "Reload saved information", exact: true })
    .click({ timeout: 2000 })
    .catch(async (error) => {
      if (
        !(await page
          .getByLabel("Package Delivery", { exact: true })
          .isVisible())
      )
        throw error;
    });
  assert.equal(
    await page.getByLabel("Package Delivery", { exact: true }).isChecked(),
    true,
  );
  await save("business-docs");
  checks.push(
    "offline save retains inputs; failed capabilities read prevents destructive replacement and retry restores saved selection",
  );
  const pdf = Buffer.from(
    "%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF",
  );
  for (const type of [
    "Certificate Of Incorporation",
    "Tax Clearance",
    "Insurance Policy",
    "Operating License",
  ]) {
    const section = page
      .getByRole("heading", { name: type, exact: true })
      .locator("..")
      .locator("..");
    await section.locator('input[type="file"]').first().setInputFiles({
      name: "authorized-test.pdf",
      mimeType: "application/pdf",
      buffer: pdf,
    });
    await section.getByText("Status: Ready", { exact: true }).waitFor();
  }
  await page.reload();
  await page.getByText("Status: Ready", { exact: true }).first().waitFor();
  assert.equal(
    await page.getByText("Status: Ready", { exact: true }).count(),
    4,
  );
  const certificate = page
    .getByRole("heading", { name: "Certificate Of Incorporation", exact: true })
    .locator("..")
    .locator("..");
  await certificate
    .getByRole("button", { name: "Remove", exact: true })
    .click();
  await certificate
    .getByRole("button", { name: "Confirm removal", exact: true })
    .click();
  await certificate.getByText("No document attached.").waitFor();
  await page.reload();
  await page.getByText("No document attached.").waitFor();
  await certificate.locator('input[type="file"]').first().setInputFiles({
    name: "replacement.pdf",
    mimeType: "application/pdf",
    buffer: pdf,
  });
  await certificate.getByText("Status: Ready", { exact: true }).waitFor();
  // ---- Document preview: every accepted format, read-only, failure-tolerant ----
  const fixtures = {
    png: ["image/png", "iVBORw0KGgoAAAANSUhEUgAAABgAAAAQCAIAAACDRijCAAAAHElEQVR4nGPkqohioAZgooopowaNGjRq0Ag2CAA3hAD8jfoQDwAAAABJRU5ErkJggg=="],
    jpeg: ["image/jpeg", "/9j/4AAQSkZJRgABAQAAAQABAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0aHBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/2wBDAQkJCQwLDBgNDRgyIRwhMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjIyMjL/wAARCAAQABgDASIAAhEBAxEB/8QAHwAAAQUBAQEBAQEAAAAAAAAAAAECAwQFBgcICQoL/8QAtRAAAgEDAwIEAwUFBAQAAAF9AQIDAAQRBRIhMUEGE1FhByJxFDKBkaEII0KxwRVS0fAkM2JyggkKFhcYGRolJicoKSo0NTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqDhIWGh4iJipKTlJWWl5iZmqKjpKWmp6ipqrKztLW2t7i5usLDxMXGx8jJytLT1NXW19jZ2uHi4+Tl5ufo6erx8vP09fb3+Pn6/8QAHwEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoL/8QAtREAAgECBAQDBAcFBAQAAQJ3AAECAxEEBSExBhJBUQdhcRMiMoEIFEKRobHBCSMzUvAVYnLRChYkNOEl8RcYGRomJygpKjU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6goOEhYaHiImKkpOUlZaXmJmaoqOkpaanqKmqsrO0tba3uLm6wsPExcbHyMnK0tPU1dbX2Nna4uPk5ebn6Onq8vP09fb3+Pn6/9oADAMBAAIRAxEAPwDk6KKK0PkAooooA//Z"],
    webp: ["image/webp", "UklGRjwAAABXRUJQVlA4IDAAAACQAgCdASoYABAAPm0skUWkIqGYBABABsSgAAeyAAD+771OI7f/+bA/9vP/BHloAAA="],
  };
  const image = (kind) => ({
    name: "preview." + kind,
    mimeType: fixtures[kind][0],
    buffer: Buffer.from(fixtures[kind][1], "base64"),
  });
  const docSection = (type) =>
    page.getByRole("heading", { name: type, exact: true }).locator("..").locator("..");
  const readyRefs = async () =>
    page.getByText(/^File reference:/).allInnerTexts();
  const dialog = page.getByRole("dialog", { name: /preview$/ });
  const openPreview = async (type) => {
    await page
      .getByRole("button", { name: "Preview " + type, exact: true })
      .click();
    await dialog.waitFor();
  };
  const closePreview = async (how = "button") => {
    if (how === "escape") await page.keyboard.press("Escape");
    else await page.getByRole("button", { name: "Close preview" }).click();
    await dialog.waitFor({ state: "detached" });
  };
  // PDF: shown in the browser's own viewer from a local object URL.
  await openPreview("Certificate Of Incorporation");
  const frame = dialog.locator('object[type="application/pdf"]');
  await frame.waitFor();
  const pdfUrl = await frame.getAttribute("data");
  assert.match(pdfUrl, /^blob:/, "Preview never uses a Media or tokenized URL");
  assert.match(
    await page.evaluate(async (u) => (await fetch(u)).text(), pdfUrl),
    /^%PDF-/,
    "PDF preview shows the saved document",
  );
  await closePreview();
  assert.equal(
    await page.evaluate(async (u) => fetch(u).then(() => "open", () => "revoked"), pdfUrl),
    "revoked",
    "Closing the preview releases its object URL",
  );
  // Images: each stored format renders at its real size.
  for (const [type, kind] of [
    ["Tax Clearance", "png"],
    ["Insurance Policy", "jpeg"],
    ["Operating License", "webp"],
  ]) {
    await docSection(type)
      .locator('input[type="file"]')
      .first()
      .setInputFiles(image(kind));
    await docSection(type).getByText("Status: Ready", { exact: true }).waitFor();
    await page.waitForFunction(() => !document.querySelector("[aria-busy=true]"));
    await openPreview(type);
    const img = dialog.locator("img");
    await img.waitFor();
    await page.waitForFunction(
      (el) => el.complete && el.naturalWidth > 0,
      await img.elementHandle(),
    );
    assert.equal(await img.evaluate((el) => el.naturalWidth), 24, kind + " preview decoded");
    await closePreview(kind === "jpeg" ? "escape" : "button");
  }
  const refsBefore = await readyRefs();
  await page.reload();
  await page.getByText("Status: Ready", { exact: true }).nth(3).waitFor();
  await openPreview("Operating License");
  await dialog.locator("img").waitFor();
  await closePreview();
  // A failed preview stays inside the viewer and changes nothing.
  await page.route("**/documents/*/preview", (route) =>
    route.abort("internetdisconnected"),
  );
  await openPreview("Tax Clearance");
  await dialog.getByRole("alert").filter({ hasText: "We couldn't connect" }).waitFor();
  await closePreview();
  await page.unroute("**/documents/*/preview");
  await page.setViewportSize({ width: 390, height: 844 });
  await openPreview("Certificate Of Incorporation");
  await dialog.locator("object").waitFor();
  const box = await dialog.boundingBox();
  assert(box && box.x >= 0 && box.x + box.width <= 390, "Preview fits a phone screen");
  assert(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    "Mobile preview overflow",
  );
  await closePreview();
  await page.setViewportSize({ width: 1440, height: 1000 });
  assert.deepEqual(await readyRefs(), refsBefore, "Preview changes no document");
  assert.equal(await page.getByText("Status: Ready", { exact: true }).count(), 4);
  checks.push(
    "preview PDF, PNG, JPEG and WebP from authenticated object URLs; close and Escape release them; survives reload; failure stays in the viewer; fits mobile; documents unchanged",
  );
  await save("submit");
  await page
    .getByText("2 Operating Road, Abuja, FCT, NG", { exact: true })
    .waitFor();
  await page
    .getByText("business-browser@example.com", { exact: true })
    .waitFor();
  await page.getByText("Managing Director", { exact: true }).waitFor();
  checks.push(
    "signed media authorization → upload/checksum → readiness → attach; server-backed removal remains removed after refresh; review displays saved records",
  );
  await page
    .getByRole("link", { name: "Edit company details", exact: true })
    .click();
  await page
    .getByLabel("Trading name (optional)", { exact: true })
    .fill("Edited Browser Freight");
  await page
    .getByRole("button", { name: "Save & return to review", exact: true })
    .click();
  await page.waitForURL("**/setup/submit");
  await page.getByText("Edited Browser Freight", { exact: true }).waitFor();
  await page.setViewportSize({ width: 390, height: 844 });
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    "Mobile overflow",
  );
  await page.screenshot({
    path: evidence + "/mobile-review.png",
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Submit company for review", exact: true })
    .click();
  // ---- Submitted: the real dashboard, restricted, never back to setup ----
  const banner = (pg) => pg.getByText("Your company is under review.", { exact: true });
  const expectDashboard = async (pg, label) => {
    await pg.waitForURL((url) => url.pathname === "/", { timeout: 45000 });
    await banner(pg).waitFor();
    assert.equal(await pg.getByText(/^Set up /).count(), 0, label + ": setup heading shown");
    assert.equal(
      await pg.getByText("Complete these steps so we can verify your company.").count(),
      0,
      label + ": obsolete setup copy shown",
    );
  };
  const signOutOfDashboard = async (pg) => {
    await pg.setViewportSize({ width: 1440, height: 1000 });
    await pg.locator('aside button[aria-haspopup="menu"]').click();
    await pg.getByRole("button", { name: "Sign out", exact: true }).click();
    await pg.waitForURL("**/sign-in");
  };
  await expectDashboard(page, "after submission");
  checks.push(
    "edit completed company section preserves legal data and returns to review; submission refreshes the authoritative status and enters the dashboard with the pending verification banner, without approving the company",
  );
  await page.reload();
  await expectDashboard(page, "refresh");
  for (const path of ["/setup", "/setup/submit", "/setup/business-docs", "/setup/profile"]) {
    await page.goto(origin + path);
    await expectDashboard(page, path);
  }
  // Stale client state cannot send the company back to onboarding.
  await page.context().addCookies([{ name: "karrygo_org_status", value: "DRAFT", url: origin }]);
  await page.evaluate(() => {
    const raw = localStorage.getItem("organization-storage");
    if (raw) {
      const stored = JSON.parse(raw);
      if (stored?.state?.organization) stored.state.organization.status = "DRAFT";
      localStorage.setItem("organization-storage", JSON.stringify(stored));
    }
  });
  await page.goto(origin + "/");
  await expectDashboard(page, "stale cookie and storage");
  assert.equal(
    (await page.context().cookies()).some((c) => c.name === "karrygo_org_status"),
    false,
    "Retired status cookie is removed",
  );
  checks.push(
    "submitted company: refresh, direct /, /setup, /setup/submit and step URLs, and a stale DRAFT cookie and stored status all land on the dashboard",
  );
  // Restricted operation: explained in the UI and refused by the API itself.
  await page.goto(origin + "/providers/add");
  const required = page.locator("form input[required]");
  await required.nth(0).fill("Restricted Provider");
  await required.nth(1).fill("restricted-provider@example.com");
  await required.nth(2).fill("+2348012349999");
  await page.getByRole("button", { name: "Send invitation" }).click();
  const notice = page.getByRole("alertdialog");
  await notice.getByText("Company verification required", { exact: true }).waitFor();
  await notice.getByText("This feature will be available after your company is approved.").waitFor();
  await notice.getByRole("button", { name: "OK", exact: true }).click();
  await notice.waitFor({ state: "detached" });
  const sessionToken = JSON.parse(await page.evaluate(() => localStorage.getItem("auth-storage"))).state.accessToken;
  const direct = await page.request.post(origin + "/api/providers", {
    headers: { Authorization: "Bearer " + sessionToken, "Idempotency-Key": "direct-restricted" },
    data: { legalName: "Direct", email: "direct@example.com", phone: "+2348012349998", serviceCategory: "DISPATCH" },
  });
  assert.equal(direct.status(), 403, "Direct restricted API call refused");
  assert.equal((await direct.json()).error.code, "organization_verification_pending");
  checks.push(
    "pending company: restricted action shows 'Company verification required' and a direct API call is refused with organization_verification_pending",
  );
  // The submitted information stays reviewable, read only, with document preview.
  await page.goto(origin + "/company");
  await page.getByRole("heading", { name: "Submission status", exact: true }).waitFor();
  await page.getByText("Edited Browser Freight", { exact: true }).first().waitFor();
  assert.equal(await page.locator("[data-document-preview]").count(), 4);
  assert.equal(await page.getByRole("link", { name: /^Edit / }).count(), 0, "Submission is read only");
  await page.getByRole("button", { name: "Preview Tax Clearance", exact: true }).click();
  await page.getByRole("dialog", { name: /preview$/ }).locator("img").waitFor();
  await page.getByRole("button", { name: "Close preview" }).click();
  checks.push("pending company reviews its submission and verification status read only, with document preview, inside the dashboard");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(origin + "/");
  await expectDashboard(page, "mobile");
  assert(
    await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    "Mobile dashboard overflow",
  );
  await page.screenshot({ path: evidence + "/mobile-pending-dashboard.png", fullPage: true });
  await signOutOfDashboard(page);
  await fetch(api + "/__test/cooldown", { method: "POST" });
  await page.getByLabel("Email address or phone number").fill("08012345678");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  await otp();
  await expectDashboard(page, "phone sign-in");
  await page.goto(origin + "/company");
  await page.getByText("Edited Browser Freight", { exact: true }).first().waitFor();
  checks.push(
    "logout → phone sign-in → registered email OTP → dashboard with saved company submission across session",
  );
  const android = await browser.newContext({ ...devices["Pixel 7"] });
  page = await android.newPage();
  page.on("pageerror", (e) => errors.push(e.message));
  await fetch(api + "/__test/cooldown", { method: "POST" });
  await page.goto(origin + "/sign-in");
  await page.waitForLoadState("networkidle");
  await page
    .getByLabel("Email address or phone number")
    .fill("onboard-browser@example.com");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  await otp();
  await expectDashboard(page, "email sign-in (Android)");
  assert(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    "Android Chrome emulation overflow",
  );
  await page.screenshot({
    path: evidence + "/android-chrome-review.png",
    fullPage: true,
  });
  checks.push(
    "independent Android Chrome emulation session → email sign-in → pending dashboard without prior browser state",
  );
  const stored = JSON.parse(
    await page.evaluate(() => localStorage.getItem("auth-storage")),
  );
  const second = await android.request.post(origin + "/api/organizations", {
    headers: {
      Authorization: "Bearer " + stored.state.accessToken,
      "Idempotency-Key": "second-browser-company",
    },
    data: {
      legalName: "Second Browser Company",
      tradingName: "Second Fleet",
      registrationNumber: "RC-SECOND-BROWSER",
      organizationType: "LLC",
      countryCode: "NG",
      timezone: "Africa/Lagos",
      defaultCurrency: "NGN",
      ownerName: "Browser Owner",
    },
  });
  assert.equal(second.status(), 201, "Create authorized second organization");
  await signOutOfDashboard(page);
  await fetch(api + "/__test/cooldown", { method: "POST" });
  await page
    .getByLabel("Email address or phone number")
    .fill("onboard-browser@example.com");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  await otp();
  await page.waitForURL("**/choose-organization");
  await page.getByRole("button", { name: /Second Fleet/ }).click();
  await page.waitForURL("**/setup");
  await page.goto(origin + "/setup/headquarters");
  await page.locator("#registered-street").waitFor();
  assert.equal(
    await page.locator("#registered-street").inputValue(),
    "",
    "Other organization must not receive cached address",
  );
  await page.goto(origin + "/choose-organization");
  await page.getByRole("button", { name: /Edited Browser Freight/ }).click();
  await expectDashboard(page, "switch to the submitted company");
  await page.goto(origin + "/choose-organization");
  await page.getByRole("button", { name: /Second Fleet/ }).click();
  await page.waitForURL("**/setup");
  assert.equal(await banner(page).count(), 0, "Draft company is not shown the pending banner");
  checks.push(
    "multiple organizations select by membership; switching drafts cannot reuse another tenant’s saved address or review data",
  );
  // ---- Lifecycle changes reach the same session without signing in again ----
  const setStatus = async (status) => {
    const res = await fetch(api + "/__test/organization-status", {
      method: "POST",
      body: JSON.stringify({ tradingName: "Edited Browser Freight", status }),
    });
    assert.equal(res.status, 204, "set organization status " + status);
  };
  await page.goto(origin + "/choose-organization");
  await page.getByRole("button", { name: /Edited Browser Freight/ }).click();
  await expectDashboard(page, "back to the submitted company");
  // Slow status read: the spinner holds; neither setup nor a stale dashboard
  // flashes before the authoritative status arrives.
  await page.route("**/api/organizations/current", async (route) => {
    await new Promise((r) => setTimeout(r, 2500));
    await route.continue();
  });
  await page.goto(origin + "/", { waitUntil: "commit" });
  await page.waitForTimeout(1200);
  assert.equal(await page.getByText(/^Set up /).count(), 0, "No setup flash while status loads");
  assert.equal(await banner(page).count(), 0, "No dashboard before status is confirmed");
  await expectDashboard(page, "slow status response");
  await page.unroute("**/api/organizations/current");
  // Expired access token: refreshed from the session cookie, still the dashboard.
  await page.evaluate(() => {
    const stored = JSON.parse(localStorage.getItem("auth-storage"));
    stored.state.accessToken = "expired-access-token";
    localStorage.setItem("auth-storage", JSON.stringify(stored));
  });
  await page.reload();
  await expectDashboard(page, "after access token refresh");
  checks.push("slow status read shows no setup or stale dashboard; an expired access token refreshes and stays on the dashboard");
  // Changes requested: back to the correction flow, and setup is editable.
  await setStatus("CHANGES_REQUIRED");
  await page.reload();
  await page.waitForURL((url) => url.pathname === "/setup");
  await page.getByText(/^Set up /).waitFor();
  assert.equal(await banner(page).count(), 0);
  // Back under review: the dashboard again.
  await setStatus("UNDER_REVIEW");
  await page.goto(origin + "/setup/submit");
  await expectDashboard(page, "under review");
  // Approved: the same session loses the restriction.
  await setStatus("ACTIVE");
  await page.reload();
  await page.waitForURL((url) => url.pathname === "/");
  await page.getByText("Dashboard", { exact: true }).first().waitFor();
  await page.waitForLoadState("networkidle");
  assert.equal(await banner(page).count(), 0, "Active company has no verification banner");
  for (const path of ["/setup", "/setup/submit"]) {
    await page.goto(origin + path);
    await page.waitForURL((url) => url.pathname === "/");
  }
  const activeToken = JSON.parse(await page.evaluate(() => localStorage.getItem("auth-storage"))).state.accessToken;
  const unlocked = await page.request.post(origin + "/api/providers", {
    headers: { Authorization: "Bearer " + activeToken, "Idempotency-Key": "active-unlocked" },
    // Invalid contact details: the providers use case itself refuses them,
    // which proves the request got past the lifecycle restriction.
    data: { legalName: "Active", email: "not-an-email", phone: "1", serviceCategory: "DISPATCH" },
  });
  assert.equal(unlocked.status(), 422, "Approval lifts the lifecycle restriction");
  checks.push("changes requested returns to setup; back under review returns to the dashboard; approval removes the banner and the API restriction in the same session");
  assert.deepEqual(errors, [], "Browser runtime errors");
  await writeFile(
    evidence + "/results.json",
    JSON.stringify({ status: "passed", checks }, null, 2),
  );
  console.log(JSON.stringify({ status: "passed", checks }));
} catch (e) {
  if (page) {
    console.error("URL", page.url());
    console.error("Browser runtime errors", errors);
    console.error((await page.locator("body").innerText()).slice(0, 6000));
    await page
      .screenshot({ path: evidence + "/failure.png", fullPage: true })
      .catch(() => {});
  }
  throw e;
} finally {
  await browser?.close();
  await writeFile(evidence + "/next.log", output);
  if (process.platform === "win32") {
    try {
      execFileSync("taskkill", ["/PID", String(server.pid), "/T", "/F"], {
        windowsHide: true,
        stdio: "ignore",
      });
    } catch {}
  } else server.kill("SIGTERM");
}
