// src/app/(dashboard)/operations/assignments/[assignmentId]/history/page.tsx

"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import {
  Clock,
  CheckCircle2,
  XCircle,
  Send,
  Eye,
  Flag,
  UserCheck,
  UserX,
  RefreshCw,
  Shield,
  Package,
  Truck,
  User,
  Calendar,
  AlertCircle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { marketplaceService } from "@/lib/services/marketplaceService";
import type { StateHistoryEntry } from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Status → icon + color
//
// Icons map the "to" status of a transition, not a free-text label. That's
// the only field the backend guarantees; it's a stable enum, so the mapping
// is total and correct.
// ---------------------------------------------------------------------------

type IconSpec = {
  Icon: typeof CheckCircle2;
  bg: string;
  color: string;
};

function iconForStatus(status: string): IconSpec {
  switch (status) {
    case "ACCEPTED":
    case "READY_FOR_EXECUTION":
    case "COMPLETED":
      return { Icon: CheckCircle2, bg: "bg-green-bg", color: "text-green" };
    case "DECLINED":
      return { Icon: XCircle, bg: "bg-red-bg", color: "text-red" };
    case "CANCELLED":
      return { Icon: UserX, bg: "bg-red-bg", color: "text-red" };
    case "EXPIRED":
      return { Icon: Clock, bg: "bg-surface-2", color: "text-ink-500" };
    case "PENDING_PROVIDER_RESPONSE":
    case "CREATED":
      return { Icon: Send, bg: "bg-primary-bg", color: "text-primary" };
    case "EXECUTION_START_PENDING":
      return { Icon: Package, bg: "bg-primary-bg", color: "text-primary" };
    case "IN_PROGRESS":
      return { Icon: Truck, bg: "bg-amber-bg", color: "text-amber" };
    case "RECONCILIATION_REQUIRED":
      return { Icon: RefreshCw, bg: "bg-amber-bg", color: "text-amber" };
    default:
      return { Icon: Clock, bg: "bg-surface-2", color: "text-ink-500" };
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case "CREATED":
      return "Created";
    case "PENDING_PROVIDER_RESPONSE":
      return "Awaiting provider response";
    case "ACCEPTED":
      return "Accepted by provider";
    case "READY_FOR_EXECUTION":
      return "Ready for execution";
    case "EXECUTION_START_PENDING":
      return "Execution start requested";
    case "IN_PROGRESS":
      return "In progress";
    case "COMPLETED":
      return "Completed";
    case "DECLINED":
      return "Declined by provider";
    case "EXPIRED":
      return "Expired";
    case "CANCELLED":
      return "Cancelled";
    case "RECONCILIATION_REQUIRED":
      return "Reconciliation required";
    default:
      return status || "—";
  }
}

function badgeClass(status: string): string {
  switch (status) {
    case "ACCEPTED":
    case "READY_FOR_EXECUTION":
    case "COMPLETED":
      return "bg-green-bg text-green";
    case "DECLINED":
    case "CANCELLED":
      return "bg-red-bg text-red";
    case "EXPIRED":
      return "bg-surface-2 text-ink-600";
    case "IN_PROGRESS":
    case "RECONCILIATION_REQUIRED":
      return "bg-amber-bg text-amber";
    default:
      return "bg-primary-bg text-primary";
  }
}

function actorLabel(entry: StateHistoryEntry): string {
  const type = entry.ActorType || "";
  const id = entry.ActorID || "";
  if (type === "PROVIDER") {
    return `Provider ${id}`.trim();
  }
  if (type === "ORGANIZATION_USER") {
    return `Dispatcher ${id}`.trim();
  }
  if (type === "SYSTEM") {
    return "System";
  }
  return (type + (id ? ` · ${id}` : "")).trim() || "—";
}

