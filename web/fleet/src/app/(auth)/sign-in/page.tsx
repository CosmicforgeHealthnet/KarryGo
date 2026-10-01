// src/app/(auth)/sign-in/page.tsx

"use client";

import Link from "next/link";
import { Mail, Phone, ArrowRight, ShieldCheck } from "lucide-react";
import { useContactAuthForm } from "@/lib/hooks/useContactAuthForm";

export default function SignInPage() {
  const {
    email,
    phone,
    state,
    submitting,
    canSubmit,
    handleEmailChange,
    handlePhoneChange,
    handleSubmit,
  } = useContactAuthForm();

  return (
    <div>
      <h1 className="font-display text-[40px] font-light leading-tight tracking-[-0.02em] text-[var(--ink-950)]">
        Sign in
      </h1>
      <p className="mt-3 text-[16px] text-[var(--ink-500)]">
        Enter the email and phone number on file for your account.
      </p>

      <form onSubmit={handleSubmit} noValidate className="mt-9 space-y-5">
        <div>
          <label
            htmlFor="signin-email"
            className="mb-2 block text-[13px] font-medium uppercase tracking-wide text-[var(--ink-500)]"
          >
            Work email
          </label>
          <div className="relative flex items-center">
            <Mail className="pointer-events-none absolute left-3.5 h-[18px] w-[18px] text-[var(--ink-500)]" />
            <input
              id="signin-email"
              type="email"
              autoComplete="username"
              value={email}
              onChange={(e) => handleEmailChange(e.target.value)}
              placeholder="you@company.com"
              disabled={submitting}
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] py-3 pl-11 pr-4 text-[16px] text-[var(--ink-950)] outline-none transition placeholder:text-[var(--ink-500)]/60 focus:border-[var(--primary)] focus:bg-[var(--surface)] focus:shadow-[0_0_0_3px_rgba(0,194,168,0.12)] disabled:opacity-50"
            />
          </div>
        </div>

        <div>
          <label
            htmlFor="signin-phone"
            className="mb-2 block text-[13px] font-medium uppercase tracking-wide text-[var(--ink-500)]"
          >
            Phone number
          </label>
          <div className="relative flex items-center">
            <Phone className="pointer-events-none absolute left-3.5 h-[18px] w-[18px] text-[var(--ink-500)]" />
            <input
              id="signin-phone"
              type="tel"
              autoComplete="tel"
              value={phone}
              onChange={(e) => handlePhoneChange(e.target.value)}
              placeholder="+234 801 234 5678"
              disabled={submitting}
              className="w-full rounded-lg border border-[var(--line)] bg-[var(--surface)] py-3 pl-11 pr-4 text-[16px] text-[var(--ink-950)] outline-none transition placeholder:text-[var(--ink-500)]/60 focus:border-[var(--primary)] focus:bg-[var(--surface)] focus:shadow-[0_0_0_3px_rgba(0,194,168,0.12)] disabled:opacity-50"
            />
          </div>
        </div>

        {state.status === "error" && (
          <p role="alert" className="text-[14px] text-[var(--red)]">
            {state.kind === "invalid" &&
              "Enter a valid email address and phone number — both are required."}
            {state.kind === "rate_limited" &&
              "Too many attempts. Wait a moment before trying again."}
            {state.kind === "unavailable" &&
              "Sign-in is temporarily unavailable. Try again shortly."}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting || !canSubmit}
          className="flex w-full items-center justify-center gap-2 rounded-md bg-[var(--primary)] px-4 py-3.5 text-[15px] font-semibold text-white transition hover:bg-[var(--primary-hover)] disabled:cursor-not-allowed disabled:opacity-40"
        >
          {submitting ? "Sending code…" : "Continue"}
          {!submitting && <ArrowRight className="h-4 w-4" />}
        </button>
      </form>

      <p className="mt-6 text-center text-[13.5px] text-[var(--ink-500)]">
        New to KarryGo?{" "}
        <Link
          href="/sign-up"
          className="font-semibold text-[var(--primary)] hover:underline"
        >
          Create an account
        </Link>
      </p>

      <p className="mt-6 text-[13.5px] leading-relaxed text-[var(--ink-500)]">
        We&apos;ll send a one-time code to confirm it&apos;s you. Your contact
        details are never shared with other organizations.
      </p>

      
    </div>
  );
}