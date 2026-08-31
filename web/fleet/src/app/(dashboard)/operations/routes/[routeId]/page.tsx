// src/app/(dashboard)/operations/routes/[routeId]/page.tsx

"use client";

import { useState, useEffect, useRef } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import {
  MapPin,
  Truck,
  Package,
  Clock,
  CheckCircle2,
  AlertCircle,
  Edit3,
  Rocket,
  TrendingUp,
  GitBranch,
  User,
  Layers,
  Navigation,
} from "lucide-react";
import { ROUTE_DETAIL } from "@/lib/mock-routes-data";

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Draft: { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" },
  Locked: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Published: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const STOP_STATUS_STYLES = {
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: CheckCircle2 },
  Pending: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: Clock },
  Skipped: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: AlertCircle },
  Exception: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: AlertCircle },
};

export default function RouteOverviewTab() {
  const params = useParams();
  const routeId = params.routeId as string;

  const [loading, setLoading] = useState(true);
  const [route, setRoute] = useState(ROUTE_DETAIL);
  const [hoveredStopIndex, setHoveredStopIndex] = useState<number | null>(null);
  const stopRefs = useRef<(HTMLDivElement | null)[]>([]);
  const scrollContainerRef = useRef<HTMLDivElement>(null);

  const totalStops = route.stops.length;
  const completedStops = route.stops.filter((s) => s.status === "Completed").length;
  const progress = Math.round((completedStops / totalStops) * 100);
  const isPublished = route.status === "Published";
  const isDraft = route.status === "Draft";

  const activeStopIndex = route.stops.findIndex((s) => s.status !== "Completed");
  const activeStopIndexDisplay = activeStopIndex === -1 ? totalStops - 1 : activeStopIndex;

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  useEffect(() => {
    if (!loading && scrollContainerRef.current) {
      const container = scrollContainerRef.current;
      const activeElement = container.children[activeStopIndexDisplay] as HTMLElement;
      if (activeElement) {
        const containerWidth = container.offsetWidth;
        const elementLeft = activeElement.offsetLeft;
        const elementWidth = activeElement.offsetWidth;
        container.scrollTo({
          left: elementLeft - containerWidth / 2 + elementWidth / 2,
          behavior: "smooth",
        });
      }
    }
  }, [loading, activeStopIndexDisplay]);

  const scrollToStop = (index: number) => {
    const element = stopRefs.current[index];
    if (element) {
      element.scrollIntoView({ behavior: "smooth", block: "center" });
    }
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading route...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Stats grid ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <MapPin className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Stops</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalStops}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Completed</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{completedStops}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">ETA</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{route.eta}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Truck className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Distance</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{route.distance}</div>
        </div>
      </div>

      {/* ── Step indicator ── */}
      <div className="rounded-lg border border-line bg-white p-5">
        {/* Header */}
        <div className="flex flex-wrap items-center justify-between gap-3 mb-5">
          <div className="flex items-center gap-4">
            <span className="text-sm font-medium text-ink-600">
              <span className="font-semibold text-ink-950">{completedStops}</span> of {totalStops} stops complete
            </span>
            <span className="hidden sm:block w-px h-5 bg-line" />
            <span className="hidden sm:flex items-center gap-1.5 text-sm text-ink-600">
              <Navigation className="h-4 w-4 text-primary" />
              Active: <span className="font-medium text-ink-950">Stop {activeStopIndexDisplay + 1}</span>
            </span>
          </div>
          <div className="flex items-center gap-3">
            <span className="text-sm font-medium text-ink-700">{progress}%</span>
            <div className="h-1.5 w-24 rounded-full bg-surface-3 overflow-hidden">
              <div className="h-full rounded-full bg-primary transition-all duration-500" style={{ width: `${progress}%` }} />
            </div>
          </div>
        </div>

        {/* Desktop: Steps */}
        <div className="hidden sm:flex items-center justify-between w-full gap-1">
          {route.stops.map((stop, index) => {
            const isCompleted = stop.status === "Completed";
            const isActive = index === activeStopIndexDisplay && !isCompleted;
            const isLast = index === totalStops - 1;

            return (
              <div key={stop.id} className="flex items-center flex-1">
                <button
                  onClick={() => scrollToStop(index)}
                  className="flex flex-col items-center gap-1.5 group flex-1 transition-opacity hover:opacity-80"
                >
                  <div
                    className={`flex h-10 w-10 items-center justify-center rounded-full transition-colors ${
                      isCompleted 
                        ? "bg-primary text-white" 
                        : isActive 
                          ? "border-2 border-primary text-primary bg-white shadow-[0_0_0_3px_rgba(43,44,170,0.15)]" 
                          : "border border-line text-ink-400 bg-white hover:border-ink-300"
                    }`}
                  >
                    {isCompleted ? (
                      <CheckCircle2 className="h-5 w-5" />
                    ) : (
                      <span className={`text-sm font-medium ${isActive ? "text-primary" : ""}`}>
                        {index + 1}
                      </span>
                    )}
                  </div>
                  <span className={`text-[10px] font-medium text-center max-w-[80px] truncate ${
                    isCompleted ? "text-green" : isActive ? "text-primary" : "text-ink-500"
                  }`}>
                    {stop.location}
                  </span>
                  <span className="text-[8px] text-ink-400 uppercase tracking-wide">
                    {stop.type}
                  </span>
                </button>

                {!isLast && (
                  <div className={`flex-1 h-px mx-1 ${isCompleted ? "bg-primary" : "bg-line"}`} />
                )}
              </div>
            );
          })}
        </div>

        {/* Mobile: Scrollable */}
        <div 
          ref={scrollContainerRef}
          className="flex sm:hidden items-center gap-4 overflow-x-auto py-3 px-1 scrollbar-hide snap-x"
        >
          {route.stops.map((stop, index) => {
            const isCompleted = stop.status === "Completed";
            const isActive = index === activeStopIndexDisplay && !isCompleted;
            const isLast = index === totalStops - 1;

            return (
              <div key={stop.id} className="flex items-center flex-shrink-0 snap-start">
                <button
                  onClick={() => scrollToStop(index)}
                  className="flex flex-col items-center gap-1 min-w-[64px]"
                >
                  <div
                    className={`flex h-10 w-10 items-center justify-center rounded-full transition-colors ${
                      isCompleted 
                        ? "bg-primary text-white" 
                        : isActive 
                          ? "border-2 border-primary text-primary bg-white shadow-[0_0_0_3px_rgba(43,44,170,0.15)]" 
                          : "border border-line text-ink-400 bg-white"
                    }`}
                  >
                    {isCompleted ? (
                      <CheckCircle2 className="h-5 w-5" />
                    ) : (
                      <span className={`text-sm font-medium ${isActive ? "text-primary" : ""}`}>
                        {index + 1}
                      </span>
                    )}
                  </div>
                  <span className={`text-[9px] font-medium text-center ${
                    isCompleted ? "text-green" : isActive ? "text-primary" : "text-ink-500"
                  }`}>
                    {stop.location.length > 10 ? stop.location.slice(0, 10) + "…" : stop.location}
                  </span>
                </button>
                {!isLast && (
                  <div className={`w-5 h-px mx-0.5 ${isCompleted ? "bg-primary" : "bg-line"}`} />
                )}
              </div>
            );
          })}
        </div>
      </div>

      {/* ── Two column layout ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* Stops list */}
        <div className="lg:col-span-2">
          <div className="rounded-lg border border-line bg-white">
            <div className="flex items-center justify-between border-b border-line px-4 py-3">
              <div className="flex items-center gap-2">
                <MapPin className="h-4 w-4 text-ink-400" />
                <span className="text-sm font-semibold text-ink-950">Stops</span>
              </div>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {totalStops}
              </span>
            </div>

            <div className="divide-y divide-line max-h-[400px] overflow-y-auto">
              {route.stops.map((stop, index) => {
                const stopStyle = STOP_STATUS_STYLES[stop.status as keyof typeof STOP_STATUS_STYLES] || STOP_STATUS_STYLES.Pending;
                const StopIcon = stopStyle.icon;
                const isLast = index === totalStops - 1;
                const isCompleted = stop.status === "Completed";
                const isActive = index === activeStopIndexDisplay && !isCompleted;

                return (
                  <div
                    key={stop.id}
                    ref={(el) => { stopRefs.current[index] = el; }}
                    className={`px-4 py-3 transition-colors ${
                      isActive ? "bg-primary-bg/30" : ""
                    } ${!isActive && !isCompleted ? "hover:bg-surface-2" : ""}`}
                  >
                    <div className="flex gap-3">
                      <div className="flex flex-col items-center">
                        <div
                          className={`flex h-8 w-8 items-center justify-center rounded-full text-xs font-bold ${
                            isCompleted ? "bg-green-bg text-green" : isActive ? "bg-primary-bg text-primary" : "bg-surface-2 text-ink-500"
                          }`}
                        >
                          {index + 1}
                        </div>
                        {!isLast && (
                          <div className={`mt-1 h-6 w-px ${isCompleted ? "bg-green/20" : "bg-line"}`} />
                        )}
                      </div>

                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="text-sm font-semibold text-ink-950">
                            {stop.type}: {stop.location}
                          </span>
                          <span
                            className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${stopStyle.bg} ${stopStyle.text}`}
                          >
                            <StopIcon className="h-3 w-3" />
                            {stop.status}
                          </span>
                          {isActive && (
                            <span className="flex items-center gap-1 rounded-full bg-primary-bg px-2 py-0.5 text-[10px] font-semibold text-primary">
                              <Navigation className="h-3 w-3" />
                              Active
                            </span>
                          )}
                        </div>
                        <div className="mt-0.5 text-xs text-ink-500">{stop.address}</div>
                        {stop.timeWindow && (
                          <div className="mt-1 flex items-center gap-1 text-[10px] text-ink-500">
                            <Clock className="h-3 w-3" />
                            Window: {stop.timeWindow}
                          </div>
                        )}
                        {stop.assignedTo && (
                          <div className="mt-1 flex items-center gap-1.5 text-[10px] text-ink-500">
                            <User className="h-3 w-3" />
                            {stop.assignedTo}
                          </div>
                        )}
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>

            <div className="flex flex-wrap items-center gap-4 border-t border-line px-4 py-3">
              <span className="text-xs text-ink-500">
                <span className="font-medium text-ink-700">Distance:</span> {route.distance}
              </span>
              <span className="text-xs text-ink-500">
                <span className="font-medium text-ink-700">Time:</span> {route.duration}
              </span>
              <span className="text-xs text-ink-500">
                <span className="font-medium text-ink-700">ETA:</span> {route.eta}
              </span>
            </div>
          </div>

          {route.instructions && (
            <div className="rounded-lg border border-line bg-white p-4">
              <span className="text-xs font-medium uppercase tracking-wide text-ink-500">Instructions</span>
              <p className="mt-1 text-sm text-ink-700">{route.instructions}</p>
            </div>
          )}
        </div>

        {/* Sidebar */}
        <div className="space-y-4">
          {/* Provider */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="flex items-center gap-2 mb-3">
              <User className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Provider</span>
            </div>
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary-bg text-sm font-bold text-primary">
                {route.provider.name.split(" ").map((n) => n[0]).join("")}
              </div>
              <div>
                <div className="text-sm font-semibold text-ink-950">{route.provider.name}</div>
                <div className="flex items-center gap-2 text-xs text-ink-500">
                  <span className="flex items-center gap-0.5">
                    <span className="text-amber">★</span>
                    {route.provider.rating}
                  </span>
                  <span className="text-ink-300">·</span>
                  <span className="font-mono text-[10px]">{route.provider.id}</span>
                </div>
              </div>
            </div>
          </div>

          {/* Asset */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="flex items-center gap-2 mb-3">
              <Truck className="h-4 w-4 text-ink-400" />
              <span className="text-sm font-semibold text-ink-950">Asset</span>
            </div>
            <div>
              <div className="text-sm font-semibold text-ink-950">{route.asset.name}</div>
              <div className="flex items-center gap-2 text-xs text-ink-500">
                <span className="font-mono text-[10px]">{route.asset.id}</span>
                <span className="text-ink-300">·</span>
                <span>{route.asset.type}</span>
              </div>
            </div>
          </div>

          {/* Actions */}
          <div className="rounded-lg border border-line bg-white p-4">
            <span className="text-sm font-semibold text-ink-950 mb-3 block">Quick actions</span>
            <div className="space-y-2">
              <Link
                href={`/operations/routes/${routeId}/builder`}
                className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-700 hover:bg-surface-2 transition-colors"
              >
                <Edit3 className="h-3.5 w-3.5" />
                Edit route
              </Link>
              <Link
                href={`/operations/routes/${routeId}/optimize`}
                className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-700 hover:bg-surface-2 transition-colors"
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
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}