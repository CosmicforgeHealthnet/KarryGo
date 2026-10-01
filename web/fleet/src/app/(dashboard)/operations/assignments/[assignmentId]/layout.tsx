// src/app/(dashboard)/operations/assignments/[assignmentId]/layout.tsx

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname, useParams } from "next/navigation";
import { marketplaceService } from "@/lib/services/marketplaceService";
import type { Assignment } from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Status → human label + visual pill
// ---------------------------------------------------------------------------

function statusLabel(status: string): string {
  switch (status) {
    case "CREATED":
      return "Created";
    case "PENDING_PROVIDER_RESPONSE":
      return "Pending response";
    case "ACCEPTED":
      return "Accepted";
    case "READY_FOR_EXECUTION":
      return "Ready";
    case "EXECUTION_START_PENDING":
      return "Starting";
    case "IN_PROGRESS":
      return "In progress";
    case "COMPLETED":
      return "Completed";
    case "DECLINED":
      return "Declined";
    case "EXPIRED":
      return "Expired";
    case "CANCELLED":
      return "Cancelled";
    case "RECONCILIATION_REQUIRED":
      return "Needs reconciliation";
    default:
      return status || "—";
  }
}

function statusPillClass(status: string): string {
  switch (status) {
    case "ACCEPTED":
    case "READY_FOR_EXECUTION":
    case "COMPLETED":
      return "bg-green-bg text-green";
    case "PENDING_PROVIDER_RESPONSE":
    case "CREATED":
      return "bg-amber-bg text-amber";
    case "DECLINED":
    case "CANCELLED":
      return "bg-red-bg text-red";
    case "EXECUTION_START_PENDING":
    case "IN_PROGRESS":
      return "bg-primary-bg text-primary";
    case "RECONCILIATION_REQUIRED":
      return "bg-amber-bg text-amber";
    default:
      return "bg-surface-2 text-ink-600";
  }
}

const TABS = [
  { label: "Overview", segment: "" },
  { label: "Reassign", segment: "reassign" },
  { label: "History", segment: "history" },
];

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

export default function AssignmentDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const assignmentId =
    typeof params.assignmentId === "string" ? params.assignmentId : "";

  const [assignment, setAssignment] = useState<Assignment | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!assignmentId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    marketplaceService
      .getAssignment(assignmentId)
      .then((data) => {
        if (!cancelled) setAssignment(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setError("Assignment not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load assignment.";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [assignmentId]);

  const basePath = `/operations/assignments/${assignmentId}`;

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5">
            <h1>{assignmentId}</h1>
            {loading ? (
              <span className="h-6 w-24 rounded-full bg-surface-2" />
            ) : assignment ? (
              <span
                className={`rounded-full px-2.5 py-1 text-xs font-semibold ${statusPillClass(assignment.Status)}`}
              >
                {statusLabel(assignment.Status)}
              </span>
            ) : error ? (
              <span className="rounded-full bg-red-bg px-2.5 py-1 text-xs font-semibold text-red">
                Unavailable
              </span>
            ) : null}
          </div>
          {assignment && (
            <p className="mt-0.5 text-sm text-ink-500">
              {assignment.OfferID ? (
                <>
                  Offer{" "}
                  <span className="font-mono text-ink-700">
                    {assignment.OfferID}
                  </span>
                  {" · "}
                </>
              ) : null}
              Provider{" "}
              <span className="font-mono text-ink-700">
                {assignment.MembershipID}
              </span>
            </p>
          )}
          {!assignment && !loading && error && (
            <p className="mt-0.5 text-sm text-ink-500">{error}</p>
          )}
        </div>
        <Link
          href="/operations/assignments"
          className="flex-shrink-0 text-sm font-medium text-ink-600 hover:text-primary transition-colors"
        >
          ← Back to assignments
        </Link>
      </div>

      {/* ── Tabs ── */}
      <div className="flex gap-1 border-b border-line">
        {TABS.map((tab) => {
          const href = tab.segment ? `${basePath}/${tab.segment}` : basePath;
          const active = pathname === href;
          return (
            <Link
              key={tab.label}
              href={href}
              className={`relative px-3.5 py-2.5 text-sm font-semibold transition-colors ${
                active ? "text-primary" : "text-ink-600 hover:text-ink-950"
              }`}
            >
              {tab.label}
              {active && (
                <span className="absolute inset-x-0 -bottom-px h-[2px] rounded-full bg-primary" />
              )}
            </Link>
          );
        })}
      </div>

      {/* ── Content ── */}
      <div>{children}</div>
    </div>
  );
}