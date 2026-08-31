// src/app/(dashboard)/operations/control-tower/exceptions/page.tsx

"use client";

import { useState, useEffect } from "react";
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
import { EXCEPTIONS_DATA } from "@/lib/mock-control-tower-data";

type Exception = {
  id: string;
  type: "Late" | "Off-Route" | "Stale Location" | "Rejected Assignment" | "Expired Assignment" | "Safety" | "Capacity";
  severity: "Critical" | "High" | "Medium" | "Low";
  subject: string;
  subjectType: "Trip" | "Provider" | "Asset" | "Assignment";
  description: string;
  branch: string;
  status: "Open" | "Acknowledged" | "In Progress" | "Resolved";
  createdAt: string;
  age: string;
  assignedTo?: string;
};

const SEVERITY_STYLES = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: AlertCircle },
  High: { bg: "bg-red-bg/70", text: "text-red", dot: "bg-red", icon: AlertTriangle },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: AlertCircle },
  Low: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: Info },
};

const STATUS_STYLES = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Acknowledged: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  "In Progress": { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
};

const TYPE_ICONS = {
  Late: Clock,
  "Off-Route": Navigation,
  "Stale Location": MapPin,
  "Rejected Assignment": X,
  "Expired Assignment": Clock,
  Safety: Shield,
  Capacity: Package,
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const SEVERITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const STATUS_FILTERS = ["All", "Open", "Acknowledged", "In Progress", "Resolved"];

export default function ExceptionsPage() {
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [data, setData] = useState<Exception[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(EXCEPTIONS_DATA);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.subject.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase());

    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesSeverity && matchesStatus && matchesBranch;
  });

  const openCount = data.filter((d) => d.status === "Open").length;
  const criticalCount = data.filter((d) => d.severity === "Critical").length;
  const resolvedCount = data.filter((d) => d.status === "Resolved").length;

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
            href="/operations/rescue"
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
            const severityStyle = SEVERITY_STYLES[item.severity as keyof typeof SEVERITY_STYLES] || SEVERITY_STYLES.Low;
            const statusStyle = STATUS_STYLES[item.status as keyof typeof STATUS_STYLES] || STATUS_STYLES.Open;
            const SeverityIcon = severityStyle.icon;
            const TypeIcon = TYPE_ICONS[item.type as keyof typeof TYPE_ICONS] || AlertCircle;
            const isCritical = item.severity === "Critical" || item.severity === "High";

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
                  </div>
                  <div className="mt-1 text-sm font-medium text-ink-950">{item.subject}</div>
                  <div className="mt-0.5 text-sm text-ink-600">{item.description}</div>
                  <div className="mt-1.5 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                    <span className="flex items-center gap-1">
                      <MapPin className="h-3.5 w-3.5" />
                      {item.branch}
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
                  </div>
                </div>

                <div className="flex items-center gap-2 flex-shrink-0">
                  {item.status !== "Resolved" && (
                    <>
                      <button className="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-hover transition-colors">
                        <Flag className="inline h-3.5 w-3.5 mr-1" />
                        Acknowledge
                      </button>
                      <Link
                        href={`/operations/rescue`}
                        className="rounded-md border border-line bg-white px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                      >
                        <LifeBuoy className="h-3.5 w-3.5" />
                        Rescue
                      </Link>
                    </>
                  )}
                  {item.status === "Resolved" && (
                    <span className="text-xs font-medium text-green">✓ Resolved</span>
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