// components/features/auth/ContactAuthForm.tsx
//
// Shared email + phone form for sign-up and sign-in.

"use client";

import { Mail, Phone, ArrowRight, RotateCw } from "lucide-react";
import { PHONE_COUNTRIES, findPhoneCountry } from "@/lib/auth/contact";
import { useContactAuthForm } from "@/lib/hooks/useContactAuthForm";

const INPUT =
  "h-[58px] w-full rounded-[12px] border bg-[var(--field-bg)] text-[20px] text-[var(--ink-950)] outline-none transition placeholder:text-[var(--ink-400)] focus:border-[var(--label-accent)] focus:shadow-[0_0_0_4px_rgba(0,194,168,0.14)] disabled:opacity-50";
const LABEL = "mb-3 block text-[clamp(14px,1.1vw,18px)] font-bold text-[var(--ink-950)]";

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
    <form onSubmit={handleSubmit} noValidate className="mt-[clamp(28px,5vh,52px)] space-y-[clamp(18px,3vh,28px)]">
      <div>
        <label htmlFor={emailId} className={LABEL}>
          Work email
        </label>
        <div className="relative flex items-center">
          <Mail className="pointer-events-none absolute left-4 h-[22px] w-[22px] text-[var(--ink-500)]" />
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
            className={`${INPUT} pl-[56px] pr-4 ${fieldErrors.email ? "border-[var(--red)]" : "border-[var(--field-border)]"}`}
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
            className={`${INPUT} border-[var(--field-border)] px-3 sm:w-[132px] sm:shrink-0`}
          >
            {PHONE_COUNTRIES.map((c) => (
              <option key={c.iso} value={c.iso}>
                {c.flag} +{c.dialCode}
              </option>
            ))}
          </select>
          <div className="relative flex flex-1 items-center">
            <Phone className="pointer-events-none absolute left-4 h-[22px] w-[22px] text-[var(--ink-500)]" />
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
              className={`${INPUT} pl-[56px] pr-4 ${fieldErrors.phone ? "border-[var(--red)]" : "border-[var(--field-border)]"}`}
            />
          </div>
        </div>
        {(fieldErrors.phone || normalizedPhone) && (
          <p
            id={`${phoneId}-hint`}
            className={`mt-2 text-[13px] ${fieldErrors.phone ? "text-[var(--red)]" : "text-[var(--ink-500)]"}`}
          >
            {fieldErrors.phone ?? `We'll use ${normalizedPhone}.`}
          </p>
        )}
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
        className="!mt-[clamp(32px,6.5vh,64px)] flex h-[clamp(56px,7.4vh,70px)] w-full items-center justify-center gap-2.5 rounded-[12px] bg-[var(--label-accent)] px-5 text-[21px] font-bold text-[var(--surface)] transition hover:brightness-110 active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-50"
      >
        {submitting && <RotateCw className="h-5 w-5 animate-spin" />}
        {submitting ? "Sending code…" : "Continue"}
        {!submitting && <ArrowRight className="h-[22px] w-[22px]" />}
      </button>
    </form>
  );
}
