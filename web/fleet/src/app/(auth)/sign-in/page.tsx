// src/app/(auth)/sign-in/page.tsx

"use client";

import Link from "next/link";
import { ContactAuthForm } from "@/components/features/auth/ContactAuthForm";

export default function SignInPage() {
  return (
    <div>
      <h1 className="font-display text-[40px] font-light leading-tight tracking-[-0.02em] text-[var(--ink-950)]">
        Sign in
      </h1>
      <p className="mt-3 text-[16px] text-[var(--ink-500)]">
        Enter the email and phone number on file for your account.
      </p>

      <ContactAuthForm idPrefix="signin" />

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