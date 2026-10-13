import { neon } from "@neondatabase/serverless";

const databaseUrl = process.env.DATABASE_URL?.trim();
if (!databaseUrl) {
  console.error("[waitlist] DATABASE_URL is missing. No connection was attempted.");
  process.exit(1);
}

try {
  const sql = neon(databaseUrl);
  // This statement cannot insert any rows. It verifies database connectivity,
  // insert authorization, the expected columns and the unique conflict target.
  await sql`
    INSERT INTO public.waitlist (email, segment)
    SELECT 'healthcheck@example.invalid', 'customer'
    WHERE FALSE
    ON CONFLICT (email, segment) DO NOTHING
  `;
  console.log("[waitlist] Database, table, INSERT privilege and unique index check passed. No rows written.");
} catch (error) {
  console.error(
    "[waitlist] Database check failed:",
    error instanceof Error ? error.message : "Unknown error",
  );
  process.exitCode = 1;
}
