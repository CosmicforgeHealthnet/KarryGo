// src/app/(auth)/sign-up/page.tsx

"use client";

import { Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Mail, Phone, ArrowRight, ShieldCheck } from "lucide-react";
import { useContactAuthForm } from "@/lib/hooks/useContactAuthForm";

function SignUpContent() {
  const searchParams = useSearchParams();
  const invitationToken = searchParams.get("invitationToken") || undefined;

  const {
    email,
    phone,
    state,
    submitting,
    canSubmit,
    handleEmailChange,
    handlePhoneChange,
    handleSubmit,
  } = useContactAuthForm({ invitationToken });

  return (
    <div>
      <h1 className="font-display text-[40px] font-light leading-tight tracking-[-0.02em] text-ink-950">
        Create your account
      </h1>
      <p className="mt-3 text-[16px] text-ink-600">
        Enter your email and phone number to get started. You&apos;ll set up
        your company details next.
      </p>

      <form onSubmit={handleSubmit} noValidate className="mt-9 space-y-5">
        <div>
          <label
            htmlFor="signup-email"
            className="mb-2 block text-[13px] font-medium uppercase tracking-wide text-ink-500"
          >
            Work email
          </label>
          <div className="relative flex items-center">
            <Mail className="pointer-events-none absolute left-3.5 h-[18px] w-[18px] text-ink-400" />
            <input
              id="signup-email"
              type="email"
              autoComplete="username"
              value={email}
              onChange={(e) => handleEmailChange(e.target.value)}
              placeholder="you@company.com"
              disabled={submitting}
              className="w-full rounded-lg border border-line bg-surface-2 py-3 pl-11 pr-4 text-[16px] text-ink-950 outline-none transition placeholder:text-ink-500/60 focus:border-primary focus:bg-surface disabled:opacity-50"
            />
          </div>
        </div>

        <div>
          <label
            htmlFor="signup-phone"
            className="mb-2 block text-[13px] font-medium uppercase tracking-wide text-ink-500"
          >
            Phone number
          </label>
          <div className="relative flex items-center">
            <Phone className="pointer-events-none absolute left-3.5 h-[18px] w-[18px] text-ink-400" />
            <input
              id="signup-phone"
              type="tel"
              autoComplete="tel"
              value={phone}
              onChange={(e) => handlePhoneChange(e.target.value)}
              placeholder="+234 801 234 5678"
              disabled={submitting}
              className="w-full rounded-lg border border-line bg-surface-2 py-3 pl-11 pr-4 text-[16px] text-ink-950 outline-none transition placeholder:text-ink-500/60 focus:border-primary focus:bg-surface disabled:opacity-50"
            />
          </div>
        </div>

        {state.status === "error" && (
          <p role="alert" className="text-[14px] text-red">
            {state.kind === "invalid" &&
              "Enter a valid email address and phone number — both are required."}
            {state.kind === "rate_limited" &&
              "Too many attempts. Wait a moment before trying again."}
            {state.kind === "unavailable" &&
              "Sign-up is temporarily unavailable. Try again shortly."}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting || !canSubmit}
          className="flex w-full items-center justify-center gap-2 rounded-md bg-primary px-4 py-3.5 text-[15px] font-semibold text-white transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-40"
        >
          {submitting ? "Sending code…" : "Continue"}
          {!submitting && <ArrowRight className="h-4 w-4" />}
        </button>
      </form>

      <p className="mt-6 text-center text-[13.5px] text-ink-500">
        Already have an account?{" "}
        <Link href="/sign-in" className="font-semibold text-primary hover:underline">
          Sign in
        </Link>
      </p>

      <p className="mt-6 text-[13.5px] leading-relaxed text-ink-500">
        We&apos;ll send a one-time code to confirm it&apos;s you. Your contact
        details are never shared with other organizations.
      </p>

    </div>
  );
}

export default function SignUpPage() {
  return (
    <Suspense fallback={null}>
      <SignUpContent />
    </Suspense>
  );
}