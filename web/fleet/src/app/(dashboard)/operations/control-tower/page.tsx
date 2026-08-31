// src/app/(dashboard)/operations/control-tower/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  MapPin,
  Truck,
  Package,
  Clock,
  CheckCircle2,
  AlertCircle,
  Navigation,
  Users,
  Eye,
  SlidersHorizontal,
  X,
  Search,
  ChevronDown,
  Map,
  List,
  Gauge,
  Activity,
  Radio,
  Wifi,
  WifiOff,
  ArrowRight,
  Bell,
  LifeBuoy,
} from "lucide-react";
import { CONTROL_TOWER_DATA } from "@/lib/mock-control-tower-data";

type ActiveWork = {
  id: string;
  type: "Trip" | "Assignment" | "Route";
  provider: string;
  asset: string;
  service: "Dispatch" | "Hauling";
  branch: string;
  status: "Active" | "Idle" | "Exception" | "Completed";
  location: string;
  lastUpdated: string;
  progress: number;
  eta: string;
  isLive: boolean;
  signal: "strong" | "medium" | "weak";
};

const STATUS_STYLES = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Idle: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Exception: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Completed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const SIGNAL_STYLES = {
  strong: { icon: Wifi, color: "text-green" },
  medium: { icon: Wifi, color: "text-amber" },
  weak: { icon: WifiOff, color: "text-red" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Active", "Idle", "Exception", "Completed"];

export default function ControlTowerPage() {
  const [loading, setLoading] = useState(true);
  const [viewMode, setViewMode] = useState<"map" | "list">("list");
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [statusFilter, setStatusFilter] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [data, setData] = useState<ActiveWork[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(CONTROL_TOWER_DATA);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.provider.toLowerCase().includes(search.toLowerCase()) ||
      item.asset.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;

    const matchesStatus = statusFilter === "All" || item.status === statusFilter;

    return matchesSearch && matchesBranch && matchesStatus;
  });

  const activeCount = data.filter((d) => d.status === "Active").length;
  const exceptionCount = data.filter((d) => d.status === "Exception").length;
  const idleCount = data.filter((d) => d.status === "Idle").length;

  const hasActiveFilters = statusFilter !== "All" || selectedBranch !== "All branches" || search !== "";

  const clearFilters = () => {
    setStatusFilter("All");
    setSelectedBranch("All branches");
    setSearch("");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading control tower...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Control Tower</h1>
          <p className="mt-1.5">Live operational view of all active work, provider locations, and exceptions.</p>
        </div>
        <div className="flex items-center gap-2.5 flex-wrap">
          <Link
            href="/operations/alerts"
            className="flex h-10 items-center gap-1.5 rounded-lg bg-amber px-3.5 text-sm font-semibold text-white transition-colors hover:bg-amber/90"
          >
            <Bell className="h-3.5 w-3.5" />
            Alerts
          </Link>

          <Link
            href="/operations/control-tower/exceptions"
            className="flex h-10 items-center gap-1.5 rounded-lg bg-red px-3.5 text-sm font-semibold text-white transition-colors hover:bg-red/90"
          >
            <AlertCircle className="h-3.5 w-3.5" />
            {exceptionCount} exceptions
          </Link>

          <div className="flex rounded-lg border border-line bg-white p-0.5">
            <button
              onClick={() => setViewMode("list")}
              className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                viewMode === "list"
                  ? "bg-primary text-white"
                  : "text-ink-600 hover:bg-surface-2"
              }`}
            >
              <List className="h-3.5 w-3.5" />
              List
            </button>
            <button
              onClick={() => setViewMode("map")}
              className={`flex items-center gap-1.5 rounded-md px-3 py-1.5 text-xs font-medium transition-colors ${
                viewMode === "map"
                  ? "bg-primary text-white"
                  : "text-ink-600 hover:bg-surface-2"
              }`}
            >
              <Map className="h-3.5 w-3.5" />
              Map
            </button>
          </div>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Activity className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{activeCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Idle</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{idleCount}</div>
        </div>
        <div className="rounded-lg border border-red/20 bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Exceptions</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-red">{exceptionCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Gauge className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Utilization</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">84%</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, provider, or asset..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {statusFilter}
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
          {(statusFilter !== "All" || selectedBranch !== "All branches") && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(statusFilter !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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
          {filteredData.length} items
        </span>
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
                    onClick={() => setStatusFilter(status)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      statusFilter === status
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</span>
              <div className="flex flex-wrap gap-1.5">
                {BRANCHES.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedBranch === branch
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {branch}
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

      {/* ── List View ── */}
      {viewMode === "list" && (
        <div className="flex flex-col gap-2.5">
          {filteredData.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
              <div className="rounded-full bg-surface-2 p-4">
                <Activity className="h-8 w-8 text-ink-400" />
              </div>
              <h3 className="mt-3 text-sm font-semibold text-ink-950">No active work</h3>
              <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
                No active work matches your current filters. Try adjusting your search criteria.
              </p>
            </div>
          ) : (
            filteredData.map((item) => {
              const statusStyle = STATUS_STYLES[item.status as keyof typeof STATUS_STYLES] || STATUS_STYLES.Idle;
              const SignalIcon = SIGNAL_STYLES[item.signal]?.icon || Wifi;
              const signalColor = SIGNAL_STYLES[item.signal]?.color || "text-ink-400";
              const isException = item.status === "Exception";
              const isActive = item.status === "Active";

              return (
                <Link
                  key={item.id}
                  href={
                    item.type === "Trip"
                      ? `/operations/trips/${item.id}`
                      : item.type === "Assignment"
                      ? `/operations/assignments/${item.id}`
                      : `/operations/routes/${item.id}`
                  }
                  className={`flex flex-col gap-2.5 rounded-lg border p-3.5 transition-colors sm:flex-row sm:items-center sm:justify-between ${
                    isException
                      ? "border-red/30 bg-red-bg/20 hover:bg-red-bg/30"
                      : isActive
                      ? "border-green/20 bg-white hover:bg-surface-2"
                      : "border-line bg-white hover:bg-surface-2"
                  }`}
                >
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-semibold text-ink-950">{item.id}</span>
                      <span className="font-mono text-[10px] text-ink-400">{item.type}</span>
                      <span
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                        {item.status}
                      </span>
                      {item.isLive && (
                        <span className="flex items-center gap-1 rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
                          <span className="relative flex h-1.5 w-1.5">
                            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                          </span>
                          Live
                        </span>
                      )}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-ink-500">
                      <span className="flex items-center gap-1">
                        <Users className="h-3.5 w-3.5" />
                        {item.provider}
                      </span>
                      <span className="text-ink-300">·</span>
                      <span className="flex items-center gap-1">
                        <Truck className="h-3.5 w-3.5" />
                        {item.asset}
                      </span>
                      <span className="text-ink-300">·</span>
                      <span className="flex items-center gap-1">
                        <MapPin className="h-3.5 w-3.5" />
                        {item.branch}
                      </span>
                      <span className="text-ink-300">·</span>
                      <span className="flex items-center gap-1">
                        <SignalIcon className={`h-3.5 w-3.5 ${signalColor}`} />
                        {item.location}
                      </span>
                    </div>
                  </div>

                  <div className="flex flex-wrap items-center gap-4">
                    <div className="flex items-center gap-2">
                      <div className="h-1.5 w-20 rounded-full bg-surface-3 overflow-hidden">
                        <div
                          className={`h-full rounded-full ${item.progress === 100 ? "bg-green" : isException ? "bg-red" : "bg-primary"}`}
                          style={{ width: `${item.progress}%` }}
                        />
                      </div>
                      <span className="text-[10px] font-medium text-ink-500">{item.progress}%</span>
                    </div>

                    <div className="flex items-center gap-1.5 text-xs text-ink-600">
                      <Clock className="h-3.5 w-3.5 text-ink-400" />
                      <span className="font-medium">{item.eta}</span>
                    </div>

                    <span className="text-xs font-medium text-ink-400 transition-colors hover:text-primary">
                      View →
                    </span>
                  </div>
                </Link>
              );
            })
          )}
        </div>
      )}

      {/* ── Map View ── */}
      {viewMode === "map" && (
        <div className="rounded-lg border border-line bg-white p-8">
          <div className="flex flex-col items-center justify-center py-12 text-center">
            <div className="rounded-full bg-surface-2 p-4">
              <Map className="h-10 w-10 text-ink-400" />
            </div>
            <h3 className="mt-4 text-base font-semibold text-ink-950">Map View</h3>
            <p className="mt-1.5 max-w-md text-sm text-ink-500">
              Interactive map showing provider locations, active trips, and exceptions.
              <br />
              <span className="text-xs text-ink-400">Map integration coming soon.</span>
            </p>
            <button
              onClick={() => setViewMode("list")}
              className="mt-4 rounded-md bg-primary px-4 py-2 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
            >
              Switch to list view
            </button>
          </div>
        </div>
      )}

      {/* ── Legend ── */}
      <div className="rounded-lg border border-line bg-white p-3">
        <div className="flex flex-wrap items-center gap-4 text-xs text-ink-600">
          <span className="font-medium text-ink-700">Status:</span>
          <span className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-green" />
            Active
          </span>
          <span className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-primary" />
            Idle
          </span>
          <span className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-red" />
            Exception
          </span>
          <span className="flex items-center gap-1.5">
            <span className="h-2 w-2 rounded-full bg-ink-400" />
            Completed
          </span>
          <span className="text-ink-300">|</span>
          <span className="font-medium text-ink-700">Signal:</span>
          <span className="flex items-center gap-1.5">
            <Wifi className="h-3.5 w-3.5 text-green" />
            Strong
          </span>
          <span className="flex items-center gap-1.5">
            <Wifi className="h-3.5 w-3.5 text-amber" />
            Medium
          </span>
          <span className="flex items-center gap-1.5">
            <WifiOff className="h-3.5 w-3.5 text-red" />
            Weak
          </span>
        </div>
      </div>
    </div>
  );
}