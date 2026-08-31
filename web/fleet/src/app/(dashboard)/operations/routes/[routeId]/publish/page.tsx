// src/app/(dashboard)/operations/routes/[routeId]/publish/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  Rocket,
  CheckCircle2,
  AlertCircle,
  MapPin,
  User,
  Truck,
  Clock,
  XCircle,
  FileCheck,
  ArrowRight,
  RefreshCw,
  Shield,
} from "lucide-react";
import { ROUTE_DETAIL } from "@/lib/mock-routes-data";

type ValidationItem = {
  label: string;
  status: "valid" | "warning" | "invalid";
  message: string;
};

export default function RoutePublishPage() {
  const params = useParams();
  const router = useRouter();
  const routeId = params.routeId as string;

  const [loading, setLoading] = useState(true);
  const [publishing, setPublishing] = useState(false);
  const [published, setPublished] = useState(false);
  const [route, setRoute] = useState(ROUTE_DETAIL);

  const [validations] = useState<ValidationItem[]>([
    { label: "All stops have valid locations", status: "valid", message: "6 stops verified" },
    { label: "Provider assigned", status: "valid", message: "Peter Okafor (P-044)" },
    { label: "Asset assigned", status: "valid", message: "Honda ACE 110 (BR-88)" },
    { label: "Route has at least 2 stops", status: "valid", message: "6 stops" },
    { label: "Time windows are valid", status: "warning", message: "2 stops have no time window" },
    { label: "Safety compliance", status: "valid", message: "All checks passed" },
  ]);

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  const handlePublish = async () => {
    setPublishing(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setPublishing(false);
    setPublished(true);

    setTimeout(() => {
      router.push(`/operations/routes/${routeId}`);
    }, 1500);
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading...</span>
        </div>
      </div>
    );
  }

  if (published) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-green/20 bg-white p-12 text-center">
        <div className="rounded-full bg-green-bg p-4">
          <Rocket className="h-10 w-10 text-green" />
        </div>
        <h1 className="mt-4 text-2xl font-bold text-ink-950">Route published!</h1>
        <p className="mt-1.5 max-w-md text-sm text-ink-600">
          The route has been published and is now available for execution.
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
            className="rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Back to routes
          </Link>
        </div>
      </div>
    );
  }

  const allValid = validations.every((v) => v.status !== "invalid");
  const hasWarnings = validations.some((v) => v.status === "warning");

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm text-ink-500">
            Review and publish route <span className="font-mono">{routeId}</span>
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href={`/operations/routes/${routeId}`}
            className="flex items-center gap-2 rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Back
          </Link>
          <button
            onClick={handlePublish}
            disabled={!allValid || publishing}
            className="flex items-center gap-2 rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed"
          >
            {publishing ? (
              <>
                <RefreshCw className="h-4 w-4 animate-spin" />
                Publishing...
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

      {/* ── Route Summary ── */}
      <div className="rounded-lg border border-line bg-white p-4">
        <div className="flex flex-wrap items-center gap-4 text-sm">
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Route:</span>
            <span className="font-semibold text-ink-950">{route.name}</span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <User className="h-4 w-4 text-ink-400" />
            <span className="text-ink-700">{route.provider.name}</span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <Truck className="h-4 w-4 text-ink-400" />
            <span className="text-ink-700">{route.asset.name}</span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span className="text-ink-700">{route.branch}</span>
          </span>
        </div>
      </div>

      {/* ── Validation checks ── */}
      <div className="rounded-lg border border-line bg-white">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <FileCheck className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Validation checks</span>
          </div>
          {allValid && !hasWarnings && (
            <span className="flex items-center gap-1 rounded-full bg-green-bg px-2.5 py-0.5 text-xs font-semibold text-green">
              <CheckCircle2 className="h-3 w-3" />
              All checks passed
            </span>
          )}
          {hasWarnings && allValid && (
            <span className="flex items-center gap-1 rounded-full bg-amber-bg px-2.5 py-0.5 text-xs font-semibold text-amber">
              <AlertCircle className="h-3 w-3" />
              Warnings
            </span>
          )}
          {!allValid && (
            <span className="flex items-center gap-1 rounded-full bg-red-bg px-2.5 py-0.5 text-xs font-semibold text-red">
              <XCircle className="h-3 w-3" />
              Issues found
            </span>
          )}
        </div>

        <div className="divide-y divide-line">
          {validations.map((item, index) => {
            const Icon =
              item.status === "valid"
                ? CheckCircle2
                : item.status === "warning"
                ? AlertCircle
                : XCircle;
            const color =
              item.status === "valid"
                ? "text-green"
                : item.status === "warning"
                ? "text-amber"
                : "text-red";
            const bg =
              item.status === "valid"
                ? "bg-green-bg/10"
                : item.status === "warning"
                ? "bg-amber-bg/10"
                : "bg-red-bg/10";

            return (
              <div
                key={index}
                className={`flex items-center gap-3 px-4 py-3 ${bg}`}
              >
                <Icon className={`h-4 w-4 ${color} flex-shrink-0`} />
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium text-ink-950">{item.label}</div>
                  <div className="text-xs text-ink-500">{item.message}</div>
                </div>
                <span className={`text-xs font-semibold ${color}`}>
                  {item.status}
                </span>
              </div>
            );
          })}
        </div>
      </div>

      {/* ── Warning notice ── */}
      {hasWarnings && allValid && (
        <div className="rounded-lg border border-amber/20 bg-amber-bg/20 p-4">
          <div className="flex items-start gap-3">
            <AlertCircle className="h-5 w-5 text-amber flex-shrink-0 mt-0.5" />
            <div>
              <div className="text-sm font-semibold text-amber">Warnings detected</div>
              <p className="text-sm text-ink-600">
                The route has warnings that may affect performance. You can still publish, but review the warnings above.
              </p>
            </div>
          </div>
        </div>
      )}

      {/* ── Stops preview ── */}
      <div className="rounded-lg border border-line bg-white">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Route stops</span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {route.stops.length}
            </span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {route.stops.slice(0, 5).map((stop, index) => (
            <div key={stop.id} className="flex items-center gap-3 px-4 py-2.5 text-sm">
              <span className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-surface-2 text-[10px] font-bold text-ink-500">
                {index + 1}
              </span>
              <span className="text-ink-700">{stop.type}: {stop.location}</span>
              {index < route.stops.length - 1 && index < 4 && (
                <ArrowRight className="h-3 w-3 text-ink-300 ml-auto" />
              )}
            </div>
          ))}
          {route.stops.length > 5 && (
            <div className="px-4 py-2.5 text-xs text-ink-500">
              +{route.stops.length - 5} more stops
            </div>
          )}
        </div>
      </div>
    </div>
  );
}