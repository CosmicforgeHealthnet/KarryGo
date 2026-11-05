// components/features/auth/ContactAuthForm.tsx
//
// Shared email + phone form for sign-up and sign-in.

"use client";

import { Mail, Phone, ArrowRight, RotateCw } from "lucide-react";
import { PHONE_COUNTRIES, findPhoneCountry } from "@/lib/auth/contact";
import { useContactAuthForm } from "@/lib/hooks/useContactAuthForm";

const INPUT =
  "w-full rounded-lg border bg-[var(--surface)] py-3 text-[16px] text-[var(--ink-950)] outline-none transition placeholder:text-[var(--ink-500)]/60 focus:border-[var(--primary)] focus:shadow-[0_0_0_3px_rgba(0,194,168,0.12)] disabled:opacity-50";
const LABEL = "mb-2 block text-[13px] font-medium uppercase tracking-wide text-[var(--ink-500)]";

interface ContactAuthFormProps {
  idPrefix: string;
  invitationToken?: string;
}

export function ContactAuthForm({ idPrefix, invitationToken }: ContactAuthFormProps) {
  const {
    email,
    phone,
    country,
    state,
    submitting,
    canSubmit,
    fieldErrors,
    normalizedPhone,
    handleEmailChange,
    handlePhoneChange,
    handleCountryChange,
    handleBlur,
    handleSubmit,
  } = useContactAuthForm({ invitationToken });

  const selected = findPhoneCountry(country);
  const emailId = `${idPrefix}-email`;
  const phoneId = `${idPrefix}-phone`;
  const countryId = `${idPrefix}-country`;
  const formError = state.status === "error" ? state.error : null;
  // Field-level problems are shown next to the field, not repeated below.
  const showFormError =
    formError && !["invalid_email", "missing_email", "invalid_phone", "missing_phone"].includes(formError.kind);

  return (
    <form onSubmit={handleSubmit} noValidate className="mt-9 space-y-5">
      <div>
        <label htmlFor={emailId} className={LABEL}>
          Work email
        </label>
        <div className="relative flex items-center">
          <Mail className="pointer-events-none absolute left-3.5 h-[18px] w-[18px] text-[var(--ink-500)]" />
          <input
            id={emailId}
            type="email"
            inputMode="email"
            autoComplete="username"
            value={email}
            onChange={(e) => handleEmailChange(e.target.value)}
            onBlur={() => handleBlur("email")}
            placeholder="you@company.com"
            disabled={submitting}
            aria-invalid={!!fieldErrors.email}
            aria-describedby={fieldErrors.email ? `${emailId}-error` : undefined}
            className={`${INPUT} pl-11 pr-4 ${fieldErrors.email ? "border-[var(--red)]" : "border-[var(--line)]"}`}
          />
        </div>
        {fieldErrors.email && (
          <p id={`${emailId}-error`} className="mt-1.5 text-[13px] text-[var(--red)]">
            {fieldErrors.email}
          </p>
        )}
      </div>

      <div>
        <label htmlFor={phoneId} className={LABEL}>
          Phone number
        </label>
        <div className="flex flex-col gap-2 sm:flex-row">
          <label htmlFor={countryId} className="sr-only">
            Country code
          </label>
          <select
            id={countryId}
            value={country}
            onChange={(e) => handleCountryChange(e.target.value)}
            disabled={submitting}
            autoComplete="tel-country-code"
            className={`${INPUT} border-[var(--line)] px-3 sm:w-[150px] sm:shrink-0`}
          >
            {PHONE_COUNTRIES.map((c) => (
              <option key={c.iso} value={c.iso}>
                {c.flag} {c.iso} +{c.dialCode}
              </option>
            ))}
          </select>
          <div className="relative flex flex-1 items-center">
            <Phone className="pointer-events-none absolute left-3.5 h-[18px] w-[18px] text-[var(--ink-500)]" />
            <input
              id={phoneId}
              type="tel"
              inputMode="tel"
              autoComplete="tel-national"
              value={phone}
              onChange={(e) => handlePhoneChange(e.target.value)}
              onBlur={() => handleBlur("phone")}
              placeholder={selected.iso === "NG" ? "0801 234 5678" : "Phone number"}
              disabled={submitting}
              aria-invalid={!!fieldErrors.phone}
              aria-describedby={`${phoneId}-hint`}
              className={`${INPUT} pl-11 pr-4 ${fieldErrors.phone ? "border-[var(--red)]" : "border-[var(--line)]"}`}
            />
          </div>
        </div>
        <p
          id={`${phoneId}-hint`}
          className={`mt-1.5 text-[13px] ${fieldErrors.phone ? "text-[var(--red)]" : "text-[var(--ink-500)]"}`}
        >
          {fieldErrors.phone ??
            (normalizedPhone
              ? `We'll use ${normalizedPhone}.`
              : `${selected.name} (+${selected.dialCode}). For another country, pick it above or type the full number starting with +.`)}
        </p>
      </div>

      {showFormError && formError && (
        <div role="alert" className="rounded-lg border border-[var(--red)]/30 bg-[var(--red-bg)] px-4 py-3">
          <p className="text-[14px] text-[var(--red)]">{formError.message}</p>
          {formError.retryable && formError.kind !== "rate_limited" && (
            <button
              type="button"
              onClick={() => handleSubmit()}
              disabled={submitting}
              className="mt-2 inline-flex items-center gap-1.5 text-[13px] font-semibold text-[var(--red)] hover:underline disabled:opacity-50"
            >
              <RotateCw className="h-3.5 w-3.5" /> Try again
            </button>
          )}
          {(formError.code || formError.requestId) && (
            <p className="mt-2 text-[11.5px] text-[var(--ink-500)]">
              Reference: {[formError.code, formError.requestId].filter(Boolean).join(" · ")}
            </p>
          )}
        </div>
      )}

      <button
        type="submit"
        disabled={submitting || !canSubmit}
        aria-busy={submitting}
        className="flex w-full items-center justify-center gap-2 rounded-md bg-[var(--primary)] px-4 py-3.5 text-[15px] font-semibold text-white transition hover:bg-[var(--primary-hover)] disabled:cursor-not-allowed disabled:opacity-40"
      >
        {submitting ? "Sending code…" : "Continue"}
        {!submitting && <ArrowRight className="h-4 w-4" />}
      </button>
    </form>
  );
}
