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
  token?: string
): Promise<T> {
  const headers = new Headers(options.headers);
  headers.set("Content-Type", "application/json");
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const res = await fetch(`${API_BASE_URL}${path}`, {
    ...options,
    headers,
  });

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