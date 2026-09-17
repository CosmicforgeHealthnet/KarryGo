// src/app/(dashboard)/operations/alerts/[alertId]/page.tsx

"use client";

import React, { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  ArrowLeft,
  Bell,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Clock,
  MapPin,
  RefreshCw,
  Users,
  Navigation,
  Shield,
  Truck,
  Flag,
  Check,
  User,
  FileText,
  ExternalLink,
  Activity,
  Info,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import {
  alertsService,
  newIdempotencyKey,
} from "@/lib/services/alertsService";
import type { OperationalAlert } from "@/lib/types/alerts";

// ---------------------------------------------------------------------------
// Presentation helpers — same mappings as the list page, kept here so this
// file is self-contained.
// ---------------------------------------------------------------------------

function severityKey(severity: string): "Critical" | "High" | "Medium" | "Low" {
  switch ((severity || "").toUpperCase()) {
    case "CRITICAL":
      return "Critical";
    case "HIGH":
      return "High";
    case "MEDIUM":
      return "Medium";
    case "LOW":
      return "Low";
    default:
      return "Low";
  }
}

const SEVERITY_STYLES: Record<
  "Critical" | "High" | "Medium" | "Low",
  { bg: string; text: string; dot: string; icon: typeof AlertCircle }
> = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: AlertCircle },
  High: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: AlertTriangle },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: AlertCircle },
  Low: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: Info },
};

function statusKey(status: string):
  | "Open"
  | "Acknowledged"
  | "InProgress"
  | "Resolved"
  | "Dismissed"
  | "Unknown" {
  switch ((status || "").toUpperCase()) {
    case "OPEN":
      return "Open";
    case "ACKNOWLEDGED":
      return "Acknowledged";
    case "IN_PROGRESS":
      return "InProgress";
    case "RESOLVED":
      return "Resolved";
    case "DISMISSED":
      return "Dismissed";
    default:
      return "Unknown";
  }
}

const STATUS_STYLES: Record<
  "Open" | "Acknowledged" | "InProgress" | "Resolved" | "Dismissed" | "Unknown",
  { label: string; bg: string; text: string; dot: string }
