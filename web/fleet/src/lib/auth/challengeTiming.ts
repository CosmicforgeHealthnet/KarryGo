// Compute OTP countdowns from the server's absolute deadlines.
// A page hidden on mobile may skip interval ticks; do not extend an OTP's life.
export function otpDeadline(iso: string | null | undefined, fallbackSeconds: number, now = Date.now()): number {
  const parsed = iso ? Date.parse(iso) : NaN;
  return Number.isFinite(parsed) ? parsed : now + fallbackSeconds * 1000;
}

export function remainingSeconds(deadlineMs: number, now = Date.now()): number {
  return Math.max(0, Math.ceil((deadlineMs - now) / 1000));
}
