// Documents-step visual regression captures (desktop and mobile).
//
// Captures the documents step empty and with every required document, masking
// values that differ per run (file references, attachment dates). Run before
// and after a change: with the Preview control hidden, the after captures must
// be pixel-identical to the before captures.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdir } from "node:fs/promises";
import { chromium } from "@playwright/test";

const api = process.env.FLEET_AUTH_E2E_API;
assert(api?.startsWith("http://127.0.0.1:"), "Disposable local API required");
const label = process.env.FLEET_SHOT_LABEL || "run";
const mode = process.env.FLEET_PERF_NEXT || "start";
const origin = "http://127.0.0.1:3417";
const out = `../../.codex-cache/fleet-onboarding-screens/${label}`;
await mkdir(out, { recursive: true });

const server = spawn(
  process.execPath,
  ["node_modules/next/dist/bin/next", mode, "--hostname", "127.0.0.1", "--port", "3417"],
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
  // A full Chromium build (FLEET_BROWSER_EXECUTABLE) includes the built-in PDF
  // viewer that Playwright's headless shell lacks.
  browser = await chromium.launch(
    process.env.FLEET_BROWSER_EXECUTABLE
      ? { headless: true, executablePath: process.env.FLEET_BROWSER_EXECUTABLE, args: ["--headless=new"] }
      : { headless: true },
  );
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, deviceScaleFactor: 1 });
  const page = await context.newPage();
  page.setDefaultTimeout(60000);
  await page.goto(origin + "/sign-up");
  await page.waitForLoadState("networkidle");
  await page.getByLabel("Work email").fill("shots-owner@example.com");
  await page.getByLabel("Phone number", { exact: true }).fill("08012345688");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  const code = (await (await fetch(api + "/__test/otp")).json()).code;
  const inputs = page.locator('input[inputmode="numeric"]');
  for (let i = 0; i < 6; i++) await inputs.nth(i).fill(code[i]);
  await page.getByRole("button", { name: "Verify", exact: true }).click();
  await page.waitForURL("**/setup/create-company");
  await page.getByLabel("Legal company name").fill("Screens Logistics Limited");
  await page.getByLabel("Registration number").fill("RC-SCREENS-ONBOARD");
  await page.getByLabel("Company type").selectOption("PRIVATE_LIMITED");
  await page.getByLabel("Your full name").fill("Screens Owner");
  await page.getByRole("button", { name: "Save & continue", exact: true }).click();
  await page.waitForURL("**/setup/headquarters");

  const masks = () => [page.getByText(/^File reference:/), page.getByText(/^Attached /)];
  const settle = async () => {
    await page.waitForLoadState("networkidle");
    await page.evaluate(() => document.fonts.ready);
    await page.waitForTimeout(400);
  };
  const shoot = async (name) => {
    for (const [device, size] of [["desktop", { width: 1440, height: 1000 }], ["mobile", { width: 390, height: 844 }]]) {
      await page.setViewportSize(size);
      await settle();
      await page.screenshot({
        path: `${out}/${name}-${device}.png`, fullPage: true, animations: "disabled",
        caret: "hide", mask: masks(),
      });
      // The same page with the Preview control removed from layout.
      await page.addStyleTag({ content: '[data-document-preview]{display:none!important}', }).then(async (tag) => {
        await page.waitForTimeout(100);
        await page.screenshot({
          path: `${out}/${name}-${device}-without-preview.png`, fullPage: true, animations: "disabled",
          caret: "hide", mask: masks(),
        });
        await tag.evaluate((el) => el.remove());
      });
    }
    await page.setViewportSize({ width: 1440, height: 1000 });
  };

  await page.goto(origin + "/setup/business-docs");
  await page.getByText("No document attached.").nth(3).waitFor();
  await shoot("documents-empty");

  const pdf = Buffer.from("%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF");
  for (const type of ["Certificate Of Incorporation", "Tax Clearance", "Insurance Policy", "Operating License"]) {
    const section = page.getByRole("heading", { name: type, exact: true }).locator("..").locator("..");
    await section.locator('input[type="file"]').first().setInputFiles({ name: "shot.pdf", mimeType: "application/pdf", buffer: pdf });
    await section.getByText("Status: Ready", { exact: true }).waitFor();
  }
  await page.reload();
  await page.getByText("Status: Ready", { exact: true }).nth(3).waitFor();
  await shoot("documents-ready");
  // The viewer itself (present only after the change).
  if (await page.locator("[data-document-preview]").count()) {
    const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAABgAAAAQCAIAAACDRijCAAAAHElEQVR4nGPkqohioAZgooopowaNGjRq0Ag2CAA3hAD8jfoQDwAAAABJRU5ErkJggg==", "base64");
    const tax = page.getByRole("heading", { name: "Tax Clearance", exact: true }).locator("..").locator("..");
    await tax.locator('input[type="file"]').first().setInputFiles({ name: "tax.png", mimeType: "image/png", buffer: png });
    await page.waitForTimeout(1500);
    await tax.getByText("Status: Ready", { exact: true }).waitFor();
    if (process.env.FLEET_SHOT_PDF) {
      const certificate = page.getByRole("heading", { name: "Certificate Of Incorporation", exact: true }).locator("..").locator("..");
      await certificate.locator('input[type="file"]').first().setInputFiles(process.env.FLEET_SHOT_PDF);
      await page.waitForTimeout(1500);
      await certificate.getByText("Status: Ready", { exact: true }).waitFor();
    }
    for (const [doc, kind] of [["Tax Clearance", "image"], ["Certificate Of Incorporation", "pdf"]]) {
      for (const [device, size] of [["desktop", { width: 1440, height: 1000 }], ["mobile", { width: 390, height: 844 }]]) {
        await page.setViewportSize(size);
        await page.getByRole("button", { name: "Preview " + doc, exact: true }).click();
        await page.getByRole("dialog", { name: /preview$/ }).locator(kind === "pdf" ? "object" : "img").waitFor();
        await page.waitForTimeout(700);
        await page.screenshot({ path: `${out}/viewer-${kind}-${device}.png`, animations: "disabled", caret: "hide" });
        await page.getByRole("button", { name: "Close preview" }).click();
      }
    }
  }
  console.log(JSON.stringify({ status: "captured", out }));
} catch (e) {
  console.error(output.slice(-3000));
  throw e;
} finally {
  await browser?.close();
  server.kill("SIGTERM");
}
