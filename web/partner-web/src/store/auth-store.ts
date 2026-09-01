import { create } from "zustand";
import type { AuthAccount, AuthSession } from "@/types";

interface AuthState {
  token: string | null;
  expiresAt: string | null;
  account: AuthAccount | null;
  isAuthenticated: boolean;
  _hasHydrated: boolean;
  setSession: (session: AuthSession) => void;
  clearSession: () => void;
  setHasHydrated: (state: boolean) => void;
}

// Runtime memory only. Credentials must never be persisted or storage-hydrated.
export const useAuthStore = create<AuthState>()(
    (set) => ({
      token: null,
      expiresAt: null,
      account: null,
      isAuthenticated: false,
      _hasHydrated: false,
      setSession: ({ token, expires_at, account }) =>
        set({ token, expiresAt: expires_at, account, isAuthenticated: true }),
      clearSession: () =>
        set({ token: null, expiresAt: null, account: null, isAuthenticated: false }),
      setHasHydrated: (state) => set({ _hasHydrated: state }),
    })
);
