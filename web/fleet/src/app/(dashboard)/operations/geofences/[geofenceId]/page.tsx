// src/app/(dashboard)/operations/geofences/[geofenceId]/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  ArrowLeft,
  MapPin,
  Clock,
  Calendar,
  Circle,
  AlertCircle,
  CheckCircle2,
  XCircle,
  ShieldAlert,
  User,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { geofenceService } from "@/lib/services/geofenceService";
import type { Geofence, Point } from "@/lib/types/geofence";

function geometrySummary(g: Geofence): {
  type: string;
  detail: string;
  points: Point[];
} {
  const t = (g.geometryType || "").toUpperCase();
  if (t === "CIRCLE" && g.geometry?.center) {
    const r = g.geometry.radiusMeters ?? 0;
    return {
      type: "Circle",
      detail: `${Math.round(r)} m radius`,
      points: [g.geometry.center],
    };
  }
  if (t === "POLYGON" && g.geometry?.points) {
    return {
      type: "Polygon",
      detail: `${Math.max(0, g.geometry.points.length - 1)} vertices`,
      points: g.geometry.points,
    };
  }
  return { type: g.geometryType || "—", detail: "—", points: [] };
}

function statusInfo(status: string): {
  label: string;
  bg: string;
  text: string;
  dot: string;
} {
  if ((status || "").toUpperCase() === "ACTIVE") {
    return {
      label: "Active",
      bg: "bg-green-bg",
      text: "text-green",
      dot: "bg-green",
    };
  }
  return {
    label: "Inactive",
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
  };
}