function formatTimestamp(iso: string): string {
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  return new Date(t).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function transitionLabel(entry: StateHistoryEntry): string {
  // A transition from X to X is an in-place event (e.g. acknowledgement);
  // the "to" status alone would read oddly, so use a dedicated label.
  if (entry.FromStatus === entry.ToStatus) {
    switch (entry.ToStatus) {
      case "PENDING_PROVIDER_RESPONSE":
        return "Receipt acknowledged";
      default:
        return statusLabel(entry.ToStatus);
    }
  }
  return statusLabel(entry.ToStatus);
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AssignmentHistoryPage() {
  const params = useParams();
  const assignmentId =
    typeof params.assignmentId === "string" ? params.assignmentId : "";

  const [history, setHistory] = useState<StateHistoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!assignmentId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    marketplaceService
      .getAssignmentHistory(assignmentId)
      .then((data) => {
        if (!cancelled) setHistory(data);
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
            "Failed to load history.";
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

  // ---- Loading ----
  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface p-5">
        <div className="space-y-5">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="flex gap-4">
              <div className="h-9 w-9 flex-shrink-0 rounded-full bg-surface-2" />
              <div className="flex-1 pt-0.5">
                <div className="h-4 w-48 rounded bg-surface-2" />
                <div className="mt-2 h-3 w-32 rounded bg-surface-2" />
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  }

  // ---- Error / not-found ----
  if (error) {
    return (
      <div className="rounded-lg border border-line bg-surface">
        <EmptyState
          image="/images/alerts-empty.png"
          title={error}
          description="The assignment history is not available."
          actionHref="/operations/assignments"
          actionLabel="Back to assignments"
        />
      </div>
    );
  }

  // ---- Empty ----
  if (history.length === 0) {
    return (
      <div className="rounded-lg border border-line bg-surface">
        <EmptyState
          image="/images/alerts-empty.png"
          title="No history yet"
          description="State changes will appear here as the assignment progresses."
        />
      </div>
    );
  }

  // The backend returns newest first; we render newest first, matching the
  // "NEW" badge on the top entry.
  const ordered = history;

  return (
    <div className="space-y-5">
      <div className="rounded-lg border border-line bg-surface p-5">
        <div className="flex flex-col gap-0">
          {ordered.map((ev, i) => {
            const { Icon, bg, color } = iconForStatus(ev.ToStatus);
            const isFirst = i === 0;
            const isLast = i === ordered.length - 1;

            return (
              <div
                key={ev.ID}
                className={`relative flex gap-4 ${!isLast ? "pb-6" : "pb-0"}`}
              >
                {/* Timeline line */}
                {!isLast && (
                  <div className="absolute left-[18px] top-[34px] bottom-0 w-px bg-line" />
                )}

                {/* Icon circle */}
                <div className="relative z-10 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full">
                  <div
                    className={`flex h-full w-full items-center justify-center rounded-full ${bg}`}
                  >
                    <Icon className={`h-4 w-4 ${color}`} />
                  </div>
                  {isFirst && (
                    <span className="absolute -top-1 -right-1 flex h-4 w-4 items-center justify-center rounded-full bg-green text-[8px] font-bold text-white">
                      NEW
                    </span>
                  )}
                </div>

                {/* Content */}
                <div className="min-w-0 flex-1 pt-0.5">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-semibold text-ink-950">
                      {transitionLabel(ev)}
                    </span>
                    {ev.ReasonCode && (
                      <span
                        className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${badgeClass(ev.ToStatus)}`}
                      >
                        {ev.ReasonCode}
                      </span>
                    )}
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-2 text-sm text-ink-500">
                    <div className="flex items-center gap-1">
                      <User className="h-3.5 w-3.5" />
                      {actorLabel(ev)}
                    </div>
                    <span className="text-ink-400">·</span>
                    <div className="flex items-center gap-1">
                      <Calendar className="h-3.5 w-3.5" />
                      {formatTimestamp(ev.OccurredAt)}
                    </div>
                    {ev.FromStatus && ev.FromStatus !== ev.ToStatus && (
                      <>
                        <span className="text-ink-400">·</span>
                        <span className="text-xs text-ink-500">
                          from {statusLabel(ev.FromStatus)}
                        </span>
                      </>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}