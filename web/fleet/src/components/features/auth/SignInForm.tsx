"use client";

import { useRef, useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowRight } from "lucide-react";
import { DEFAULT_PHONE_COUNTRY, PHONE_COUNTRIES, emailIssue, normalizePhone } from "@/lib/auth/contact";
import { classifyAuthError, type AuthError } from "@/lib/auth/authErrors";
import { authService } from "@/lib/services/authService";

export function SignInForm() {
  const router = useRouter();
  const params = useSearchParams();
  const [identifier, setIdentifier] = useState(params.get("identifier") ?? "");
  const [country, setCountry] = useState(DEFAULT_PHONE_COUNTRY);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<AuthError | null>(null);
  const [validation, setValidation] = useState("");
  const busy = useRef(false);
  const isPhone = /^[+\d(]/.test(identifier.trim()) && !identifier.includes("@");

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (busy.current) return;
    const phone = normalizePhone(identifier, country);
    if (isPhone ? !phone.ok : emailIssue(identifier) !== null) {
      setValidation(isPhone ? "Enter a valid phone number for the selected country." : "Enter a valid email address or phone number.");
      return;
    }
    busy.current = true;
    setSubmitting(true);
    setError(null);
    setValidation("");
    try {
      const result = await authService.signIn(isPhone && phone.ok ? phone.e164 : identifier.trim().toLowerCase());
      const query = new URLSearchParams({ challengeId: result.challengeId, destination: result.maskedDestination, intent: "sign-in" });
      if (result.expiresAt) query.set("expiresAt", result.expiresAt);
      if (result.resendAvailableAt) query.set("resendAt", result.resendAvailableAt);
      query.set("expiresIn", String(result.expiresInSeconds));
      query.set("resendIn", String(result.resendAfterSeconds));
      router.push(`/verify?${query}`);
    } catch (failure) {
      setError(classifyAuthError(failure));
    } finally {
      busy.current = false;
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate className="mt-9 space-y-5">
      <div>
        <label htmlFor="signin-identifier" className="mb-3 block text-[clamp(14px,1.1vw,18px)] font-bold text-[var(--ink-950)]">Email address or phone number</label>
        <input id="signin-identifier" type="text" autoComplete="username" value={identifier}
          onChange={(event) => { setIdentifier(event.target.value); setError(null); setValidation(""); }}
          placeholder="Enter your email or phone number" disabled={submitting}
          aria-invalid={!!validation} aria-describedby={validation ? "signin-validation" : "signin-hint"}
          className="h-[58px] w-full rounded-[12px] border border-[var(--line)] bg-[var(--field-bg)] px-4 text-[16px] text-[var(--ink-950)] outline-none focus:border-[var(--label-accent)] disabled:opacity-50" />
        {isPhone && <div className="mt-3">
          <label htmlFor="signin-country" className="mb-1 block text-sm text-[var(--ink-600)]">Country code</label>
          <select id="signin-country" value={country} onChange={(event) => setCountry(event.target.value)} disabled={submitting}
            className="w-full rounded-lg border border-[var(--line)] bg-[var(--field-bg)] p-3 text-[var(--ink-950)]">
            {PHONE_COUNTRIES.map((item) => <option key={item.iso} value={item.iso}>{item.flag} {item.name} +{item.dialCode}</option>)}
          </select>
        </div>}
        <p id="signin-hint" className="mt-2 text-sm text-[var(--ink-500)]">Your code will go to the email registered with your account.</p>
        {validation && <p id="signin-validation" role="alert" className="mt-2 text-sm text-red">{validation}</p>}
      </div>
      {error && <div role="alert" className="rounded-lg border border-red/30 p-4 text-sm text-red">
        <p>{error.message}</p>
        {error.requestId && <p className="mt-2 text-xs">Reference: {error.requestId}</p>}
      </div>}
      <button type="submit" disabled={submitting || !identifier.trim()} aria-busy={submitting}
        className="flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 py-3.5 text-[15px] font-semibold text-white hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-40">
        {submitting ? "Requesting code…" : "Continue"}<ArrowRight className="h-4 w-4" />
      </button>
    </form>
  );
}
