"use client";

import { Suspense, useEffect, useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { ArrowLeft } from "lucide-react";

const MOCK_VALID_CODE = "123456";
const EXPIRY_SECONDS = 180;
const MAX_ATTEMPTS = 5;

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "error"; kind: "invalid" | "expired" | "locked" };

function ReverifyContent() {
  const router = useRouter();
  const searchParams = useSearchParams();

  const action = searchParams.get("action") ?? "this action";
  const returnTo = searchParams.get("returnTo") ?? "/";

  const [code, setCode] = useState("");
  const [state, setState] = useState<SubmitState>({ status: "idle" });
  const [expiresIn, setExpiresIn] = useState(EXPIRY_SECONDS);
  const [attempts, setAttempts] = useState(0);

  useEffect(() => {
    const timer = setInterval(() => {
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

    router.push(`${returnTo}?reverified=1`);
  }

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

  function handleCancel() {
    router.push(returnTo);
  }

  const attemptsRemaining = MAX_ATTEMPTS - attempts;

  return (
    <div>
      <button
        type="button"
        onClick={handleCancel}
        className="mb-6 flex items-center gap-1.5 text-[13px] font-medium text-ink-500 transition hover:text-primary"
      >
        <ArrowLeft className="h-3.5 w-3.5" />
        Back
      </button>

      <h1 className="font-display text-[40px] font-light leading-tight tracking-[-0.02em] text-ink-950">
        Confirm it&apos;s you
      </h1>
      <p className="mt-3 text-[16px] text-ink-600">
        This action needs a fresh verification:{" "}
        <span className="font-medium text-ink-950">{action}</span>
      </p>

      <form onSubmit={handleSubmit} noValidate className="mt-9">
        <label
          htmlFor="code"
          className="mb-2 block text-[13px] font-medium uppercase tracking-wide text-ink-500"
        >
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
          className="w-full rounded-xl border border-line bg-surface-2 px-3.5 py-2.5 text-center text-xl font-semibold tracking-[0.4em] text-ink-950 outline-none transition focus:border-primary focus:bg-surface disabled:opacity-60"
        />

        {state.status === "error" && (
          <p role="alert" className="mt-2.5 text-[14px] text-red">
            {state.kind === "invalid" &&
              `That code isn't right. ${attemptsRemaining} attempt${attemptsRemaining === 1 ? "" : "s"} left.`}
            {state.kind === "expired" &&
              "This verification has expired. Cancel and try again."}
            {state.kind === "locked" &&
              "Too many attempts. Cancel and request a new code to try again."}
          </p>
        )}

        {!expired && state.status !== "error" && (
          <p className="mt-2.5 text-[14px] text-ink-500">
            Expires in {formatTime(expiresIn)}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting || code.length !== 6 || expired || locked}
          className="mt-7 w-full rounded-md bg-primary px-4 py-3.5 text-[15px] font-semibold text-white transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-40"
        >
          {submitting ? "Verifying…" : "Confirm"}
        </button>

        <button
          type="button"
          onClick={handleCancel}
          className="mt-4 w-full text-center text-[14px] font-medium text-ink-800 transition hover:text-primary"
        >
          Cancel
        </button>
      </form>
    </div>
  );
}

export default function ReverifyPage() {
  return (
    <Suspense fallback={null}>
      <ReverifyContent />
    </Suspense>
  );
}

function formatTime(totalSeconds: number) {
  const m = Math.floor(totalSeconds / 60);
  const s = totalSeconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}