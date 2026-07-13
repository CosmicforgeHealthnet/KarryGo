"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { OtpInput } from "@/components/auth/otp-input";
import { Button } from "@/components/ui/button";
import { useOtpTimer } from "@/hooks/use-otp-timer";
import { useOnboardingStore } from "@/store/onboarding-store";

export default function OtpConfirmationPage() {
  const router = useRouter();
  const { dialCode, phone } = useOnboardingStore();
  const [otp, setOtp] = useState<string[]>(Array(6).fill(""));
  const { formatted, canResend, reset } = useOtpTimer();

  const code = otp.join("");
  const canContinue = code.length === 6;

  const handleContinue = () => {
    if (!canContinue) return;
    router.push("/business-info");
  };

  const handleResend = () => {
    if (!canResend) return;
    reset();
    setOtp(Array(6).fill(""));
  };

  return (
    <div>
      <Link
        href="/signup"
        className="mb-4 inline-flex h-8 w-8 items-center justify-center text-ink-900"
        aria-label="Back"
      >
        <ArrowLeft className="h-5 w-5" />
      </Link>

      <h1 className="text-2xl font-bold text-ink-900">OTP Confirmation!</h1>
      <p className="mt-2 text-sm text-ink-500">
        We sent you a 6-digit code via your number
      </p>
      <p className="text-sm font-semibold text-brand-500">
        {dialCode} {phone || "1234 5676"}
      </p>

      <div className="mt-8">
        <p className="mb-3 text-sm font-medium text-ink-900">Enter OTP</p>
        <OtpInput value={otp} onChange={setOtp} />
      </div>

      <div className="mt-4 flex items-center justify-between text-sm">
        <button
          type="button"
          onClick={handleResend}
          disabled={!canResend}
          className="font-semibold text-brand-500 disabled:text-ink-300"
        >
          Resend Code
        </button>
        <span className="text-brand-500">{formatted}</span>
      </div>

      <div className="mt-10">
        <Button onClick={handleContinue} disabled={!canContinue}>
          Continue
        </Button>
      </div>
    </div>
  );
}