// lib/stores/authStore.ts

import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { AuthState, Organization, Tokens } from '@/lib/types/auth';
import { authService } from '@/lib/services/authService';

interface AuthStore extends AuthState {
  _hasHydrated: boolean;
  setHasHydrated: (v: boolean) => void;
  setAuth: (tokens: Tokens, identityId: string, organizations: Organization[]) => void;
  setTokensOnly: (tokens: Tokens) => void;
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

export const useAuthStore = create<AuthStore>()(
  persist(
    (set, get) => ({
      ...initialState,
      _hasHydrated: false,

      setHasHydrated: (v: boolean) => set({ _hasHydrated: v }),

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
      },

      setTokensOnly: (tokens: Tokens) => {
        set({
          accessToken: tokens.accessToken,
          refreshToken: tokens.refreshToken,
          sessionId: tokens.sessionId,
          accessExpiresAt: tokens.accessExpiresAt,
          refreshExpiresAt: tokens.refreshExpiresAt,
        });
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
        } catch (error) {
          set({ error: 'Failed to select organization', isLoading: false });
          throw error;
        }
      },

      logout: async () => {
        try {
          await authService.logout();
        } catch {
        } finally {
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
        refreshToken: state.refreshToken,
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