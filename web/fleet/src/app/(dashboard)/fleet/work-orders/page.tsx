// src/app/(dashboard)/fleet/work-orders/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  Search,
  Plus,
  Wrench,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Calendar,
  User,
  Flame,
  AlertTriangle,
  Timer,
} from "lucide-react";

type WorkOrder = {
  id: string;
  assetId: string;
  assetName: string;
  reason: string;
  priority: "Critical" | "High" | "Medium" | "Low";
  state: "Open" | "In Progress" | "Ready for Review" | "Completed" | "Closed";
  assignedTo: string | null;
  scheduledDate: string;
  estimatedHours: string;
  branch: string;
  downtime: string;
};

const WORK_ORDERS: WorkOrder[] = [
  {
    id: "WO-1917",
    assetId: "BR-204",
    assetName: "Honda ACE 110",
    reason: "Front brake failure - safety critical",
    priority: "Critical",
    state: "In Progress",
    assignedTo: "T. Adeyemi",
    scheduledDate: "Today",
    estimatedHours: "3 hrs",
    branch: "Ikeja",
    downtime: "2 days",
  },
  {
    id: "WO-1916",
    assetId: "VN-029",
    assetName: "Ford Transit",
    reason: "Engine knocking sound",
    priority: "High",
    state: "Open",
    assignedTo: null,
    scheduledDate: "Tomorrow",
    estimatedHours: "5 hrs",
    branch: "Ikeja",
    downtime: "1 day",
  },
  {
    id: "WO-1915",
    assetId: "TR-098",
    assetName: "MAN TGS",
    reason: "Scheduled maintenance - 450k km service",
    priority: "Medium",
    state: "Ready for Review",
    assignedTo: "M. Bello",
    scheduledDate: "Yesterday",
    estimatedHours: "8 hrs",
    branch: "Apapa",
    downtime: "3 days",
  },
  {
    id: "WO-1914",
    assetId: "BR-188",
    assetName: "Yamaha FZ",
    reason: "Chain replacement",
    priority: "Low",
    state: "Completed",
    assignedTo: "A. Yusuf",
    scheduledDate: "2 days ago",
    estimatedHours: "1.5 hrs",
    branch: "Yaba",
    downtime: "2 hrs",
  },
  {
    id: "WO-1913",
    assetId: "VN-041",
    assetName: "Mercedes Sprinter",
    reason: "Air conditioning repair",
    priority: "Low",
    state: "Closed",
    assignedTo: "C. Uche",
    scheduledDate: "3 days ago",
    estimatedHours: "2 hrs",
    branch: "Lekki",
    downtime: "4 hrs",
  },
  {
    id: "WO-1912",
    assetId: "TR-112",
    assetName: "Scania R500",
    reason: "Transmission inspection",
    priority: "High",
    state: "In Progress",
    assignedTo: "K. Ahmed",
    scheduledDate: "Today",
    estimatedHours: "6 hrs",
    branch: "Apapa",
    downtime: "2 days",
  },
];

const PRIORITY_STYLES: Record<string, { text: string; badge: string; icon: React.ReactNode }> = {
  Critical: { text: "text-red", badge: "bg-red-bg text-red", icon: <Flame className="h-3.5 w-3.5" /> },
  High: { text: "text-amber", badge: "bg-amber-bg text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { text: "text-primary", badge: "bg-primary-bg text-primary", icon: <Clock className="h-3.5 w-3.5" /> },
  Low: { text: "text-ink-500", badge: "bg-surface-2 text-ink-500", icon: <Timer className="h-3.5 w-3.5" /> },
};

const STATE_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  "Ready for Review": { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const PRIORITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const STATE_FILTERS = ["All", "Open", "In Progress", "Ready for Review", "Completed", "Closed"];

export default function WorkOrdersPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedPriority, setSelectedPriority] = useState("All");
  const [selectedState, setSelectedState] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<WorkOrder[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(WORK_ORDERS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.assetId.toLowerCase().includes(search.toLowerCase()) ||
      item.assetName.toLowerCase().includes(search.toLowerCase()) ||
      item.reason.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesPriority = selectedPriority === "All" || item.priority === selectedPriority;
    const matchesState = selectedState === "All" || item.state === selectedState;

    return matchesSearch && matchesBranch && matchesPriority && matchesState;
  });

  const openCount = data.filter((d) => d.state === "Open" || d.state === "In Progress").length;
  const criticalCount = data.filter((d) => d.priority === "Critical").length;
  const completedCount = data.filter((d) => d.state === "Completed" || d.state === "Closed").length;

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedPriority !== "All" ||
    selectedState !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedPriority("All");
    setSelectedState("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading work orders...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Work Orders</h1>
          <p className="mt-1.5">
            Work orders with diagnosis, tasks, approvals, and return-to-service decisions.
          </p>
        </div>
        <button className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover">
          <Plus className="h-3.5 w-3.5" />
          Create Work Order
        </button>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{openCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <Flame className="h-[18px] w-[18px]" />
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
            <span className="text-xs font-semibold text-ink-600">Completed</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{completedCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, asset, or reason..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Priority:</span>
          {selectedPriority}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">State:</span>
          {selectedState}
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
              {(selectedBranch !== "All branches" ? 1 : 0) +
                (selectedPriority !== "All" ? 1 : 0) +
                (selectedState !== "All" ? 1 : 0)}
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
          {filteredData.length} work orders
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Priority</span>
              <div className="flex flex-wrap gap-1.5">
                {PRIORITY_FILTERS.map((priority) => (
                  <button
                    key={priority}
                    onClick={() => setSelectedPriority(priority)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedPriority === priority
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {priority}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">State</span>
              <div className="flex flex-wrap gap-1.5">
                {STATE_FILTERS.map((state) => (
                  <button
                    key={state}
                    onClick={() => setSelectedState(state)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedState === state
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {state}
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

      {/* ── Work Order Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Wrench className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No work orders found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No work orders match your current filters. Create a new work order to get started.
          </p>
          <button className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Create Work Order
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filteredData.map((item) => {
            const priorityStyle = PRIORITY_STYLES[item.priority] || PRIORITY_STYLES.Low;
            const stateStyle = STATE_STYLES[item.state] || STATE_STYLES.Open;
            const isCritical = item.priority === "Critical" || item.priority === "High";

            return (
              <Link
                key={item.id}
                href={`/fleet/work-orders/${item.id}`}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  isCritical
                    ? "border-red/30 bg-red-bg/10 hover:border-red/50 hover:bg-red-bg/20"
                    : item.priority === "High"
                    ? "border-amber/30 bg-amber-bg/10 hover:border-amber/50 hover:bg-amber-bg/20"
                    : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div className={`flex h-10 w-10 items-center justify-center rounded-md ${priorityStyle.badge}`}>
                      <Wrench className="h-4 w-4" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">{item.id}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.assetId}</span>
                      </div>
                      <div className="text-xs text-ink-500">{item.assetName}</div>
                    </div>
                  </div>
                  <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${priorityStyle.badge}`}>
                    {priorityStyle.icon}
                    {item.priority}
                  </span>
                </div>

                <div className="mt-2">
                  <p className="text-sm text-ink-700 line-clamp-2">{item.reason}</p>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <User className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{item.assignedTo || "Unassigned"}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Calendar className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.scheduledDate}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.estimatedHours}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-2">
                    <span className={`flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[10px] font-semibold ${stateStyle.bg} ${stateStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${stateStyle.dot}`} />
                      {item.state}
                    </span>
                    <span className="text-[10px] text-ink-400">Downtime: {item.downtime}</span>
                  </div>
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