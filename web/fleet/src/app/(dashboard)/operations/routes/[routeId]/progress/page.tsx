// src/app/(dashboard)/operations/routes/[routeId]/progress/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  GitBranch,
  MapPin,
  CheckCircle2,
  Clock,
  AlertCircle,
  RefreshCw,
  User,
  Truck,
  Navigation,
  SkipForward,
  RotateCcw,
} from "lucide-react";
import { ROUTE_DETAIL } from "@/lib/mock-routes-data";

const STOP_STATUS_STYLES = {
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Skipped: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Exception: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function RouteProgressPage() {
  const params = useParams();
  const router = useRouter();
  const routeId = params.routeId as string;

  const [loading, setLoading] = useState(true);
  const [route, setRoute] = useState(ROUTE_DETAIL);

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading progress...</span>
        </div>
      </div>
    );
  }

  const totalStops = route.stops.length;
  const completedStops = route.stops.filter((s) => s.status === "Completed").length;
  const progress = Math.round((completedStops / totalStops) * 100);
  const isComplete = progress === 100;

  const currentStopIndex = route.stops.findIndex((s) => s.status !== "Completed");
  const currentStop = currentStopIndex !== -1 ? route.stops[currentStopIndex] : null;

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm text-ink-500">
            Live progress for this route
          </p>
        </div>
        <Link
          href={`/operations/routes/${routeId}`}
          className="flex items-center gap-2 rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          Back
        </Link>
      </div>

      {/* ── Progress Overview ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <CheckCircle2 className="h-5 w-5 text-green" />
              <span className="text-base font-semibold text-ink-950">
                {progress}%
              </span>
            </div>
            <span className="hidden sm:block w-px h-6 bg-line" />
            <div className="flex items-center gap-2">
              <span className="text-sm text-ink-600">
                <span className="font-semibold text-ink-950">{completedStops}</span> of {totalStops} stops
              </span>
            </div>
            <span className="hidden sm:block w-px h-6 bg-line" />
            <div className="flex items-center gap-2">
              <Navigation className="h-4 w-4 text-primary" />
              <span className="text-sm text-ink-600">
                Current: <span className="font-medium text-ink-950">
                  {currentStop ? `${currentStop.type} - ${currentStop.location}` : "Complete!"}
                </span>
              </span>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-sm text-ink-500">ETA: {route.eta}</span>
            {isComplete && (
              <span className="flex items-center gap-1 rounded-full bg-green-bg px-2.5 py-0.5 text-xs font-semibold text-green">
                <CheckCircle2 className="h-3.5 w-3.5" />
                Complete
              </span>
            )}
          </div>
        </div>

        {/* Progress bar */}
        <div className="mt-3 h-2 rounded-full bg-surface-3 overflow-hidden">
          <div
            className={`h-full rounded-full transition-all duration-500 ${
              isComplete ? "bg-green" : "bg-primary"
            }`}
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>

      {/* ── Stop Status ── */}
      <div className="rounded-lg border border-line bg-white">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <GitBranch className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Stop Status</span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {totalStops}
            </span>
          </div>
        </div>

        <div className="divide-y divide-line">
          {route.stops.map((stop, index) => {
            const statusStyle = STOP_STATUS_STYLES[stop.status as keyof typeof STOP_STATUS_STYLES] || STOP_STATUS_STYLES.Pending;
            const isCompleted = stop.status === "Completed";
            const isCurrent = index === currentStopIndex && !isCompleted;

            return (
              <div
                key={stop.id}
                className={`flex items-center gap-3 px-4 py-3 transition-colors ${
                  isCompleted
                    ? ""
                    : isCurrent
                    ? "bg-primary-bg/20"
                    : "hover:bg-surface-2"
                }`}
              >
                <div
                  className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full text-xs font-bold ${
                    isCompleted
                      ? "bg-green-bg text-green"
                      : isCurrent
                      ? "bg-primary-bg text-primary ring-2 ring-primary/20"
                      : "bg-surface-2 text-ink-500"
                  }`}
                >
                  {index + 1}
                </div>

                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-sm font-medium text-ink-950">
                      {stop.type}: {stop.location}
                    </span>
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                    >
                      <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                      {stop.status}
                    </span>
                    {isCurrent && (
                      <span className="flex items-center gap-1 rounded-full bg-primary-bg px-2 py-0.5 text-[10px] font-semibold text-primary">
                        <Navigation className="h-3 w-3" />
                        Current
                      </span>
                    )}
                  </div>
                  <div className="text-xs text-ink-500">{stop.address}</div>
                </div>

                {isCurrent && !isCompleted && (
                  <div className="flex items-center gap-1.5 flex-shrink-0">
                    <button className="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover">
                      <CheckCircle2 className="inline h-3.5 w-3.5 mr-1" />
                      Complete
                    </button>
                    <button className="rounded-md border border-line bg-white px-2.5 py-1.5 text-xs font-medium text-ink-600 transition-colors hover:bg-surface-2">
                      <SkipForward className="h-3.5 w-3.5" />
                    </button>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* ── Actions ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <Link
          href={`/operations/routes/${routeId}/builder`}
          className="flex items-center justify-center gap-2 rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2 sm:col-span-2"
        >
          <GitBranch className="h-4 w-4" />
          Edit route
        </Link>
        <button className="flex items-center justify-center gap-2 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm font-semibold text-red transition-colors hover:bg-red-bg/70">
          <RotateCcw className="h-4 w-4" />
          Replan route
        </button>
      </div>

      {/* ── Provider & Asset ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2 mb-2">
            <User className="h-4 w-4 text-ink-400" />
            <span className="text-xs font-medium text-ink-600">Provider</span>
          </div>
          <div className="flex items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-full bg-primary-bg text-xs font-bold text-primary">
              {route.provider.name.split(" ").map((n) => n[0]).join("")}
            </div>
            <div>
              <div className="text-sm font-semibold text-ink-950">{route.provider.name}</div>
              <div className="text-xs text-ink-500">
                <span className="flex items-center gap-0.5">
                  <span className="text-amber">★</span>
                  {route.provider.rating}
                </span>
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2 mb-2">
            <Truck className="h-4 w-4 text-ink-400" />
            <span className="text-xs font-medium text-ink-600">Asset</span>
          </div>
          <div>
            <div className="text-sm font-semibold text-ink-950">{route.asset.name}</div>
            <div className="text-xs text-ink-500">
              <span className="font-mono text-[10px]">{route.asset.id}</span>
              <span className="text-ink-300">·</span>
              <span>{route.asset.type}</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}