// Onboarding documents-page performance measurement.
//
// Runs the real Fleet frontend (production build when FLEET_PERF_NEXT=start)
// against a disposable PostgreSQL-backed Fleet API with a signed Media test
// authority, and records the time and API requests each user action costs.
// Chromium network emulation adds the round-trip latency measured against the
// staging deployment, so local numbers reflect remote request chains rather
// than loopback speed. The same script measures before and after a change.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { writeFile, mkdir } from "node:fs/promises";
import { chromium } from "@playwright/test";

const api = process.env.FLEET_AUTH_E2E_API;
assert(api?.startsWith("http://127.0.0.1:"), "Disposable local API required");
const label = process.env.FLEET_PERF_LABEL || "run";
const mode = process.env.FLEET_PERF_NEXT || "start";
const port = 3417; // the harness auth origin allowlist
const origin = `http://127.0.0.1:${port}`;
const evidence = "../../.codex-cache/fleet-onboarding-perf";
await mkdir(evidence, { recursive: true });

// Staging round trips measured from outside the deployment: 0.31-0.68 s.
const PROFILES = {
  staging: { latency: 300, downloadThroughput: -1, uploadThroughput: -1 },
  // Chromium's "Slow 4G" preset (mobile).
  slow4g: {
    latency: 562.5,
    downloadThroughput: (1.6 * 1024 * 1024) / 8,
    uploadThroughput: (750 * 1024) / 8,
  },
};

const server = spawn(
  process.execPath,
  ["node_modules/next/dist/bin/next", mode, "--hostname", "127.0.0.1", "--port", String(port)],
  { env: { ...process.env, FLEET_API_BASE_URL: api }, stdio: ["ignore", "pipe", "pipe"] },
);
let output = "";
server.stdout.on("data", (b) => (output += b));
server.stderr.on("data", (b) => (output += b));

const template = (url) =>
  new URL(url).pathname
    .replace(/\b[a-z]+_[0-9a-f]{16,}\b/g, ":id")
    .replace(/mobj_[0-9a-f]+/g, ":file");
const median = (xs) => {
  const s = [...xs].sort((a, b) => a - b);
  return s.length % 2 ? s[(s.length - 1) / 2] : Math.round((s[s.length / 2 - 1] + s[s.length / 2]) / 2);
};

