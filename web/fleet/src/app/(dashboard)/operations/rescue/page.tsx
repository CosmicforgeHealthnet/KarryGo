// src/app/(dashboard)/operations/rescue/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  LifeBuoy,
  Search,
  ChevronDown,
  MapPin,
  Clock,
  AlertCircle,
  AlertTriangle,
  CheckCircle2,
  SlidersHorizontal,
  X,
  Eye,
  Flag,
  Users,
  Truck,
  Package,
  Phone,
  MessageSquare,
  ArrowRight,
  Filter,
  User,
  Calendar,
  Navigation,
  Shield,
  Wrench,
  FileText,
} from "lucide-react";
import { RESCUE_CASES } from "@/lib/mock-rescue-data";

type RescueCase = {
  id: string;
  title: string;
  description: string;
  severity: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "Assigned" | "In Progress" | "Resolved" | "Closed";
  type: "Vehicle Breakdown" | "Trip Delay" | "Provider Issue" | "Safety Incident" | "Customer Issue" | "Weather" | "Other";
  assignmentId?: string;
  tripId?: string;
  providerId?: string;
  branch: string;
  assignedTo?: string;
  createdAt: string;
  age: string;
  updatedAt: string;
  location: string;
  actions: {
    label: string;
    timestamp: string;
  }[];
};

const SEVERITY_STYLES = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: AlertCircle, border: "border-red/30" },
  High: { bg: "bg-red-bg/70", text: "text-red", dot: "bg-red", icon: AlertTriangle, border: "border-red/20" },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: AlertCircle, border: "border-amber/20" },
  Low: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: AlertCircle, border: "border-primary/20" },
};

const STATUS_STYLES = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Assigned: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  "In Progress": { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const TYPE_ICONS = {
  "Vehicle Breakdown": Wrench,
  "Trip Delay": Clock,
  "Provider Issue": User,
  "Safety Incident": Shield,
  "Customer Issue": MessageSquare,
  "Weather": Navigation,
  "Other": FileText,
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const SEVERITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const STATUS_FILTERS = ["All", "Open", "Assigned", "In Progress", "Resolved", "Closed"];

export default function RescuePage() {
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [data, setData] = useState<RescueCase[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(RESCUE_CASES);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.title.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.location.toLowerCase().includes(search.toLowerCase());

    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesSeverity && matchesStatus && matchesBranch;
  });

  const openCount = data.filter((d) => d.status === "Open" || d.status === "Assigned").length;
  const criticalCount = data.filter((d) => d.severity === "Critical").length;
  const resolvedCount = data.filter((d) => d.status === "Resolved" || d.status === "Closed").length;

  const hasActiveFilters = selectedSeverity !== "All" || selectedStatus !== "All" || selectedBranch !== "All branches" || search !== "";

  const clearFilters = () => {
    setSelectedSeverity("All");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
    setSearch("");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading rescue cases...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Rescue Cases</h1>
          <p className="mt-1.5">
            Operational rescue cases linked to assignments, trips, and providers. Coordinate replacement and resolve issues.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href="/operations/rescue/create"
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <LifeBuoy className="h-3.5 w-3.5" />
            Create
          </Link>
        </div>
      </div>

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
            placeholder="Search by ID, title, location..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Severity:</span>
          {selectedSeverity}
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
          {(selectedSeverity !== "All" || selectedStatus !== "All" || selectedBranch !== "All branches") && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedSeverity !== "All" ? 1 : 0) + (selectedStatus !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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
          {filteredData.length} cases
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
                    {severity}
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

      {/* ── Cases list ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <LifeBuoy className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No rescue cases</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No rescue cases match your current filters. Everything is running smoothly!
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
            const severityStyle = SEVERITY_STYLES[item.severity as keyof typeof SEVERITY_STYLES] || SEVERITY_STYLES.Low;
            const statusStyle = STATUS_STYLES[item.status as keyof typeof STATUS_STYLES] || STATUS_STYLES.Open;
            const SeverityIcon = severityStyle.icon;
            const TypeIcon = TYPE_ICONS[item.type as keyof typeof TYPE_ICONS] || FileText;
            const isCritical = item.severity === "Critical" || item.severity === "High";
            const isOpen = item.status === "Open" || item.status === "Assigned";

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
                      {item.severity}
                    </span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                      {item.status}
                    </span>
                    <span className="flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                      <TypeIcon className="h-3 w-3" />
                      {item.type}
                    </span>
                    {isOpen && (
                      <span className="flex items-center gap-1 rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
                        <span className="relative flex h-1.5 w-1.5">
                          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                          <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                        </span>
                        Active
                      </span>
                    )}
                  </div>
                  <div className="mt-1 text-sm font-semibold text-ink-950">{item.title}</div>
                  <div className="mt-0.5 text-sm text-ink-600">{item.description}</div>
                  <div className="mt-1.5 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                    <span className="flex items-center gap-1">
                      <MapPin className="h-3.5 w-3.5" />
                      {item.location}
                    </span>
                    <span className="text-ink-300">·</span>
                    <span className="flex items-center gap-1">
                      <Clock className="h-3.5 w-3.5" />
                      {item.age}
                    </span>
                    {item.assignedTo && (
                      <>
                        <span className="text-ink-300">·</span>
                        <span className="flex items-center gap-1">
                          <Users className="h-3.5 w-3.5" />
                          {item.assignedTo}
                        </span>
                      </>
                    )}
                    {item.tripId && (
                      <>
                        <span className="text-ink-300">·</span>
                        <span className="flex items-center gap-1">
                          <Navigation className="h-3.5 w-3.5" />
                          <Link
                            href={`/operations/trips/${item.tripId}`}
                            className="font-mono text-primary hover:underline"
                          >
                            {item.tripId}
                          </Link>
                        </span>
                      </>
                    )}
                  </div>

                  {/* Action timeline */}
                  {item.actions.length > 0 && (
                    <div className="mt-2 flex flex-wrap items-center gap-2 border-t border-line pt-2">
                      {item.actions.slice(0, 2).map((action, idx) => (
                        <span
                          key={idx}
                          className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] text-ink-400"
                        >
                          {action.label}
                        </span>
                      ))}
                      {item.actions.length > 2 && (
                        <span className="text-[10px] text-ink-400">
                          +{item.actions.length - 2} more
                        </span>
                      )}
                    </div>
                  )}
                </div>

                <div className="flex items-center gap-2 flex-shrink-0">
                  {isOpen ? (
                    <>
                      <button className="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-hover transition-colors">
                        <Flag className="inline h-3.5 w-3.5 mr-1" />
                        Assign
                      </button>
                      <button className="rounded-md border border-line bg-white px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors">
                        <Eye className="h-3.5 w-3.5" />
                      </button>
                    </>
                  ) : (
                    <Link
                      href={`/operations/rescue/${item.id}`}
                      className="rounded-md border border-line bg-white px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                    >
                      <Eye className="inline h-3.5 w-3.5 mr-1" />
                      View
                    </Link>
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