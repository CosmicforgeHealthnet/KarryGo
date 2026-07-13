import { create } from "zustand";
import type { OnboardingStep } from "@/types";

interface OnboardingState {
  step: OnboardingStep;
  dialCode: string;
  phone: string;
  businessName: string;
  businessPhone: string;
  businessEmail: string;
  state: string;
  city: string;
  logoFile: File | null;
  photoFile: File | null;
  setStep: (step: OnboardingStep) => void;
  setPhone: (dialCode: string, phone: string) => void;
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
  dialCode: "+234",
  phone: "",
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
  setPhone: (dialCode, phone) => set({ dialCode, phone }),
  setBusinessInfo: (info) => set(info),
  setLogoFile: (logoFile) => set({ logoFile }),
  setPhotoFile: (photoFile) => set({ photoFile }),
  reset: () => set(initialState),
}));