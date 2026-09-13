"use client";

import { Suspense, useEffect, useRef, useState, type FormEvent, type KeyboardEvent, type ClipboardEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, Mail, ShieldCheck, ShieldAlert, Lock, RotateCw, CheckCircle2 } from "lucide-react";
import { authService } from "@/lib/services/authService";
import { useAuthStore } from "@/lib/stores/authStore";

const CODE_LENGTH = 6;
const RESEND_SECONDS = 30;
const EXPIRY_SECONDS = 300;
const MAX_ATTEMPTS = 5;

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "success" }
  | { status: "error"; kind: "invalid" | "expired" | "access_denied" | "locked" };

function VerifyContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const challengeId = searchParams.get("challengeId") || "";
  const destination = searchParams.get("destination") ?? "your contact";

  const [digits, setDigits] = useState<string[]>(Array(CODE_LENGTH).fill(""));
  const [state, setState] = useState<SubmitState>({ status: "idle" });
  const [resendIn, setResendIn] = useState(RESEND_SECONDS);
  const [expiresIn, setExpiresIn] = useState(EXPIRY_SECONDS);
  const [attempts, setAttempts] = useState(0);
  const [justResent, setJustResent] = useState(false);

  const inputRefs = useRef<Array<HTMLInputElement | null>>([]);

  const { setAuth, setLoading, setError } = useAuthStore();

  useEffect(() => {
    const timer = setInterval(() => {
      setResendIn((s) => Math.max(0, s - 1));
      setExpiresIn((s) => Math.max(0, s - 1));
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  const code = digits.join("");
  const submitting = state.status === "submitting";
  const success = state.status === "success";
  const expired = expiresIn === 0;
  const locked = state.status === "error" && state.kind === "locked";
  const hasError = state.status === "error";

  async function submitCode(candidate: string) {
    if (expired) {
      setState({ status: "error", kind: "expired" });
      return;
    }

    if (!challengeId) {
      setState({ status: "error", kind: "access_denied" });
      return;
    }

    if (submitting || success) return;

    setState({ status: "submitting" });
    setLoading(true);
    setError(null);

    try {
      const response = await authService.verifyOTP({
        challengeId,
        code: candidate,
      });

      const data = response.data;

      if (data?.nextStep === "CREATE_ORGANIZATION" || data?.nextStep === "ONBOARDING_REQUIRED") {
        if (data?.tokens) {
          setAuth(data.tokens, data.identityId, data.organizations || []);
        }
        setState({ status: "success" });
        setTimeout(() => {
          router.push("/setup");
        }, 800);
        return;
      }

      if (data?.nextStep === "SELECT_ORGANIZATION" && data?.tokens) {
        setAuth(data.tokens, data.identityId, data.organizations || []);
        setState({ status: "success" });
        setTimeout(() => {
          router.push("/choose-organization");
        }, 800);
        return;
      }

      setState({ status: "error", kind: "access_denied" });
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "";
      
      if (message.toLowerCase().includes("expired")) {
        setState({ status: "error", kind: "expired" });
      } else {
        const nextAttempts = attempts + 1;
        setAttempts(nextAttempts);
        if (nextAttempts >= MAX_ATTEMPTS) {
          setState({ status: "error", kind: "locked" });
        } else {
          setState({ status: "error", kind: "invalid" });
        }
      }
      setError(message);
    } finally {
      setState((prev) => prev.status === "success" ? prev : { ...prev, status: "idle" });
      setLoading(false);
    }
  }

  function updateDigit(index: number, value: string) {
    if (success || submitting) return;
    
    const clean = value.replace(/\D/g, "");
    if (!clean) {
      setDigits((prev) => {
        const next = [...prev];
        next[index] = "";
        return next;
      });
      return;
    }

    setDigits((prev) => {
      const next = [...prev];
      next[index] = clean[clean.length - 1];
      return next;
    });

    if (hasError && !locked) setState({ status: "idle" });

    if (index < CODE_LENGTH - 1) {
      inputRefs.current[index + 1]?.focus();
    }
  }

  function handleKeyDown(index: number, e: KeyboardEvent<HTMLInputElement>) {
    if (success || submitting) return;
    
    if (e.key === "Backspace") {
      if (digits[index]) {
        updateDigit(index, "");
      } else if (index > 0) {
        inputRefs.current[index - 1]?.focus();
        updateDigit(index - 1, "");
      }
    } else if (e.key === "ArrowLeft" && index > 0) {
      inputRefs.current[index - 1]?.focus();
    } else if (e.key === "ArrowRight" && index < CODE_LENGTH - 1) {
      inputRefs.current[index + 1]?.focus();
    }
  }

  function handlePaste(e: ClipboardEvent<HTMLInputElement>) {
    if (success || submitting) return;
    
    e.preventDefault();
    const pasted = e.clipboardData.getData("text").replace(/\D/g, "").slice(0, CODE_LENGTH);
    if (!pasted) return;

    const next = Array(CODE_LENGTH).fill("");
    for (let i = 0; i < pasted.length; i++) next[i] = pasted[i];
    setDigits(next);

    const focusIndex = Math.min(pasted.length, CODE_LENGTH - 1);
    inputRefs.current[focusIndex]?.focus();
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (locked || submitting || success || code.length !== CODE_LENGTH) return;
    submitCode(code);
  }

  async function handleResend() {
    if (resendIn > 0 || submitting || success) return;
    
    try {
      await authService.resendOTP({ challengeId });
      setResendIn(RESEND_SECONDS);
      setExpiresIn(EXPIRY_SECONDS);
      setAttempts(0);
      setState({ status: "idle" });
      setDigits(Array(CODE_LENGTH).fill(""));
      inputRefs.current[0]?.focus();
      setJustResent(true);
      setTimeout(() => setJustResent(false), 3000);
    } catch {
      // Silently fail on resend, user can try again
    }
  }

  const attemptsRemaining = MAX_ATTEMPTS - attempts;
  const boxIsLocked = submitting || expired || locked || success;

  return (
    <div>
      <button
        type="button"
        onClick={() => router.push("/sign-in")}
        className="mb-6 flex items-center gap-1.5 text-[13px] font-medium text-ink-500 transition hover:text-primary"
      >
        <ArrowLeft className="h-3.5 w-3.5" />
        Back to sign in
      </button>

      <div className="mb-6 flex items-center gap-3 rounded-xl border border-line bg-surface-2 px-4 py-3">
        <Mail className="h-[18px] w-[18px] shrink-0 text-primary" />
        <p className="text-[14px] leading-relaxed text-ink-600">
          Code sent to{" "}
          <span className="font-semibold text-ink-950">{destination}</span>
        </p>
      </div>

      <form onSubmit={handleSubmit} noValidate>
        <label className="mb-2 block text-[13px] font-semibold text-ink-800">
          Verification code
        </label>

        <div className="flex justify-between gap-2">
          {digits.map((digit, i) => (
            <input
              key={i}
              ref={(el) => {
                inputRefs.current[i] = el;
              }}
              type="text"
              inputMode="numeric"
              maxLength={1}
              autoComplete={i === 0 ? "one-time-code" : "off"}
              autoFocus={i === 0}
              value={digit}
              onChange={(e) => updateDigit(i, e.target.value)}
              onKeyDown={(e) => handleKeyDown(i, e)}
              onPaste={handlePaste}
              disabled={boxIsLocked}
              aria-label={`Digit ${i + 1} of ${CODE_LENGTH}`}
              className={`h-14 w-full max-w-[52px] rounded-xl border text-center text-xl font-semibold text-ink-950 outline-none transition disabled:opacity-60 ${
                hasError && !locked
                  ? "border-red bg-red-bg focus:border-red"
                  : success
                  ? "border-green bg-green-bg focus:border-green"
                  : "border-line bg-surface-2 focus:border-primary focus:bg-white"
              }`}
            />
          ))}
        </div>

        <div className="mt-3 min-h-[20px]">
          {state.status === "error" && (
            <p role="alert" className="flex items-center gap-1.5 text-[13px] font-medium text-red">
              {state.kind === "locked" ? (
                <Lock className="h-3.5 w-3.5 shrink-0" />
              ) : (
                <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
              )}
              {state.kind === "invalid" &&
                `That code isn't right. ${attemptsRemaining} attempt${attemptsRemaining === 1 ? "" : "s"} left.`}
              {state.kind === "expired" &&
                "This code has expired. Request a new one."}
              {state.kind === "access_denied" &&
                "Access denied. Contact your organization administrator."}
              {state.kind === "locked" &&
                "Too many attempts. Request a new code to try again."}
            </p>
          )}

          {state.status === "success" && (
            <p className="flex items-center gap-1.5 text-[13px] font-medium text-green">
              <CheckCircle2 className="h-4 w-4" />
              Verification successful! Redirecting...
            </p>
          )}

          {!expired && state.status !== "error" && state.status !== "success" && (
            <p className="flex items-center gap-1.5 text-[13px] text-ink-500">
              {justResent ? (
                <>
                  <ShieldCheck className="h-3.5 w-3.5 shrink-0 text-green" />
                  Code resent.
                </>
              ) : (
                `Code expires in ${formatTime(expiresIn)}`
              )}
            </p>
          )}
        </div>

        <button
          type="submit"
          disabled={submitting || code.length !== CODE_LENGTH || expired || locked || success}
          className="mt-4 flex w-full items-center justify-center gap-2 rounded-xl px-4 py-3 text-[15px] font-semibold text-white transition disabled:cursor-not-allowed disabled:opacity-50 bg-primary hover:bg-primary-hover"
        >
          {submitting ? (
            <>
              <RotateCw className="h-4 w-4 animate-spin" />
              Verifying…
            </>
          ) : success ? (
            <>
              <CheckCircle2 className="h-4 w-4" />
              Verified
            </>
          ) : (
            "Verify"
          )}
        </button>

        <button
          type="button"
          onClick={handleResend}
          disabled={resendIn > 0 || submitting || success}
          className="mt-3 w-full text-center text-[13px] font-semibold text-primary transition hover:text-primary-hover disabled:cursor-not-allowed disabled:text-ink-500"
        >
          {resendIn > 0 ? `Resend code in ${resendIn}s` : "Resend code"}
        </button>
      </form>
    </div>
  );
}

export default function VerifyPage() {
  return (
    <Suspense fallback={null}>
      <VerifyContent />
    </Suspense>
  );
}

function formatTime(totalSeconds: number) {
  const m = Math.floor(totalSeconds / 60);
  const s = totalSeconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}