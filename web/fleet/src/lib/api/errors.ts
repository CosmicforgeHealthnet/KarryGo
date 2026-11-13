// A safe, user-facing description of a failed API request: the server's own
// message (the backend writes these for people), its error code and the
// request id support can trace. Internal 5xx details are never shown.

export interface ApiErrorInfo {
  message: string;
  code?: string;
  requestId?: string;
  status?: number;
}

type ErrorEnvelope = {
  error?: { code?: string; message?: string; requestId?: string; request_id?: string };
};

export function describeApiError(error: unknown, fallback = 'Something went wrong. Please try again.'): ApiErrorInfo {
  const e = error as {
    response?: { status?: number; data?: ErrorEnvelope; headers?: Record<string, unknown> };
  };
  if (!e?.response) {
    return { message: "We couldn't connect. Check your connection and try again." };
  }
  const status = e.response.status;
  const body = e.response.data?.error;
  const headerId = e.response.headers?.['x-request-id'];
  const requestId = body?.requestId ?? body?.request_id ?? (typeof headerId === 'string' ? headerId : undefined);
  if (status === 401) {
    return { message: 'Your session expired. Sign in again to continue.', status, requestId };
  }
  if (status && status >= 500 && status !== 503) {
    return { message: fallback, status, requestId, code: body?.code };
  }
  return { message: body?.message || fallback, code: body?.code, requestId, status };
}

// The error code of a failed request, if the server sent one.
export function apiErrorCode(error: unknown): string | undefined {
  return (error as { response?: { data?: ErrorEnvelope } })?.response?.data?.error?.code;
}

export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `idem-${Date.now()}-${Math.random().toString(36).slice(2, 11)}`;
}
