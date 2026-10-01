// src/app/(dashboard)/operations/routes/[routeId]/optimize/page.tsx

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  RefreshCw,
  CheckCircle2,
  AlertCircle,
  TrendingUp,
  MapPin,
  ArrowRight,
  Sparkles,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { routeService } from "@/lib/services/routeService";
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

export default function RouteOptimizePage() {
  const params = useParams();
  const routeId = typeof params.routeId === "string" ? params.routeId : "";

  const [route, setRoute] = useState<RoutePlan | null>(null);
  const [stops, setStops] = useState<RouteStop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [requested, setRequested] = useState(false);

  const loadAll = async () => {
    const [r, s] = await Promise.all([
      routeService.getRoute(routeId),
      routeService.listStops(routeId),
    ]);
    setRoute(r);
    setStops(s);
  };

  useEffect(() => {
    if (!routeId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    loadAll()
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [routeId]);

  const handleOptimize = async () => {
    if (!route) return;
    setSubmitting(true);
    setActionError(null);
    try {
      await routeService.optimizeRoute(route.id, {
        expectedVersion: route.version,
      });
      setRequested(true);
      // The endpoint returns 202 and enqueues a job. Refetch once shortly
      // after so the page reflects the new OPTIMIZATION_PENDING status; the
      // final result appears on the next visit.
      setTimeout(() => {
        loadAll().catch(() => {
          // Silent; the banner already shows the request was accepted.
        });
      }, 1500);
    } catch (err: any) {
      setActionError(
        err?.response?.data?.error?.message ??
          err?.message ??
          "Could not request optimization.",
      );
    } finally {
      setSubmitting(false);
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
  const optimizable = status === "DRAFT" || status === "OPTIMIZED" || status === "READY";
  const isPending = status === "OPTIMIZATION_PENDING";

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm text-ink-500">
            Request optimization of the current stop order for this route.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href={`/operations/routes/${routeId}`}
            className="flex items-center gap-2 rounded-lg border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Back
          </Link>
          {optimizable && !isPending && (
            <button
              onClick={handleOptimize}
              disabled={submitting}
              className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:cursor-not-allowed disabled:opacity-60"
            >
              {submitting ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  Requesting…
                </>
              ) : (
                <>
                  <TrendingUp className="h-4 w-4" />
                  Request optimization
                </>
              )}
            </button>
          )}
        </div>
      </div>

      {actionError && (
        <div className="flex items-start gap-2 rounded-lg border border-red/20 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{actionError}</span>
        </div>
      )}

      {/* Status banner */}
      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div
              className={`rounded-full p-2 ${
                isPending
                  ? "bg-amber-bg"
                  : requested
                  ? "bg-green-bg"
                  : "bg-primary-bg"
              }`}
            >
              {isPending ? (
                <RefreshCw className="h-5 w-5 text-amber animate-spin" />
              ) : requested ? (
                <CheckCircle2 className="h-5 w-5 text-green" />
              ) : (
                <TrendingUp className="h-5 w-5 text-primary" />
              )}
            </div>
            <div>
              <div className="text-sm font-semibold text-ink-950">
                {isPending
                  ? "Optimization in progress"
                  : requested
                  ? "Optimization requested"
                  : "Ready to request optimization"}
              </div>
              <div className="text-xs text-ink-500">
                {isPending
                  ? "The optimizer is calculating a better stop order. This page will reflect the result once it completes."
                  : requested
                  ? "The request was accepted. Refresh in a moment to see the updated route."
                  : "Click 'Request optimization' to submit the current stop list to the optimizer."}
              </div>
            </div>
          </div>
          <div className="flex items-center gap-3 text-xs text-ink-500">
            <span>Stops: {stops.length}</span>
            <span className="text-ink-400">·</span>
            <span>
              Distance:{" "}
              {route.plannedDistance
                ? `${route.plannedDistance.toFixed(1)} ${route.distanceUnit || "km"}`
                : "—"}
            </span>
          </div>
        </div>
      </div>

      {/* Optimizer reference */}
      {route.optimizationReference && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-center gap-2 mb-2">
            <Sparkles className="h-4 w-4 text-amber" />
            <span className="text-sm font-semibold text-ink-950">
              Latest optimization
            </span>
          </div>
          <div className="grid grid-cols-2 gap-3 text-xs">
            <div>
              <div className="text-[10px] uppercase tracking-wide text-ink-500">
                Reference
              </div>
              <div className="mt-0.5 font-mono text-ink-900 break-all">
                {route.optimizationReference}
              </div>
            </div>
            <div>
              <div className="text-[10px] uppercase tracking-wide text-ink-500">
                Version
              </div>
              <div className="mt-0.5 font-mono text-ink-900">
                {route.optimizationVersion}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Current stops preview */}
      <div className="rounded-lg border border-line bg-surface">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">
              Current stop order
            </span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {stops.length}
            </span>
          </div>
        </div>
        {stops.length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
            title="No stops to optimize"
            description="Add stops in the builder before requesting optimization."
          />
        ) : (
          <div className="divide-y divide-line">
            {stops.map((stop, index) => (
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
                {index < stops.length - 1 && (
                  <ArrowRight className="h-3 w-3 text-ink-400 ml-auto" />
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}