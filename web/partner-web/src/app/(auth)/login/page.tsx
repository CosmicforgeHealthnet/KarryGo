"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useOnboardingStore } from "@/store/onboarding-store";
import { authService } from "@/services/auth-service";
import { ApiError } from "@/lib/api-client";

export default function LoginPage() {
  const router = useRouter();
  const setEmail = useOnboardingStore((s) => s.setEmail);
  const [email, setLocalEmail] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const canContinue = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email);

  const handleContinue = async () => {
    if (!canContinue || loading) return;
    setError(null);
    setLoading(true);
    try {
      await authService.startLogin(email);
      setEmail(email);
      router.push("/login-otp-confirmation");
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

  return (
    <div>
      <h1 className="text-2xl font-bold text-ink-900">Welcome back!</h1>
      <p className="mt-1 text-sm font-semibold text-brand-500">
        Let&apos;s get you signed in.
      </p>
      <p className="mt-4 text-sm text-ink-500">
        Enter your business email to continue.
      </p>

      <div className="mt-6">
        <Input
          id="businessEmail"
          label="Business Email"
          type="email"
          placeholder="you@business.com"
          value={email}
          onChange={(e) => setLocalEmail(e.target.value)}
        />
        <p className="mt-2 text-xs text-brand-500">
          We&apos;ll send you a verification code.
        </p>
        {error && <p className="mt-2 text-xs text-red-500">{error}</p>}
      </div>

      <div className="mt-8">
        <Button onClick={handleContinue} disabled={!canContinue || loading}>
          {loading ? "Sending code..." : "Continue"}
        </Button>
      </div>

      <p className="mt-8 text-center text-sm text-ink-700">
        Don&apos;t have an account?{" "}
        <a href="/signup" className="font-semibold text-brand-500">
          Sign Up
        </a>
      </p>
    </div>
  );
}