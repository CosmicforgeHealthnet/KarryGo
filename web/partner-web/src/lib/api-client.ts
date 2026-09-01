import { refreshSession } from "@/lib/browser-session";
import { useAuthStore } from "@/store/auth-store";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

export interface ApiFieldError {
  field: string;
  message: string;
}

export class ApiError extends Error {
  code: string;
  status: number;
  fields: ApiFieldError[];
  meta: Record<string, unknown>;

  constructor(
    status: number,
    code: string,
    message: string,
    fields: ApiFieldError[] = [],
    meta: Record<string, unknown> = {}
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.fields = fields;
    this.meta = meta;
  }
}

interface ApiSuccessEnvelope<T> {
  success: true;
  data?: T;
  message?: string;
}

interface ApiErrorEnvelope {
  success: false;
  error: {
    code: string;
    message: string;
    fields?: ApiFieldError[];
    [key: string]: unknown;
  };
}

async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
  token?: string,
  retried = false
): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Content-Type", "application/json");
  const currentToken = token && useAuthStore.getState().token;
  if (currentToken) {
    headers.set("Authorization", `Bearer ${currentToken}`);
  }

  const res = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    headers,
    credentials: "include",
    cache: "no-store",
  });

  if (res.status === 401 && currentToken && !retried) {
    let next: string;
    try {
      // A delayed 401 for the previous token must not rotate again.
      const latest = useAuthStore.getState().token;
      next = latest && latest !== currentToken ? latest : (await refreshSession()).token;
    } catch {
      useAuthStore.getState().clearSession();
      throw new ApiError(401, "session_expired", "Please sign in again.");
    }
    return apiFetch<T>(path, options, next, true);
  }
  if (res.status === 401 && retried) useAuthStore.getState().clearSession();

  let body: ApiSuccessEnvelope<T> | ApiErrorEnvelope;
  try {
    body = await res.json();
  } catch {
    throw new ApiError(res.status, "invalid_response", "Server returned an invalid response.");
  }

  if (!res.ok || body.success === false) {
    const errBody = body as ApiErrorEnvelope;
    const { code, message, fields, ...meta } = errBody.error ?? {
      code: "unknown_error",
      message: "Something went wrong.",
    };
    throw new ApiError(res.status, code, message, fields ?? [], meta);
  }

  return (body as ApiSuccessEnvelope<T>).data as T;
}

export const apiClient = {
  get: <T>(path: string, token?: string) => apiFetch<T>(path, { method: "GET" }, token),
  post: <T>(path: string, body?: unknown, token?: string) =>
    apiFetch<T>(path, { method: "POST", body: body ? JSON.stringify(body) : undefined }, token),
  patch: <T>(path: string, body?: unknown, token?: string) =>
    apiFetch<T>(path, { method: "PATCH", body: body ? JSON.stringify(body) : undefined }, token),
  delete: <T>(path: string, token?: string) => apiFetch<T>(path, { method: "DELETE" }, token),
};
