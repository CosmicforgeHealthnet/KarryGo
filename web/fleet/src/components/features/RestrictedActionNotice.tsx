// src/components/features/RestrictedActionNotice.tsx

"use client";

import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Lock, X } from "lucide-react";
import { LIFECYCLE_REFUSAL_EVENT } from "@/lib/api/client";

type Refusal = { code?: string; message?: string };

// Explains an action the backend refused because the company is not yet
// approved, so a restricted click is never silently ignored. Shown wherever
// the refusal happens in the dashboard; built from the existing card and
// button styles.
export function RestrictedActionNotice({ onRefusal }: { onRefusal?: () => void }) {
  const [refusal, setRefusal] = useState<Refusal | null>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const show = (event: Event) => {
      setRefusal((event as CustomEvent<Refusal>).detail ?? {});
      onRefusal?.();
    };
    window.addEventListener(LIFECYCLE_REFUSAL_EVENT, show);
    return () => window.removeEventListener(LIFECYCLE_REFUSAL_EVENT, show);
  }, [onRefusal]);

  useEffect(() => {
    if (!refusal) return;
    closeRef.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") setRefusal(null);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [refusal]);

  if (!refusal) return null;
  const changesRequired = refusal.code === "organization_changes_required";
  const title = changesRequired ? "Company changes required" : "Company verification required";
  const body = changesRequired
    ? refusal.message || "Update your company details and resubmit them for review before using this feature."
    : "This feature will be available after your company is approved.";

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/40 backdrop-blur-sm" onClick={() => setRefusal(null)} aria-hidden="true" />
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="restricted-action-title"
        aria-describedby="restricted-action-body"
        className="relative w-full max-w-md rounded-2xl border border-line bg-surface p-5 shadow-xl sm:p-6"
      >
        <div className="flex items-start gap-3">
          <div className="rounded-full bg-amber/20 p-2">
            <Lock className="h-5 w-5 text-amber" />
          </div>
          <div className="min-w-0 flex-1">
            <h2 id="restricted-action-title" className="text-[15px] font-semibold text-ink-950">
              {title}
            </h2>
            <p id="restricted-action-body" className="mt-1 text-sm text-ink-600">
              {body}
            </p>
          </div>
          <button
            type="button"
            onClick={() => setRefusal(null)}
            className="flex h-8 w-8 items-center justify-center rounded-lg text-ink-500 transition-colors hover:bg-surface-2"
            aria-label="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <Link
            href={changesRequired ? "/setup" : "/company"}
            onClick={() => setRefusal(null)}
            className="btn btn-secondary btn-sm"
          >
            {changesRequired ? "Update company details" : "View submission"}
          </Link>
          <button ref={closeRef} type="button" onClick={() => setRefusal(null)} className="btn btn-primary btn-sm">
            OK
          </button>
        </div>
      </div>
    </div>
  );
}
