// src/app/(auth)/verify/page.tsx

"use client";

import { Suspense, useEffect, useRef, useState, type FormEvent, type KeyboardEvent, type ClipboardEvent } from "react";
import { organizationDestination } from "@/lib/auth/organizationDestination";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft, Mail, ShieldCheck, ShieldAlert, Lock, RotateCw, CheckCircle2, WifiOff } from "lucide-react";
import { authService } from "@/lib/services/authService";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { organizationService } from "@/lib/services/organizationService";
import { classifyAuthError } from "@/lib/auth/authErrors";
import { challengeFailure } from "@/lib/auth/challengeErrors";
import { otpDeadline, remainingSeconds } from "@/lib/auth/challengeTiming";

const CODE_LENGTH = 6;
// Fallbacks only: the server's own timings arrive in the URL from /auth/start.
const DEFAULT_RESEND_SECONDS = 60;
const DEFAULT_EXPIRY_SECONDS = 600;

function positiveSeconds(raw: string | null, fallback: number): number {
  const n = Number(raw);
  return Number.isFinite(n) && n > 0 && n <= 3600 ? Math.floor(n) : fallback;
}
const MAX_ATTEMPTS = 5;

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "success" }
  | { status: "error"; kind: "invalid" | "expired" | "replaced" | "consumed" | "missing" | "access_denied" | "locked" | "network" | "contact_mismatch" | "account_unavailable" };

function VerifyContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [challengeId, setChallengeId] = useState(() => searchParams.get("challengeId") || "");
  const destination = searchParams.get("destination") ?? "your contact";
  const [resendSeconds, setResendSeconds] = useState(() => positiveSeconds(searchParams.get("resendIn"), DEFAULT_RESEND_SECONDS));
  const [expirySeconds, setExpirySeconds] = useState(() => positiveSeconds(searchParams.get("expiresIn"), DEFAULT_EXPIRY_SECONDS));

  const [digits, setDigits] = useState<string[]>(Array(CODE_LENGTH).fill(""));
  const [state, setState] = useState<SubmitState>({ status: "idle" });
  const [resendIn, setResendIn] = useState(resendSeconds);
  const [expiresIn, setExpiresIn] = useState(expirySeconds);
  const [attempts, setAttempts] = useState(0);
  const [justResent, setJustResent] = useState(false);
  const resendBusy = useRef(false);
  const [resending, setResending] = useState(false);
  const [resendError, setResendError] = useState<string | null>(null);
  const [errorReference, setErrorReference] = useState<string | null>(null);
  const [shake, setShake] = useState(false);

  const inputRefs = useRef<Array<HTMLInputElement | null>>([]);
  const expiryDeadlineRef = useRef(otpDeadline(searchParams.get("expiresAt"), expirySeconds));
  const resendDeadlineRef = useRef(otpDeadline(searchParams.get("resendAt"), resendSeconds));

  const { setAuth, setLoading, setError } = useAuthStore();
  const { setOrganization } = useOrganizationStore();

  useEffect(() => {
    // Compute against absolute deadlines, since mobile browsers throttle
    // or suspend interval callbacks when the page is in the background.
    const update = () => {
      setResendIn(remainingSeconds(resendDeadlineRef.current));
      setExpiresIn(remainingSeconds(expiryDeadlineRef.current));
    };
    update();
    const timer = setInterval(update, 1000);
    return () => clearInterval(timer);
  }, []);

  const code = digits.join("");
  const submitting = state.status === "submitting";
  const success = state.status === "success";
  const expired = expiresIn === 0;
  // Terminal account outcomes must not invite another OTP attempt.
  const locked = state.status === "error" && (
    state.kind === "locked" || state.kind === "contact_mismatch" || state.kind === "account_unavailable"
  );
  const restartRequired = state.status === "error" &&
    (state.kind === "replaced" || state.kind === "consumed" || state.kind === "missing");
  const blockVerify = locked || restartRequired || (state.status === "error" && state.kind === "expired");
  const hasError = state.status === "error";

  function triggerShake() {
    setShake(true);
    setTimeout(() => setShake(false), 400);
  }

  function clearAndRefocus() {
    setDigits(Array(CODE_LENGTH).fill(""));
    setTimeout(() => inputRefs.current[0]?.focus(), 0);
  }

  async function submitCode(candidate: string) {
    if (expired) {
      setState({ status: "error", kind: "expired" });
      return;
    }
    if (blockVerify) return;

    if (!challengeId) {
      setState({ status: "error", kind: "access_denied" });
      return;
    }

    if (submitting || success) return;

    setState({ status: "submitting" });
    setErrorReference(null);
    setLoading(true);
    setError(null);

    try {
      const response = await authService.verifyOTP({
        challengeId,
        code: candidate,
      });

      const data = response.data;

      if (data?.nextStep === "ONBOARDING_REQUIRED") {
        if (data?.tokens) {
          setAuth(data.tokens, data.identityId, data.organizations || []);
        }
        setState({ status: "success" });
        setLoading(false);
        const hasCompany = (data.organizations || []).length > 0;
        setTimeout(() => {
          // With a company, the dashboard reads its status and routes it.
          router.push(hasCompany ? "/" : organizationDestination(null));
        }, 800);
        return;
      }

      if (data?.nextStep === "ORGANIZATION_SELECTION_REQUIRED" && data?.tokens) {
        setAuth(data.tokens, data.identityId, data.organizations || []);
        setState({ status: "success" });
        setLoading(false);
        setTimeout(() => {
          router.push("/choose-organization");
        }, 800);
        return;
      }

      if (data?.nextStep === "READY" && data?.tokens) {
        setAuth(data.tokens, data.identityId, data.organizations || []);

        try {
          const org = await organizationService.getCurrentOrganization();
          setOrganization(org);

          setState({ status: "success" });
          setLoading(false);
          setTimeout(() => {
            router.push(organizationDestination(org.status));
          }, 800);
          return;
        } catch {
          // The status could not be read here. Do not guess "draft": the
          // dashboard re-reads it and routes from the authoritative answer.
          setState({ status: "success" });
          setLoading(false);
          setTimeout(() => {
            router.push("/");
          }, 800);
          return;
        }
      }

      if (data?.nextStep === "STEP_UP_GRANTED") {
        setState({ status: "success" });
        setLoading(false);
        setTimeout(() => {
          router.back();
        }, 800);
        return;
      }

      if (data?.nextStep === "CONTACT_MISMATCH" || data?.nextStep === "ACCOUNT_UNAVAILABLE") {
        setState({
          status: "error",
          kind: data.nextStep === "CONTACT_MISMATCH" ? "contact_mismatch" : "account_unavailable",
        });
        setLoading(false);
        return;
      }

      setState({ status: "error", kind: "access_denied" });
      setLoading(false);
      triggerShake();
    } catch (err) {
      const failure = classifyAuthError(err);
      const reason = challengeFailure(failure.code, failure.kind);
      setErrorReference([failure.code, failure.requestId].filter(Boolean).join(" · ") || null);
      if (reason === "invalid") {
        const nextAttempts = attempts + 1;
        setAttempts(nextAttempts);
        setState({ status: "error", kind: nextAttempts >= MAX_ATTEMPTS ? "locked" : "invalid" });
      } else {
        setState({ status: "error", kind: reason });
      }

      setError(failure.message);
      setLoading(false);
      triggerShake();
      if (reason === "invalid") {
        clearAndRefocus();
      }
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
    if (blockVerify || expired || submitting || success || code.length !== CODE_LENGTH) return;
    submitCode(code);
  }

  async function handleResend() {
    if (resendBusy.current || resendIn > 0 || submitting || success || restartRequired) return;
    resendBusy.current = true;
    setResending(true);

    setResendError(null);
    setErrorReference(null);
    try {
      const resent = await authService.resendOTP({ challengeId });
      if (!resent?.challengeId) {
        setResendError("The server did not return a new verification request. Please start again.");
        return;
      }
      const nextResend = positiveSeconds(String(resent.resendAfterSeconds ?? ""), resendSeconds);
      const nextExpiry = positiveSeconds(String(resent.expiresInSeconds ?? ""), expirySeconds);
      resendDeadlineRef.current = otpDeadline(resent.resendAvailableAt, nextResend);
      expiryDeadlineRef.current = otpDeadline(resent.expiresAt, nextExpiry);
      setResendSeconds(nextResend);
      setExpirySeconds(nextExpiry);
      setResendIn(remainingSeconds(resendDeadlineRef.current));
      setExpiresIn(remainingSeconds(expiryDeadlineRef.current));

      // An expired challenge is replaced with a new ID by the backend.
      // Always send that new ID during verification and on page refresh.
      setChallengeId(resent.challengeId);
      const params = new URLSearchParams(searchParams.toString());
      params.set("challengeId", resent.challengeId);
      params.set("expiresIn", String(nextExpiry));
      params.set("resendIn", String(nextResend));
      if (resent.expiresAt) params.set("expiresAt", resent.expiresAt);
      else params.delete("expiresAt");
      if (resent.resendAvailableAt) params.set("resendAt", resent.resendAvailableAt);
      else params.delete("resendAt");
      router.replace(`/verify?${params.toString()}`);

      setAttempts(0);
      setState({ status: "idle" });
      setDigits(Array(CODE_LENGTH).fill(""));
      inputRefs.current[0]?.focus();
      setJustResent(true);
      setTimeout(() => setJustResent(false), 3000);
    } catch (err) {
      const failure = classifyAuthError(err);
      const reason = challengeFailure(failure.code, failure.kind);
      if (reason === "replaced" || reason === "consumed" || reason === "missing") {
        setState({ status: "error", kind: reason });
      } else if (reason === "expired") {
        // Older servers cannot resend expired codes; restart is necessary.
        setState({ status: "error", kind: "missing" });
      }
      const detail = [failure.code, failure.requestId].filter(Boolean).join(" · ");
      setResendError(`${reason === "replaced" || reason === "consumed" || reason === "missing" || reason === "expired"
        ? "This verification request is no longer active. Start again."
        : failure.message}${detail ? ` (Reference: ${detail})` : ""}`);
    } finally { resendBusy.current = false; setResending(false); }
  }

  const attemptsRemaining = MAX_ATTEMPTS - attempts;
  const boxIsLocked = submitting || expired || blockVerify || success;

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
          Verification email requested for{" "}
          <span className="font-semibold text-ink-950">{destination}</span>
        </p>
      </div>

      <form onSubmit={handleSubmit} noValidate>
        <label className="mb-2 block text-[13px] font-semibold text-ink-800">
          Verification code
        </label>

        <div className={`flex justify-between gap-2 ${shake ? "animate-[otp-shake_0.4s_ease-in-out]" : ""}`}>
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
              aria-invalid={hasError}
              className={`h-14 w-full max-w-[52px] rounded-xl border text-center text-xl font-semibold text-ink-950 outline-none transition disabled:opacity-60 ${
                hasError && !locked
                  ? "border-red bg-red-bg focus:border-red"
                  : success
                  ? "border-green bg-green-bg focus:border-green"
                  : "border-line bg-surface-2 focus:border-primary focus:bg-surface"
              }`}
            />
          ))}
        </div>

        <div className="mt-3 min-h-[20px]" role="status" aria-live="polite">
          {state.status === "error" && (
            <p role="alert" className="flex items-center gap-1.5 text-[13px] font-medium text-red">
              {state.kind === "locked" ? (
                <Lock className="h-3.5 w-3.5 shrink-0" />
              ) : state.kind === "network" ? (
                <WifiOff className="h-3.5 w-3.5 shrink-0" />
              ) : (
                <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
              )}
              {state.kind === "invalid" &&
                `That code isn't right. ${attemptsRemaining} attempt${attemptsRemaining === 1 ? "" : "s"} left.`}
              {state.kind === "expired" &&
                "This code has expired. Request a new one."}
              {state.kind === "replaced" &&
                "A newer verification request replaced this code. Start again to use the latest code."}
              {state.kind === "consumed" &&
                "This verification code was already used. Start again if you still need to sign in."}
              {state.kind === "missing" &&
                "This verification request was not found. Start again to request a new code."}
              {state.kind === "access_denied" &&
                "Access denied. Contact your organization administrator."}
              {state.kind === "contact_mismatch" &&
                "These details do not match one account. Use your registered email and phone number, or contact Karryam support."}
              {state.kind === "account_unavailable" &&
                "This account is not available. Contact Karryam support to continue."}
              {state.kind === "locked" &&
                "Too many attempts. Request a new code to try again."}
              {state.kind === "network" &&
                "Couldn't reach the server. Check your connection and try again."}
            </p>
          )}
          {state.status === "error" && errorReference && (
            <p className="mt-1 text-[11px] text-ink-500">Reference: {errorReference}</p>
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

          {expired && state.status !== "error" && state.status !== "success" && (
            <p className="flex items-center gap-1.5 text-[13px] font-medium text-red">
              <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
              This code has expired. Request a new one.
            </p>
          )}
        </div>

        <button
          type="submit"
          disabled={submitting || code.length !== CODE_LENGTH || expired || blockVerify || success}
          className="mt-4 flex h-[clamp(56px,7.4vh,70px)] w-full items-center justify-center gap-2.5 rounded-[12px] bg-label-accent px-5 text-[21px] font-bold text-surface transition hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-50"
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

        {restartRequired ? (
          <button
            type="button"
            onClick={() => router.replace("/sign-in")}
            className="mt-3 w-full text-center text-[13px] font-semibold text-primary hover:underline"
          >
            Start new verification
          </button>
        ) : (
          <button
            type="button"
            onClick={handleResend}
            disabled={resending || resendIn > 0 || submitting || success}
            className="mt-3 w-full text-center text-[13px] font-semibold text-primary transition hover:text-primary-hover disabled:cursor-not-allowed disabled:text-ink-500"
          >
            {resendIn > 0 ? `Resend code in ${resendIn}s` : expired ? "Request new code" : "Resend code"}
          </button>
        )}

        {resendError && (
          <p role="alert" className="mt-2 flex items-center justify-center gap-1.5 text-[12px] font-medium text-red">
            <ShieldAlert className="h-3.5 w-3.5 shrink-0" />
            {resendError}
          </p>
        )}
      </form>

      <style jsx>{`
        @keyframes otp-shake {
          0%, 100% { transform: translateX(0); }
          20% { transform: translateX(-6px); }
          40% { transform: translateX(6px); }
          60% { transform: translateX(-4px); }
          80% { transform: translateX(4px); }
        }
      `}</style>
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