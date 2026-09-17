// lib/hooks/useContactAuthForm.ts

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { authService } from "@/lib/services/authService";
import { useAuthStore } from "@/lib/stores/authStore";

export type ContactAuthErrorKind = "invalid" | "rate_limited" | "unavailable";

export type ContactAuthState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "error"; kind: ContactAuthErrorKind };

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
const PHONE_PATTERN = /^\+?[0-9]{7,15}$/;

function isValidEmail(value: string) {
  return EMAIL_PATTERN.test(value.trim());
}

function isValidPhone(value: string) {
  return PHONE_PATTERN.test(value.replace(/[\s()-]/g, ""));
}

function normalizePhone(value: string) {
  const cleaned = value.replace(/[\s()-]/g, "");
  return cleaned.startsWith("+") ? cleaned : `+${cleaned}`;
}

function maskEmail(value: string) {
  const [local, domain] = value.split("@");
  if (!domain) return value;
  const visible = local.slice(0, 1);
  return `${visible}${"*".repeat(Math.max(local.length - 1, 1))}@${domain}`;
}

interface UseContactAuthFormOptions {
  invitationToken?: string;
}

export function useContactAuthForm({ invitationToken }: UseContactAuthFormOptions = {}) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [state, setState] = useState<ContactAuthState>({ status: "idle" });
  const { setLoading, setError } = useAuthStore();

  const emailValid = isValidEmail(email);
  const phoneValid = isValidPhone(phone);
  const canSubmit = emailValid && phoneValid;
  const submitting = state.status === "submitting";

  function resetErrorOnEdit() {
    if (state.status === "error") setState({ status: "idle" });
  }

  function handleEmailChange(value: string) {
    setEmail(value);
    resetErrorOnEdit();
  }

  function handlePhoneChange(value: string) {
    setPhone(value);
    resetErrorOnEdit();
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();

    if (!canSubmit) {
      setState({ status: "error", kind: "invalid" });
      return;
    }

    setState({ status: "submitting" });
    setLoading(true);
    setError(null);

    try {
      const normalizedEmail = email.trim();
      const normalizedPhone = normalizePhone(phone);

      const response = await authService.startOTP({
        email: normalizedEmail,
        phone: normalizedPhone,
        ...(invitationToken ? { invitationToken } : {}),
      });

      if (!response.challengeId) {
        throw new Error("No challengeId received");
      }

      router.push(
        `/verify?challengeId=${response.challengeId}&destination=${encodeURIComponent(maskEmail(normalizedEmail))}`
      );
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Something went wrong. Please try again.";
      const lower = message.toLowerCase();
      if (lower.includes("rate") || lower.includes("too many")) {
        setState({ status: "error", kind: "rate_limited" });
      } else if (lower.includes("valid email") || lower.includes("required")) {
        setState({ status: "error", kind: "invalid" });
      } else {
        setState({ status: "error", kind: "unavailable" });
      }
      setError(message);
      setLoading(false);
    }
  }

  return {
    email,
    phone,
    state,
    submitting,
    canSubmit,
    handleEmailChange,
    handlePhoneChange,
    handleSubmit,
  };
}