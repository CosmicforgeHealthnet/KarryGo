import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { AuthAccount } from "@/types";

interface AuthState {
  token: string | null;
  expiresAt: string | null;
  account: AuthAccount | null;
  isAuthenticated: boolean;
  _hasHydrated: boolean; // ADD THIS
  setSession: (session: { token: string; expiresAt: string; account: AuthAccount }) => void;
  clearSession: () => void;
  setHasHydrated: (state: boolean) => void; // ADD THIS
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      expiresAt: null,
      account: null,
      isAuthenticated: false,
      _hasHydrated: false, // ADD THIS
      setSession: ({ token, expiresAt, account }) =>
        set({ token, expiresAt, account, isAuthenticated: true }),
      clearSession: () =>
        set({ token: null, expiresAt: null, account: null, isAuthenticated: false }),
      setHasHydrated: (state) => set({ _hasHydrated: state }), // ADD THIS
    }),
    {
      name: "karrygo-partner-auth",
      onRehydrateStorage: () => (state) => {
        state?.setHasHydrated(true);
      },
    }
  )
);