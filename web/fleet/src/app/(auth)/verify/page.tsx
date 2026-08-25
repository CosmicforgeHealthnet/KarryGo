"use client";

import { Suspense, useEffect, useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";

const MOCK_VALID_CODE = "123456";
const RESEND_SECONDS = 30;
const EXPIRY_SECONDS = 300;

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "error"; kind: "invalid" | "expired" | "access_denied" };

function VerifyContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const destination = searchParams.get("destination") ?? "your contact";

  const [code, setCode] = useState("");
  const [state, setState] = useState<SubmitState>({ status: "idle" });
  const [resendIn, setResendIn] = useState(RESEND_SECONDS);
  const [expiresIn, setExpiresIn] = useState(EXPIRY_SECONDS);

  useEffect(() => {
    const timer = setInterval(() => {
      setResendIn((s) => Math.max(0, s - 1));
      setExpiresIn((s) => Math.max(0, s - 1));
    }, 1000);
    return () => clearInterval(timer);
  }, []);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();

    if (expiresIn === 0) {
      setState({ status: "error", kind: "expired" });
      return;
    }

    setState({ status: "submitting" });

    // MOCK: accepts a fixed code until the real verify endpoint is wired in.
    await new Promise((resolve) => setTimeout(resolve, 500));

    if (code !== MOCK_VALID_CODE) {
      setState({ status: "error", kind: "invalid" });
      return;
    }

    router.push("/choose-organization");
  }

  function handleResend() {
    if (resendIn > 0) return;
    setResendIn(RESEND_SECONDS);
    setExpiresIn(EXPIRY_SECONDS);
    setState({ status: "idle" });
    setCode("");
  }

  const submitting = state.status === "submitting";
  const expired = expiresIn === 0;

  return (
    <form onSubmit={handleSubmit} noValidate>
      <p className="mb-4 text-[12px] leading-relaxed text-ink-600">
        Enter the code we sent to{" "}
        <span className="font-semibold text-ink-950">{destination}</span>.
      </p>

      <label htmlFor="code" className="mb-1.5 block text-[12px] font-semibold text-ink-800">
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
          if (state.status === "error") setState({ status: "idle" });
        }}
        placeholder="000000"
        disabled={submitting || expired}
        className="w-full rounded-xl border border-line bg-surface-2 px-3.5 py-2.5 text-center text-lg font-semibold tracking-[0.4em] text-ink-950 outline-none transition focus:border-blue-600 focus:bg-white disabled:opacity-60"
      />

      {state.status === "error" && (
        <p role="alert" className="mt-2 text-[12px] font-medium text-red">
          {state.kind === "invalid" && "That code isn't right. Try again."}
          {state.kind === "expired" &&
            "This code has expired. Request a new one."}
          {state.kind === "access_denied" &&
            "Access denied. Contact your organization administrator."}
        </p>
      )}

      {!expired && state.status !== "error" && (
        <p className="mt-2 text-[11px] text-ink-500">
          Code expires in {formatTime(expiresIn)}
        </p>
      )}

      <button
        type="submit"
        disabled={submitting || code.length !== 6 || expired}
        className="mt-4 flex w-full items-center justify-center rounded-xl bg-blue-700 px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {submitting ? "Verifying…" : "Verify"}
      </button>

      <button
        type="button"
        onClick={handleResend}
        disabled={resendIn > 0}
        className="mt-3 w-full text-center text-[12px] font-semibold text-blue-700 transition hover:text-blue-600 disabled:cursor-not-allowed disabled:text-ink-500"
      >
        {resendIn > 0 ? `Resend code in ${resendIn}s` : "Resend code"}
      </button>
    </form>
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