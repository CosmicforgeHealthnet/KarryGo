// src/app/(dashboard)/operations/assignments/[assignmentId]/reassign/page.tsx

"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { CANDIDATES } from "@/lib/mock-marketplace-data";
import {
  RefreshCw,
  CheckCircle2,
  Clock,
  Shield,
  Users,
  ArrowRight,
  X,
} from "lucide-react";

export default function ReassignPage() {
  const router = useRouter();
  const [selected, setSelected] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const eligible = CANDIDATES.filter((c) => c.eligibility !== "Blocked");

  async function handleReassign() {
    if (!selected) return;
    setSubmitting(true);
    await new Promise((r) => setTimeout(r, 600));
    router.push("/operations/assignments");
  }

  return (
    <div className="space-y-5">
      {/* ── Info note ── */}
      <div className="rounded-lg border border-primary/20 bg-primary-bg/30 p-4">
        <div className="flex items-start gap-3">
          <RefreshCw className="h-5 w-5 text-primary flex-shrink-0 mt-0.5" />
          <div>
            <p className="text-sm text-ink-700">
              Reassigning creates a new pending assignment for the replacement provider.
              The original assignment's history is preserved.
            </p>
            <div className="mt-2 flex flex-wrap gap-2">
              <span className="inline-flex items-center gap-1.5 rounded-full border border-line bg-white px-2.5 py-1 text-xs font-medium text-ink-600">
                <CheckCircle2 className="h-3 w-3 text-green" />
                Original preserved
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-full border border-line bg-white px-2.5 py-1 text-xs font-medium text-ink-600">
                <Clock className="h-3 w-3 text-amber" />
                New pending created
              </span>
              <span className="inline-flex items-center gap-1.5 rounded-full border border-line bg-white px-2.5 py-1 text-xs font-medium text-ink-600">
                <Shield className="h-3 w-3 text-primary" />
                Provider chooses
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* ── Candidate list ── */}
      <div>
        <div className="mb-3 flex items-center gap-2">
          <Users className="h-4 w-4 text-ink-400" />
          <span className="text-sm font-semibold text-ink-950">
            Available replacements
          </span>
          <span className="rounded-full bg-primary-bg px-2 py-0.5 text-xs font-semibold text-primary">
            {eligible.length}
          </span>
        </div>

        {eligible.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-12 text-center">
            <Users className="h-8 w-8 text-ink-300" />
            <p className="mt-3 text-sm font-medium text-ink-950">No eligible replacements</p>
            <p className="text-sm text-ink-500">No providers are currently available</p>
          </div>
        ) : (
          <div className="space-y-2">
            {eligible.map((c) => {
              const isSelected = selected === c.id;

              return (
                <button
                  key={c.id}
                  onClick={() => setSelected(c.id)}
                  className={`w-full flex items-center justify-between gap-4 rounded-lg border p-4 text-left transition-colors ${
                    isSelected
                      ? "border-primary bg-primary-bg"
                      : "border-line bg-white hover:bg-surface-2"
                  }`}
                >
                  <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-ink-950">{c.name}</span>
                      {!isSelected && (
                        <span className="text-xs text-ink-400 hidden sm:inline">
                          Click to select
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 flex items-center gap-2 text-sm text-ink-500">
                      <span className="font-mono">{c.asset}</span>
                      <span className="text-ink-300">·</span>
                      <span>{c.distanceToPickup} to pickup</span>
                    </div>
                  </div>
                  <div
                    className={`flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full border-2 transition-colors ${
                      isSelected
                        ? "border-primary bg-primary text-white"
                        : "border-ink-300"
                    }`}
                  >
                    {isSelected && <CheckCircle2 className="h-4 w-4" />}
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>

      {/* ── Actions ── */}
      <div className="flex flex-col-reverse gap-2.5 sm:flex-row sm:justify-end">
        <Link
          href={`/operations/assignments/ASN-731968`}
          className="flex items-center justify-center gap-2 rounded-lg border border-line bg-white px-5 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <X className="h-4 w-4" />
          Cancel
        </Link>
        <button
          onClick={handleReassign}
          disabled={!selected || submitting}
          className="flex items-center justify-center gap-2 rounded-lg bg-primary px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed"
        >
          {submitting ? (
            <>
              <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
              Reassigning...
            </>
          ) : (
            <>
              Confirm reassignment
              <ArrowRight className="h-4 w-4" />
            </>
          )}
        </button>
      </div>
    </div>
  );
}