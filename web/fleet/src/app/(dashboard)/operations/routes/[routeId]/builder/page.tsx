// src/app/(dashboard)/operations/routes/[routeId]/builder/page.tsx

"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  MapPin,
  Plus,
  X,
  Save,
  Trash2,
  Clock,
  AlertCircle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { routeService } from "@/lib/services/routeService";
import type { RoutePlan, RouteStop, CreateStopInput } from "@/lib/types/route";

const STOP_TYPES = [
  "PICKUP",
  "DELIVERY",
  "WAREHOUSE_TRANSFER",
  "RETURN",
  "DEPOT_START",
  "DEPOT_END",
];

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

function stopStatusLabel(s: string): string {
  switch ((s || "").toUpperCase()) {
    case "PLANNED":         return "Planned";
    case "ASSIGNED":        return "Assigned";
    case "EN_ROUTE":        return "En route";
    case "ARRIVED":         return "Arrived";
    case "SERVICE_STARTED": return "Service started";
    case "COMPLETED":       return "Completed";
    case "FAILED":          return "Failed";
    case "SKIPPED":         return "Skipped";
    case "CANCELLED":       return "Cancelled";
    default:                return s || "—";
  }
}

function formatTimestamp(iso: string | undefined): string {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "";
  return new Date(t).toLocaleString(undefined, {
    month: "short", day: "numeric",
    hour: "2-digit", minute: "2-digit",
  });
}

// The backend's add-stop input. Fields the mock used but the backend has no
// column for (a free-text address, an arbitrary "waypoint") are not present.
type StopForm = {
  stopType: string;
  locationReference: string;
  latitude: string;
  longitude: string;
  timeWindowStart: string;
  timeWindowEnd: string;
  serviceDurationSeconds: string;
};

const EMPTY_FORM: StopForm = {
  stopType: "PICKUP",
  locationReference: "",
  latitude: "",
  longitude: "",
  timeWindowStart: "",
  timeWindowEnd: "",
  serviceDurationSeconds: "",
};

function formToInput(form: StopForm): CreateStopInput | null {
  const lat = form.latitude.trim() === "" ? undefined : Number(form.latitude);
  const lng = form.longitude.trim() === "" ? undefined : Number(form.longitude);
  if (lat !== undefined && (!Number.isFinite(lat) || lat < -90 || lat > 90)) return null;
  if (lng !== undefined && (!Number.isFinite(lng) || lng < -180 || lng > 180)) return null;
  if ((lat === undefined) !== (lng === undefined)) return null;

  const dur = form.serviceDurationSeconds.trim() === ""
    ? undefined
    : Number(form.serviceDurationSeconds);

  return {
    stopType: form.stopType,
    locationReference: form.locationReference.trim() || undefined,
    latitude: lat,
    longitude: lng,
    timeWindowStart: form.timeWindowStart || undefined,
    timeWindowEnd: form.timeWindowEnd || undefined,
    serviceDurationSeconds: dur,
  };
}

