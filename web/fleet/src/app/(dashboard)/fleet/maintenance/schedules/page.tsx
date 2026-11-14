// src/app/(dashboard)/fleet/maintenance/schedules/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  Search,
  Plus,
  Calendar,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Wrench,
} from "lucide-react";

import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { maintenanceService } from "@/lib/services/maintenanceService";
import { structureService } from "@/lib/services/structureService";
import type { AssetReference, MaintenanceScheduleRecord } from "@/lib/types/maintenance";
import type { Branch } from "@/lib/types/structure";
import { formatDate, formatNumber, triggerTypeLabel } from "@/lib/utils/maintenanceLabels";

// Display state derived from the stored status and next due date. The
// backend stores ACTIVE, PAUSED, COMPLETED or CANCELLED; an active schedule
// whose next due date has passed (or falls within 7 days) is shown as
// Overdue (or Due).
type DisplayStatus = "Due" | "Overdue" | "Scheduled" | "Paused" | "Completed" | "Cancelled";

const DAY_MS = 24 * 60 * 60 * 1000;

function displayStatus(s: MaintenanceScheduleRecord, today: number): DisplayStatus {
  if (s.status === "PAUSED") return "Paused";
  if (s.status === "COMPLETED") return "Completed";
  if (s.status === "CANCELLED") return "Cancelled";
  if (s.nextDueDate && today > 0) {
    const due = new Date(s.nextDueDate).getTime();
    if (due < today) return "Overdue";
    if (due - today <= 7 * DAY_MS) return "Due";
  }
  return "Scheduled";
}

function nextDueLabel(s: MaintenanceScheduleRecord): string {
  const parts: string[] = [];
  if (s.nextDueDate) parts.push(formatDate(s.nextDueDate));
  if (s.nextDueOdometer != null) parts.push(`${formatNumber(s.nextDueOdometer)} km`);
  if (s.nextDueHours != null) parts.push(`${formatNumber(s.nextDueHours)} h`);
  return parts.length ? parts.join(" · ") : "—";
}

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Due: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Overdue: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Scheduled: { bg: "bg-primary-bg", text: "text-primary", icon: <Calendar className="h-3.5 w-3.5" /> },
  Paused: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-3.5 w-3.5" /> },
};

const PRIORITY_STYLES: Record<string, string> = {
  Critical: "text-red bg-red-bg",
  Standard: "text-primary bg-primary-bg",
};

const ALL_BRANCHES = "";
const STATUS_FILTERS = ["All", "Due", "Overdue", "Scheduled", "Paused", "Completed", "Cancelled"];

export default function MaintenanceSchedulesPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState(ALL_BRANCHES);
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<MaintenanceScheduleRecord[]>([]);
  const [assets, setAssets] = useState<AssetReference[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [today, setToday] = useState(0);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  useEffect(() => {
    maintenanceService
      .listScheduleRecords()
      .then((rows) => {
        setData(rows);
        const now = new Date();
        setToday(new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime());
        setError(null);
      })
      .catch((e) => setError(describeApiError(e, "Maintenance schedules could not be loaded.")))
      .finally(() => setLoading(false));
    // Labels only: the schedules still show if these lookups fail.
    maintenanceService.listAssetReferences().then(setAssets).catch(() => setAssets([]));
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const assetOf = (id: string) => assets.find((a) => a.id === id);
  const branchName = (id?: string) => (id ? branches.find((b) => b.id === id)?.name ?? id : "All branches");

  const filteredData = data.filter((item) => {
    const asset = assetOf(item.assetId);
    const q = search.trim().toLowerCase();
    const matchesSearch =
      q === "" ||
      item.id.toLowerCase().includes(q) ||
      item.assetId.toLowerCase().includes(q) ||
      (asset ? `${asset.assetCode} ${asset.manufacturer} ${asset.model}`.toLowerCase().includes(q) : false);

    const matchesBranch = selectedBranch === ALL_BRANCHES || asset?.branchId === selectedBranch;
    const matchesStatus = selectedStatus === "All" || displayStatus(item, today) === selectedStatus;

    return matchesSearch && matchesBranch && matchesStatus;
  });

  const overdueCount = data.filter((d) => displayStatus(d, today) === "Overdue").length;
  const dueCount = data.filter((d) => displayStatus(d, today) === "Due").length;
  const scheduledCount = data.filter((d) => displayStatus(d, today) === "Scheduled").length;

  const hasActiveFilters = selectedBranch !== ALL_BRANCHES || selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch(ALL_BRANCHES);
    setSelectedStatus("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading schedules...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Maintenance Schedules</h1>
          <p className="mt-1.5">Scheduled maintenance and inspection triggers for fleet assets.</p>
        </div>
        <Link
          href="/fleet/maintenance/schedules/create"
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Create Schedule
        </Link>
      </div>

      <ApiErrorNotice error={error} />

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Overdue</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{overdueCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Due Soon</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{dueCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Calendar className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Scheduled</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{scheduledCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID or asset..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {branchName(selectedBranch)}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {selectedStatus}
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
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedBranch !== ALL_BRANCHES ? 1 : 0) + (selectedStatus !== "All" ? 1 : 0)}
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

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">{filteredData.length} schedules</span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
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
                    {status}
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

      {/* ── Schedules Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Calendar className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">{data.length === 0 ? "No schedules yet" : "No schedules found"}</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            {data.length === 0 ? "Create a schedule to plan maintenance for an asset." : "No maintenance schedules match your current filters."}
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((item) => {
            const status = displayStatus(item, today);
            const priority = item.critical ? "Critical" : "Standard";
            const statusStyle = STATUS_STYLES[status] || STATUS_STYLES.Scheduled;
            const priorityStyle = PRIORITY_STYLES[priority];
            const isOverdue = status === "Overdue";
            const isDue = status === "Due";
            const asset = assetOf(item.assetId);

            return (
              <Link
                href={`/fleet/maintenance/schedules/${item.id}`}
                key={item.id}
                className={`group block rounded-lg border p-4 transition-colors cursor-pointer ${
                  isOverdue
                    ? "border-red/30 bg-red-bg/10 hover:border-red/50 hover:bg-red-bg/20"
                    : isDue
                    ? "border-amber/30 bg-amber-bg/10 hover:border-amber/50 hover:bg-amber-bg/20"
                    : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-2.5">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-md ${
                        isOverdue
                          ? "bg-red-bg text-red"
                          : isDue
                          ? "bg-amber-bg text-amber"
                          : "bg-primary-bg text-primary"
                      }`}
                    >
                      <Wrench className="h-4 w-4" />
                    </div>
                    <div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-sm font-semibold text-ink-950">{asset?.assetCode || item.assetId}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.id}</span>
                      </div>
                      <div className="text-xs text-ink-500">{asset ? [asset.manufacturer, asset.model].filter(Boolean).join(" ") || "—" : "—"}</div>
                    </div>
                  </div>
                  <div className="flex flex-col items-end gap-0.5">
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                    >
                      {statusStyle.icon}
                      {status}
                    </span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${priorityStyle}`}>
                      {priority}
                    </span>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{asset ? branchName(asset.branchId) : "—"}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>
                      {triggerTypeLabel(item.triggerType)}
                      {item.intervalValue != null ? ` · every ${formatNumber(item.intervalValue)}` : ""}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500 col-span-2">
                    <Calendar className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>Next due: {nextDueLabel(item)}</span>
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="text-[10px] text-ink-400 truncate max-w-[180px]">{triggerTypeLabel(item.triggerType)}</div>
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
                  </div>
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}