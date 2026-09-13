// lib/stores/onboardingStore.ts

import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface CompanyData {
  legalName: string;
  tradingName: string;
  registrationNumber: string;
  countryCode: string;
  organizationType: string;
  timezone: string;
  defaultCurrency: string;
  ownerName: string;
  ownerEmail: string;
  ownerPhone: string;
}

export interface CapabilityData {
  code: string;
  enabled: boolean;
}

export interface DocumentData {
  type: string;
  fileId: string;
  status: 'pending' | 'uploaded' | 'verified' | 'rejected';
}

export interface OnboardingState {
  organizationId: string | null;
  currentStep: number;
  companyData: Partial<CompanyData>;
  capabilities: CapabilityData[];
  documents: DocumentData[];
  verificationStatus: string | null;
  walletStatus: string | null;
  isSubmitted: boolean;
  isComplete: boolean;
  isLoading: boolean;
  error: string | null;
}

interface OnboardingStore extends OnboardingState {
  setOrganizationId: (id: string) => void;
  setCurrentStep: (step: number) => void;
  updateCompanyData: (data: Partial<CompanyData>) => void;
  setCapabilities: (capabilities: CapabilityData[]) => void;
  toggleCapability: (code: string, enabled: boolean) => void;
  addDocument: (document: DocumentData) => void;
  updateDocumentStatus: (fileId: string, status: DocumentData['status']) => void;
  setVerificationStatus: (status: string) => void;
  setWalletStatus: (status: string) => void;
  setSubmitted: (submitted: boolean) => void;
  setComplete: (complete: boolean) => void;
  setLoading: (isLoading: boolean) => void;
  setError: (error: string | null) => void;
  resetOnboarding: () => void;
}

const initialState: OnboardingState = {
  organizationId: null,
  currentStep: 0,
  companyData: {},
  capabilities: [],
  documents: [],
  verificationStatus: null,
  walletStatus: null,
  isSubmitted: false,
  isComplete: false,
  isLoading: false,
  error: null,
};

export const useOnboardingStore = create<OnboardingStore>()(
  persist(
    (set, get) => ({
      ...initialState,

      setOrganizationId: (id: string) => set({ organizationId: id }),

      setCurrentStep: (step: number) => set({ currentStep: step }),

      updateCompanyData: (data: Partial<CompanyData>) =>
        set((state) => ({
          companyData: { ...state.companyData, ...data },
        })),

      setCapabilities: (capabilities: CapabilityData[]) => set({ capabilities }),

      toggleCapability: (code: string, enabled: boolean) =>
        set((state) => ({
          capabilities: state.capabilities.map((cap) =>
            cap.code === code ? { ...cap, enabled } : cap
          ),
        })),

      addDocument: (document: DocumentData) =>
        set((state) => ({
          documents: [...state.documents, document],
        })),

      updateDocumentStatus: (fileId: string, status: DocumentData['status']) =>
        set((state) => ({
          documents: state.documents.map((doc) =>
            doc.fileId === fileId ? { ...doc, status } : doc
          ),
        })),

      setVerificationStatus: (status: string) => set({ verificationStatus: status }),

      setWalletStatus: (status: string) => set({ walletStatus: status }),

      setSubmitted: (submitted: boolean) => set({ isSubmitted: submitted }),

      setComplete: (complete: boolean) => set({ isComplete: complete }),

      setLoading: (isLoading: boolean) => set({ isLoading }),

      setError: (error: string | null) => set({ error }),

      resetOnboarding: () => set(initialState),
    }),
    {
      name: 'onboarding-storage',
      partialize: (state) => ({
        organizationId: state.organizationId,
        currentStep: state.currentStep,
        companyData: state.companyData,
        capabilities: state.capabilities,
        documents: state.documents,
        verificationStatus: state.verificationStatus,
        walletStatus: state.walletStatus,
        isSubmitted: state.isSubmitted,
        isComplete: state.isComplete,
      }),
    }
  )
);