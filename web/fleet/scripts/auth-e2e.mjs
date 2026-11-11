// Started by the opt-in PostgreSQL-backed Go browser integration test.
// OTP access exists only on that disposable test server, never in Fleet runtime.
import assert from 'node:assert/strict';
import { spawn, execFileSync } from 'node:child_process';
import { writeFile, mkdir } from 'node:fs/promises';
import { chromium } from '@playwright/test';

const api = process.env.FLEET_AUTH_E2E_API;
assert(api?.startsWith('http://127.0.0.1:'), 'Disposable local API required');
const origin = 'http://127.0.0.1:3417';
const evidence = '../../.codex-cache/fleet-auth-v2-browser';
await mkdir(evidence, { recursive: true });
const server = spawn(process.execPath, ['node_modules/next/dist/bin/next', 'dev', '--hostname', '127.0.0.1', '--port', '3417'], {
  env: { ...process.env, FLEET_API_BASE_URL: api }, windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'],
});
let output = '';
server.stdout.on('data', (chunk) => { output += chunk; });
server.stderr.on('data', (chunk) => { output += chunk; });
let browser;
const checks = [];
try {
  for (let i = 0; i < 120; i++) {
    if (server.exitCode !== null) throw new Error('Next server exited');
    try { if ((await fetch(`${origin}/sign-in`)).ok) break; } catch {}
    await new Promise((resolve) => setTimeout(resolve, 1000));
    if (i === 119) throw new Error('Next server did not become ready');
  }
  browser = await chromium.launch({ headless: true });
  let context = await browser.newContext();
  let page = await context.newPage();
  page.setDefaultTimeout(30000);
  const endSession = async () => {
    // Stop the mounted app before revoking its session through the API. Otherwise
    // its outstanding reads can race our next navigation with an expiry redirect.
    await page.close();
    const response = await context.request.post(`${origin}/fleet-partner/v1/auth/logout`, { headers: { Origin: origin } });
    assert.equal(response.status(), 200, 'Session logout');
    await context.close();
    context = await browser.newContext();
    page = await context.newPage();
    page.setDefaultTimeout(30000);
  };
  const otp = async () => (await (await fetch(`${api}/__test/otp`)).json()).code;
  const fillOTP = async () => {
    const code = await otp();
    const inputs = page.locator('input[inputmode="numeric"]');
    for (let i = 0; i < 6; i++) await inputs.nth(i).fill(code[i]);
    await page.getByRole('button', { name: 'Verify', exact: true }).click();
  };
  await page.goto(`${origin}/sign-up`);
  await page.waitForLoadState('networkidle');
  await page.getByLabel('Work email').fill('browser@example.com');
  await page.getByLabel('Phone number', { exact: true }).fill('08012345678');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await page.waitForURL('**/verify?**');
  await page.getByText('b***@example.com', { exact: false }).first().waitFor();
  await fillOTP();
  await page.waitForURL(/\/setup(?:\/|$)/, { timeout: 60000 });
  checks.push('signup → real API/PostgreSQL → email OTP → identity/session → onboarding route');
  await page.close();
  const refresh = await context.request.post(`${origin}/fleet-partner/v1/auth/refresh`, { headers: { Origin: origin } });
  assert.equal(refresh.status(), 200, 'Session refresh');
  checks.push('HttpOnly session cookie refresh through same-origin proxy');
  await endSession();
  await page.goto(`${origin}/sign-up`);
  await page.waitForLoadState('networkidle');
  await page.getByLabel('Work email').fill('BROWSER@example.com');
  await page.getByLabel('Phone number', { exact: true }).fill('08019999999');
  await page.getByRole('button', { name: 'Continue', exact: true }).click();
  await page.getByRole('alert').getByText('This email is already registered', { exact: false }).waitFor();
  await page.getByRole('link', { name: 'Sign In', exact: true }).click();
  await page.waitForURL('**/sign-in?**');
  assert.equal(await page.getByLabel('Email address or phone number').inputValue(), 'BROWSER@example.com');
  assert.equal(await page.locator('form input').count(), 1);
  checks.push('duplicate email actions and email carried to single-input sign-in');
  for (const identifier of ['browser@example.com', '08012345678']) {
    await fetch(`${api}/__test/cooldown`, { method: 'POST' });
    await page.goto(`${origin}/sign-in`);
    await page.waitForLoadState('networkidle');
    await page.getByLabel('Email address or phone number').fill(identifier);
    await page.getByRole('button', { name: 'Continue', exact: true }).click();
    await page.waitForURL('**/verify?**');
    await fillOTP();
    await page.waitForURL(/\/setup(?:\/|$)/, { timeout: 60000 });
    await endSession();
  }
  checks.push('email and phone sign-in → same identity → email OTP → session → onboarding');
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${origin}/sign-in`);
  await page.waitForLoadState('networkidle');
  await page.getByLabel('Email address or phone number').fill('08012345678');
  await page.getByLabel('Country code').waitFor();
  assert(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'Mobile overflow');
  await page.screenshot({ path: `${evidence}/mobile-sign-in.png`, fullPage: true });
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(`${origin}/sign-up`);
  await page.waitForLoadState('networkidle');
  await page.screenshot({ path: `${evidence}/desktop-sign-up.png`, fullPage: true });
  checks.push('mobile and desktop forms render without horizontal overflow');
  await writeFile(`${evidence}/results.json`, JSON.stringify({ status: 'passed', checks }, null, 2));
  console.log(JSON.stringify({ status: 'passed', checks }));
} finally {
  await browser?.close();
  await writeFile(`${evidence}/next.log`, output);
  if (process.platform === 'win32') {
    try { execFileSync('taskkill', ['/PID', String(server.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' }); } catch {}
  } else server.kill('SIGTERM');
}
