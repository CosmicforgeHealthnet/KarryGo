// src/app/(dashboard)/operations/routes/[routeId]/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  MapPin,
  Truck,
  Clock,
  CheckCircle2,
  AlertCircle,
  Edit3,
  TrendingUp,
  Rocket,
  User,
  Navigation,
  Circle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { routeService } from "@/lib/services/routeService";
import type { RoutePlan, RouteStop } from "@/lib/types/route";

function stopStatusStyle(status: string): {
  bg: string;
  text: string;
  dot: string;
  Icon: typeof CheckCircle2;
} {
  switch ((status || "").toUpperCase()) {
    case "COMPLETED":
      return { bg: "bg-green-bg", text: "text-green", dot: "bg-green", Icon: CheckCircle2 };
    case "SERVICE_STARTED":
    case "ARRIVED":
      return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", Icon: Navigation };
    case "SKIPPED":
    case "FAILED":
      return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", Icon: AlertCircle };
    case "CANCELLED":
      return { bg: "bg-red-bg", text: "text-red", dot: "bg-red", Icon: AlertCircle };
    default:
      return { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", Icon: Clock };
  }
}

function stopStatusLabel(status: string): string {
  switch ((status || "").toUpperCase()) {
    case "PLANNED":         return "Planned";
    case "ASSIGNED":        return "Assigned";
    case "EN_ROUTE":        return "En route";
    case "ARRIVED":         return "Arrived";
    case "SERVICE_STARTED": return "Service started";
    case "COMPLETED":       return "Completed";
    case "FAILED":          return "Failed";
    case "SKIPPED":         return "Skipped";
    case "CANCELLED":       return "Cancelled";
    default:                return status || "—";
  }
}

function formatTimestamp(iso: string | undefined): string {
  if (!iso) return "—";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "—";
  return new Date(t).toLocaleString(undefined, {
    month: "short", day: "numeric",
    hour: "2-digit", minute: "2-digit",
  });
}

function durationDisplay(seconds: number): string {
  if (!seconds) return "—";
  if (seconds < 60) return `${seconds}s`;
  const m = Math.floor(seconds / 60);
  if (m < 60) return `${m} min`;
  const h = Math.floor(m / 60);
  return `${h}h ${m % 60}m`;
}

export default function RouteOverviewTab() {
  const params = useParams();
  const routeId = typeof params.routeId === "string" ? params.routeId : "";

  const [route, setRoute] = useState<RoutePlan | null>(null);
  const [stops, setStops] = useState<RouteStop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!routeId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    Promise.all([routeService.getRoute(routeId), routeService.listStops(routeId)])
      .then(([r, s]) => {
        if (cancelled) return;
        setRoute(r);
        setStops(s);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) setError("Route not found.");
        else
          setError(
            err?.response?.data?.error?.message ??
              err?.message ??
              "Failed to load route.",
          );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [routeId]);

  const progress = useMemo(() => {
    const total = stops.length;
    if (total === 0) return { completed: 0, total: 0, pct: 0 };
    const completed = stops.filter(
      (s) => (s.status || "").toUpperCase() === "COMPLETED",
    ).length;
    return { completed, total, pct: Math.round((completed / total) * 100) };
  }, [stops]);

  const activeStopIndex = useMemo(
    () => stops.findIndex((s) => (s.status || "").toUpperCase() !== "COMPLETED"),
    [stops],
  );

  if (loading) {
    return (
      <div className="space-y-5">
        <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-24 rounded-lg border border-line bg-surface" />
          ))}
        </div>
        <div className="h-48 rounded-lg border border-line bg-surface" />
      </div>
    );
  }

  if (error || !route) {
    return (
      <div className="rounded-lg border border-line bg-surface">
        <EmptyState
          image="/images/trips-empty.png"
          title={error ?? "Route not found"}
          description="The route may have been deleted or the URL may be incorrect."
          actionHref="/operations/routes"
          actionLabel="Back to routes"
        />
      </div>
    );
  }

  const isDraft = (route.status || "").toUpperCase() === "DRAFT";
  const isPublished = (route.status || "").toUpperCase() === "PUBLISHED";

  return (
    <div className="space-y-5">
      {/* Stats */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <MapPin className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Stops</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {stops.length}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Completed</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {progress.completed}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Duration</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {durationDisplay(route.plannedDurationSeconds)}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Truck className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Distance</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {route.plannedDistance
              ? `${route.plannedDistance.toFixed(1)} ${route.distanceUnit || "km"}`
              : "—"}
          </div>
        </div>
      </div>

      {/* Progress strip */}
      <div className="rounded-lg border border-line bg-surface p-5">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <span className="text-sm text-ink-600">
            <span className="font-semibold text-ink-950">{progress.completed}</span> of{" "}
            {progress.total} stops complete
          </span>
          <div className="flex items-center gap-3">
            <span className="text-sm font-medium text-ink-700">{progress.pct}%</span>
            <div className="h-1.5 w-24 rounded-full bg-surface-3 overflow-hidden">
              <div
                className="h-full rounded-full bg-primary transition-all duration-500"
                style={{ width: `${progress.pct}%` }}
              />
            </div>
          </div>
        </div>

        {stops.length === 0 ? (
          <p className="text-sm text-ink-500">No stops yet.</p>
        ) : (
          <div className="flex items-center gap-1 overflow-x-auto scrollbar-hide pb-1">
            {stops.map((stop, index) => {
              const s = (stop.status || "").toUpperCase();
              const isCompleted = s === "COMPLETED";
              const isActive = index === activeStopIndex && !isCompleted;
              const isLast = index === stops.length - 1;
              return (
                <div key={stop.id} className="flex items-center flex-shrink-0">
                  <div className="flex flex-col items-center gap-1.5 min-w-[64px]">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-full text-sm font-medium ${
                        isCompleted
                          ? "bg-primary text-white"
                          : isActive
                          ? "border-2 border-primary text-primary bg-surface"
                          : "border border-line text-ink-400 bg-surface"
                      }`}
                    >
                      {isCompleted ? <CheckCircle2 className="h-5 w-5" /> : index + 1}
                    </div>
                    <span
                      className={`text-[10px] font-medium text-center truncate max-w-[80px] ${
                        isCompleted ? "text-green" : isActive ? "text-primary" : "text-ink-500"
                      }`}
                    >
                      {stop.locationReference || stop.stopType}
                    </span>
                  </div>
                  {!isLast && (
                    <div
                      className={`mx-1 h-px w-6 ${isCompleted ? "bg-primary" : "bg-line"}`}
                    />
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>

      {/* Stops list + sidebar */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        <div className="space-y-5 lg:col-span-2">
          <div className="rounded-lg border border-line bg-surface">
            <div className="flex items-center justify-between border-b border-line px-4 py-3">
              <div className="flex items-center gap-2">
                <MapPin className="h-4 w-4 text-ink-400" />
                <span className="text-sm font-semibold text-ink-950">Stops</span>
              </div>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {stops.length}
              </span>
            </div>

            {stops.length === 0 ? (
              <EmptyState
                image="/images/trips-empty.png"
                title="No stops on this route"
                description="Stops will appear here once they are added."
              />
            ) : (
              <div className="divide-y divide-line max-h-[400px] overflow-y-auto">
                {stops.map((stop, index) => {
                  const st = stopStatusStyle(stop.status);
                  const Icon = st.Icon;
                  const isCompleted = (stop.status || "").toUpperCase() === "COMPLETED";
                  const isActive = index === activeStopIndex && !isCompleted;
                  return (
                    <div
                      key={stop.id}
                      className={`px-4 py-3 transition-colors ${
                        isActive ? "bg-primary-bg/30" : "hover:bg-surface-2"
                      }`}
                    >
                      <div className="flex gap-3">
                        <div
                          className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                            isCompleted
                              ? "bg-green-bg text-green"
                              : isActive
                              ? "bg-primary-bg text-primary"
                              : "bg-surface-2 text-ink-500"
                          }`}
                        >
                          {index + 1}
                        </div>
                        <div className="flex-1 min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="text-sm font-semibold text-ink-950">
                              {stop.stopType}
                            </span>
                            <span
                              className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${st.bg} ${st.text}`}
                            >
                              <Icon className="h-3 w-3" />
                              {stopStatusLabel(stop.status)}
                            </span>
                            {isActive && (
                              <span className="flex items-center gap-1 rounded-full bg-primary-bg px-2 py-0.5 text-[10px] font-semibold text-primary">
                                <Navigation className="h-3 w-3" />
                                Active
                              </span>
                            )}
                          </div>
                          {stop.locationReference && (
                            <div className="mt-0.5 text-xs text-ink-600">
                              {stop.locationReference}
                            </div>
                          )}
                          {stop.timeWindowStart && stop.timeWindowEnd && (
                            <div className="mt-1 flex items-center gap-1 text-[10px] text-ink-500">
                              <Clock className="h-3 w-3" />
                              Window: {formatTimestamp(stop.timeWindowStart)} →{" "}
                              {formatTimestamp(stop.timeWindowEnd)}
                            </div>
                          )}
                          {stop.plannedArrivalAt && (
                            <div className="mt-0.5 text-[10px] text-ink-500">
                              Planned arrival: {formatTimestamp(stop.plannedArrivalAt)}
                            </div>
                          )}
                          {stop.actualArrivalAt && (
                            <div className="mt-0.5 text-[10px] text-green">
                              Actual arrival: {formatTimestamp(stop.actualArrivalAt)}
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        </div>

        <div className="space-y-4">
          <div className="rounded-lg border border-line bg-surface p-4">
            <div className="mb-3 flex items-center gap-2">
              <Truck className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Assignment</span>
            </div>
            <div className="space-y-2 text-xs">
              <div className="flex justify-between gap-2">
                <span className="text-ink-500">Provider</span>
                <span className="font-mono text-ink-900 truncate">
                  {route.providerId || "—"}
                </span>
              </div>
              <div className="flex justify-between gap-2 border-t border-line pt-2">
                <span className="text-ink-500">Vehicle</span>
                <span className="font-mono text-ink-900 truncate">
                  {route.vehicleOrTruckReference || "—"}
                </span>
              </div>
              <div className="flex justify-between gap-2 border-t border-line pt-2">
                <span className="text-ink-500">Service</span>
                <span className="text-ink-900">{route.executionService || "—"}</span>
              </div>
              <div className="flex justify-between gap-2 border-t border-line pt-2">
                <span className="text-ink-500">Version</span>
                <span className="font-mono text-ink-900">{route.version}</span>
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-line bg-surface p-4">
            <div className="mb-3 flex items-center gap-2">
              <User className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Quick actions</span>
            </div>
            <div className="space-y-2">
              <Link
                href={`/operations/routes/${routeId}/builder`}
                className="flex w-full items-center gap-2 rounded-md border border-line bg-surface px-3 py-2 text-xs font-medium text-ink-700 hover:bg-surface-2 transition-colors"
              >
                <Edit3 className="h-3.5 w-3.5" />
                Edit route
              </Link>
              <Link
                href={`/operations/routes/${routeId}/optimize`}
                className="flex w-full items-center gap-2 rounded-md border border-line bg-surface px-3 py-2 text-xs font-medium text-ink-700 hover:bg-surface-2 transition-colors"
              >
                <TrendingUp className="h-3.5 w-3.5" />
                Optimize
              </Link>
              {isDraft && (
                <Link
                  href={`/operations/routes/${routeId}/publish`}
                  className="flex w-full items-center gap-2 rounded-md bg-primary px-3 py-2 text-xs font-semibold text-white hover:bg-primary-hover transition-colors"
                >
                  <Rocket className="h-3.5 w-3.5" />
                  Publish route
                </Link>
              )}
              {isPublished && (
                <Link
                  href={`/operations/routes/${routeId}/progress`}
                  className="flex w-full items-center gap-2 rounded-md border border-line bg-surface px-3 py-2 text-xs font-medium text-ink-700 hover:bg-surface-2 transition-colors"
                >
                  <Circle className="h-3.5 w-3.5" />
                  View progress
                </Link>
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}