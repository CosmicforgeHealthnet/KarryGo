"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  MapPin,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  Clock,
  Building2,
  Home,
  Truck,
  Package,
  Navigation,
  Plus,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { crmService } from "@/lib/services/crmService";
import type {
  LocationView,
  LocationType,
  RecordStatus,
} from "@/lib/types/crm";
import {
  locationTypeLabel,
  recordStatusLabel,
  recordStatusTone,
  timeAgo,
} from "@/lib/utils/crmLabels";

const TYPE_FILTERS: Array<{ value: "ALL" | LocationType; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "PICKUP", label: "Pickup" },
  { value: "DELIVERY", label: "Delivery" },
  { value: "WAREHOUSE", label: "Warehouse" },
  { value: "BILLING", label: "Billing" },
  { value: "OFFICE", label: "Office" },
  { value: "DEPOT", label: "Depot" },
  { value: "SITE", label: "Site" },
  { value: "PORT", label: "Port" },
  { value: "TERMINAL", label: "Terminal" },
  { value: "OTHER", label: "Other" },
];

const STATUS_FILTERS: Array<{ value: "ALL" | RecordStatus; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "ACTIVE", label: "Active" },
  { value: "INACTIVE", label: "Inactive" },
  { value: "ARCHIVED", label: "Archived" },
];

function locationIcon(type: LocationType) {
  switch (type) {
    case "PICKUP":
      return <Package className="h-[18px] w-[18px]" />;
    case "DELIVERY":
      return <Truck className="h-[18px] w-[18px]" />;
    case "BILLING":
      return <Building2 className="h-[18px] w-[18px]" />;
    case "WAREHOUSE":
      return <Home className="h-[18px] w-[18px]" />;
    default:
      return <MapPin className="h-[18px] w-[18px]" />;
  }
}

function locationIconTone(type: LocationType) {
  switch (type) {
    case "PICKUP":
      return { bg: "bg-primary-bg", text: "text-primary" };
    case "DELIVERY":
      return { bg: "bg-green-bg", text: "text-green" };
    case "BILLING":
      return { bg: "bg-amber-bg", text: "text-amber" };
    case "WAREHOUSE":
      return { bg: "bg-surface-2", text: "text-ink-700" };
    default:
      return { bg: "bg-surface-2", text: "text-ink-500" };
  }
}

export default function CustomerLocationsPage() {
  const router = useRouter();
  const params = useParams();
  const customerId = params.customerId as string;

  const [search, setSearch] = useState("");
  const [typeFilter, setTypeFilter] = useState<"ALL" | LocationType>("ALL");
  const [statusFilter, setStatusFilter] = useState<"ALL" | RecordStatus>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<LocationView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchLocations = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const result = await crmService.listLocations(customerId, {
          status: statusFilter === "ALL" ? undefined : statusFilter,
          locationType: typeFilter === "ALL" ? undefined : typeFilter,
          limit: 100,
        });
        setData(result.data);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load locations.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [customerId, statusFilter, typeFilter]
  );

  useEffect(() => {
    if (customerId) fetchLocations();
  }, [customerId, fetchLocations]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return data;
    return data.filter((l) => {
      const haystack = [l.label, l.city, l.stateOrRegion, l.countryCode]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [data, search]);

  const hasActiveFilters =
    typeFilter !== "ALL" || statusFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setTypeFilter("ALL");
    setStatusFilter("ALL");
  };

  const stats = useMemo(() => {
    const total = data.length;
    const active = data.filter((d) => d.status === "ACTIVE").length;
    const withGeofence = data.filter((d) => d.geofenceReference).length;
    return { total, active, withGeofence };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading locations…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Customer Locations
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Pickup, delivery, billing, and operating sites.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchLocations({ silent: true })}
            disabled={refreshing}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() =>
              router.push(`/crm/customers/${customerId}/locations/create`)
            }
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Location
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchLocations()}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-3 gap-2.5">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Total
          </div>
          <div className="text-lg font-bold text-ink-950">{stats.total}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Active
          </div>
          <div className="text-lg font-bold text-green">{stats.active}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            With geofence
          </div>
          <div className="text-lg font-bold text-primary">{stats.withGeofence}</div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by label, city, state…"
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-600 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(typeFilter !== "ALL" ? 1 : 0) + (statusFilter !== "ALL" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-surface px-3 text-sm font-medium text-ink-500 transition hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {showFilters && (
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Type
              </div>
              <div className="flex flex-wrap gap-1.5">
                {TYPE_FILTERS.map((t) => (
                  <button
                    key={t.value}
                    onClick={() => setTypeFilter(t.value)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      typeFilter === t.value
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {t.label}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Status
              </div>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((s) => (
                  <button
                    key={s.value}
                    onClick={() => setStatusFilter(s.value)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      statusFilter === s.value
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s.label}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 transition hover:bg-surface-2"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/trips-empty.png"
            title={
              hasActiveFilters
                ? "No locations match your filters"
                : "No locations yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Add your first location for this customer."
            }
            action={
              hasActiveFilters ? (
                <button
                  onClick={clearFilters}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Clear filters
                </button>
              ) : (
                <button
                  onClick={() =>
                    router.push(`/crm/customers/${customerId}/locations/create`)
                  }
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Add location
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filtered.map((item) => {
            const iconTone = locationIconTone(item.locationType);
            const recordTone = recordStatusTone(item.status);
            return (
              <div
                key={item.id}
                onClick={() =>
                  router.push(`/crm/customers/${customerId}/locations/${item.id}`)
                }
                className="group cursor-pointer rounded-xl border border-line bg-surface p-4 transition hover:shadow-md"
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl ${iconTone.bg} ${iconTone.text}`}
                    >
                      {locationIcon(item.locationType)}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.label}
                        </span>
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                        <span className="text-[12px] font-medium text-ink-500">
                          {locationTypeLabel(item.locationType)}
                        </span>
                        <span className="text-ink-300">·</span>
                        <span className="text-[12px] text-ink-500">
                          {item.city}, {item.stateOrRegion}
                        </span>
                        <span className="text-ink-300">·</span>
                        <span
                          className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${recordTone.bg} ${recordTone.text}`}
                        >
                          {recordStatusLabel(item.status)}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 space-y-1.5 text-sm">
                  <div className="flex items-center gap-2 text-ink-600">
                    <MapPin className="h-4 w-4 text-ink-400" />
                    <span>
                      {item.city}, {item.stateOrRegion} · {item.countryCode}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 text-ink-600">
                    <Clock className="h-4 w-4 text-ink-400" />
                    <span className="text-[12px]">{item.timezone}</span>
                  </div>
                  {item.hasCoordinates && (
                    <div className="flex items-center gap-2 text-ink-600">
                      <Navigation className="h-4 w-4 text-ink-400" />
                      <span className="text-[12px] text-ink-500">
                        Coordinates available
                      </span>
                    </div>
                  )}
                </div>

                <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-line pt-2.5">
                  {item.geofenceReference && (
                    <span className="rounded-full bg-primary-bg px-2 py-0.5 text-[10px] font-medium text-primary">
                      Geofence
                    </span>
                  )}
                  {item.hasAccessInstructions && (
                    <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-600">
                      Access notes
                    </span>
                  )}
                  {item.hasSafetyInstructions && (
                    <span className="rounded-full bg-amber-bg px-2 py-0.5 text-[10px] font-medium text-amber">
                      Safety notes
                    </span>
                  )}
                  <span className="ml-auto text-[11px] text-ink-400">
                    Added {timeAgo(item.createdAt)}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}