import { create } from "zustand";
import type { OnboardingStep } from "@/types";

interface OnboardingState {
  step: OnboardingStep;
  email: string;
  businessName: string;
  businessPhone: string;
  businessEmail: string;
  state: string;
  city: string;
  logoFile: File | null;
  photoFile: File | null;
  setStep: (step: OnboardingStep) => void;
  setEmail: (email: string) => void;
  setBusinessInfo: (info: {
    businessName: string;
    businessPhone: string;
    businessEmail: string;
    state: string;
    city: string;
  }) => void;
  setLogoFile: (file: File | null) => void;
  setPhotoFile: (file: File | null) => void;
  reset: () => void;
}

const initialState = {
  step: "business-info" as OnboardingStep,
  email: "",
  businessName: "",
  businessPhone: "",
  businessEmail: "",
  state: "",
  city: "",
  logoFile: null,
  photoFile: null,
};

export const useOnboardingStore = create<OnboardingState>((set) => ({
  ...initialState,
  setStep: (step) => set({ step }),
  setEmail: (email) => set({ email }),
  setBusinessInfo: (info) => set(info),
  setLogoFile: (logoFile) => set({ logoFile }),
  setPhotoFile: (photoFile) => set({ photoFile }),
  reset: () => set(initialState),
}));