// src/app/(auth)/sign-up/page.tsx

"use client";

import { Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { ContactAuthForm } from "@/components/features/auth/ContactAuthForm";

function SignUpContent() {
  const searchParams = useSearchParams();
  const invitationToken = searchParams.get("invitationToken") || undefined;

  return (
    <div>
      <span className="inline-flex rounded-[10px] bg-primary-bg px-3.5 py-2 text-[clamp(14px,1.08vw,18px)] font-bold text-label-accent">
        Get started
      </span>
      <h1 className="mt-4 text-[clamp(34px,3vw,50px)] font-bold leading-[1.1] tracking-[-0.035em] text-[var(--ink-950)]">
        Create your account
      </h1>
      <p className="mt-5 text-[clamp(15px,1.08vw,18px)] leading-relaxed text-ink-600">
        Enter your email and phone number to get started. You&apos;ll set up
        your company details next.
      </p>

      <ContactAuthForm idPrefix="signup" invitationToken={invitationToken} />

      <p className="mt-[38px] text-center text-[clamp(14px,1.15vw,19px)] text-ink-500">
        Already have an account?{" "}
        <Link href="/sign-in" className="font-bold text-label-accent hover:underline">
          Sign in
        </Link>
      </p>

      <p className="mt-5 text-center text-[13.5px] leading-relaxed text-ink-500">
        Already registered? Use your original email and phone number to sign in.
        We never create a second Fleet account for the same verified contact details.
        If your phone number has changed, contact support.
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