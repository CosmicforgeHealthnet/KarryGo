"use client";

import { Suspense, useEffect, useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft } from "lucide-react";

const MOCK_VALID_CODE = "123456";
const RESEND_SECONDS = 30;
const EXPIRY_SECONDS = 300;
const MAX_ATTEMPTS = 5;

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "error"; kind: "invalid" | "expired" | "access_denied" | "locked" };

function VerifyContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const destination = searchParams.get("destination") ?? "your contact";

  const [code, setCode] = useState("");
  const [state, setState] = useState<SubmitState>({ status: "idle" });
  const [resendIn, setResendIn] = useState(RESEND_SECONDS);
  const [expiresIn, setExpiresIn] = useState(EXPIRY_SECONDS);
  const [attempts, setAttempts] = useState(0);
  const [justResent, setJustResent] = useState(false);

  useEffect(() => {
    const timer = setInterval(() => {
      setResendIn((s) => Math.max(0, s - 1));
      setExpiresIn((s) => Math.max(0, s - 1));
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  const submitting = state.status === "submitting";
  const expired = expiresIn === 0;
  const locked = state.status === "error" && state.kind === "locked";

  async function submitCode(candidate: string) {
    if (expired) {
      setState({ status: "error", kind: "expired" });
      return;
    }

    setState({ status: "submitting" });

    // MOCK: accepts a fixed code until the real verify endpoint is wired in.
    await new Promise((resolve) => setTimeout(resolve, 500));

    if (candidate !== MOCK_VALID_CODE) {
      const nextAttempts = attempts + 1;
      setAttempts(nextAttempts);
      if (nextAttempts >= MAX_ATTEMPTS) {
        setState({ status: "error", kind: "locked" });
      } else {
        setState({ status: "error", kind: "invalid" });
      }
      return;
    }

    router.push("/choose-organization");
  }

  // Auto-submit the instant a full 6-digit code is entered.
  useEffect(() => {
    if (code.length === 6 && state.status === "idle") {
      submitCode(code);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [code]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (locked || submitting) return;
    submitCode(code);
  }

  function handleResend() {
    if (resendIn > 0) return;
    setResendIn(RESEND_SECONDS);
    setExpiresIn(EXPIRY_SECONDS);
    setAttempts(0);
    setState({ status: "idle" });
    setCode("");
    setJustResent(true);
    setTimeout(() => setJustResent(false), 3000);
  }

  const attemptsRemaining = MAX_ATTEMPTS - attempts;

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

      <form onSubmit={handleSubmit} noValidate>
        <p className="mb-4 text-[14px] leading-relaxed text-ink-600">
          Enter the code we sent to{" "}
          <span className="font-semibold text-ink-950">{destination}</span>.
        </p>

        <label htmlFor="code" className="mb-1.5 block text-[13px] font-semibold text-ink-800">
          Verification code
        </label>
        <input
          id="code"
          type="text"
          inputMode="numeric"
          maxLength={6}
          autoComplete="one-time-code"
          value={code}
          onChange={(e) => {
            setCode(e.target.value.replace(/\D/g, ""));
            if (state.status === "error" && state.kind !== "locked") {
              setState({ status: "idle" });
            }
          }}
          placeholder="000000"
          disabled={submitting || expired || locked}
          className="w-full rounded-xl border border-line bg-surface-2 px-3.5 py-2.5 text-center text-xl font-semibold tracking-[0.4em] text-ink-950 outline-none transition focus:border-primary focus:bg-white disabled:opacity-60"
        />

        {state.status === "error" && (
          <p role="alert" className="mt-2 text-[13px] font-medium text-red">
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

        {!expired && state.status !== "error" && (
          <p className="mt-2 text-[13px] text-ink-500">
            {justResent ? "Code resent." : `Code expires in ${formatTime(expiresIn)}`}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting || code.length !== 6 || expired || locked}
          className="mt-4 flex w-full items-center justify-center rounded-xl bg-primary px-4 py-2.5 text-[15px] font-semibold text-white transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-50"
        >
          {submitting ? "Verifying…" : "Verify"}
        </button>

        <button
          type="button"
          onClick={handleResend}
          disabled={resendIn > 0}
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