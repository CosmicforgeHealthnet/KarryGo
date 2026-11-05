// lib/hooks/useContactAuthForm.ts

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { authService } from "@/lib/services/authService";
import { useAuthStore } from "@/lib/stores/authStore";
import {
  DEFAULT_PHONE_COUNTRY,
  emailIssue,
  maskEmail,
  normalizePhone,
  type FieldIssue,
} from "@/lib/auth/contact";
import { authErrorMessage, classifyAuthError, type AuthError } from "@/lib/auth/authErrors";

export type ContactAuthState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "error"; error: AuthError };

export interface ContactFieldErrors {
  email?: string;
  phone?: string;
}

function emailMessage(issue: FieldIssue | null): string | undefined {
  if (issue === "missing") return authErrorMessage("missing_email");
  if (issue === "invalid") return authErrorMessage("invalid_email");
  return undefined;
}

function phoneMessage(issue: FieldIssue | null): string | undefined {
  if (issue === "missing") return authErrorMessage("missing_phone");
  if (issue === "invalid") return authErrorMessage("invalid_phone");
  return undefined;
}

interface UseContactAuthFormOptions {
  invitationToken?: string;
}

export function useContactAuthForm({ invitationToken }: UseContactAuthFormOptions = {}) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [phone, setPhone] = useState("");
  const [country, setCountry] = useState(DEFAULT_PHONE_COUNTRY);
  const [touched, setTouched] = useState({ email: false, phone: false });
  const [serverFieldErrors, setServerFieldErrors] = useState<ContactFieldErrors>({});
  const [state, setState] = useState<ContactAuthState>({ status: "idle" });
  const { setLoading, setError } = useAuthStore();

  const emailProblem = emailIssue(email);
  const phoneResult = normalizePhone(phone, country);
  const phoneProblem = phoneResult.ok ? null : phoneResult.issue;
  const canSubmit = emailProblem === null && phoneProblem === null;
  const submitting = state.status === "submitting";

  // A field's own error shows once the user has left it; a server-reported
  // error shows until they edit that field again.
  const fieldErrors: ContactFieldErrors = {
    email: serverFieldErrors.email ?? (touched.email ? emailMessage(emailProblem) : undefined),
    phone: serverFieldErrors.phone ?? (touched.phone ? phoneMessage(phoneProblem) : undefined),
  };

  function clearFormError() {
    if (state.status === "error") setState({ status: "idle" });
  }

  function handleEmailChange(value: string) {
    setEmail(value);
    setServerFieldErrors((e) => ({ ...e, email: undefined }));
    clearFormError();
  }

  function handlePhoneChange(value: string) {
    setPhone(value);
    setServerFieldErrors((e) => ({ ...e, phone: undefined }));
    clearFormError();
  }

  function handleCountryChange(iso: string) {
    setCountry(iso);
    setServerFieldErrors((e) => ({ ...e, phone: undefined }));
    clearFormError();
  }

  function handleBlur(field: "email" | "phone") {
    setTouched((t) => ({ ...t, [field]: true }));
  }

  async function handleSubmit(e?: FormEvent) {
    e?.preventDefault();
    if (submitting) return;

    setTouched({ email: true, phone: true });
    if (!canSubmit || !phoneResult.ok) return;

    setState({ status: "submitting" });
    setServerFieldErrors({});
    setLoading(true);
    setError(null);

    const normalizedEmail = email.trim();
    try {
      const response = await authService.startOTP({
        email: normalizedEmail,
        phone: phoneResult.e164,
        ...(invitationToken ? { invitationToken } : {}),
      });

      if (!response?.challengeId) {
        throw new Error("The server did not return a verification challenge.");
      }

      setLoading(false);
      const destination = response.maskedDestination || maskEmail(normalizedEmail);
      const params = new URLSearchParams({ challengeId: response.challengeId, destination });
      if (response.expiresInSeconds) params.set("expiresIn", String(response.expiresInSeconds));
      if (response.resendAfterSeconds) params.set("resendIn", String(response.resendAfterSeconds));
      router.push(`/verify?${params.toString()}`);
    } catch (err) {
      const error = classifyAuthError(err);
      if (error.kind === "invalid_email" || error.kind === "missing_email") {
        setServerFieldErrors({ email: error.message });
      } else if (error.kind === "invalid_phone" || error.kind === "missing_phone") {
        setServerFieldErrors({ phone: error.message });
      }
      if (error.code || error.requestId) {
        // Kept for support; never shown as the primary message.
        console.warn("auth start failed", { code: error.code, status: error.status, requestId: error.requestId });
      }
      setState({ status: "error", error });
      setError(error.message);
      setLoading(false);
    }
  }

  return {
    email,
    phone,
    country,
    state,
    submitting,
    canSubmit,
    fieldErrors,
    normalizedPhone: phoneResult.ok ? phoneResult.e164 : null,
    handleEmailChange,
    handlePhoneChange,
    handleCountryChange,
    handleBlur,
    handleSubmit,
  };
}
