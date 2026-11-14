// src/app/(dashboard)/operations/control-tower/exceptions/page.tsx

"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  AlertCircle,
  Clock,
  MapPin,
  Truck,
  Package,
  Search,
  ChevronDown,
  SlidersHorizontal,
  X,
  Flag,
  Users,
  AlertTriangle,
  Navigation,
  Shield,
  CheckCircle2,
  Info,
  LifeBuoy,
} from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { alertsService } from "@/lib/services/alertsService";
import { structureService } from "@/lib/services/structureService";
import type { OperationalAlert } from "@/lib/types/alerts";
import type { Branch } from "@/lib/types/structure";

// Operational exceptions are the dispatch alerts raised by the control tower
// (GET /alerts): late, off-route, stale location, rejected or expired
// assignments, safety and capacity.

const SEVERITY_STYLES = {
  CRITICAL: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: AlertCircle },
  HIGH: { bg: "bg-red-bg/70", text: "text-red", dot: "bg-red", icon: AlertTriangle },
  MEDIUM: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: AlertCircle },
  LOW: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: Info },
};

const STATUS_STYLES = {
  OPEN: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  ACKNOWLEDGED: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  IN_PROGRESS: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  RESOLVED: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  DISMISSED: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const typeIcon = (alertType: string) => {
  const t = alertType.toUpperCase();
  if (t.includes("LATE") || t.includes("EXPIRED")) return Clock;
  if (t.includes("ROUTE")) return Navigation;
  if (t.includes("STALE") || t.includes("LOCATION") || t.includes("GEOFENCE")) return MapPin;
  if (t.includes("REJECT") || t.includes("DECLIN")) return X;
  if (t.includes("SAFETY")) return Shield;
  if (t.includes("CAPACITY")) return Package;
  if (t.includes("PROVIDER")) return Users;
  if (t.includes("VEHICLE") || t.includes("ASSET")) return Truck;
  return AlertCircle;
};

const ALL_BRANCHES = "";
const SEVERITY_FILTERS = ["All", "CRITICAL", "HIGH", "MEDIUM", "LOW"];
const STATUS_FILTERS = ["All", "OPEN", "ACKNOWLEDGED", "IN_PROGRESS", "RESOLVED", "DISMISSED"];

const ageLabel = (iso: string, now: number) => {
  const minutes = Math.max(0, Math.floor((now - new Date(iso).getTime()) / 60000));
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 48) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
};

const subjectOf = (a: OperationalAlert) =>
  a.canonicalTripReference || a.assignmentId || a.shipmentId || a.routePlanId || `${humanize(a.sourceType)} ${a.sourceId}`;