export default function RouteBuilderPage() {
  const params = useParams();
  const router = useRouter();
  const routeId = typeof params.routeId === "string" ? params.routeId : "";

  const [route, setRoute] = useState<RoutePlan | null>(null);
  const [stops, setStops] = useState<RouteStop[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [addOpen, setAddOpen] = useState(false);
  const [form, setForm] = useState<StopForm>(EMPTY_FORM);
  const [busy, setBusy] = useState<"add" | string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

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

  const refetchStops = async () => {
    const s = await routeService.listStops(routeId);
    setStops(s);
  };

  const handleAdd = async () => {
    const input = formToInput(form);
    if (!input) {
      setActionError("Latitude and longitude must both be set, and within valid ranges.");
      return;
    }
    setBusy("add");
    setActionError(null);
    try {
      await routeService.createStop(routeId, input);
      await refetchStops();
      setAddOpen(false);
      setForm(EMPTY_FORM);
    } catch (err: any) {
      setActionError(
        err?.response?.data?.error?.message ??
          err?.message ??
          "Could not add stop.",
      );
    } finally {
      setBusy(null);
    }
  };

  const handleDelete = async (stop: RouteStop) => {
    setBusy(stop.id);
    setActionError(null);
    try {
      await routeService.deleteStop(routeId, stop.id);
      await refetchStops();
    } catch (err: any) {
      setActionError(
        err?.response?.data?.error?.message ??
          err?.message ??
          "Could not delete stop.",
      );
    } finally {
      setBusy(null);
    }
  };

  if (loading) {
    return (
      <div className="space-y-5">
        <div className="h-16 rounded-lg border border-line bg-surface" />
        <div className="h-72 rounded-lg border border-line bg-surface" />
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

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm text-ink-500">
            Add or remove stops for this route. The backend assigns sequence
            numbers automatically.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href={`/operations/routes/${routeId}`}
            className="flex items-center gap-2 rounded-lg border border-line bg-surface px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Cancel
          </Link>
        </div>
      </div>

      {/* Route summary */}
      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="flex flex-wrap items-center gap-4 text-sm">
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Route:</span>
            <span className="font-semibold text-ink-950">
              {route.routeReference || route.id}
            </span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Stops:</span>
            <span className="font-semibold text-ink-950">{stops.length}</span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Service:</span>
            <span className="font-semibold text-ink-950">
              {route.executionService}
            </span>
          </span>
          <span className="hidden sm:block w-px h-5 bg-line" />
          <span className="flex items-center gap-2">
            <span className="font-medium text-ink-600">Branch:</span>
            <span className="font-mono font-semibold text-ink-950">
              {route.branchId || "—"}
            </span>
          </span>
        </div>
      </div>

      {actionError && (
        <div className="flex items-start gap-2 rounded-lg border border-red/20 bg-red-bg px-4 py-3 text-sm text-red">
          <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          <span>{actionError}</span>
        </div>
      )}

      {/* Stops card */}
      <div className="rounded-lg border border-line bg-surface">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <div className="flex items-center gap-2">
            <MapPin className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Stops</span>
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {stops.length}
            </span>
          </div>
          {isDraft && (
            <button
              onClick={() => setAddOpen(true)}
              disabled={addOpen}
              className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              <Plus className="h-3.5 w-3.5" />
              Add stop
            </button>
          )}
        </div>

        {!isDraft && (
          <div className="border-b border-line bg-amber-bg/30 px-4 py-3 text-xs text-amber">
            Stops can only be added or removed while the route is in Draft.
          </div>
        )}

        {addOpen && (
          <div className="border-b border-line p-4 bg-primary-bg/10">
            <div className="mb-3 flex items-center justify-between">
              <span className="text-sm font-semibold text-ink-950">
                Add new stop
              </span>
              <button
                onClick={() => {
                  setAddOpen(false);
                  setForm(EMPTY_FORM);
                  setActionError(null);
                }}
                className="text-ink-400 hover:text-ink-600"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div>
                <label className="text-xs font-medium text-ink-600">Type</label>
                <select
                  value={form.stopType}
                  onChange={(e) =>
                    setForm((p) => ({ ...p, stopType: e.target.value }))
                  }
                  className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
                >
                  {STOP_TYPES.map((t) => (
                    <option key={t} value={t}>
                      {stopTypeLabel(t)}
                    </option>
                  ))}
                </select>
              </div>
              <div>
                <label className="text-xs font-medium text-ink-600">
                  Location reference
                </label>
                <input
                  value={form.locationReference}
                  onChange={(e) =>
                    setForm((p) => ({ ...p, locationReference: e.target.value }))
                  }
                  placeholder="e.g. Ikeja Mall"
                  className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
                />
              </div>
              <div>
                <label className="text-xs font-medium text-ink-600">
                  Latitude
                </label>
                <input
                  value={form.latitude}
                  onChange={(e) =>
                    setForm((p) => ({ ...p, latitude: e.target.value }))
                  }
                  placeholder="e.g. 6.5244"
                  className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
                />
              </div>
              <div>
                <label className="text-xs font-medium text-ink-600">
                  Longitude
                </label>
                <input
                  value={form.longitude}
                  onChange={(e) =>
                    setForm((p) => ({ ...p, longitude: e.target.value }))
                  }
                  placeholder="e.g. 3.3792"
                  className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
                />
              </div>
              <div>
                <label className="text-xs font-medium text-ink-600">
                  Time window start
                </label>
                <input
                  type="datetime-local"
                  value={form.timeWindowStart}
                  onChange={(e) =>
                    setForm((p) => ({ ...p, timeWindowStart: e.target.value }))
                  }
                  className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
              </div>
              <div>
                <label className="text-xs font-medium text-ink-600">
                  Time window end
                </label>
                <input
                  type="datetime-local"
                  value={form.timeWindowEnd}
                  onChange={(e) =>
                    setForm((p) => ({ ...p, timeWindowEnd: e.target.value }))
                  }
                  className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
              </div>
              <div className="sm:col-span-2">
                <label className="text-xs font-medium text-ink-600">
                  Service duration (seconds)
                </label>
                <input
                  value={form.serviceDurationSeconds}
                  onChange={(e) =>
                    setForm((p) => ({
                      ...p,
                      serviceDurationSeconds: e.target.value,
                    }))
                  }
                  placeholder="e.g. 300"
                  className="mt-1 w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20 placeholder:text-ink-400"
                />
              </div>
            </div>
            <div className="mt-3 flex gap-2.5">
              <button
                onClick={handleAdd}
                disabled={busy === "add"}
                className="flex items-center gap-2 rounded-md bg-primary px-4 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
              >
                {busy === "add" ? (
                  <>
                    <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                    Adding…
                  </>
                ) : (
                  <>
                    <Plus className="h-4 w-4" />
                    Add stop
                  </>
                )}
              </button>
              <button
                onClick={() => {
                  setAddOpen(false);
                  setForm(EMPTY_FORM);
                  setActionError(null);
                }}
                className="rounded-md border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                Cancel
              </button>
            </div>
          </div>
        )}

        {stops.length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
            title="No stops on this route"
            description="Add the first stop to begin building the route."
          />
        ) : (
          <div className="divide-y divide-line">
            {stops.map((stop, index) => (
              <div
                key={stop.id}
                className="group flex items-center gap-4 px-4 py-3 transition-colors hover:bg-surface-2"
              >
                <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full bg-primary-bg text-xs font-bold text-primary">
                  {index + 1}
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-semibold text-ink-950">
                      {stopTypeLabel(stop.stopType)}
                    </span>
                    {stop.locationReference && (
                      <span className="text-sm text-ink-600">
                        {stop.locationReference}
                      </span>
                    )}
                    <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                      {stopStatusLabel(stop.status)}
                    </span>
                  </div>
                  <div className="flex flex-wrap items-center gap-3 text-[11px] text-ink-500">
                    {stop.latitude != null && stop.longitude != null && (
                      <span className="font-mono">
                        {stop.latitude.toFixed(6)}, {stop.longitude.toFixed(6)}
                      </span>
                    )}
                    {stop.timeWindowStart && stop.timeWindowEnd && (
                      <span className="flex items-center gap-1">
                        <Clock className="h-3 w-3" />
                        {formatTimestamp(stop.timeWindowStart)} →{" "}
                        {formatTimestamp(stop.timeWindowEnd)}
                      </span>
                    )}
                  </div>
                </div>
                {isDraft && (
                  <button
                    onClick={() => handleDelete(stop)}
                    disabled={busy === stop.id}
                    className="p-1.5 rounded-md text-ink-400 transition-colors hover:bg-red-bg hover:text-red disabled:opacity-60"
                  >
                    {busy === stop.id ? (
                      <div className="h-4 w-4 animate-spin rounded-full border-2 border-red/30 border-t-red" />
                    ) : (
                      <Trash2 className="h-4 w-4" />
                    )}
                  </button>
                )}
              </div>
            ))}
          </div>
        )}

        <div className="flex flex-wrap items-center justify-between gap-2 border-t border-line px-4 py-3 text-xs text-ink-500">
          <span className="font-medium">{stops.length} stops in route</span>
          <span>Sequence is assigned by the backend.</span>
        </div>
      </div>
    </div>
  );
}