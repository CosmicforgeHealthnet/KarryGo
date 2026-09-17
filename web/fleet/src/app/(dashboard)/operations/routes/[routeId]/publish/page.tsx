// src/app/(dashboard)/operations/routes/[routeId]/publish/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  Rocket,
  CheckCircle2,
  AlertCircle,
  MapPin,
  Truck,
  XCircle,
  User,
  ArrowRight,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import {
  routeService,
  newIdempotencyKey,
} from "@/lib/services/routeService";
import type { RoutePlan, RouteStop } from "@/lib/types/route";

function stopTypeLabel(t: string): string {
  switch ((t || "").toUpperCase()) {
    case "PICKUP":              return "Pickup";
    case "DELIVERY":            return "Delivery";
    case "WAREHOUSE_TRANSFER":  return "Warehouse transfer";
    case "RETURN":              return "Return";
    case "DEPOT_START":         return "Depot start";
    case "DEPOT_END":           return "Depot end";
    default:                    return t || "—";
  }
}

function statusLabel(status: string): string {
  switch ((status || "").toUpperCase()) {
    case "DRAFT":                return "Draft";
    case "OPTIMIZATION_PENDING": return "Optimizing";
    case "OPTIMIZED":            return "Optimized";
    case "READY":                return "Ready";
    case "APPROVED":             return "Approved";
    case "PUBLISHED":            return "Published";
    case "IN_PROGRESS":          return "In progress";
    case "COMPLETED":            return "Completed";
    case "PARTIALLY_COMPLETED":  return "Partial";
    case "FAILED":               return "Failed";
    case "CANCELLED":            return "Cancelled";
    default:                     return status || "—";
  }
}