> = {
  Open: { label: "Open", bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Acknowledged: { label: "Acknowledged", bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  InProgress: { label: "In progress", bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Resolved: { label: "Resolved", bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Dismissed: { label: "Dismissed", bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Unknown: { label: "Unknown", bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const TYPE_ICONS: Record<string, typeof Bell> = {
  SAFETY_BLOCK: Shield,
  TRIP_SLA: Navigation,
  DOCUMENT_EXPIRY: Truck,
  MAINTENANCE_DUE: Activity,
  NO_ELIGIBLE_PROVIDER: Users,
};

const TYPE_LABELS: Record<string, string> = {
  SAFETY_BLOCK: "Safety block",
  TRIP_SLA: "Trip SLA",
  DOCUMENT_EXPIRY: "Document expiry",
  MAINTENANCE_DUE: "Maintenance",
  NO_ELIGIBLE_PROVIDER: "No eligible provider",
};

function typeLabel(alertType: string): string {
  return TYPE_LABELS[alertType] || alertType || "—";
}

function typeIcon(alertType: string) {
  return TYPE_ICONS[alertType] || Bell;
}

function formatTimestamp(iso: string | undefined): string {
  if (!iso) return "—";
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

function slaCountdown(slaDueAtIso: string): { text: string; minutes: number } {
  const t = new Date(slaDueAtIso).getTime();
  if (!Number.isFinite(t)) return { text: "—", minutes: Number.MAX_SAFE_INTEGER };
  const minutes = Math.floor((t - Date.now()) / 60_000);
  if (minutes <= 0) return { text: "Breached", minutes };
  if (minutes < 60) return { text: `${minutes} min remaining`, minutes };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { text: `${hours} h remaining`, minutes };
  const days = Math.floor(hours / 24);
  return { text: `${days} d remaining`, minutes };
}

function canAcknowledge(a: OperationalAlert): boolean {
  return a.status === "OPEN";
}

function canResolve(a: OperationalAlert): boolean {
  return a.status === "OPEN" || a.status === "ACKNOWLEDGED" || a.status === "IN_PROGRESS";
}

// ---------------------------------------------------------------------------
// Derived timeline from real timestamps
//
// The backend does not return an actions array. It stores CreatedAt,
// AcknowledgedAt, and ResolvedAt as individual columns. We render one row per
// real timestamp. No event is fabricated; when a timestamp is empty, no row
// appears for it.
// ---------------------------------------------------------------------------

type TimelineEvent = {
  key: string;
  label: string;
  timestamp: string;
  tone: "primary" | "amber" | "green";
};

function buildTimeline(a: OperationalAlert): TimelineEvent[] {
  const events: TimelineEvent[] = [];

  if (a.createdAt) {
    events.push({
      key: "created",
      label: "Alert created",
      timestamp: a.createdAt,
      tone: "primary",
    });
  }
  if (a.acknowledgedAt) {
    events.push({
      key: "acknowledged",
      label: "Acknowledged",
      timestamp: a.acknowledgedAt,
      tone: "amber",
    });
  }
  if (a.resolvedAt) {
    events.push({
      key: "resolved",
      label: a.resolutionCode ? `Resolved — ${a.resolutionCode}` : "Resolved",
      timestamp: a.resolvedAt,
      tone: "green",
    });
  }

  // Newest first, matching the list page's visual language.
  events.sort(
    (x, y) => new Date(y.timestamp).getTime() - new Date(x.timestamp).getTime(),
  );
  return events;
}

const TIMELINE_TONE: Record<"primary" | "amber" | "green", { bg: string; color: string }> = {
  primary: { bg: "bg-primary-bg", color: "text-primary" },
  amber: { bg: "bg-amber-bg", color: "text-amber" },
  green: { bg: "bg-green-bg", color: "text-green" },
};

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AlertDetailPage() {
  const params = useParams();
  const alertId = typeof params.alertId === "string" ? params.alertId : "";

  const [alert, setAlert] = useState<OperationalAlert | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [busy, setBusy] = useState<"ack" | "resolve" | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const [resolveOpen, setResolveOpen] = useState(false);
  const [resolutionCode, setResolutionCode] = useState("");
  const [resolutionReason, setResolutionReason] = useState("");

  // One key per action attempt. Refreshed when the user starts a new action,
  // stable across retries of that same action.
  const [ackKey, setAckKey] = useState(() => newIdempotencyKey());
  const [resolveKey, setResolveKey] = useState(() => newIdempotencyKey());

  useEffect(() => {
    if (!alertId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    alertsService
      .getAlert(alertId)
      .then((data) => {
        if (!cancelled) setAlert(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setError("Alert not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load alert.";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [alertId]);

  const timeline = useMemo(() => (alert ? buildTimeline(alert) : []), [alert]);

  const handleAcknowledge = async () => {
    if (!alert) return;
    setBusy("ack");
    setActionError(null);
    try {
      const updated = await alertsService.acknowledgeAlert(
        alert.id,
        { expectedVersion: alert.version },
        ackKey,
      );
      setAlert(updated);
      setAckKey(newIdempotencyKey());
    } catch (err: any) {
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        "Could not acknowledge the alert.";
      setActionError(message);
    } finally {
      setBusy(null);
    }
  };

  const handleResolve = async () => {
    if (!alert) return;
    setBusy("resolve");
    setActionError(null);
    try {
      const updated = await alertsService.resolveAlert(
        alert.id,
        {
          expectedVersion: alert.version,
          resolutionCode: resolutionCode.trim() || undefined,
          resolutionReason: resolutionReason.trim() || undefined,
        },
        resolveKey,
      );
      setAlert(updated);
      setResolveOpen(false);
      setResolutionCode("");
      setResolutionReason("");
      setResolveKey(newIdempotencyKey());
    } catch (err: any) {
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        "Could not resolve the alert.";
      setActionError(message);
    } finally {
      setBusy(null);
    }
  };

  // ---- Loading ----
  if (loading) {
    return (
      <div className="mx-auto max-w-5xl space-y-5">
        <div className="flex items-center gap-3">
          <div className="h-9 w-9 rounded-xl border border-line bg-surface" />
          <div>
            <div className="h-6 w-32 rounded bg-surface-2" />
            <div className="mt-1.5 h-3 w-56 rounded bg-surface-2" />
          </div>
        </div>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
          <div className="space-y-5 lg:col-span-2">
            <div className="h-64 rounded-xl border border-line bg-surface" />
            <div className="h-40 rounded-xl border border-line bg-surface" />
          </div>
          <div className="space-y-4">
            <div className="h-32 rounded-xl border border-line bg-surface" />
            <div className="h-48 rounded-xl border border-line bg-surface" />
          </div>
        </div>
      </div>
    );
  }

  // ---- Error / not found ----
  if (error || !alert) {
    return (
      <div className="mx-auto max-w-5xl space-y-5">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/alerts"
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-600 transition hover:bg-surface-2 hover:text-ink-900"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>{alertId}</h1>
            <p className="mt-0.5 text-[13.5px] text-ink-600">Alert details</p>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-surface">
          <EmptyState
            image="/images/alerts-empty.png"
            title={error ?? "Alert not found"}
            description="This alert may have been resolved and removed, or the URL may be incorrect."
            actionHref="/operations/alerts"
            actionLabel="Back to alerts"
          />
        </div>
      </div>
    );
  }

  const sev = SEVERITY_STYLES[severityKey(alert.severity)];
  const stat = STATUS_STYLES[statusKey(alert.status)];
  const SeverityIcon = sev.icon;
  const TypeIcon = typeIcon(alert.alertType);
  const isResolved = alert.status === "RESOLVED" || alert.status === "DISMISSED";
  const sla = slaCountdown(alert.slaDueAt);
  const breached = sla.text === "Breached";
  const showAck = canAcknowledge(alert);
  const showResolve = canResolve(alert);

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/alerts"
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-600 transition hover:bg-surface-2 hover:text-ink-900"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                {alert.id}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${sev.bg} ${sev.text}`}
              >
                <SeverityIcon className="h-3.5 w-3.5" />
                {severityKey(alert.severity)}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${stat.bg} ${stat.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${stat.dot}`} />
                {stat.label}
              </span>
            </div>
            <p className="mt-0.5 text-[13.5px] text-ink-600">
              {typeLabel(alert.alertType)} · {alert.branchId || "No branch"} ·{" "}
              {alert.sourceType}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {showAck && (
            <button
              onClick={handleAcknowledge}
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm transition hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
            >
              {busy === "ack" ? (
                <>
                  <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                  Acknowledging…
                </>
              ) : (
                <>
                  <Flag className="h-3.5 w-3.5" />
                  Acknowledge
                </>
              )}
            </button>
          )}
          {showResolve && !resolveOpen && (
            <button
              onClick={() => {
                setResolveOpen(true);
                setActionError(null);
              }}
              disabled={busy !== null}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-green px-3.5 text-[11.5px] font-semibold text-white shadow-sm transition hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
            >
              <Check className="h-3.5 w-3.5" />
              Resolve
            </button>
          )}
          {isResolved && (
            <span className="flex h-9 items-center gap-1.5 rounded-xl bg-green-bg px-3.5 text-[11.5px] font-semibold text-green">
              <CheckCircle2 className="h-3.5 w-3.5" />
              {stat.label}
            </span>
          )}
        </div>
      </div>

      {actionError && (
        <div className="flex items-start gap-2 rounded-lg border border-red/20 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{actionError}</span>
        </div>
      )}

      {/* ── Resolve form (inline) ── */}
      {resolveOpen && showResolve && (
        <div className="rounded-xl border border-line bg-surface p-4 shadow-sm">
          <div className="mb-3 flex items-center gap-2">
            <CheckCircle2 className="h-4 w-4 text-green" />
            <h3 className="text-[13px] font-semibold text-ink-950">
              Resolve this alert
            </h3>
          </div>
          <div className="space-y-3">
            <div>
              <label
                htmlFor="resolution-code"
                className="mb-1 block text-[11px] font-medium uppercase tracking-wide text-ink-500"
              >
                Resolution code (optional)
              </label>
              <input
                id="resolution-code"
                type="text"
                value={resolutionCode}
                onChange={(e) => setResolutionCode(e.target.value)}
                placeholder="e.g. FIXED, FALSE_ALARM, DUPLICATE"
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
              />
            </div>
            <div>
              <label
                htmlFor="resolution-reason"
                className="mb-1 block text-[11px] font-medium uppercase tracking-wide text-ink-500"
              >
                Reason (optional)
              </label>
              <textarea
                id="resolution-reason"
                value={resolutionReason}
                onChange={(e) => setResolutionReason(e.target.value)}
                placeholder="What was done to resolve this alert?"
                rows={3}
                className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
              />
            </div>
            <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <button
                type="button"
                onClick={() => {
                  setResolveOpen(false);
                  setResolutionCode("");
                  setResolutionReason("");
                  setActionError(null);
                }}
                disabled={busy === "resolve"}
                className="flex items-center justify-center rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2 disabled:opacity-60"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={handleResolve}
                disabled={busy === "resolve"}
                className="flex items-center justify-center gap-2 rounded-lg bg-green px-4 py-2 text-sm font-semibold text-white transition-colors hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {busy === "resolve" ? (
                  <>
                    <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                    Resolving…
                  </>
                ) : (
                  <>
                    <Check className="h-3.5 w-3.5" />
                    Confirm resolve
                  </>
                )}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ── Main content ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* LEFT COLUMN */}
        <div className="space-y-5 lg:col-span-2">
          {/* Alert details */}
          <div className="rounded-xl border border-line bg-surface p-4 shadow-sm">
            <div className="mb-3 flex items-center gap-2">
              <TypeIcon className="h-4 w-4 text-ink-400" />
              <h3 className="text-[13px] font-semibold text-ink-950">
                Alert details
              </h3>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">
                  Source
                </div>
                <div className="mt-0.5 text-[13px] font-medium text-ink-950">
                  {alert.sourceType}{" "}
                  <span className="font-mono text-ink-600">
                    {alert.sourceId}
                  </span>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">
                  Branch
                </div>
                <div className="mt-0.5 font-mono text-[13px] font-medium text-ink-950">
                  {alert.branchId || "—"}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">
                  Created
                </div>
                <div className="mt-0.5 text-[13px] font-medium text-ink-950">
                  {formatTimestamp(alert.createdAt)}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">
                  SLA
                </div>
                <div
                  className={`mt-0.5 text-[13px] font-medium ${
                    breached ? "text-red" : "text-ink-950"
                  }`}
                >
                  {sla.text}
                </div>
              </div>
              {alert.ownerUserId && (
                <div className="col-span-2 border-t border-line pt-3">
                  <div className="text-[10px] font-medium uppercase tracking-wide text-ink-500">
                    Owner
                  </div>
                  <div className="mt-0.5 flex items-center gap-2">
                    <User className="h-3.5 w-3.5 text-ink-400" />
                    <span className="font-mono text-[13px] font-medium text-ink-950">
                      {alert.ownerUserId}
                    </span>
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* Timeline */}
          <div className="rounded-xl border border-line bg-surface p-4 shadow-sm">
            <div className="mb-3 flex items-center gap-2">
              <Clock className="h-4 w-4 text-ink-400" />
              <h3 className="text-[13px] font-semibold text-ink-950">
                Timeline
              </h3>
            </div>
            {timeline.length === 0 ? (
              <p className="text-[13px] text-ink-500">
                No timestamps recorded yet.
              </p>
            ) : (
              <div className="space-y-3">
                {timeline.map((ev, index) => {
                  const tone = TIMELINE_TONE[ev.tone];
                  const isLast = index === timeline.length - 1;
                  return (
                    <div key={ev.key} className="flex gap-3">
                      <div className="flex flex-col items-center">
                        <div
                          className={`flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full ${tone.bg}`}
                        >
                          <span
                            className={`h-1.5 w-1.5 rounded-full bg-current ${tone.color}`}
                          />
                        </div>
                        {!isLast && <div className="mt-1 w-px flex-1 bg-line" />}
                      </div>
                      <div className="pt-1">
                        <div className="text-[13px] font-medium text-ink-950">
                          {ev.label}
                        </div>
                        <div className="text-[11.5px] text-ink-500">
                          {formatTimestamp(ev.timestamp)}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        {/* RIGHT COLUMN */}
        <div className="space-y-4">
          {/* Related resources — only what the alert actually carries */}
          {(alert.canonicalTripReference ||
            alert.assignmentId ||
            alert.shipmentId ||
            alert.routePlanId) && (
            <div className="rounded-xl border border-line bg-surface p-4 shadow-sm">
              <h3 className="mb-2.5 text-[13px] font-semibold text-ink-950">
                Related resources
              </h3>
              <div className="space-y-2">
                {alert.canonicalTripReference && (
                  <Link
                    href={`/operations/trips/${encodeURIComponent(alert.canonicalTripReference)}`}
                    className="flex items-center gap-2 rounded-lg border border-line bg-surface px-3 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-surface-2"
                  >
                    <Navigation className="h-3.5 w-3.5" />
                    Trip:{" "}
                    <span className="font-mono">
                      {alert.canonicalTripReference}
                    </span>
                    <ExternalLink className="ml-auto h-3.5 w-3.5 text-ink-400" />
                  </Link>
                )}
                {alert.assignmentId && (
                  <Link
                    href={`/operations/assignments/${encodeURIComponent(alert.assignmentId)}`}
                    className="flex items-center gap-2 rounded-lg border border-line bg-surface px-3 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-surface-2"
                  >
                    <FileText className="h-3.5 w-3.5" />
                    Assignment:{" "}
                    <span className="font-mono">{alert.assignmentId}</span>
                    <ExternalLink className="ml-auto h-3.5 w-3.5 text-ink-400" />
                  </Link>
                )}
                {alert.shipmentId && (
                  <div className="flex items-center gap-2 rounded-lg border border-line bg-surface px-3 py-2 text-[12px] font-medium text-ink-700">
                    <Truck className="h-3.5 w-3.5" />
                    Shipment:{" "}
                    <span className="font-mono">{alert.shipmentId}</span>
                  </div>
                )}
                {alert.routePlanId && (
                  <div className="flex items-center gap-2 rounded-lg border border-line bg-surface px-3 py-2 text-[12px] font-medium text-ink-700">
                    <Navigation className="h-3.5 w-3.5" />
                    Route:{" "}
                    <span className="font-mono">{alert.routePlanId}</span>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Metadata */}
          <div className="rounded-xl border border-line bg-surface p-4 shadow-sm">
            <h3 className="mb-2.5 text-[13px] font-semibold text-ink-950">
              Metadata
            </h3>
            <div className="space-y-2 text-[12px]">
              <div className="flex justify-between">
                <span className="text-ink-500">Alert ID</span>
                <span className="font-mono font-medium text-ink-950">
                  {alert.id}
                </span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Type</span>
                <span className="font-medium text-ink-950">
                  {typeLabel(alert.alertType)}
                </span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Created</span>
                <span className="font-medium text-ink-950">
                  {formatTimestamp(alert.createdAt)}
                </span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Last updated</span>
                <span className="font-medium text-ink-950">
                  {formatTimestamp(alert.updatedAt)}
                </span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Severity</span>
                <span className={`font-medium ${sev.text}`}>
                  {severityKey(alert.severity)}
                </span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Status</span>
                <span className={`font-medium ${stat.text}`}>{stat.label}</span>
              </div>
              <div className="flex justify-between border-t border-line pt-2">
                <span className="text-ink-500">Version</span>
                <span className="font-mono font-medium text-ink-950">
                  {alert.version}
                </span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}