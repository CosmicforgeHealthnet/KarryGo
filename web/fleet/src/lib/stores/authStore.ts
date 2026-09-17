// lib/stores/authStore.ts

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { AuthState, Organization, Tokens } from '@/lib/types/auth';
import { authService } from '@/lib/services/authService';

interface AuthStore extends AuthState {
  _hasHydrated: boolean;
  setHasHydrated: (v: boolean) => void;
  setAuth: (tokens: Tokens, identityId: string, organizations: Organization[]) => void;
  setTokensOnly: (tokens: Omit<Tokens, 'refreshToken'>) => void;
  setOrganizations: (organizations: Organization[]) => void;
  selectOrganization: (membershipId: string) => Promise<void>;
  logout: () => Promise<void>;
  refreshSession: () => Promise<boolean>;
  clearError: () => void;
  setLoading: (isLoading: boolean) => void;
  setError: (error: string | null) => void;
}

const initialState: AuthState = {
  identityId: null,
  accessToken: null,
  refreshToken: null,
  sessionId: null,
  accessExpiresAt: null,
  refreshExpiresAt: null,
  organizations: [],
  selectedOrganization: null,
  isAuthenticated: false,
  isLoading: false,
  error: null,
};

// Proactive refresh: fire this many ms before accessExpiresAt so the token
// is renewed ahead of use instead of waiting for a request to 401 first.
const REFRESH_BUFFER_MS = 60_000;
const MIN_REFRESH_DELAY_MS = 5_000;

// Lives outside the store on purpose: a setTimeout handle isn't
// serializable and has no business in persisted or even in-memory zustand
// state. Module-scoped so it survives across store updates within one tab.
let refreshTimer: ReturnType<typeof setTimeout> | null = null;

function clearScheduledRefresh() {
  if (refreshTimer) {
    clearTimeout(refreshTimer);
    refreshTimer = null;
  }
}

function scheduleRefresh(accessExpiresAt: string | null) {
  clearScheduledRefresh();
  if (!accessExpiresAt) return;

  const expiresAtMs = new Date(accessExpiresAt).getTime();
  if (Number.isNaN(expiresAtMs)) return;

  const delay = Math.max(expiresAtMs - Date.now() - REFRESH_BUFFER_MS, MIN_REFRESH_DELAY_MS);

  refreshTimer = setTimeout(async () => {
    // Dynamic import avoids a circular top-level import between this file
    // and lib/api/client.ts (client.ts imports useAuthStore for
    // getState()/setTokensOnly()). By the time this callback actually
    // runs, both modules are fully evaluated, so this is safe.
    try {
      const { refreshAccessToken } = await import('@/lib/api/client');
      await refreshAccessToken();
      // On success, setTokensOnly (below) reschedules the next refresh.
    } catch {
      // refreshAccessToken already calls logout() internally on failure.
      // Nothing further to do here.
    }
  }, delay);
}

export const useAuthStore = create<AuthStore>()(
  persist(
    (set, get) => ({
      ...initialState,
      _hasHydrated: false,

      setHasHydrated: (v: boolean) => {
        set({ _hasHydrated: v });
        // On rehydrate (page load), pick up wherever the persisted
        // accessExpiresAt left off, so a fresh page load still schedules
        // a proactive refresh instead of only relying on the next 401.
        if (v) {
          scheduleRefresh(get().accessExpiresAt);
        }
      },

      setAuth: (tokens: Tokens, identityId: string, organizations: Organization[]) => {
        set({
          accessToken: tokens.accessToken,
          refreshToken: tokens.refreshToken,
          sessionId: tokens.sessionId,
          accessExpiresAt: tokens.accessExpiresAt,
          refreshExpiresAt: tokens.refreshExpiresAt,
          identityId,
          organizations,
          isAuthenticated: true,
          isLoading: false,
          error: null,
          selectedOrganization: organizations.length === 1 ? organizations[0] : null,
        });
        scheduleRefresh(tokens.accessExpiresAt);
      },

      setTokensOnly: (tokens: Omit<Tokens, 'refreshToken'>) => {
        set({
          accessToken: tokens.accessToken,
          sessionId: tokens.sessionId,
          accessExpiresAt: tokens.accessExpiresAt,
          refreshExpiresAt: tokens.refreshExpiresAt,
        });
        scheduleRefresh(tokens.accessExpiresAt);
      },

      setOrganizations: (organizations: Organization[]) => {
        set({ organizations });
      },

      selectOrganization: async (membershipId: string) => {
        try {
          set({ isLoading: true, error: null });
          const response = await authService.selectOrganization({ membershipId });
          const organization = get().organizations.find((org) => org.membershipId === membershipId);
          set({
            accessToken: response.accessToken,
            refreshToken: response.refreshToken,
            sessionId: response.sessionId,
            accessExpiresAt: response.accessExpiresAt,
            refreshExpiresAt: response.refreshExpiresAt,
            selectedOrganization: organization || null,
            isLoading: false,
          });
          scheduleRefresh(response.accessExpiresAt);
        } catch (error) {
          set({ error: 'Failed to select organization', isLoading: false });
          throw error;
        }
      },

    logout: async () => {
  clearScheduledRefresh();
  try {
    await authService.logout();
  } catch {
  } finally {
    document.cookie = "karrygo_has_session=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
    document.cookie = "karrygo_org_status=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
    set({ ...initialState, _hasHydrated: true });
  }
},
      refreshSession: async (): Promise<boolean> => {
        const token = get().accessToken;
        if (!token) {
          set({ isAuthenticated: false });
          return false;
        }
        set({ isAuthenticated: true });
        return true;
      },

      clearError: () => set({ error: null }),
      setLoading: (isLoading: boolean) => set({ isLoading }),
      setError: (error: string | null) => set({ error }),
    }),
    {
      name: 'auth-storage',
      partialize: (state) => ({
        accessToken: state.accessToken,
        sessionId: state.sessionId,
        accessExpiresAt: state.accessExpiresAt,
        refreshExpiresAt: state.refreshExpiresAt,
        identityId: state.identityId,
        organizations: state.organizations,
        selectedOrganization: state.selectedOrganization,
      }),
      onRehydrateStorage: () => (state) => {
        state?.setHasHydrated(true);
      },
    }
  )
);