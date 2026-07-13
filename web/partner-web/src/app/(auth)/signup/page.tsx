"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { PhoneInput } from "@/components/auth/phone-input";
import { Button } from "@/components/ui/button";

export default function SignupPage() {
  const router = useRouter();
  const [dialCode, setDialCode] = useState("+234");
  const [phone, setPhone] = useState("");

  const canContinue = phone.length >= 10;

  const handleContinue = () => {
    if (!canContinue) return;
    router.push("/otp-confirmation");
  };

  return (
    <div>
      <h1 className="text-2xl font-bold text-ink-900">Welcome to KarryGo!</h1>
      <p className="mt-1 text-sm font-semibold text-brand-500">
        Let&apos;s get you moving.
      </p>
      <p className="mt-4 text-sm text-ink-500">
        Enter your phone number to continue.
      </p>

      <div className="mt-6">
        <label className="mb-2 block text-sm font-medium text-ink-900">
          Enter your Phone Number
        </label>
        <PhoneInput
          value={phone}
          onChange={setPhone}
          dialCode={dialCode}
          onDialCodeChange={setDialCode}
        />
        <p className="mt-2 text-xs text-brand-500">
          We&apos;ll send you a verification code.
        </p>
      </div>

      <div className="mt-8">
        <Button onClick={handleContinue} disabled={!canContinue}>
          Continue
        </Button>
      </div>

      <div className="mt-6 flex items-center gap-3">
        <div className="h-px flex-1 bg-ink-100" />
        <span className="text-xs text-ink-300">or</span>
        <div className="h-px flex-1 bg-ink-100" />
      </div>

      <div className="mt-6 space-y-3">
        <Button variant="social" type="button">
          <GoogleIcon />
          Continue with Google
        </Button>
        <Button variant="social-dark" type="button">
          <AppleIcon />
          Continue with Apple
        </Button>
      </div>

      <p className="mt-8 text-center text-sm text-ink-700">
        Already have an account?{" "}
        <a href="/login" className="font-semibold text-brand-500">
          Log In
        </a>
      </p>
    </div>
  );
}

function GoogleIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden>
      <path fill="#4285F4" d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.9c1.7-1.57 2.7-3.88 2.7-6.62z" />
      <path fill="#34A853" d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.9-2.26c-.8.54-1.84.86-3.06.86-2.35 0-4.34-1.59-5.05-3.72H.95v2.33A9 9 0 0 0 9 18z" />
      <path fill="#FBBC05" d="M3.95 10.7A5.4 5.4 0 0 1 3.66 9c0-.59.1-1.17.29-1.7V4.97H.95A9 9 0 0 0 0 9c0 1.45.35 2.83.95 4.03l3-2.33z" />
      <path fill="#EA4335" d="M9 3.58c1.32 0 2.51.45 3.44 1.35l2.58-2.58C13.46.89 11.43 0 9 0A9 9 0 0 0 .95 4.97l3 2.33C4.66 5.17 6.65 3.58 9 3.58z" />
    </svg>
  );
}

function AppleIcon() {
  return (
    <svg width="16" height="18" viewBox="0 0 16 18" fill="white" aria-hidden>
      <path d="M13.15 9.53c-.02-1.9 1.55-2.82 1.62-2.86-.88-1.3-2.26-1.48-2.75-1.5-1.17-.12-2.28.69-2.87.69-.6 0-1.5-.67-2.47-.65-1.27.02-2.44.74-3.09 1.87-1.32 2.28-.34 5.66.94 7.51.63.9 1.37 1.92 2.35 1.88.94-.04 1.3-.6 2.44-.6s1.46.6 2.46.58c1.02-.02 1.66-.92 2.28-1.83.72-1.05 1.02-2.06 1.03-2.11-.02-.01-1.97-.75-1.99-3zM11.19 3.6c.52-.63.87-1.5.77-2.38-.75.03-1.66.5-2.2 1.12-.48.55-.9 1.44-.78 2.29.83.06 1.68-.42 2.21-1.03z" />
    </svg>
  );
}