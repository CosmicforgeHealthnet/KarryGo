// Classify verification errors by their stable backend code, not a
// generic 401 response or an inferred browser countdown.
// This module intentionally has no React imports so it can be regression-tested.
export type ChallengeFailure =
  | "invalid" | "expired" | "replaced" | "consumed" | "missing"
  | "access_denied" | "locked" | "network" | "contact_mismatch" | "account_unavailable";

export function challengeFailure(code: string | undefined, kind: string): ChallengeFailure {
  switch (code) {
    case "otp_challenge_expired": return "expired";
    case "otp_challenge_superseded": return "replaced";
    case "otp_challenge_consumed": return "consumed";
    case "otp_challenge_not_found": return "missing";
    case "otp_attempt_limit_exceeded": return "locked";
    case "identity_pair_conflict": return "contact_mismatch";
    case "identity_suspended": return "account_unavailable";
  }
  if (kind === "network") return "network";
  if (kind === "rate_limited") return "locked";
  if (kind === "configuration" || kind === "unavailable" || kind === "server_error") {
    return "access_denied";
  }
  return "invalid";
}
