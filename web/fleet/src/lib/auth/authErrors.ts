// lib/auth/authErrors.ts
//
// Turns a failed auth request into something the form can show. Decisions are
// made from the HTTP status and the server's structured error code, never by
// searching the human-readable message for words.
//
// Kept free of imports so it can be unit-tested with Node's built-in runner.

export type AuthErrorKind =
  | "invalid_email"
  | "missing_email"
  | "invalid_phone"
  | "missing_phone"
  | "invalid_request"
  | "configuration"
  | "unauthorized"
  | "rate_limited"
  | "delivery_failed"
  | "server_error"
  | "unavailable"
  | "network";

export interface AuthError {
  kind: AuthErrorKind;
  // Short text safe to show the user. Never contains internal details.
  message: string;
  // Kept for troubleshooting (shown in small print, logged by support).
  code?: string;
  requestId?: string;
  status?: number;
  retryable: boolean;
}

interface ServerFieldViolation {
  field?: string;
  message?: string;
}

interface ServerErrorBody {
  code?: string;
  message?: string;
  requestId?: string;
  request_id?: string;
  details?: ServerFieldViolation[];
  fields?: ServerFieldViolation[];
}

// Shape of an axios error, described structurally so this file needs no import.
interface HttpLikeError {
  response?: {
    status?: number;
    data?: { error?: ServerErrorBody } | string;
    headers?: Record<string, unknown>;
  };
  request?: unknown;
  code?: string;
  message?: string;
}

const MESSAGES: Record<AuthErrorKind, string> = {
  invalid_email: "Enter a valid email address, like you@company.com.",
  missing_email: "Enter your email address.",
  invalid_phone: "Enter a valid phone number for the selected country.",
  missing_phone: "Enter your phone number.",
  invalid_request: "Check your details and try again.",
  configuration:
    "Registration isn't available from this site right now because of a service configuration problem. Please try again later or contact support with the reference below.",
  unauthorized: "Your session has expired or the code wasn't accepted. Please start again.",
  rate_limited: "Too many attempts. Please wait a minute before trying again.",
  delivery_failed: "We couldn't send your verification code. Please try again in a moment.",
  server_error: "Something went wrong on our side. Please try again shortly.",
  unavailable: "The service is temporarily unavailable. Please try again shortly.",
  network: "We couldn't reach the server. Check your connection and try again.",
};

export function authErrorMessage(kind: AuthErrorKind): string {
  return MESSAGES[kind];
}

function build(kind: AuthErrorKind, extra: Partial<AuthError> = {}): AuthError {
  const retryable = !["invalid_email", "missing_email", "invalid_phone", "missing_phone", "invalid_request", "configuration"].includes(kind);
  return { kind, message: MESSAGES[kind], retryable, ...extra };
}

function fieldKind(violations: ServerFieldViolation[]): AuthErrorKind | null {
  for (const v of violations) {
    const field = (v.field ?? "").toLowerCase();
    const missing = /required/i.test(v.message ?? "") && !/valid/i.test(v.message ?? "");
    if (field === "phone") return missing ? "missing_phone" : "invalid_phone";
    if (field === "email") return missing ? "missing_email" : "invalid_email";
  }
  return null;
}

export function classifyAuthError(err: unknown): AuthError {
  const e = (err ?? {}) as HttpLikeError;
  const response = e.response;

  if (!response) {
    // No HTTP response at all: offline, DNS, CORS failure or timeout.
    return build("network", { code: e.code });
  }

  const status = response.status ?? 0;
  const body = typeof response.data === "object" && response.data ? response.data.error : undefined;
  const code = body?.code;
  const requestId = body?.requestId ?? body?.request_id;
  const meta = { code, requestId, status };

  if (code === "untrusted_origin") return build("configuration", meta);
  if (status === 429 || code === "rate_limited" || code === "authentication_rate_limited" || code === "otp_resend_too_soon") {
    return build("rate_limited", meta);
  }
  if (code === "otp_delivery_unavailable") return build("delivery_failed", meta);
  if (status === 422 || status === 400 || code === "validation_failed") {
    const kind = fieldKind([...(body?.details ?? []), ...(body?.fields ?? [])]);
    return build(kind ?? "invalid_request", meta);
  }
  if (status === 401) return build("unauthorized", meta);
  if (status === 403) return build("configuration", meta);
  if (status === 503 || status === 502 || status === 504) return build("unavailable", meta);
  if (status >= 500) return build("server_error", meta);
  return build("server_error", meta);
}
