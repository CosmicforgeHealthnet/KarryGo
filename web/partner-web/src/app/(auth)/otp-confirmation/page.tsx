"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { OtpInput } from "@/components/auth/otp-input";
import { Button } from "@/components/ui/button";
import { useOtpTimer } from "@/hooks/use-otp-timer";
import { useOnboardingStore } from "@/store/onboarding-store";
import { useAuthStore } from "@/store/auth-store";
import { authService } from "@/services/auth-service";
import { ApiError } from "@/lib/api-client";

export default function OtpConfirmationPage() {
  const router = useRouter();
  const { email } = useOnboardingStore();
  const setSession = useAuthStore((s) => s.setSession);
  const [otp, setOtp] = useState<string[]>(Array(6).fill(""));
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [resending, setResending] = useState(false);
  const { formatted, canResend, reset } = useOtpTimer();

  useEffect(() => {
    if (!email) {
      router.replace("/signup");
    }
  }, [email, router]);

  const code = otp.join("");
  const canContinue = code.length === 6 && !!email;

  const handleContinue = async () => {
    if (!canContinue || loading) return;
    setError(null);
    setLoading(true);
    try {
      const session = await authService.verifySignup(email, code);
      setSession(session);
      router.push("/business-info");
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Something went wrong. Please try again.");
      }
    } finally {
      setLoading(false);
    }
  };

  const handleResend = async () => {
    if (!canResend || resending) return;
    setError(null);
    setResending(true);
    try {
      await authService.resendSignup(email);
      reset();
      setOtp(Array(6).fill(""));
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Could not resend code. Please try again.");
      }
    } finally {
      setResending(false);
    }
  };

  if (!email) {
    return null;
  }

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
        We sent you a 6-digit code via your email
      </p>
      <p className="text-sm font-semibold text-brand-500">
        {email || "your email"}
      </p>

      <div className="mt-8">
        <p className="mb-3 text-sm font-medium text-ink-900">Enter OTP</p>
        <OtpInput value={otp} onChange={setOtp} />
        {error && <p className="mt-2 text-xs text-red-500">{error}</p>}
      </div>

      <div className="mt-4 flex items-center justify-between text-sm">
        <button
          type="button"
          onClick={handleResend}
          disabled={!canResend || resending}
          className="font-semibold text-brand-500 disabled:text-ink-300"
        >
          {resending ? "Resending..." : "Resend Code"}
        </button>
        <span className="text-brand-500">{formatted}</span>
      </div>

      <div className="mt-10">
        <Button onClick={handleContinue} disabled={!canContinue || loading}>
          {loading ? "Verifying..." : "Continue"}
        </Button>
      </div>
    </div>
  );
}