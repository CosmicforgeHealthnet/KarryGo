// src/app/(dashboard)/operations/alerts/page.tsx

"use client";

import React, { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Bell,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  Clock,
  MapPin,
  Search,
  SlidersHorizontal,
  X,
  Eye,
  Flag,
  Users,
  Navigation,
  Shield,
  Truck,
  Info,
  Activity,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { alertsService } from "@/lib/services/alertsService";
import type { OperationalAlert } from "@/lib/types/alerts";

// ---------------------------------------------------------------------------
// Presentation helpers
//
// Every mapping here is keyed on a real backend enum value. Alert severity is
// free text on the backend (no CHECK constraint), so unknown values fall back
// to a neutral style rather than disappearing.
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

// Alert types emitted by the backend. Backend values (from the dashboard
// usecase's buildAlertTitle) are a closed set; anything else falls back to a
// generic icon so an unfamiliar type still renders.
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
  NO_ELIGIBLE_PROVIDER: "No provider",
};

function typeLabel(alertType: string): string {
  return TYPE_LABELS[alertType] || alertType || "—";
}

function typeIcon(alertType: string) {
  return TYPE_ICONS[alertType] || Bell;
}

// SLA countdown from SLA due timestamp. Positive = time remaining,
// non-positive = breached.
function slaCountdown(slaDueAtIso: string): { text: string; minutes: number } {
  const t = new Date(slaDueAtIso).getTime();
  if (!Number.isFinite(t)) return { text: "—", minutes: Number.MAX_SAFE_INTEGER };
  const minutes = Math.floor((t - Date.now()) / 60_000);
  if (minutes <= 0) return { text: "Breached", minutes };
  if (minutes < 60) return { text: `${minutes} min`, minutes };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { text: `${hours} h`, minutes };
  const days = Math.floor(hours / 24);
  return { text: `${days} d`, minutes };
}

