// Vercel does not validate runtime integrations when it reports a build as READY.
// Block production promotion when the public waitlist database is unconfigured.
if (process.env.VERCEL_ENV === "production") {
  const raw = process.env.DATABASE_URL?.trim();
  let valid = false;
  if (raw) {
    try {
      const url = new URL(raw);
      valid =
        (url.protocol === "postgres:" || url.protocol === "postgresql:") &&
        Boolean(url.hostname && url.username && url.pathname.length > 1);
    } catch {
      valid = false;
    }
  }
  if (!valid) {
    console.error(
      "[karryam-landing] Refusing production build: configure a valid DATABASE_URL in Vercel Production.",
    );
    process.exitCode = 1;
  }
}
