// src/app/(dashboard)/operations/routes/[routeId]/optimize/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  TrendingUp,
  RefreshCw,
  CheckCircle2,
  AlertCircle,
  Clock,
  MapPin,
  ArrowRight,
  Gauge,
  Fuel,
  Zap,
  Sparkles,
} from "lucide-react";
import { ROUTE_DETAIL } from "@/lib/mock-routes-data";

export default function RouteOptimizePage() {
  const params = useParams();
  const router = useRouter();
  const routeId = params.routeId as string;

  const [loading, setLoading] = useState(true);
  const [optimizing, setOptimizing] = useState(false);
  const [route, setRoute] = useState(ROUTE_DETAIL);
  const [optimized, setOptimized] = useState(false);
  const [optimizationResult, setOptimizationResult] = useState<{
    distanceSaved: string;
    timeSaved: string;
    improvements: string[];
  } | null>(null);

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  const handleOptimize = async () => {
    setOptimizing(true);
    await new Promise((resolve) => setTimeout(resolve, 2000));
    setOptimizing(false);
    setOptimized(true);
    setOptimizationResult({
      distanceSaved: "2.4 km",
      timeSaved: "8 min",
      improvements: [
        "Reordered stops for optimal route",
        "Reduced backtracking by 34%",
        "Prioritized time-sensitive deliveries",
      ],
    });
  };

  const handleApply = () => {
    router.push(`/operations/routes/${routeId}/builder`);
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading optimization...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm text-ink-500">
            Optimize stop order for this route
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href={`/operations/routes/${routeId}`}
            className="flex items-center gap-2 rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Back
          </Link>
          {!optimized ? (
            <button
              onClick={handleOptimize}
              disabled={optimizing}
              className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed"
            >
              {optimizing ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  Optimizing...
                </>
              ) : (
                <>
                  <Zap className="h-4 w-4" />
                  Run optimization
                </>
              )}
            </button>
          ) : (
            <button
              onClick={handleApply}
              className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
            >
              <CheckCircle2 className="h-4 w-4" />
              Apply optimization
            </button>
          )}
        </div>
      </div>

      {/* ── Optimization Status ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className={`rounded-full p-2 ${
              optimized ? "bg-green-bg" : optimizing ? "bg-amber-bg" : "bg-primary-bg"
            }`}>
              {optimized ? (
                <CheckCircle2 className="h-5 w-5 text-green" />
              ) : optimizing ? (
                <RefreshCw className="h-5 w-5 text-amber animate-spin" />
              ) : (
                <TrendingUp className="h-5 w-5 text-primary" />
              )}
            </div>
            <div>
              <div className="text-sm font-semibold text-ink-950">
                {optimized
                  ? "Optimization complete"
                  : optimizing
                  ? "Optimizing route..."
                  : "Ready to optimize"}
              </div>
              <div className="text-xs text-ink-500">
                {optimized
                  ? "Route optimized with improved stop order"
                  : optimizing
                  ? "Calculating optimal stop sequence..."
                  : "Click 'Run optimization' to find the optimal route"}
              </div>
            </div>
          </div>
          <div className="flex items-center gap-3 text-xs text-ink-500">
            <span>Stops: {route.stops.length}</span>
            <span className="text-ink-300">·</span>
            <span>Distance: {route.distance}</span>
          </div>
        </div>
      </div>

      {/* ── Results ── */}
      {optimized && optimizationResult && (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="rounded-lg border border-green/20 bg-white p-4">
            <div className="flex items-center gap-2">
              <Fuel className="h-4 w-4 text-green" />
              <span className="text-xs font-medium text-green">Distance saved</span>
            </div>
            <div className="mt-1 text-xl font-bold text-ink-950">{optimizationResult.distanceSaved}</div>
          </div>
          <div className="rounded-lg border border-green/20 bg-white p-4">
            <div className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-green" />
              <span className="text-xs font-medium text-green">Time saved</span>
            </div>
            <div className="mt-1 text-xl font-bold text-ink-950">{optimizationResult.timeSaved}</div>
          </div>
          <div className="rounded-lg border border-green/20 bg-white p-4">
            <div className="flex items-center gap-2">
              <Gauge className="h-4 w-4 text-green" />
              <span className="text-xs font-medium text-green">Efficiency gain</span>
            </div>
            <div className="mt-1 text-xl font-bold text-ink-950">34%</div>
          </div>
        </div>
      )}

      {/* ── Improvement details ── */}
      {optimized && optimizationResult && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2 mb-3">
            <Sparkles className="h-4 w-4 text-amber" />
            <span className="text-sm font-semibold text-ink-950">Improvements</span>
          </div>
          <ul className="space-y-2">
            {optimizationResult.improvements.map((item, index) => (
              <li key={index} className="flex items-start gap-2.5 text-sm text-ink-700">
                <CheckCircle2 className="h-4 w-4 text-green flex-shrink-0 mt-0.5" />
                {item}
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* ── Current stops preview ── */}
      <div className="rounded-lg border border-line bg-white">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Current stops</span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {route.stops.length}
            </span>
          </div>
          {optimized && (
            <span className="flex items-center gap-1 rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
              <CheckCircle2 className="h-3 w-3" />
              Optimized
            </span>
          )}
        </div>
        <div className="divide-y divide-line">
          {route.stops.slice(0, 6).map((stop, index) => (
            <div key={stop.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
              <span className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-surface-2 text-[10px] font-bold text-ink-500">
                {index + 1}
              </span>
              <span className="text-ink-700">{stop.type}: {stop.location}</span>
              {index < route.stops.length - 1 && index < 5 && (
                <ArrowRight className="h-3 w-3 text-ink-300 ml-auto" />
              )}
            </div>
          ))}
          {route.stops.length > 6 && (
            <div className="px-4 py-2.5 text-xs text-ink-500">
              +{route.stops.length - 6} more stops
            </div>
          )}
        </div>
      </div>
    </div>
  );
}