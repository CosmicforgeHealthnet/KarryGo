import { spawnSync } from "node:child_process";
import { strict as assert } from "node:assert";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const script = fileURLToPath(new URL("./check-production-env.mjs", import.meta.url));
function check(overrides) {
  return spawnSync(process.execPath, [script], {
    encoding: "utf8",
    env: { ...process.env, VERCEL_ENV: "production", DATABASE_URL: "", ...overrides },
  });
}

test("production fails without a database connection", () => {
  const result = check({});
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /DATABASE_URL/);
});

test("production rejects a malformed connection", () => {
  assert.notEqual(check({ DATABASE_URL: "https://not-a-database.example" }).status, 0);
});

test("production accepts a valid Postgres connection string", () => {
  assert.equal(
    check({ DATABASE_URL: "postgresql://user:password@host.example/karryam?sslmode=require" }).status,
    0,
  );
});

test("local and preview builds are unaffected", () => {
  assert.equal(check({ VERCEL_ENV: "preview" }).status, 0);
  assert.equal(check({ VERCEL_ENV: "development" }).status, 0);
});
