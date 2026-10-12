import { isSegment, type Segment } from "./segments";

export type WaitlistInput = { email: string; segment: Segment };

type Result =
  | { ok: true; data: WaitlistInput }
  | { ok: false; error: string };

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/;

export function validateWaitlist(input: unknown): Result {
  const body = (input ?? {}) as Record<string, unknown>;
  const email = typeof body.email === "string" ? body.email.trim().toLowerCase() : "";

  if (!email || email.length > 254 || !EMAIL_RE.test(email)) {
    return { ok: false, error: "Enter a valid email address." };
  }
  if (!isSegment(body.segment)) {
    return { ok: false, error: "Choose what describes you." };
  }
  return { ok: true, data: { email, segment: body.segment } };
}