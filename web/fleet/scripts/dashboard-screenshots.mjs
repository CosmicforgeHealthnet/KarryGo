// Dashboard and setup visual regression captures across lifecycle states.
//
// Against the disposable API: a draft company's setup home, then the same
// company's dashboard while ACTIVE and while UNDER_REVIEW, desktop and mobile.
// Run before and after a change and compare: the draft setup page and the
// active dashboard must be identical; the pending dashboard may differ only by
// the verification banner.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { chromium } from "@playwright/test";

const api = process.env.FLEET_AUTH_E2E_API;
assert(api?.startsWith("http://127.0.0.1:"), "Disposable local API required");
const label = process.env.FLEET_SHOT_LABEL || "run";
const origin = "http://127.0.0.1:3417";
const out = `../../.codex-cache/fleet-dashboard-screens/${label}`;
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
try {
  for (let i = 0; i < 180; i++) {
    try {
      if ((await fetch(origin + "/sign-in")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 1000));
  }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 1 });
  const page = await context.newPage();
  page.setDefaultTimeout(60000);
  // External map tiles and styles vary run to run; keep them out.
  await page.route(/^https?:\/\/(?!127\.0\.0\.1)/, (route) => route.abort());
  const otp = async () => {
    const code = (await (await fetch(api + "/__test/otp")).json()).code;
    const inputs = page.locator('input[inputmode="numeric"]');
    for (let i = 0; i < 6; i++) await inputs.nth(i).fill(code[i]);
    await page.getByRole("button", { name: "Verify", exact: true }).click();
  };
  const setStatus = async (status) => {
    const res = await fetch(api + "/__test/organization-status", {
      method: "POST",
      body: JSON.stringify({ tradingName: "Shots Freight", status }),
    });
    assert.equal(res.status, 204);
  };
  const settle = async () => {
    await page.waitForLoadState("networkidle");
    await page.evaluate(() => document.fonts.ready);
    await page.waitForTimeout(1500);
  };
  const shoot = async (name) => {
    for (const [device, size] of [["desktop", { width: 1440, height: 1000 }], ["mobile", { width: 390, height: 844 }]]) {
      await page.setViewportSize(size);
      await settle();
      await page.screenshot({
        path: `${out}/${name}-${device}.png`, fullPage: true, animations: "disabled", caret: "hide",
        // Per-run identifiers and live map/chart canvases.
        mask: [page.getByText(/^ID: /), page.locator("canvas"), page.locator(".maplibregl-map")],
      });
      if (name.startsWith("dashboard-pending")) {
        const tag = await page.addStyleTag({ content: 'main div:has(> [role="status"][aria-label$="Pending Review"]){display:none!important}' });
        await page.waitForTimeout(150);
        await page.screenshot({
          path: `${out}/${name}-${device}-without-banner.png`, fullPage: true, animations: "disabled", caret: "hide",
          mask: [page.getByText(/^ID: /), page.locator("canvas"), page.locator(".maplibregl-map")],
        });
        await tag.evaluate((el) => el.remove());
      }
    }
    await page.setViewportSize({ width: 1440, height: 1000 });
  };
  const signIn = async () => {
    await fetch(api + "/__test/cooldown", { method: "POST" });
    await page.goto(origin + "/sign-in");
    await page.getByLabel("Email address or phone number").fill("shots-dashboard@example.com");
    await page.getByRole("button", { name: "Continue", exact: true }).click();
    await page.waitForURL("**/verify?**");
    await otp();
  };
  const signOut = async () => {
    await page.context().clearCookies();
    await page.evaluate(() => localStorage.clear());
  };

  await page.goto(origin + "/sign-up");
  await page.waitForLoadState("networkidle");
  await page.getByLabel("Work email").fill("shots-dashboard@example.com");
  await page.getByLabel("Phone number", { exact: true }).fill("08012345677");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  await otp();
  await page.waitForURL("**/setup/create-company");
  await page.getByLabel("Legal company name").fill("Shots Logistics Limited");
  await page.getByLabel("Trading name (optional)", { exact: true }).fill("Shots Freight");
  await page.getByLabel("Registration number").fill("RC-SHOTS-DASHBOARD");
  await page.getByLabel("Company type").selectOption("PRIVATE_LIMITED");
  await page.getByLabel("Your full name").fill("Shots Owner");
  await page.getByRole("button", { name: "Save & continue", exact: true }).click();
  await page.waitForURL("**/setup/headquarters");

  // Draft company: setup home.
  await page.goto(origin + "/setup");
  await page.getByText(/^Set up /).waitFor();
  await shoot("setup-home-draft");

  // Active company: a fresh sign-in, the dashboard.
  await setStatus("ACTIVE");
  await signOut();
  await signIn();
  await page.waitForURL((url) => url.pathname === "/");
  await page.getByText("Shots Freight").first().waitFor();
  await shoot("dashboard-active");

  // Submitted company under review: a fresh sign-in.
  await setStatus("UNDER_REVIEW");
  await signOut();
  await signIn();
  const landed = await Promise.race([
    page.waitForURL((url) => url.pathname === "/", { timeout: 30000 }).then(() => "/"),
    page.waitForURL("**/setup/**", { timeout: 30000 }).then(() => "setup"),
  ]).catch(() => "neither");
  console.log(JSON.stringify({ underReviewSignInLandedOn: landed, url: page.url() }));
  if (landed === "/") {
    await page.getByText("Shots Freight").first().waitFor();
    await shoot("dashboard-pending");
  } else {
    await settle();
    await page.screenshot({ path: `${out}/under-review-sign-in-desktop.png`, fullPage: true });
  }
  console.log(JSON.stringify({ status: "captured", out }));
} catch (e) {
  console.error(output.slice(-3000));
  throw e;
} finally {
  await browser?.close();
  server.kill("SIGTERM");
}
