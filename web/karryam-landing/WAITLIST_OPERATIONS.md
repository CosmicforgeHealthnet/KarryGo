# Karryam landing waitlist — deployment requirements

The public `/api/waitlist/start` endpoint inserts records into PostgreSQL.
A successful Vercel build does **not** validate that database or its schema.

## Provision/restore service

1. In Neon, create or select the dedicated landing-page PostgreSQL database.
2. Run `db/001_waitlist.sql` in that database's SQL editor. If a pre-existing
   waitlist table contains duplicate `(email, segment)` pairs, inspect and
   reconcile duplicates before applying the unique index.
3. Add **DATABASE_URL** to the **Production** environment of the Vercel project
   **karryam-landing**. Use a server-side Postgres connection string with TLS,
   never a `NEXT_PUBLIC_` variable. Do not commit or share the password.
4. Redeploy the target commit (or promote the reviewed fix from staging).
5. With the same database connection string in your local process environment,
   run `npm run waitlist:check` from `web/karryam-landing`. This check validates
   the insert path with zero inserted records.
6. Submit a real test email using each role on the public waitlist. Confirm the
   API returns success and that rows are present in Neon before declaring fixed.

## Regression controls

- `npm run test:deploy`: checks that production builds reject missing or
  invalid database configuration without blocking local/preview builds.
- `npm run build`: production builds are blocked when `DATABASE_URL`
  is absent/malformed. Keep the previous functioning deployment live instead
  of promoting an unconfigured one.
- `npm run waitlist:check`: verify connectivity, schema, INSERT permission and
  the unique index **after** creating the database and before promotion.

**Do not merge into the auto-deployed staging branch until the production
DATABASE_URL and schema have been provisioned and the checks pass.** Configure
the required database first, then merge/redeploy; otherwise registrations
remain unavailable. This change adds protections, but cannot create Neon
credentials or validate a database it has no access to.