function timeAgo(iso: string): string {
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "—";
  const sec = Math.max(1, Math.floor((Date.now() - then) / 1000));
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  return `${day}d ago`;
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AlertsPage() {
  const [alerts, setAlerts] = useState<OperationalAlert[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    alertsService
      .listAlerts({ limit: 100 })
      .then((data) => {
        if (!cancelled) setAlerts(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          "Failed to load alerts.";
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const severities = useMemo(() => {
    const set = new Set<string>();
    alerts.forEach((a) => {
      const key = severityKey(a.severity);
      if (key) set.add(key);
    });
    return ["All", ...Array.from(set)];
  }, [alerts]);

  const statuses = useMemo(() => {
    const set = new Set<string>();
    alerts.forEach((a) => {
      const key = STATUS_STYLES[statusKey(a.status)].label;
      set.add(key);
    });
    return ["All", ...Array.from(set)];
  }, [alerts]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return alerts.filter((a) => {
      if (selectedSeverity !== "All" && severityKey(a.severity) !== selectedSeverity) {
        return false;
      }
      if (
        selectedStatus !== "All" &&
        STATUS_STYLES[statusKey(a.status)].label !== selectedStatus
      ) {
        return false;
      }
      if (q === "") return true;
      const haystack = [
        a.id,
        a.sourceId,
        a.alertType,
        a.branchId ?? "",
        a.canonicalTripReference ?? "",
        a.assignmentId ?? "",
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [alerts, search, selectedSeverity, selectedStatus]);

  const openCount = useMemo(
    () =>
      alerts.filter(
        (a) => a.status === "OPEN" || a.status === "ACKNOWLEDGED" || a.status === "IN_PROGRESS",
      ).length,
    [alerts],
  );
  const criticalCount = useMemo(
    () =>
      alerts.filter(
        (a) =>
          (a.severity || "").toUpperCase() === "CRITICAL" ||
          (a.severity || "").toUpperCase() === "HIGH",
      ).length,
    [alerts],
  );
  const resolvedCount = useMemo(
    () => alerts.filter((a) => a.status === "RESOLVED").length,
    [alerts],
  );

  const hasActiveFilters = selectedSeverity !== "All" || selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedSeverity("All");
    setSelectedStatus("All");
  };

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Operational Alerts</h1>
          <p className="mt-1.5">
            Operational alerts with severity, condition, source, owner, and SLA.
            Acknowledge and resolve issues quickly.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href="/operations/control-tower"
            className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <Eye className="h-3.5 w-3.5" />
            Tower
          </Link>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/20 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Open</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-red">
            {openCount}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertTriangle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Critical</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-red">
            {criticalCount}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Resolved</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {resolvedCount}
          </div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search alerts..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedSeverity !== "All" ? 1 : 0) + (selectedStatus !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filtered.length} alerts
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Severity
              </span>
              <div className="flex flex-wrap gap-1.5">
                {severities.map((s) => (
                  <button
                    key={s}
                    onClick={() => setSelectedSeverity(s)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedSeverity === s
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Status
              </span>
              <div className="flex flex-wrap gap-1.5">
                {statuses.map((s) => (
                  <button
                    key={s}
                    onClick={() => setSelectedStatus(s)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === s
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Alerts list ── */}
      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading alerts…
        </div>
      ) : alerts.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/alerts-empty.png"
            title="No alerts"
            description="Operational alerts will appear here when something needs attention."
          />
        </div>
      ) : filtered.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/alerts-empty.png"
            title="No alerts match your filters"
            description="Try adjusting your search or clearing the filters."
            action={
              <button
                onClick={clearFilters}
                className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
              >
                Clear filters
              </button>
            }
          />
        </div>
      ) : (
        <div className="flex flex-col gap-2.5">
          {filtered.map((a) => {
            const sev = SEVERITY_STYLES[severityKey(a.severity)];
            const stat = STATUS_STYLES[statusKey(a.status)];
            const SeverityIcon = sev.icon;
            const TypeIcon = typeIcon(a.alertType);
            const isCritical = severityKey(a.severity) === "Critical" || severityKey(a.severity) === "High";
            const isOpen = a.status === "OPEN";
            const sla = slaCountdown(a.slaDueAt);
            const breached = sla.text === "Breached";

            return (
              <div
                key={a.id}
                className={`flex flex-col gap-2.5 rounded-lg border p-3.5 transition-colors sm:flex-row sm:items-start sm:justify-between ${
                  isCritical
                    ? "border-red/30 bg-red-bg/20 hover:bg-red-bg/30"
                    : "border-line bg-surface hover:bg-surface-2"
                }`}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Link
                      href={`/operations/alerts/${a.id}`}
                      className="font-mono text-sm font-semibold text-ink-950 hover:text-primary transition-colors"
                    >
                      {a.id}
                    </Link>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${sev.bg} ${sev.text}`}>
                      <SeverityIcon className="h-3 w-3" />
                      {severityKey(a.severity)}
                    </span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${stat.bg} ${stat.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${stat.dot}`} />
                      {stat.label}
                    </span>
                    <span className="flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                      <TypeIcon className="h-3 w-3" />
                      {typeLabel(a.alertType)}
                    </span>
                    {isOpen && (
                      <span className="flex items-center gap-1 rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
                        <span className="relative flex h-1.5 w-1.5">
                          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                          <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                        </span>
                        Active
                      </span>
                    )}
                  </div>

                  <div className="mt-1.5 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                    <span className="flex items-center gap-1">
                      <MapPin className="h-3.5 w-3.5" />
                      {a.branchId || "—"}
                    </span>
                    <span className="text-ink-400">·</span>
                    <span className="flex items-center gap-1">
                      <Clock className="h-3.5 w-3.5" />
                      {timeAgo(a.createdAt)}
                    </span>
                    <span className="text-ink-400">·</span>
                    <span className="flex items-center gap-1">
                      <Bell className="h-3.5 w-3.5" />
                      <span className="font-medium text-ink-700">{a.sourceType}</span>
                      <span className="font-mono">{a.sourceId}</span>
                    </span>
                    {a.ownerUserId && (
                      <>
                        <span className="text-ink-400">·</span>
                        <span className="flex items-center gap-1">
                          <Users className="h-3.5 w-3.5" />
                          <span className="font-mono">{a.ownerUserId}</span>
                        </span>
                      </>
                    )}
                    <span className="text-ink-400">·</span>
                    <span className="flex items-center gap-1">
                      <Clock className="h-3.5 w-3.5" />
                      <span className="font-medium text-ink-700">SLA:</span>
                      <span className={breached ? "font-semibold text-red" : "text-ink-500"}>
                        {sla.text}
                      </span>
                    </span>
                  </div>
                </div>

                <div className="flex flex-shrink-0 items-center gap-2">
                  <Link
                    href={`/operations/alerts/${a.id}`}
                    className="inline-flex items-center gap-1 rounded-md border border-line bg-surface px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                  >
                    <Eye className="h-3.5 w-3.5" />
                    View
                  </Link>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}