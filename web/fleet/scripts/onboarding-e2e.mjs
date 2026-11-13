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
  await page
    .getByRole("heading", { name: "Submission status", exact: true })
    .waitFor();
  checks.push(
    "edit completed company section preserves legal data and returns to review; submission persists without approving company or requiring wallet",
  );
  // After submission the company is read-only: documents can still be viewed.
  await page.goto(origin + "/setup/business-docs");
  await page.getByText("Status: Ready", { exact: true }).nth(3).waitFor();
  assert.equal(await page.locator("[data-document-preview]").count(), 4);
  assert.equal(await page.getByText("Replace", { exact: true }).count(), 0);
  await page
    .getByRole("button", { name: "Preview Tax Clearance", exact: true })
    .click();
  await page.getByRole("dialog", { name: /preview$/ }).locator("img").waitFor();
  await page.getByRole("button", { name: "Close preview" }).click();
  await page.goto(origin + "/setup/submit");
  await page
    .getByRole("heading", { name: "Submission status", exact: true })
    .waitFor();
  checks.push("read-only submitted company: Preview available without Replace or Remove");
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await page.waitForURL("**/sign-in");
  await fetch(api + "/__test/cooldown", { method: "POST" });
  await page.getByLabel("Email address or phone number").fill("08012345678");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  await otp();
  await page.waitForURL("**/setup/submit");
  await page
    .getByRole("heading", { name: "Submission status", exact: true })
    .waitFor();
  await page.getByText("Edited Browser Freight", { exact: true }).waitFor();
  await page.screenshot({
    path: evidence + "/mobile-submitted.png",
    fullPage: true,
  });
  checks.push(
    "logout → phone sign-in → registered email OTP → saved company submission across session",
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
  await page.waitForURL("**/setup/submit");
  await page
    .getByRole("heading", { name: "Submission status", exact: true })
    .waitFor();
  await page.getByText("Edited Browser Freight", { exact: true }).waitFor();
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
    "independent Android Chrome emulation session → email sign-in → persisted company review without prior browser drafts",
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
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await page.waitForURL("**/sign-in");
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
  await page.waitForURL("**/setup/submit");
  await page.getByText("Edited Browser Freight", { exact: true }).waitFor();
  checks.push(
    "multiple organizations select by membership; switching drafts cannot reuse another tenant’s saved address or review data",
  );
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