const results = { label, mode, profiles: PROFILES, measurements: {} };
let browser;
try {
  for (let i = 0; i < 180; i++) {
    if (server.exitCode !== null) throw new Error("Next server exited\n" + output);
    try {
      if ((await fetch(origin + "/sign-in")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 1000));
    if (i === 179) throw new Error("Next server not ready");
  }
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  page.setDefaultTimeout(60000);
  const requests = [];
  const network = [];
  page.on("request", (r) => {
    if (r.url().includes("/api/")) requests.push(`${r.method()} ${template(r.url())}`);
    else network.push(`${r.method()} ${template(r.url())}${r.headers()["rsc"] ? " (rsc)" : ""}`);
  });
  // Per-request timeline for one cold load: what the page waits on.
  let timeline = null;
  page.on("request", (r) => {
    if (!timeline) return;
    const entry = { what: `${r.method()} ${template(r.url())}`, start: Date.now() - timeline.t0 };
    timeline.entries.push(entry);
    r.response().then(() => (entry.end = Date.now() - timeline?.t0)).catch(() => {});
  });
  const cdp = await context.newCDPSession(page);
  const throttle = (name) =>
    cdp.send("Network.emulateNetworkConditions", { offline: false, ...PROFILES[name] });
  const unthrottle = () =>
    cdp.send("Network.emulateNetworkConditions", {
      offline: false, latency: 0, downloadThroughput: -1, uploadThroughput: -1,
    });
  // Exact timings, taken in the page: a MutationObserver resolves with
  // performance.now() on the first frame where the condition holds. Polling
  // from the test (locator.waitFor backs off to 500 ms) would quantize results.
  const inPage = (condition, arg) =>
    page.evaluate(
      ([src, value]) =>
        new Promise((resolve) => {
          const check = new Function("arg", "return (" + src + ")(arg)");
          if (check(value)) return resolve(performance.now());
          const observer = new MutationObserver(() => {
            if (check(value)) {
              observer.disconnect();
              resolve(performance.now());
            }
          });
          observer.observe(document, { subtree: true, childList: true, characterData: true, attributes: true });
        }),
      [condition.toString(), arg],
    );
  const timed = async (action, condition, arg, settleMs = 1500) => {
    requests.length = 0;
    network.length = 0;
    const t0 = await page.evaluate(() => performance.now());
    const done = inPage(condition, arg);
    await action();
    const ms = Math.round((await done) - t0);
    const during = [...network];
    await page.waitForTimeout(settleMs);
    const counts = {};
    for (const r of requests) counts[r] = (counts[r] ?? 0) + 1;
    return { ms, apiRequests: requests.length, byEndpoint: counts, otherRequestsBeforeVisible: during };
  };
  // A full page load, timed from navigation start to the condition.
  const coldLoad = async (condition, arg) => {
    requests.length = 0;
    await page.goto(origin + "/setup/business-docs", { waitUntil: "commit" });
    const ms = Math.round(await inPage(condition, arg));
    await page.waitForTimeout(1500);
    const counts = {};
    for (const r of requests) counts[r] = (counts[r] ?? 0) + 1;
    return { ms, apiRequests: requests.length, byEndpoint: counts };
  };
  const readyCount = (n) =>
    new Function(`return () => document.body.innerText.split("Status: Ready").length - 1 === ${n}`)();
  const emptyCount = (n) =>
    new Function(`return () => document.body.innerText.split("No document attached.").length - 1 === ${n}`)();
  const section = (type) =>
    page.getByRole("heading", { name: type, exact: true }).locator("..").locator("..");
  const types = ["Certificate Of Incorporation", "Tax Clearance", "Insurance Policy", "Operating License"];
  const ready = page.getByText("Status: Ready", { exact: true });
  const empty = page.getByText("No document attached.");
  const waitCount = async (locator, n) => {
    await locator.nth(n - 1).waitFor();
  };

  // ---- Account and company (unmeasured setup) ----
  await page.goto(origin + "/sign-up");
  await page.waitForLoadState("networkidle");
  await page.getByLabel("Work email").fill("perf-owner@example.com");
  await page.getByLabel("Phone number", { exact: true }).fill("08012345699");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  const code = (await (await fetch(api + "/__test/otp")).json()).code;
  const inputs = page.locator('input[inputmode="numeric"]');
  for (let i = 0; i < 6; i++) await inputs.nth(i).fill(code[i]);
  await page.getByRole("button", { name: "Verify", exact: true }).click();
  await page.waitForURL("**/setup/create-company");
  await page.getByLabel("Legal company name").fill("Perf Logistics Limited");
  await page.getByLabel("Registration number").fill("RC-PERF-ONBOARD");
  await page.getByLabel("Company type").selectOption("PRIVATE_LIMITED");
  await page.getByLabel("Your full name").fill("Perf Owner");
  await page.getByRole("button", { name: "Save & continue", exact: true }).click();
  await page.waitForURL("**/setup/headquarters");

  const pdf = Buffer.from("%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF");
  const file = (name) => ({ name, mimeType: "application/pdf", buffer: pdf });

  const steps = 'nav[aria-label="Setup steps"]';
  const refs = () => [...document.querySelectorAll("p")]
    .map((p) => p.textContent ?? "")
    .filter((t) => t.startsWith("File reference:"));
  for (const profile of ["staging", "slow4g"]) {
    const m = {};
    results.measurements[profile] = m;
    await throttle(profile);

    if (profile === "staging") {
      m.coldLoadNoDocuments = await coldLoad(emptyCount(4));
      m.uploads = [];
      for (const [i, type] of types.entries()) {
        m.uploads.push(
          await timed(
            () => section(type).locator('input[type="file"]').first().setInputFiles(file("perf.pdf")),
            readyCount(i + 1),
          ),
        );
      }
    }

    // Cold load with every required document, with a request timeline for the
    // first run and a median over five.
    timeline = { t0: Date.now(), entries: [] };
    const first = await coldLoad(readyCount(4));
    m.coldLoadTimeline = { visibleAt: first.ms, requests: timeline.entries };
    timeline = null;
    const loads = [];
    for (let i = 0; i < 5; i++) loads.push(await coldLoad(readyCount(4)));
    m.coldLoadAllDocuments = {
      medianMs: median(loads.map((l) => l.ms)),
      runsMs: loads.map((l) => l.ms),
      apiRequests: loads[0].apiRequests,
      byEndpoint: loads[0].byEndpoint,
    };

    // Client-side navigation away and back through the step list.
    if (process.env.FLEET_PERF_IDLE_BEFORE_NAV) await page.waitForTimeout(Number(process.env.FLEET_PERF_IDLE_BEFORE_NAV));
    m.navigateToServices = await timed(
      () => page.locator(steps).getByRole("button", { name: "Services you offer" }).click(),
      () => location.pathname === "/setup/capabilities" && !!document.querySelector('input[type="checkbox"]'),
    );
    m.navigateBackToDocuments = await timed(
      () => page.locator(steps).getByRole("button", { name: "Documents" }).click(),
      new Function(`return () => location.pathname === "/setup/business-docs" && (${readyCount(4).toString()})()`)(),
    );

    if (profile === "staging") {
      const tax = section("Tax Clearance");
      const before = await page.evaluate(refs);
      m.replace = await timed(
        () => tax.locator('input[type="file"]').first().setInputFiles(file("replacement.pdf")),
        new Function("old", `return [...document.querySelectorAll("p")].map((p) => p.textContent ?? "").filter((t) => t.startsWith("File reference:")).filter((t) => !old.includes(t)).length === 1 && document.body.innerText.split("Status: Ready").length - 1 === 4`),
        before,
      );
      const insurance = section("Insurance Policy");
      await insurance.getByRole("button", { name: "Remove", exact: true }).click();
      m.remove = await timed(
        () => insurance.getByRole("button", { name: "Confirm removal", exact: true }).click(),
        emptyCount(1),
      );
      m.reuploadAfterRemove = await timed(
        () => insurance.locator('input[type="file"]').first().setInputFiles(file("again.pdf")),
        readyCount(4),
      );
      const previewButton = page.getByRole("button", { name: /^Preview/ });
      if (await previewButton.count()) {
        m.previewOpen = await timed(
          () => previewButton.first().click(),
          () => !!document.querySelector('[role="dialog"] object[data^="blob:"], [role="dialog"] img[src^="blob:"]'),
          undefined,
          500,
        );
        m.previewClose = await timed(
          () => page.getByRole("button", { name: "Close preview" }).click(),
          () => !document.querySelector('[role="dialog"][aria-modal="true"][aria-label$="preview"]'),
          undefined,
          500,
        );
      }
      m.saveAndContinue = await timed(
        () => page.getByRole("button", { name: "Save & continue", exact: true }).click(),
        () => location.pathname === "/setup/submit" && !!document.querySelector("h1"),
      );
    }
    await unthrottle();
  }

  // Logout and sign back in, then load documents again (fresh in-memory state).
  await page.goto(origin + "/setup/business-docs", { waitUntil: "commit" });
  await waitCount(ready, 4);
  await page.getByRole("button", { name: "Sign out", exact: true }).first().click();
  await page.waitForURL("**/sign-in");
  await fetch(api + "/__test/cooldown", { method: "POST" });
  await page.getByLabel("Email address or phone number").fill("perf-owner@example.com");
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.waitForURL("**/verify?**");
  const code2 = (await (await fetch(api + "/__test/otp")).json()).code;
  for (let i = 0; i < 6; i++) await inputs.nth(i).fill(code2[i]);
  await page.getByRole("button", { name: "Verify", exact: true }).click();
  await page.waitForURL(/\/setup/);
  await throttle("staging");
  results.measurements.afterReSignIn = { coldLoadAllDocuments: await coldLoad(readyCount(4)) };
  await unthrottle();

  await writeFile(`${evidence}/${label}.json`, JSON.stringify(results, null, 2));
  console.log(JSON.stringify(results));
} catch (e) {
  console.error(output.slice(-4000));
  throw e;
} finally {
  await browser?.close();
  server.kill("SIGTERM");
}
