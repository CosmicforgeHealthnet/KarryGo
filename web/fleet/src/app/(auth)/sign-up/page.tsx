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
      <h1 className="font-display text-[40px] font-light leading-tight tracking-[-0.02em] text-ink-950">
        Create your account
      </h1>
      <p className="mt-3 text-[16px] text-ink-600">
        Enter your email and phone number to get started. You&apos;ll set up
        your company details next.
      </p>

      <ContactAuthForm idPrefix="signup" invitationToken={invitationToken} />

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