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

type Schedule = {
  id: string;
  assetId: string;
  assetName: string;
  type: "Preventive" | "Corrective" | "Inspection";
  trigger: string;
  nextDue: string;
  status: "Due" | "Overdue" | "Scheduled" | "Completed";
  branch: string;
  priority: "High" | "Medium" | "Low";
};

const SCHEDULES: Schedule[] = [
  {
    id: "SCH-001",
    assetId: "BR-204",
    assetName: "Honda ACE 110",
    type: "Preventive",
    trigger: "Every 5,000 km",
    nextDue: "Overdue 280 km",
    status: "Overdue",
    branch: "Ikeja",
    priority: "High",
  },
  {
    id: "SCH-002",
    assetId: "VN-041",
    assetName: "Mercedes Sprinter",
    type: "Preventive",
    trigger: "Every 10,000 km",
    nextDue: "In 9 days",
    status: "Scheduled",
    branch: "Lekki",
    priority: "Medium",
  },
  {
    id: "SCH-003",
    assetId: "TR-112",
    assetName: "Scania R500",
    type: "Inspection",
    trigger: "Every 30 days",
    nextDue: "In 430 km",
    status: "Scheduled",
    branch: "Apapa",
    priority: "High",
  },
  {
    id: "SCH-004",
    assetId: "BR-188",
    assetName: "Yamaha FZ",
    type: "Preventive",
    trigger: "Every 5,000 km",
    nextDue: "In 21 days",
    status: "Scheduled",
    branch: "Yaba",
    priority: "Low",
  },
  {
    id: "SCH-005",
    assetId: "VN-029",
    assetName: "Ford Transit",
    type: "Corrective",
    trigger: "Brake pad replacement",
    nextDue: "Overdue 120 km",
    status: "Overdue",
    branch: "Ikeja",
    priority: "High",
  },
  {
    id: "SCH-006",
    assetId: "BR-171",
    assetName: "Honda CB150X",
    type: "Preventive",
    trigger: "Every 5,000 km",
    nextDue: "In 15 days",
    status: "Scheduled",
    branch: "Lekki",
    priority: "Medium",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Due: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Overdue: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Scheduled: { bg: "bg-primary-bg", text: "text-primary", icon: <Calendar className="h-3.5 w-3.5" /> },
  Completed: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
};

const PRIORITY_STYLES: Record<string, string> = {
  High: "text-red bg-red-bg",
  Medium: "text-amber bg-amber-bg",
  Low: "text-primary bg-primary-bg",
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Due", "Overdue", "Scheduled", "Completed"];

export default function MaintenanceSchedulesPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Schedule[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(SCHEDULES);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.assetId.toLowerCase().includes(search.toLowerCase()) ||
      item.assetName.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesBranch && matchesStatus;
  });

  const overdueCount = data.filter((d) => d.status === "Overdue").length;
  const dueCount = data.filter((d) => d.status === "Due").length;
  const scheduledCount = data.filter((d) => d.status === "Scheduled").length;

  const hasActiveFilters = selectedBranch !== "All branches" || selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
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
        <button className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover">
          <Plus className="h-3.5 w-3.5" />
          Create Schedule
        </button>
      </div>

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
          {selectedBranch}
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
              {(selectedBranch !== "All branches" ? 1 : 0) + (selectedStatus !== "All" ? 1 : 0)}
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
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No schedules found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No maintenance schedules match your current filters.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Scheduled;
            const priorityStyle = PRIORITY_STYLES[item.priority] || PRIORITY_STYLES.Medium;
            const isOverdue = item.status === "Overdue";
            const isDue = item.status === "Due";

            return (
              <div
                key={item.id}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
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
                        <span className="text-sm font-semibold text-ink-950">{item.id}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.assetId}</span>
                      </div>
                      <div className="text-xs text-ink-500">{item.assetName}</div>
                    </div>
                  </div>
                  <div className="flex flex-col items-end gap-0.5">
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                    >
                      {statusStyle.icon}
                      {item.status}
                    </span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${priorityStyle}`}>
                      {item.priority}
                    </span>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.trigger}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500 col-span-2">
                    <Calendar className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>Next due: {item.nextDue}</span>
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="text-[10px] text-ink-400 truncate max-w-[180px]">{item.type}</div>
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}