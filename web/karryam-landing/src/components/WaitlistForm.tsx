"use client";

import { useEffect, useState } from "react";
import { ArrowRight, Check, Loader2, Mail } from "lucide-react";
import { SEGMENTS, isSegment, type Segment } from "@/lib/segments";

export default function WaitlistForm() {
  const [email, setEmail] = useState("");
  const [segment, setSegment] = useState<Segment>("customer");
  const [status, setStatus] = useState<"idle" | "loading" | "done">("idle");
  const [error, setError] = useState("");

  // Prefill from the hero form (?email=) and CTA buttons (?segment=)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const e = params.get("email");
    const s = params.get("segment");
    if (e) setEmail(e);
    if (isSegment(s)) setSegment(s);
  }, []);

  async function onSubmit(ev: React.FormEvent<HTMLFormElement>) {
    ev.preventDefault();
    setError("");
    setStatus("loading");

    const website = new FormData(ev.currentTarget).get("website");

    try {
      const res = await fetch("/api/waitlist/start", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, segment, website }),
      });
      const data = await res.json();
      if (!res.ok) {
        setError(data.error ?? "Something went wrong. Try again.");
        setStatus("idle");
        return;
      }
      setStatus("done");
    } catch {
      setError("Network error. Check your connection and try again.");
      setStatus("idle");
    }
  }

  if (status === "done") {
    const chosen = SEGMENTS.find((s) => s.value === segment);
    return (
      <div
        role="status"
        className="relative mx-auto w-full overflow-hidden rounded-[clamp(1rem,3vw,1.75rem)] border border-white/10 bg-white/[0.04] p-[clamp(1.75rem,5vw,3.5rem)] text-center shadow-[0_30px_80px_-30px_rgba(0,194,168,0.35)] backdrop-blur-xl motion-safe:animate-[fadeIn_0.4s_ease-out]"
      >
        <div
          aria-hidden
          className="pointer-events-none absolute -top-24 left-1/2 h-48 w-48 -translate-x-1/2 rounded-full bg-aurora/25 blur-3xl"
        />
        <span className="relative mx-auto grid h-[clamp(3rem,7vw,3.75rem)] w-[clamp(3rem,7vw,3.75rem)] place-items-center rounded-full bg-aurora text-midnight shadow-[0_0_32px_-4px_rgba(0,194,168,0.8)]">
          <Check
            className="h-[clamp(1.25rem,3vw,1.75rem)] w-[clamp(1.25rem,3vw,1.75rem)]"
            strokeWidth={3}
          />
        </span>
        <h2 className="relative mt-[clamp(1.25rem,3vw,1.75rem)] text-[clamp(1.375rem,3.4vw,2rem)] font-bold tracking-tight">
          You&apos;re on the list.
        </h2>
        <p className="relative mx-auto mt-[clamp(0.75rem,2vw,1rem)] max-w-[36ch] text-[clamp(0.9rem,1.7vw,1.0625rem)] leading-[1.6] text-white/70">
          We&apos;ll email{" "}
          <span className="font-medium text-white break-all">{email}</span>{" "}
          when Karryam launches
          {chosen ? (
            <>
              {" "}
              for <span className="text-aurora">{chosen.label}</span>
            </>
          ) : null}
          .
        </p>
        <button
          type="button"
          onClick={() => {
            setStatus("idle");
            setEmail("");
          }}
          className="relative mt-[clamp(1.5rem,4vw,2.25rem)] rounded text-[clamp(0.8rem,1.5vw,0.875rem)] font-medium text-white/60 underline-offset-4 transition hover:text-white hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-aurora/60"
        >
          Add another email
        </button>

        <style>{`@keyframes fadeIn{from{opacity:0;transform:translateY(8px) scale(.98)}to{opacity:1;transform:none}}`}</style>
      </div>
    );
  }

  return (
    <form
      onSubmit={onSubmit}
      className="relative mx-auto w-full overflow-hidden rounded-[clamp(1rem,3vw,1.75rem)] border border-white/10 bg-white/[0.04] p-[clamp(1.25rem,4vw,3rem)] shadow-[0_30px_80px_-30px_rgba(0,194,168,0.3)] backdrop-blur-xl"
    >
      {/* Soft top highlight */}
      <div
        aria-hidden
        className="pointer-events-none absolute inset-x-[clamp(2rem,8vw,4rem)] top-0 h-px bg-gradient-to-r from-transparent via-aurora/60 to-transparent"
      />

      <fieldset>
        <legend className="text-[clamp(0.9rem,1.8vw,1.0625rem)] font-semibold text-white">
          Which best describes you?
        </legend>

        {/* Stacks below ~520px, two columns above. Fluid gap + fluid padding. */}
        <div className="mt-[clamp(0.875rem,2vw,1.25rem)] grid grid-cols-1 gap-[clamp(0.5rem,1.5vw,0.875rem)] min-[520px]:grid-cols-2">
          {SEGMENTS.map((s) => {
            const selected = segment === s.value;
            return (
              <label
                key={s.value}
                className={`group relative cursor-pointer rounded-[clamp(0.75rem,1.8vw,1rem)] border px-[clamp(0.875rem,2vw,1.125rem)] py-[clamp(0.75rem,2vw,1rem)] transition-all duration-200 ${
                  selected
                    ? "border-aurora bg-aurora/10 shadow-[0_0_24px_-10px_rgba(0,194,168,0.9)]"
                    : "border-white/10 bg-white/[0.02] hover:border-white/25 hover:bg-white/[0.05]"
                } focus-within:ring-2 focus-within:ring-aurora/60`}
              >
                <input
                  type="radio"
                  name="segment"
                  value={s.value}
                  checked={selected}
                  onChange={() => setSegment(s.value)}
                  className="sr-only"
                />
                <span
                  aria-hidden
                  className={`absolute right-[clamp(0.75rem,2vw,1rem)] top-[clamp(0.75rem,2vw,1rem)] grid h-[clamp(0.9rem,2vw,1rem)] w-[clamp(0.9rem,2vw,1rem)] place-items-center rounded-full border transition-all duration-200 ${
                    selected
                      ? "border-aurora bg-aurora text-midnight"
                      : "border-white/25 text-transparent group-hover:border-white/45"
                  }`}
                >
                  <Check
                    className="h-[clamp(0.55rem,1.4vw,0.65rem)] w-[clamp(0.55rem,1.4vw,0.65rem)]"
                    strokeWidth={3.5}
                  />
                </span>
                <span className="block pr-[clamp(1.25rem,3vw,1.75rem)] text-[clamp(0.875rem,1.7vw,1rem)] font-semibold text-white">
                  {s.label}
                </span>
                <span className="mt-[clamp(0.15rem,0.6vw,0.25rem)] block text-[clamp(0.72rem,1.4vw,0.8125rem)] leading-[1.45] text-white/55">
                  {s.hint}
                </span>
              </label>
            );
          })}
        </div>
      </fieldset>

      <div className="mt-[clamp(1.5rem,4vw,2.5rem)]">
        <label
          htmlFor="waitlist-email"
          className="text-[clamp(0.9rem,1.8vw,1.0625rem)] font-semibold text-white"
        >
          Email address
        </label>
        <div className="relative mt-[clamp(0.625rem,1.5vw,0.875rem)]">
          <Mail
            aria-hidden
            className="pointer-events-none absolute left-[clamp(1rem,2.4vw,1.25rem)] top-1/2 h-[clamp(1rem,2vw,1.125rem)] w-[clamp(1rem,2vw,1.125rem)] -translate-y-1/2 text-white/40"
          />
          <input
            id="waitlist-email"
            type="email"
            required
            autoComplete="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@example.com"
            aria-invalid={error ? true : undefined}
            aria-describedby={error ? "waitlist-error" : undefined}
            className="w-full rounded-full border border-white/15 bg-white/[0.06] py-[clamp(0.75rem,2vw,1rem)] pl-[clamp(2.5rem,6vw,3rem)] pr-[clamp(1rem,2.5vw,1.5rem)] text-[clamp(0.9rem,1.7vw,1rem)] text-white outline-none transition placeholder:text-white/40 focus:border-aurora/70 focus:bg-white/[0.08] focus:ring-4 focus:ring-aurora/15"
          />
        </div>
      </div>

      {/* Honeypot */}
      <input
        type="text"
        name="website"
        tabIndex={-1}
        autoComplete="off"
        aria-hidden
        className="hidden"
      />

      {error && (
        <p
          id="waitlist-error"
          role="alert"
          className="mt-[clamp(0.75rem,2vw,1rem)] rounded-[clamp(0.5rem,1.2vw,0.625rem)] border border-red-400/20 bg-red-400/10 px-[clamp(0.75rem,2vw,1rem)] py-[clamp(0.5rem,1.4vw,0.625rem)] text-[clamp(0.8rem,1.5vw,0.875rem)] text-red-300"
        >
          {error}
        </p>
      )}

      <button
        type="submit"
        disabled={status === "loading"}
        className="group mt-[clamp(1.5rem,4vw,2.5rem)] inline-flex w-full items-center justify-center gap-2 rounded-full bg-aurora px-[clamp(1.25rem,3vw,1.75rem)] py-[clamp(0.85rem,2.2vw,1.125rem)] text-[clamp(0.9rem,1.7vw,1rem)] font-semibold text-midnight shadow-[0_0_32px_-6px_rgba(0,194,168,0.75)] transition hover:brightness-110 focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-aurora/30 active:scale-[0.99] disabled:cursor-not-allowed disabled:opacity-70"
      >
        {status === "loading" ? (
          <>
            <Loader2
              className="h-[clamp(1rem,2vw,1.125rem)] w-[clamp(1rem,2vw,1.125rem)] animate-spin"
              strokeWidth={2.5}
            />
            Joining...
          </>
        ) : (
          <>
            Join the waitlist
            <ArrowRight
              className="h-[clamp(1rem,2vw,1.125rem)] w-[clamp(1rem,2vw,1.125rem)] transition-transform group-hover:translate-x-0.5"
              strokeWidth={2.5}
            />
          </>
        )}
      </button>

      <p className="mt-[clamp(1rem,2.5vw,1.5rem)] text-center text-[clamp(0.75rem,1.4vw,0.875rem)] text-white/45">
        No spam. Unsubscribe any time.
      </p>
    </form>
  );
}