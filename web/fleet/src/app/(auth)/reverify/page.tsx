"use client";

import { useEffect, useState, type FormEvent } from "react";
import { useRouter, useSearchParams } from "next/navigation";

const MOCK_VALID_CODE = "123456";
const EXPIRY_SECONDS = 180;

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "error"; kind: "invalid" | "expired" };

export default function ReverifyPage() {
  const router = useRouter();
  const searchParams = useSearchParams();

  // The screen that requested reverification passes what it's confirming
  // and where to return to on success, e.g.
  // /reverify?action=Revoke%20API%20client%20access&returnTo=/settings/integrations/api-clients
  const action = searchParams.get("action") ?? "this action";
  const returnTo = searchParams.get("returnTo") ?? "/";

  const [code, setCode] = useState("");
  const [state, setState] = useState<SubmitState>({ status: "idle" });
  const [expiresIn, setExpiresIn] = useState(EXPIRY_SECONDS);

  useEffect(() => {
    const timer = setInterval(() => {
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

    // MOCK: accepts a fixed code until the real reverification endpoint
    // is wired in.
    await new Promise((resolve) => setTimeout(resolve, 500));

    if (code !== MOCK_VALID_CODE) {
      setState({ status: "error", kind: "invalid" });
      return;
    }

    router.push(`${returnTo}?reverified=1`);
  }

  function handleCancel() {
    // Cancel safely: never proceed with the pending action.
    router.push(returnTo);
  }

  const submitting = state.status === "submitting";
  const expired = expiresIn === 0;

  return (
    <div>
      <h1 className="font-display text-[36px] font-light leading-tight tracking-[-0.02em] text-ink-950">
        Confirm it&apos;s you
      </h1>
      <p className="mt-2 text-[14px] text-ink-600">
        This action needs a fresh verification:{" "}
        <span className="font-medium text-ink-950">{action}</span>
      </p>

      <form onSubmit={handleSubmit} noValidate className="mt-9">
        <label
          htmlFor="code"
          className="mb-2 block text-[12px] font-medium uppercase tracking-wide text-ink-500"
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
            if (state.status === "error") setState({ status: "idle" });
          }}
          placeholder="000000"
          disabled={submitting || expired}
          className="w-full border-b border-line bg-transparent py-3 text-[22px] font-medium tracking-[0.5em] text-ink-950 outline-none transition placeholder:text-ink-500/40 focus:border-primary disabled:opacity-50"
        />

        {state.status === "error" && (
          <p role="alert" className="mt-2.5 text-[13px] text-red">
            {state.kind === "invalid" && "That code isn't right. Try again."}
            {state.kind === "expired" &&
              "This verification has expired. Cancel and try again."}
          </p>
        )}

        {!expired && state.status !== "error" && (
          <p className="mt-2.5 text-[12.5px] text-ink-500">
            Expires in {formatTime(expiresIn)}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting || code.length !== 6 || expired}
          className="mt-7 w-full rounded-md bg-primary px-4 py-3.5 text-[14px] font-semibold text-white transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-40"
        >
          {submitting ? "Verifying…" : "Confirm"}
        </button>

        <button
          type="button"
          onClick={handleCancel}
          className="mt-4 w-full text-center text-[13px] font-medium text-ink-800 transition hover:text-primary"
        >
          Cancel
        </button>
      </form>
    </div>
  );
}

function formatTime(totalSeconds: number) {
  const m = Math.floor(totalSeconds / 60);
  const s = totalSeconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}