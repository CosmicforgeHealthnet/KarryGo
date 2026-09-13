// src/app/(auth)/sign-in/page.tsx

"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Mail, Check, ArrowRight, ShieldCheck } from "lucide-react";
import { authService } from "@/lib/services/authService";
import { useAuthStore } from "@/lib/stores/authStore";

type SubmitState =
  | { status: "idle" }
  | { status: "submitting" }
  | { status: "error"; kind: "invalid" | "rate_limited" | "unavailable" };

function isValidIdentifier(value: string) {
  const email = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
  const phone = /^\+?[0-9]{7,15}$/;
  return email.test(value) || phone.test(value.replace(/[\s()-]/g, ""));
}

function maskIdentifier(value: string) {
  if (value.includes("@")) {
    const [local, domain] = value.split("@");
    const visible = local.slice(0, 1);
    return `${visible}${"*".repeat(Math.max(local.length - 1, 1))}@${domain}`;
  }
  return value.replace(/.(?=.{2})/g, "*");
}

export default function SignInPage() {
  const router = useRouter();
  const [identifier, setIdentifier] = useState("");
  const [state, setState] = useState<SubmitState>({ status: "idle" });
  const { setLoading, setError } = useAuthStore();

  const isValid = isValidIdentifier(identifier);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();

    if (!isValidIdentifier(identifier)) {
      setState({ status: "error", kind: "invalid" });
      return;
    }

    setState({ status: "submitting" });
    setLoading(true);
    setError(null);

    try {
      let email = "";
      let phone = "";

      if (identifier.includes("@")) {
        email = identifier;
        phone = "+2348012345678";
      } else {
        const cleaned = identifier.replace(/[\s()-]/g, "");
        phone = cleaned.startsWith("+") ? cleaned : `+${cleaned}`;
        email = "test@example.com";
      }

      const response = await authService.startOTP({ email, phone });

      if (!response.data?.challengeId) {
        throw new Error("No challengeId received");
      }

      const masked = maskIdentifier(identifier);
      router.push(`/verify?challengeId=${response.data.challengeId}&destination=${encodeURIComponent(masked)}`);
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Something went wrong. Please try again.";
      if (message.toLowerCase().includes("rate") || message.toLowerCase().includes("too many")) {
        setState({ status: "error", kind: "rate_limited" });
      } else {
        setState({ status: "error", kind: "unavailable" });
      }
      setError(message);
    } finally {
      setState({ status: "idle" });
      setLoading(false);
    }
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

        <div className="relative flex items-center">
          <Mail className="pointer-events-none absolute left-3.5 h-[18px] w-[18px] text-ink-400" />
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
            className="w-full rounded-lg border border-line bg-surface-2 py-3 pl-11 pr-11 text-[16px] text-ink-950 outline-none transition placeholder:text-ink-500/60 focus:border-primary focus:bg-white disabled:opacity-50"
          />
          {isValid && (
            <Check className="absolute right-3.5 h-[18px] w-[18px] text-green" />
          )}
        </div>

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
          className="mt-7 flex w-full items-center justify-center gap-2 rounded-md bg-primary px-4 py-3.5 text-[15px] font-semibold text-white transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-40"
        >
          {submitting ? "Sending code…" : "Continue"}
          {!submitting && <ArrowRight className="h-4 w-4" />}
        </button>
      </form>

      <p className="mt-8 text-[13.5px] leading-relaxed text-ink-500">
        We&apos;ll send a one-time code to confirm it&apos;s you. Your contact
        details are never shared with other organizations.
      </p>

      <div className="mt-6 flex items-center gap-2 rounded-lg border border-line bg-surface-2 px-4 py-3">
        <ShieldCheck className="h-[18px] w-[18px] shrink-0 text-primary" />
        <span className="text-[13px] text-ink-600">
          Trusted by fleet operators worldwide
        </span>
      </div>
    </div>
  );
}