export default function ExceptionsPage() {
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState(ALL_BRANCHES);
  const [showFilters, setShowFilters] = useState(false);
  const [data, setData] = useState<OperationalAlert[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [now, setNow] = useState(0);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [ackKeys, setAckKeys] = useState<Record<string, string>>({});
  const [acking, setAcking] = useState<string | null>(null);

  const load = useCallback(async (branchId: string) => {
    try {
      setData(await alertsService.listAlerts({ branchId: branchId || undefined, limit: 200 }));
      setNow(Date.now());
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Exceptions could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(selectedBranch));
  }, [load, selectedBranch]);

  useEffect(() => {
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const branchName = (id?: string) => (id ? branches.find((b) => b.id === id)?.name ?? id : "All branches");

  const acknowledge = async (alert: OperationalAlert) => {
    const key = ackKeys[alert.id] ?? newIdempotencyKey();
    setAckKeys((k) => ({ ...k, [alert.id]: key }));
    setAcking(alert.id);
    setError(null);
    try {
      const updated = await alertsService.acknowledgeAlert(alert.id, { expectedVersion: alert.version }, key);
      setData((rows) => rows.map((r) => (r.id === updated.id ? updated : r)));
      setAckKeys((k) => {
        const next = { ...k };
        delete next[alert.id];
        return next;
      });
    } catch (e) {
      setError(describeApiError(e, "The exception could not be acknowledged."));
    } finally {
      setAcking(null);
    }
  };

  const filteredData = data.filter((item) => {
    const q = search.trim().toLowerCase();
    const matchesSearch =
      q === "" ||
      item.id.toLowerCase().includes(q) ||
      subjectOf(item).toLowerCase().includes(q) ||
      item.alertType.toLowerCase().includes(q);

    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesSeverity && matchesStatus;
  });

  const openCount = data.filter((d) => d.status === "OPEN").length;
  const criticalCount = data.filter((d) => d.severity === "CRITICAL").length;
  const resolvedCount = data.filter((d) => d.status === "RESOLVED").length;

  const hasActiveFilters = selectedSeverity !== "All" || selectedStatus !== "All" || selectedBranch !== ALL_BRANCHES || search !== "";

  const clearFilters = () => {
    setSelectedSeverity("All");
    setSelectedStatus("All");
    setSelectedBranch(ALL_BRANCHES);
    setSearch("");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading exceptions...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1>Control Tower Exceptions</h1>
          
          </div>
          <p className="mt-1.5">
            Late, off-route, stale location, rejected/expired assignment, safety, capacity and other operational exceptions.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href="/fleet/breakdowns"
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <LifeBuoy className="h-3.5 w-3.5" />
            Rescue
          </Link>
          <Link
            href="/operations/control-tower"
            className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Back
          </Link>
        </div>
      </div>

      <ApiErrorNotice error={error} />

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Open</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{openCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg/70 text-red">
              <AlertTriangle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Critical</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{criticalCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Resolved</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{resolvedCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, subject, or description..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {branchName(selectedBranch)}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Severity:</span>
          {selectedSeverity === "All" ? "All" : humanize(selectedSeverity)}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {selectedStatus === "All" ? "All" : humanize(selectedStatus)}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {(selectedSeverity !== "All" || selectedStatus !== "All" || selectedBranch !== ALL_BRANCHES) && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedSeverity !== "All" ? 1 : 0) + (selectedStatus !== "All" ? 1 : 0) + (selectedBranch !== ALL_BRANCHES ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filteredData.length} exceptions
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Severity</span>
              <div className="flex flex-wrap gap-1.5">
                {SEVERITY_FILTERS.map((severity) => (
                  <button
                    key={severity}
                    onClick={() => setSelectedSeverity(severity)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedSeverity === severity
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {severity === "All" ? "All" : humanize(severity)}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === status
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status === "All" ? "All" : humanize(status)}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</span>
              <div className="flex flex-wrap gap-1.5">
                {[ALL_BRANCHES, ...branches.map((b) => b.id)].map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedBranch === branch
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {branchName(branch)}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Exceptions list ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <CheckCircle2 className="h-8 w-8 text-green" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No exceptions found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No exceptions match your current filters. Everything is running smoothly!
          </p>
          <button
            onClick={clearFilters}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            Clear filters
          </button>
        </div>
      ) : (
        <div className="flex flex-col gap-2.5">
          {filteredData.map((item) => {
            const severityStyle = SEVERITY_STYLES[item.severity as keyof typeof SEVERITY_STYLES] || SEVERITY_STYLES.LOW;
            const statusStyle = STATUS_STYLES[item.status as keyof typeof STATUS_STYLES] || STATUS_STYLES.OPEN;
            const SeverityIcon = severityStyle.icon;
            const TypeIcon = typeIcon(item.alertType);
            const isCritical = (item.severity === "CRITICAL" || item.severity === "HIGH") && item.status !== "RESOLVED" && item.status !== "DISMISSED";

            return (
              <div
                key={item.id}
                className={`flex flex-col gap-2.5 rounded-lg border p-3.5 transition-colors sm:flex-row sm:items-start sm:justify-between ${
                  isCritical
                    ? "border-red/30 bg-red-bg/20 hover:bg-red-bg/30"
                    : "border-line bg-white hover:bg-surface-2"
                }`}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-semibold text-ink-950">{item.id}</span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${severityStyle.dot}`} />
                      {humanize(item.severity)}
                    </span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                      {humanize(item.status)}
                    </span>
                    <span className="flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                      <TypeIcon className="h-3 w-3" />
                      {humanize(item.alertType)}
                    </span>
                  </div>
                  <div className="mt-1 text-sm font-medium text-ink-950">{subjectOf(item)}</div>
                  <div className="mt-0.5 text-sm text-ink-600">
                    {item.resolutionReason || `Raised by ${humanize(item.sourceType)} · respond by ${formatWhen(item.slaDueAt)}`}
                  </div>
                  <div className="mt-1.5 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                    <span className="flex items-center gap-1">
                      <MapPin className="h-3.5 w-3.5" />
                      {branchName(item.branchId)}
                    </span>
                    <span className="text-ink-300">·</span>
                    <span className="flex items-center gap-1">
                      <Clock className="h-3.5 w-3.5" />
                      {ageLabel(item.createdAt, now)}
                    </span>
                    {item.ownerUserId && (
                      <>
                        <span className="text-ink-300">·</span>
                        <span className="flex items-center gap-1">
                          <Users className="h-3.5 w-3.5" />
                          {item.ownerUserId}
                        </span>
                      </>
                    )}
                  </div>
                </div>

                <div className="flex items-center gap-2 flex-shrink-0">
                  {item.status !== "RESOLVED" && item.status !== "DISMISSED" && (
                    <>
                      {item.status === "OPEN" && (
                        <button
                          onClick={() => acknowledge(item)}
                          disabled={acking !== null}
                          className="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-hover transition-colors disabled:opacity-60"
                        >
                          <Flag className="inline h-3.5 w-3.5 mr-1" />
                          {acking === item.id ? "Acknowledging..." : "Acknowledge"}
                        </button>
                      )}
                      <Link
                        href={`/operations/alerts/${item.id}`}
                        className="rounded-md border border-line bg-white px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                      >
                        View
                      </Link>
                      <Link
                        href="/fleet/breakdowns"
                        className="rounded-md border border-line bg-white px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                      >
                        <LifeBuoy className="h-3.5 w-3.5" />
                        Rescue
                      </Link>
                    </>
                  )}
                  {item.status === "RESOLVED" && (
                    <span className="text-xs font-medium text-green">✓ Resolved</span>
                  )}
                  {item.status === "DISMISSED" && (
                    <span className="text-xs font-medium text-ink-500">Dismissed</span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}