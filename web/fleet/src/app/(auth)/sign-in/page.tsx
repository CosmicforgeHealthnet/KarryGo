// src/app/(auth)/sign-in/page.tsx

"use client";

import Link from "next/link";
import { ContactAuthForm } from "@/components/features/auth/ContactAuthForm";

export default function SignInPage() {
  return (
    <div>
      <span className="inline-flex rounded-[10px] bg-[var(--primary-bg)] px-3.5 py-2 text-[clamp(14px,1.08vw,18px)] font-bold text-[var(--label-accent)]">
        Welcome back
      </span>
      <h1 className="mt-4 text-[clamp(34px,3vw,50px)] font-bold leading-[1.1] tracking-[-0.035em] text-[var(--ink-950)]">
        Sign in to Karryam
      </h1>
      <p className="mt-5 text-[clamp(15px,1.08vw,18px)] leading-relaxed text-[var(--ink-600)]">
        Enter the email and phone number on file for your account.
      </p>

      <ContactAuthForm idPrefix="signin" />

      <p className="mt-[38px] text-center text-[clamp(14px,1.15vw,19px)] text-[var(--ink-500)]">
        New to Karryam?{" "}
        <Link
          href="/sign-up"
          className="font-bold text-[var(--label-accent)] hover:underline"
        >
          Create an account
        </Link>
      </p>


      
    </div>
  );
}