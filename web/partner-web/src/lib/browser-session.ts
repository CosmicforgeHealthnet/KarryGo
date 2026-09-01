import { useAuthStore } from "@/store/auth-store";
import type { AuthSession } from "@/types";

const BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";
let refreshing: Promise<AuthSession> | null = null;
let restoring: Promise<void> | null = null;
let loggingOut = false;

// The browser/Gateway exclusively handle the cookie. Single-flight also covers
// simultaneous 401s; no refresh credential is read or stored by this module.
export function refreshSession(): Promise<AuthSession> {
  if (typeof window === "undefined" || loggingOut) return Promise.reject(new Error("Session unavailable"));
  if (refreshing) return refreshing;
  refreshing = (async () => {
    const response = await fetch(`${BASE}/api/v1/partner/v1/auth/refresh`, {
      method: "POST", credentials: "include", cache: "no-store",
    });
    if (!response.ok) {
      if (response.status === 401) useAuthStore.getState().clearSession();
      throw new Error("Session refresh failed");
    }
    const body = await response.json();
    if (!body.success || !body.data?.token || !body.data?.account) throw new Error("Invalid session response");
    if (!loggingOut) useAuthStore.getState().setSession(body.data);
    return body.data as AuthSession;
  })().finally(() => { refreshing = null; });
  return refreshing;
}

export function restoreSession(): Promise<void> {
  if (restoring) return restoring;
  restoring = (async () => {
    // Delete the obsolete persisted entry without reading its contents.
    try { window.localStorage.removeItem("karrygo-partner-auth"); } catch { /* storage may be disabled */ }
    try { await refreshSession(); } catch { useAuthStore.getState().clearSession(); }
    finally { useAuthStore.getState().setHasHydrated(true); }
  })();
  return restoring;
}

export async function logoutSession(): Promise<void> {
  loggingOut = true;
  useAuthStore.getState().clearSession();
  try {
    // A pending rotation must finish before its new cookie is revoked/deleted.
    await refreshing?.catch(() => undefined);
    const response = await fetch(`${BASE}/api/v1/partner/v1/auth/logout`, {
      method: "POST", credentials: "include", cache: "no-store",
    });
    if (!response.ok) throw new Error("Logout was not confirmed; please retry.");
  } finally {
    useAuthStore.getState().clearSession();
    loggingOut = false;
  }
}
