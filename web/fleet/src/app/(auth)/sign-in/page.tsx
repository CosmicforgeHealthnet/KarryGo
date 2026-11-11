// src/app/(auth)/sign-in/page.tsx

"use client";

import Link from "next/link";
import { Suspense } from "react";
import { SignInForm } from "@/components/features/auth/SignInForm";

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
        Enter your email address or phone number. We’ll email your sign-in code.
      </p>

      <Suspense fallback={null}><SignInForm /></Suspense>

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