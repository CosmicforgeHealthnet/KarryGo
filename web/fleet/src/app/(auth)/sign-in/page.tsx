"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "error"; kind: "invalid" | "rate_limited" | "unavailable" };

function isValidIdentifier(value: string) {
  const email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  const phone = /^\+?[0-9]{7,15}$/;
  return email.test(value) || phone.test(value.replace(/[\s()-]/g, ""));
}

export default function SignInPage() {
  const router = useRouter();
  const [identifier, setIdentifier] = useState("");
  const [state, setState] = useState<SubmitState>({ status: "idle" });

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();

    if (!isValidIdentifier(identifier)) {
      setState({ status: "error", kind: "invalid" });
      return;
    }

    setState({ status: "submitting" });

    // MOCK: simulates the sign-in challenge call until fleet-partner-service
    // auth endpoints are wired in.
    await new Promise((resolve) => setTimeout(resolve, 600));

    router.push(`/verify?destination=${encodeURIComponent(maskIdentifier(identifier))}`);
  }

  const submitting = state.status === "submitting";

  return (
    <div>
      <h1 className="font-display text-[40px] font-light leading-tight tracking-[-0.02em] text-ink-950">
        Sign in
      </h1>
      <p className="mt-3 text-[16px] text-ink-600">
        Enter your work email or phone to continue.
      </p>

      <form onSubmit={handleSubmit} noValidate className="mt-9">
        <label
          htmlFor="identifier"
          className="mb-2 block text-[13px] font-medium uppercase tracking-wide text-ink-500"
        >
          Email or phone number
        </label>
        <input
          id="identifier"
          type="text"
          autoComplete="username"
          value={identifier}
          onChange={(e) => {
            setIdentifier(e.target.value);
            if (state.status === "error") setState({ status: "idle" });
          }}
          placeholder="you@company.com"
          disabled={submitting}
          className="w-full border-b border-line bg-transparent py-3 text-[18px] text-ink-950 outline-none transition placeholder:text-ink-500/60 focus:border-primary disabled:opacity-50"
        />

        {state.status === "error" && (
          <p role="alert" className="mt-2.5 text-[14px] text-red">
            {state.kind === "invalid" &&
              "Enter a valid email address or phone number."}
            {state.kind === "rate_limited" &&
              "Too many attempts. Wait a moment before trying again."}
            {state.kind === "unavailable" &&
              "Sign-in is temporarily unavailable. Try again shortly."}
          </p>
        )}

        <button
          type="submit"
          disabled={submitting || identifier.length === 0}
          className="mt-7 w-full rounded-md bg-primary px-4 py-3.5 text-[15px] font-semibold text-white transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-40"
        >
          {submitting ? "Sending code…" : "Continue"}
        </button>
      </form>

      <p className="mt-8 text-[13.5px] leading-relaxed text-ink-500">
        We&apos;ll send a one-time code to confirm it&apos;s you. Your contact
        details are never shared with other organizations.
      </p>
    </div>
  );
}

function maskIdentifier(value: string) {
  if (value.includes("@")) {
    const [local, domain] = value.split("@");
    const visible = local.slice(0, 1);
    return `${visible}${"*".repeat(Math.max(local.length - 1, 1))}@${domain}`;
  }
  return value.replace(/.(?=.{2})/g, "*");
}