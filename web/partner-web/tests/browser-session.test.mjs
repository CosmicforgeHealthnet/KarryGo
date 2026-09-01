import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
const nodeRequire = createRequire(import.meta.url);
const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Execute the actual TS clients/store with mocked browser transport, not a copy
// of their logic. No new test dependency, persistent credentials, or build output.
function runtime() {
  const cache = new Map();
  const removed = [];
  const env = { fetch: () => { throw Error('unexpected fetch'); } };
  const browser = { localStorage: { removeItem: key => removed.push(key), getItem: () => { throw Error('persistent token read'); }, setItem: () => { throw Error('persistent token write'); } } };
  function load(file) {
    if (!file.startsWith('@/')) return nodeRequire(file);
    const filename = path.join(__dirname, '../src', file.slice(2) + '.ts');
    if (cache.has(filename)) return cache.get(filename).exports;
    const mod = { exports: {} }; cache.set(filename, mod);
    const compiled = ts.transpileModule(fs.readFileSync(filename, 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText;
    new Function('require', 'module', 'exports', 'process', 'window', 'fetch', compiled)(load, mod, mod.exports, { env: { NEXT_PUBLIC_API_BASE_URL: 'https://api.example.test' } }, browser, (...args) => env.fetch(...args));
    return mod.exports;
  }
  return { env, load, removed, store: load('@/store/auth-store').useAuthStore };
}
const session = token => ({ token, expires_at: '2099-01-01T00:00:00Z', account: { id: 'partner-a' } });
const ok = data => new Response(JSON.stringify({ success: true, data }), { status: 200 });
const denied = status => new Response(JSON.stringify({ success: false, error: { code: 'denied', message: 'Denied' } }), { status });

test('reload restores via one cookie refresh and deletes only legacy storage', async () => {
  const r = runtime(); const api = r.load('@/lib/browser-session'); let calls = 0;
  r.env.fetch = async (url, opts) => { calls++; assert.match(url, /auth\/refresh$/); assert.equal(opts.credentials, 'include'); assert.equal(opts.body, undefined); return ok(session('fresh')); };
  await Promise.all([api.restoreSession(), api.restoreSession(), api.refreshSession()]);
  assert.equal(calls, 1); assert.equal(r.store.getState().token, 'fresh'); assert.equal(r.store.getState()._hasHydrated, true);
  assert.deepEqual(r.removed, ['karrygo-partner-auth']);
});

test('login and OTP verification include cookies and never request refresh JSON', async () => {
  const r = runtime(); const auth = r.load('@/services/auth-service').authService; const paths = [];
  r.env.fetch = async (url, opts) => { paths.push(url); assert.equal(opts.credentials, 'include'); assert.doesNotMatch(opts.body ?? '', /refreshToken|refresh_token/); return ok(session('access')); };
  await auth.login('a@example.test', 'test-password'); await auth.verifyLogin('a@example.test', '123456'); await auth.verifySignup('a@example.test', '123456');
  assert.equal(paths.length, 3);
});

test('concurrent 401s refresh once, bearer retries once, business errors stay business errors', async () => {
  const r = runtime(); r.store.getState().setSession(session('old')); const api = r.load('@/lib/api-client').apiClient; let refreshes = 0; let attempts = 0;
  r.env.fetch = async (url, opts) => {
    if (url.endsWith('/refresh')) { refreshes++; return ok(session('new')); }
    attempts++; return opts.headers.get('Authorization') === 'Bearer old' ? denied(401) : ok({ accepted: true });
  };
  await Promise.all([api.get('/api/v1/partner/v1/bookings', 'old'), api.get('/api/v1/partner/v1/bookings', 'old')]);
  assert.equal(refreshes, 1); assert.equal(attempts, 4);
  r.env.fetch = async () => denied(403);
  await assert.rejects(api.get('/private', 'new'), err => err.status === 403);
  assert.equal(r.store.getState().token, 'new');
});

test('refresh 401 clears memory and does not loop; retry 401 also clears memory', async () => {
  const r = runtime(); const api = r.load('@/lib/api-client').apiClient; r.store.getState().setSession(session('old')); let calls = 0;
  r.env.fetch = async () => { calls++; return denied(401); };
  await assert.rejects(api.get('/private', 'old'));
  assert.equal(calls, 2); assert.equal(r.store.getState().token, null);
  r.store.getState().setSession(session('old')); calls = 0;
  r.env.fetch = async url => { calls++; return url.endsWith('/refresh') ? ok(session('new')) : denied(401); };
  await assert.rejects(api.get('/private', 'old'));
  assert.equal(calls, 3); assert.equal(r.store.getState().token, null);
});

test('logout waits for rotation, revokes replacement cookie and clears all memory', async () => {
  const r = runtime(); const api = r.load('@/lib/browser-session'); let finish; const calls = [];
  r.env.fetch = (url, opts) => { assert.equal(opts.credentials, 'include'); calls.push(url); return url.endsWith('/refresh') ? new Promise(resolve => { finish = resolve; }) : Promise.resolve(ok({})); };
  const refresh = api.refreshSession(); const logout = api.logoutSession();
  assert.equal(calls.length, 1); finish(ok(session('late')));
  await Promise.all([refresh, logout]);
  assert.match(calls[1], /auth\/logout$/); assert.equal(r.store.getState().token, null); assert.equal(r.store.getState().account, null);
  await api.logoutSession(); assert.equal(calls.length, 3);
});

test('credential persistence and JS cookie APIs absent from auth implementation', () => {
  const store = fs.readFileSync(path.join(__dirname, '../src/store/auth-store.ts'), 'utf8');
  assert.doesNotMatch(store, /persist\(|localStorage|sessionStorage|indexedDB|document\.cookie/);
  const client = fs.readFileSync(path.join(__dirname, '../src/lib/browser-session.ts'), 'utf8');
  assert.doesNotMatch(client, /\.setItem\(|\.getItem\(|document\.cookie|indexedDB|sessionStorage/);
  assert.match(fs.readFileSync(path.join(__dirname, '../src/components/session-bootstrap.tsx'), 'utf8'), /useEffect\(.*restoreSession/);
});
