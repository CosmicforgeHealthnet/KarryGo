// Request headers for commands. The backend marks some commands idempotent
// (they need an Idempotency-Key, kept stable across retries of one attempt)
// and versioned (they need If-Match with the version the user saw, so a
// change made by someone else in between is refused instead of overwritten).

export function commandHeaders(opts: { idempotencyKey?: string; expectedVersion?: number }) {
  const headers: Record<string, string> = {};
  if (opts.idempotencyKey) headers['Idempotency-Key'] = opts.idempotencyKey;
  if (opts.expectedVersion != null) headers['If-Match'] = String(opts.expectedVersion);
  return { headers };
}

export const enc = (value: string) => encodeURIComponent(value);

export function query(params: Record<string, string | number | boolean | undefined | null>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue;
    q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : '';
}
