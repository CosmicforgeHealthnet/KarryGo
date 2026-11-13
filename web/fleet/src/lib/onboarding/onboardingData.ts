// Pure onboarding data rules: which saved records each setup page needs, the
// in-memory snapshot cache, and how document previews are typed and explained.
// No imports, so the node test runner can exercise it directly.

export type ResourceKey =
  | "profile"
  | "representatives"
  | "capabilities"
  | "documents"
  | "status";

export const RESOURCE_KEYS: readonly ResourceKey[] = [
  "profile",
  "representatives",
  "capabilities",
  "documents",
  "status",
];

// The saved records a page reads before it can render. The step list in the
// shell always needs onboarding status. A page not listed here (the review
// page, for example) waits for everything.
const PAGE_RESOURCES: Record<string, readonly ResourceKey[]> = {
  "/setup": ["status"],
  "/setup/create-company": ["status"],
  "/setup/profile": ["status"],
  "/setup/headquarters": ["profile", "status"],
  "/setup/authorized-representative": ["representatives", "status"],
  "/setup/capabilities": ["capabilities", "status"],
  "/setup/business-docs": ["documents", "status"],
  "/setup/wallet-provisioning": ["status"],
};

export function resourcesForPath(pathname: string): readonly ResourceKey[] {
  const path = pathname.length > 1 ? pathname.replace(/\/+$/, "") : pathname;
  return PAGE_RESOURCES[path] ?? RESOURCE_KEYS;
}

// ---- In-memory snapshot cache ----
//
// One entry, for the identity and organization currently being onboarded.
// It lives only in this tab's memory: nothing is written to storage, a page
// reload starts empty, and the server stays authoritative because every cached
// view is revalidated on use. Entries are keyed by identity and organization so
// one organization's records can never be shown for another.

export type CacheEntry<T> = {
  scope: string;
  value: T;
};

export function cacheScope(
  identityId: string | null | undefined,
  organizationId: string | null | undefined,
): string | null {
  if (!identityId || !organizationId) return null;
  return `${identityId}:${organizationId}`;
}

export class ScopedCache<T> {
  private entry: CacheEntry<T> | null = null;

  read(scope: string | null): T | null {
    if (!scope || !this.entry || this.entry.scope !== scope) return null;
    return this.entry.value;
  }

  // Writing a different scope replaces the previous organization's entry.
  write(scope: string | null, value: T) {
    if (!scope) return;
    this.entry = { scope, value };
  }

  // Forget everything unless it belongs to the given scope.
  retainOnly(scope: string | null) {
    if (!scope || this.entry?.scope !== scope) this.entry = null;
  }

  clear() {
    this.entry = null;
  }
}

// ---- Document preview ----

export const PREVIEWABLE_TYPES: Record<string, "pdf" | "image"> = {
  "application/pdf": "pdf",
  "image/jpeg": "image",
  "image/png": "image",
  "image/webp": "image",
};

const EXTENSIONS: Record<string, string> = {
  "application/pdf": ".pdf",
  "image/jpeg": ".jpg",
  "image/png": ".png",
  "image/webp": ".webp",
};

// The kind of viewer for a response type, or null when the browser must not
// render it. The type is taken from the server's response (Media's detected
// type), not from the file name.
export function previewKind(contentType: string | null | undefined): "pdf" | "image" | null {
  const type = (contentType ?? "").split(";")[0].trim().toLowerCase();
  return PREVIEWABLE_TYPES[type] ?? null;
}

export function previewFilename(documentType: string, contentType: string): string {
  const type = contentType.split(";")[0].trim().toLowerCase();
  const base = documentType.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "document";
  return base + (EXTENSIONS[type] ?? "");
}

type PreviewError = {
  name?: string;
  code?: string;
  response?: {
    status?: number;
    data?: { error?: { message?: string; requestId?: string; request_id?: string } };
  };
};

// A message for the preview panel. The server's safe message is preferred for
// document-specific outcomes; transport failures get a local explanation.
export function previewErrorMessage(error: unknown): { message: string; requestId?: string } {
  const e = error as PreviewError;
  const status = e?.response?.status;
  const body = e?.response?.data?.error;
  const requestId = body?.requestId ?? body?.request_id;
  const server = body?.message;
  if (!e?.response) {
    return { message: "We couldn't connect. Check your connection and try again." };
  }
  switch (status) {
    case 401:
      return { message: "Your session expired. Sign in again to view this document.", requestId };
    case 403:
      return { message: "You do not have permission to view this document.", requestId };
    case 404:
      return { message: server || "This document is no longer available.", requestId };
    case 409:
      return { message: server || "This document is not ready to preview yet.", requestId };
    case 415:
      return { message: server || "This document's format cannot be previewed.", requestId };
    case 429:
      return { message: "Too many requests. Wait a moment and try again.", requestId };
    case 503:
      return { message: server || "Document preview is temporarily unavailable. Try again shortly.", requestId };
  }
  return { message: "The document could not be loaded. Please try again.", requestId };
}