export default function RoutePublishPage() {
  const params = useParams();
  const router = useRouter();
  const routeId = typeof params.routeId === "string" ? params.routeId : "";

  const [route, setRoute] = useState<RoutePlan | null>(null);
  const [stops, setStops] = useState<RouteStop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [publishing, setPublishing] = useState(false);
  const [published, setPublished] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [overrideReason, setOverrideReason] = useState("");

  const idempotencyKey = useMemo(() => newIdempotencyKey(), []);

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

  const handlePublish = async () => {
    if (!route) return;
    setPublishing(true);
    setActionError(null);
    try {
      await routeService.publishRoute(
        route.id,
        {
          expectedVersion: route.version,
          overrideReason: overrideReason.trim() || undefined,
        },
        idempotencyKey,
      );
      setPublished(true);
      // Refetch to confirm the new state.
      const updated = await routeService.getRoute(route.id);
      setRoute(updated);
    } catch (err: any) {
      setActionError(
        err?.response?.data?.error?.message ??
          err?.message ??
          "Could not publish route.",
      );
    } finally {
      setPublishing(false);
    }
  };

  if (loading) {
    return (
      <div className="space-y-5">
        <div className="h-16 rounded-lg border border-line bg-surface" />
        <div className="h-64 rounded-lg border border-line bg-surface" />
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

  const status = (route.status || "").toUpperCase();
  const isApproved = status === "APPROVED";
  const isPublished = status === "PUBLISHED";

  // Summary of pre-conditions that the backend will enforce. This is
  // informational only -- the actual validation happens server-side.
  const conditions = [
    {
      label: "Route is approved",
      met: isApproved || isPublished,
      detail: isApproved || isPublished
        ? "Route passed approval"
        : "Route must be approved first",
    },
    {
      label: "Provider assigned",
      met: !!route.providerId,
      detail: route.providerId || "No provider assigned",
    },
    {
      label: "Vehicle assigned",
      met: !!route.vehicleOrTruckReference,
      detail: route.vehicleOrTruckReference || "No vehicle assigned",
    },
    {
      label: "Route has stops",
      met: stops.length > 0,
      detail: stops.length > 0 ? `${stops.length} stops` : "No stops",
    },
  ];
  const allReady = conditions.every((c) => c.met);

  if (published || isPublished) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-green/20 bg-surface p-12 text-center">
        <div className="rounded-full bg-green-bg p-4">
          <Rocket className="h-10 w-10 text-green" />
        </div>
        <h1 className="mt-4 text-2xl font-bold text-ink-950">Route published</h1>
        <p className="mt-1.5 max-w-md text-sm text-ink-600">
          The route is now available for execution.
        </p>
        <div className="mt-4 flex gap-3">
          <Link
            href={`/operations/routes/${routeId}`}
            className="rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            View route
          </Link>
          <Link
            href="/operations/routes"
            className="rounded-lg border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Back to routes
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm text-ink-500">
            Review and publish route{" "}
            <span className="font-mono">{route.routeReference || route.id}</span>
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href={`/operations/routes/${routeId}`}
            className="flex items-center gap-2 rounded-lg border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Back
          </Link>
          <button
            onClick={handlePublish}
            disabled={publishing || !allReady}
            className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
          >
            {publishing ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                Publishing…
              </>
            ) : (
              <>
                <Rocket className="h-4 w-4" />
                Publish route
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

      {/* Route summary */}
      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="flex flex-wrap items-center gap-4 text-sm">
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Status:</span>
            <span className="font-semibold text-ink-950">
              {statusLabel(route.status)}
            </span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <User className="h-4 w-4 text-ink-400" />
            <span className="font-mono text-ink-700">
              {route.providerId || "—"}
            </span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <Truck className="h-4 w-4 text-ink-400" />
            <span className="font-mono text-ink-700">
              {route.vehicleOrTruckReference || "—"}
            </span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span className="font-mono text-ink-700">
              {route.branchId || "—"}
            </span>
          </span>
        </div>
      </div>

      {/* Pre-conditions -- informational; the backend enforces the real rules */}
      <div className="rounded-lg border border-line bg-surface">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <span className="text-sm font-semibold text-ink-950">
            Pre-conditions
          </span>
          {allReady ? (
            <span className="flex items-center gap-1 rounded-full bg-green-bg px-2.5 py-0.5 text-xs font-semibold text-green">
              <CheckCircle2 className="h-3 w-3" />
              Ready to publish
            </span>
          ) : (
            <span className="flex items-center gap-1 rounded-full bg-red-bg px-2.5 py-0.5 text-xs font-semibold text-red">
              <XCircle className="h-3 w-3" />
              Not ready
            </span>
          )}
        </div>
        <div className="divide-y divide-line">
          {conditions.map((c) => {
            const Icon = c.met ? CheckCircle2 : XCircle;
            const color = c.met ? "text-green" : "text-red";
            return (
              <div
                key={c.label}
                className={`flex items-center gap-3 px-4 py-3 ${
                  c.met ? "bg-green-bg/5" : "bg-red-bg/5"
                }`}
              >
                <Icon className={`h-4 w-4 ${color} flex-shrink-0`} />
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium text-ink-950">
                    {c.label}
                  </div>
                  <div className="text-xs text-ink-500">{c.detail}</div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* Override reason — sent to the backend when the route carries warnings */}
      <div className="rounded-lg border border-line bg-surface p-4">
        <label
          htmlFor="override-reason"
          className="mb-2 block text-xs font-medium text-ink-700"
        >
          Override reason (optional)
        </label>
        <textarea
          id="override-reason"
          value={overrideReason}
          onChange={(e) => setOverrideReason(e.target.value)}
          placeholder="Provide a reason if this route carries warnings you are deliberately overriding."
          rows={3}
          className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
        />
        <p className="mt-1.5 text-xs text-ink-500">
          The backend requires a reason when the selected provider or vehicle
          carries a warning. It is ignored otherwise.
        </p>
      </div>

      {/* Stops preview */}
      <div className="rounded-lg border border-line bg-surface">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">
              Route stops
            </span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {stops.length}
            </span>
          </div>
        </div>
        {stops.length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
            title="No stops on this route"
            description="Add stops before publishing."
            actionHref={`/operations/routes/${routeId}/builder`}
            actionLabel="Open builder"
          />
        ) : (
          <div className="divide-y divide-line">
            {stops.slice(0, 5).map((stop, index) => (
              <div
                key={stop.id}
                className="flex items-center gap-3 px-4 py-2.5 text-sm"
              >
                <span className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-surface-2 text-[10px] font-bold text-ink-500">
                  {index + 1}
                </span>
                <span className="text-ink-700">
                  {stopTypeLabel(stop.stopType)}
                  {stop.locationReference ? ` · ${stop.locationReference}` : ""}
                </span>
                {index < stops.length - 1 && index < 4 && (
                  <ArrowRight className="h-3 w-3 text-ink-400 ml-auto" />
                )}
              </div>
            ))}
            {stops.length > 5 && (
              <div className="px-4 py-2.5 text-xs text-ink-500">
                +{stops.length - 5} more stops
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}