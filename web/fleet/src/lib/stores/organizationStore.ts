// lib/stores/organizationStore.ts

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface Organization {
  organizationId: string;
  legalName: string;
  tradingName: string;
  status: 'DRAFT' | 'PENDING' | 'BUSINESS_SUBMITTED' | 'UNDER_REVIEW' | 'APPROVED' | 'REJECTED' | 'ACTIVE' | 'SUSPENDED' | 'CLOSED' | 'CHANGES_REQUIRED';
  countryCode: string;
  timezone: string;
  defaultCurrency: string;
  version: number;
}

interface OrganizationStore {
  organization: Organization | null;
  isLoading: boolean;
  error: string | null;
  setOrganization: (org: Organization) => void;
  setLoading: (isLoading: boolean) => void;
  setError: (error: string | null) => void;
  clearOrganization: () => void;
  canPerformActions: () => boolean;
  isPending: () => boolean;
  isDraft: () => boolean;
  isApproved: () => boolean;
  isSubmitted: () => boolean;
  isUnderReview: () => boolean;
  isChangesRequired: () => boolean;
}

export const useOrganizationStore = create<OrganizationStore>()(
  persist(
    (set, get) => ({
      organization: null,
      isLoading: false,
      error: null,

      setOrganization: (org: Organization) => set({ organization: org, error: null }),
      setLoading: (isLoading: boolean) => set({ isLoading }),
      setError: (error: string | null) => set({ error }),
      clearOrganization: () => set({ organization: null, error: null }),

      canPerformActions: () => {
        const org = get().organization;
        if (!org) return false;
        return org.status === 'APPROVED' || org.status === 'ACTIVE';
      },

      isPending: () => {
        const org = get().organization;
        if (!org) return false;
        return org.status === 'BUSINESS_SUBMITTED' || 
               org.status === 'UNDER_REVIEW';
      },

      isDraft: () => {
        const org = get().organization;
        if (!org) return false;
        return org.status === 'DRAFT';
      },

      isApproved: () => {
        const org = get().organization;
        if (!org) return false;
        return org.status === 'APPROVED' || org.status === 'ACTIVE';
      },

      isSubmitted: () => {
        const org = get().organization;
        if (!org) return false;
        return org.status === 'BUSINESS_SUBMITTED';
      },

      
      isUnderReview: () => {
        const org = get().organization;
        if (!org) return false;
        return org.status === 'UNDER_REVIEW';
      },

      // ✅ New: Check if changes required
      isChangesRequired: () => {
        const org = get().organization;
        if (!org) return false;
        return org.status === 'CHANGES_REQUIRED';
      },
    }),
    {
      name: 'organization-storage',
      partialize: (state) => ({
        organization: state.organization,
      }),
    }
  )
);