function typeInfo(rules: string[]): {
  label: string;
  bg: string;
  text: string;
  Icon: typeof MapPin;
} {
  if ((rules ?? []).includes("RESTRICTED_AREA")) {
    return { label: "Restricted", bg: "bg-red-bg", text: "text-red", Icon: ShieldAlert };
  }
  if ((rules ?? []).includes("DWELL")) {
    return { label: "Dwell zone", bg: "bg-primary-bg", text: "text-primary", Icon: Circle };
  }
  return { label: "Standard", bg: "bg-surface-2", text: "text-ink-500", Icon: MapPin };
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

function eventRuleLabel(rule: string): string {
  switch ((rule || "").toUpperCase()) {
    case "ENTER":             return "Enter";
    case "EXIT":              return "Exit";
    case "DWELL":             return "Dwell";
    case "ROUTE_DEVIATION":   return "Route deviation";
    case "RESTRICTED_AREA":   return "Restricted area";
    default:                  return rule;
  }
}

export default function GeofenceDetailPage() {
  const params = useParams();
  const geofenceId = typeof params.geofenceId === "string" ? params.geofenceId : "";

  const [geofence, setGeofence] = useState<Geofence | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [busy, setBusy] = useState<"activate" | "deactivate" | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  useEffect(() => {
    if (!geofenceId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    geofenceService
      .getGeofence(geofenceId)
      .then((data) => {
        if (!cancelled) setGeofence(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setError("Geofence not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load geofence.";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [geofenceId]);

  const summary = useMemo(
    () => (geofence ? geometrySummary(geofence) : null),
    [geofence],
  );

  const handleToggleActive = async () => {
    if (!geofence) return;
    const isActive = (geofence.activeStatus || "").toUpperCase() === "ACTIVE";
    setBusy(isActive ? "deactivate" : "activate");
    setActionError(null);
    try {
      const updated = isActive
        ? await geofenceService.deactivateGeofence(geofence.id, geofence.version)
        : await geofenceService.activateGeofence(geofence.id, geofence.version);
      setGeofence(updated);
    } catch (err: any) {
      const message =
        err?.response?.data?.error?.message ??
        err?.message ??
        "Could not update geofence.";
      setActionError(message);
    } finally {
      setBusy(null);
    }
  };

  if (loading) {
    return (
      <div className="space-y-5">
        <div className="h-6 w-48 rounded bg-surface-2" />
        <div className="h-40 rounded-lg border border-line bg-surface" />
      </div>
    );
  }

  if (error || !geofence || !summary) {
    return (
      <div className="space-y-5">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/geofences"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <h1>{geofenceId}</h1>
        </div>
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/live-operations-empty.png"
            title={error ?? "Geofence not found"}
            description="The geofence may have been deleted or the URL may be incorrect."
            actionHref="/operations/geofences"
            actionLabel="Back to geofences"
          />
        </div>
      </div>
    );
  }

  const status = statusInfo(geofence.activeStatus);
  const type = typeInfo(geofence.eventRules);
  const isActive = (geofence.activeStatus || "").toUpperCase() === "ACTIVE";

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/geofences"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <div className="flex flex-wrap items-center gap-2.5">
              <h1>{geofence.name}</h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${status.bg} ${status.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${status.dot}`} />
                {status.label}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${type.bg} ${type.text}`}
              >
                <type.Icon className="h-3 w-3" />
                {type.label}
              </span>
            </div>
            <p className="mt-0.5 font-mono text-sm text-ink-500">
              {geofence.id}
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={handleToggleActive}
            disabled={busy !== null}
            className={`flex h-10 items-center gap-1.5 rounded-lg px-3.5 text-sm font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-60 ${
              isActive
                ? "border border-amber/30 bg-amber-bg text-amber hover:bg-amber-bg/70"
                : "bg-green text-white hover:opacity-90"
            }`}
          >
            {busy === "activate" || busy === "deactivate" ? (
              <>
                <div className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-current/30 border-t-current" />
                {isActive ? "Deactivating…" : "Activating…"}
              </>
            ) : isActive ? (
              <>
                <XCircle className="h-3.5 w-3.5" />
                Deactivate
              </>
            ) : (
              <>
                <CheckCircle2 className="h-3.5 w-3.5" />
                Activate
              </>
            )}
          </button>
        </div>
      </div>

      {actionError && (
        <div className="flex items-start gap-2 rounded-lg border border-red/20 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{actionError}</span>
        </div>
      )}

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* Left */}
        <div className="space-y-5">
          {/* Coordinates list — replaces the mock SVG map */}
          <div className="rounded-lg border border-line bg-surface p-4">
            <div className="mb-3 flex items-center gap-2">
              <Circle className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">
                Geometry
              </span>
            </div>
            <div className="mb-3 flex flex-wrap items-baseline gap-2 text-sm">
              <span className="font-medium text-ink-950">{summary.type}</span>
              <span className="text-ink-500">· {summary.detail}</span>
            </div>
            {summary.points.length === 0 ? (
              <p className="text-xs text-ink-500">No coordinates recorded.</p>
            ) : (
              <div className="max-h-[280px] overflow-y-auto rounded-md border border-line bg-surface-2 p-3 font-mono text-[11px] leading-6 text-ink-700">
                {summary.points.map((p, i) => (
                  <div key={i}>
                    {i.toString().padStart(2, "0")} · {p.latitude.toFixed(6)}, {p.longitude.toFixed(6)}
                  </div>
                ))}
              </div>
            )}
            {geofence.geometryReference && (
              <div className="mt-3 border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                  Reference
                </div>
                <div className="mt-0.5 font-mono text-xs text-ink-800 break-all">
                  {geofence.geometryReference}
                </div>
              </div>
            )}
          </div>

          {/* Event rules */}
          <div className="rounded-lg border border-line bg-surface p-4">
            <div className="mb-3 flex items-center gap-2">
              <ShieldAlert className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">
                Event rules
              </span>
            </div>
            {(!geofence.eventRules || geofence.eventRules.length === 0) ? (
              <p className="text-xs text-ink-500">
                No event rules configured. The geofence will not trigger on
                route events.
              </p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {geofence.eventRules.map((rule) => (
                  <span
                    key={rule}
                    className="rounded-full bg-surface-2 px-2.5 py-1 text-[11px] font-semibold text-ink-700"
                  >
                    {eventRuleLabel(rule)}
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Right */}
        <div className="space-y-5">
          <div className="rounded-lg border border-line bg-surface p-4">
            <div className="mb-4 flex items-center gap-2.5">
              <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
                <MapPin className="h-4 w-4" />
              </div>
              <span className="text-sm font-semibold text-ink-950">Details</span>
            </div>

            <div className="space-y-3">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                  ID
                </div>
                <div className="mt-0.5 font-mono text-sm text-ink-950">
                  {geofence.id}
                </div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                  Branch
                </div>
                <div className="mt-0.5 font-mono text-sm text-ink-950">
                  {geofence.branchId || "—"}
                </div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                  Created
                </div>
                <div className="mt-0.5 flex items-center gap-2 text-sm text-ink-950">
                  <Calendar className="h-4 w-4 text-ink-400" />
                  {formatTimestamp(geofence.createdAt)}
                </div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                  Last updated
                </div>
                <div className="mt-0.5 flex items-center gap-2 text-sm text-ink-950">
                  <Clock className="h-4 w-4 text-ink-400" />
                  {formatTimestamp(geofence.updatedAt)}
                </div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                  Created by
                </div>
                <div className="mt-0.5 flex items-center gap-2 font-mono text-sm text-ink-950">
                  <User className="h-4 w-4 text-ink-400" />
                  {geofence.createdBy || "—"}
                </div>
              </div>
              <div className="border-t border-line pt-3">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                  Version
                </div>
                <div className="mt-0.5 font-mono text-sm text-ink-950">
                  {geofence.version}
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}