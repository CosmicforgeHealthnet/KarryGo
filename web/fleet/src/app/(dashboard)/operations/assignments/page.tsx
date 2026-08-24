// src/app/(dashboard)/operations/assignments/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  Search,
  Package,
  Truck,
  Clock,
  SlidersHorizontal,
  X,
  ChevronUp,
  ChevronDown,
  ArrowRight,
} from "lucide-react";

type Assignment = {
  id: string;
  offerId: string;
  provider: string;
  asset: string;
  branch: string;
  status: "Pending response" | "Accepted" | "Declined" | "Expired" | "In Progress" | "Completed";
  deadline: string;
  created: string;
};

const ASSIGNMENTS: Assignment[] = [
  {
    id: "ASN-731968",
    offerId: "MKT-24062",
    provider: "Peter Okafor",
    asset: "Bike BR-88",
    branch: "Ikeja",
    status: "Pending response",
    deadline: "12 min",
    created: "2026-08-24 09:30",
  },
  {
    id: "ASN-731967",
    offerId: "MKT-24073",
    provider: "Chinedu Uche",
    asset: "Bike BR-20",
    branch: "Lekki",
    status: "Accepted",
    deadline: "45 min",
    created: "2026-08-24 08:15",
  },
  {
    id: "ASN-731966",
    offerId: "MKT-24079",
    provider: "Amina Yusuf",
    asset: "Van VN-04",
    branch: "Ikeja",
    status: "Declined",
    deadline: "Expired",
    created: "2026-08-24 07:00",
  },
  {
    id: "ASN-731965",
    offerId: "MKT-24081",
    provider: "Musa Bello",
    asset: "Truck TR-12",
    branch: "Apapa",
    status: "In Progress",
    deadline: "1.2 hrs",
    created: "2026-08-24 06:30",
  },
  {
    id: "ASN-731964",
    offerId: "MKT-24069",
    provider: "Kabiru Ahmed",
    asset: "Van VN-04",
    branch: "Apapa",
    status: "Completed",
    deadline: "Completed",
    created: "2026-08-23 16:00",
  },
  {
    id: "ASN-731963",
    offerId: "MKT-24058",
    provider: "Peter Okafor",
    asset: "Bike BR-88",
    branch: "Ikeja",
    status: "Expired",
    deadline: "Expired",
    created: "2026-08-23 14:30",
  },
];

const STATUS_STYLES: Record<Assignment["status"], { bg: string; text: string; dot: string }> = {
  "Pending response": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Accepted: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Declined: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  "In Progress": { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
};

const STATUS_OPTIONS = ["All", "Pending response", "Accepted", "Declined", "Expired", "In Progress", "Completed"];
const BRANCH_OPTIONS = ["All", "Ikeja", "Lekki", "Apapa", "Yaba"];

type SortField = "status" | "deadline" | "provider" | "id";
type SortDirection = "asc" | "desc";

const STATUS_ORDER = {
  "Pending response": 0,
  Accepted: 1,
  "In Progress": 2,
  Completed: 3,
  Declined: 4,
  Expired: 5,
};

export default function AssignmentsListPage() {
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("All");
  const [branchFilter, setBranchFilter] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [sortField, setSortField] = useState<SortField>("status");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  const filtered = ASSIGNMENTS.filter((a) => {
    const matchesSearch =
      search.trim() === "" ||
      a.id.toLowerCase().includes(search.toLowerCase()) ||
      a.offerId.toLowerCase().includes(search.toLowerCase()) ||
      a.provider.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = statusFilter === "All" || a.status === statusFilter;
    const matchesBranch = branchFilter === "All" || a.branch === branchFilter;

    return matchesSearch && matchesStatus && matchesBranch;
  });

  const sorted = [...filtered].sort((a, b) => {
    let aVal: string | number = a[sortField];
    let bVal: string | number = b[sortField];

    if (sortField === "status") {
      aVal = STATUS_ORDER[a.status];
      bVal = STATUS_ORDER[b.status];
    }

    if (aVal < bVal) return sortDirection === "asc" ? -1 : 1;
    if (aVal > bVal) return sortDirection === "asc" ? 1 : -1;
    return 0;
  });

  const hasActiveFilters = statusFilter !== "All" || branchFilter !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("All");
    setBranchFilter("All");
  };

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortField(field);
      setSortDirection("asc");
    }
  };

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return <ChevronUp className="h-3 w-3 text-ink-300" />;
    return sortDirection === "asc" ? (
      <ChevronUp className="h-3 w-3 text-primary" />
    ) : (
      <ChevronDown className="h-3 w-3 text-primary" />
    );
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <p className="ml-3">Loading assignments...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Assignments</h1>
        <p className="mt-1.5">
          All company assignments. Pending response items require attention.
        </p>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4 lg:grid-cols-6">
        <StatCard label="Total" value={ASSIGNMENTS.length} tone="blue" />
        <StatCard
          label="Pending"
          value={ASSIGNMENTS.filter((a) => a.status === "Pending response").length}
          tone="amber"
        />
        <StatCard
          label="Accepted"
          value={ASSIGNMENTS.filter((a) => a.status === "Accepted").length}
          tone="green"
        />
        <StatCard
          label="In Progress"
          value={ASSIGNMENTS.filter((a) => a.status === "In Progress").length}
          tone="blue"
        />
        <StatCard
          label="Declined"
          value={ASSIGNMENTS.filter((a) => a.status === "Declined").length}
          tone="red"
        />
        <StatCard
          label="Expired"
          value={ASSIGNMENTS.filter((a) => a.status === "Expired").length}
          tone="neutral"
        />
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search assignments..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <span className="text-ink-400">Status:</span>
          {statusFilter}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Branch:</span>
          {branchFilter}
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
          {(statusFilter !== "All" || branchFilter !== "All") && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(statusFilter !== "All" ? 1 : 0) + (branchFilter !== "All" ? 1 : 0)}
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
          {sorted.length} assignments
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_OPTIONS.map((status) => {
                  const style = status !== "All" ? STATUS_STYLES[status as Assignment["status"]] : null;
                  return (
                    <button
                      key={status}
                      onClick={() => setStatusFilter(status)}
                      className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                        statusFilter === status
                          ? style
                            ? `${style.bg} ${style.text}`
                            : "bg-primary text-white"
                          : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                      }`}
                    >
                      {status}
                    </button>
                  );
                })}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Branch</span>
              <div className="flex flex-wrap gap-1.5">
                {BRANCH_OPTIONS.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setBranchFilter(branch)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      branchFilter === branch
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

      {/* ── Table ── */}
      <div className="overflow-x-auto rounded-lg border border-line bg-white">
        <table className="w-full border-collapse text-left">
          <thead>
            <tr className="border-b border-line bg-surface-1">
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("id")} className="flex items-center gap-1 hover:text-ink-700">
                  Assignment <SortIcon field="id" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Offer</th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("provider")} className="flex items-center gap-1 hover:text-ink-700">
                  Provider <SortIcon field="provider" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Asset</th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Branch</th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("status")} className="flex items-center gap-1 hover:text-ink-700">
                  Status <SortIcon field="status" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("deadline")} className="flex items-center gap-1 hover:text-ink-700">
                  Deadline <SortIcon field="deadline" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">Action</th>
            </tr>
          </thead>
          <tbody>
            {sorted.length === 0 ? (
              <tr>
                <td colSpan={8} className="px-4 py-12 text-center text-ink-500">
                  <div className="flex flex-col items-center gap-2">
                    <Package className="h-8 w-8 text-ink-300" />
                    <p className="font-medium">No assignments match your filters</p>
                    <button onClick={clearFilters} className="text-sm text-primary hover:underline">
                      Clear filters
                    </button>
                  </div>
                </td>
              </tr>
            ) : (
              sorted.map((a) => {
                const statusStyle = STATUS_STYLES[a.status];
                const isPending = a.status === "Pending response";

                return (
                  <tr key={a.id} className="border-b border-line transition-colors hover:bg-surface-1/50">
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div>
                        <span className="font-mono font-semibold text-ink-950">{a.id}</span>
                        <div className="text-xs text-ink-500">{a.created}</div>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <Link href={`/operations/marketplace/${a.offerId}`} className="font-mono text-sm text-primary hover:underline">
                        {a.offerId}
                      </Link>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-2">
                        <div className="flex h-7 w-7 items-center justify-center rounded-full bg-primary/10 text-[10px] font-bold text-primary">
                          {a.provider.split(" ").map((n) => n[0]).join("")}
                        </div>
                        <span className="text-sm font-medium text-ink-950">{a.provider}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-1.5">
                        <Truck className="h-3.5 w-3.5 text-ink-400" />
                        <span className="text-sm text-ink-700">{a.asset}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap text-sm text-ink-700">{a.branch}</td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <span className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                        {a.status}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-1.5">
                        <Clock className={`h-3.5 w-3.5 ${isPending ? "text-amber" : "text-ink-400"}`} />
                        <span className={`text-sm font-medium ${isPending ? "text-amber" : "text-ink-600"}`}>
                          {a.deadline}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 text-right whitespace-nowrap">
                      <Link
                        href={`/operations/assignments/${a.id}`}
                        className="inline-flex items-center gap-1 rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                      >
                        View
                        <ArrowRight className="h-3 w-3" />
                      </Link>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function StatCard({ label, value, tone }: { label: string; value: number; tone: "blue" | "amber" | "green" | "red" | "neutral" }) {
  const colorMap = {
    blue: "bg-primary-bg text-primary",
    amber: "bg-amber-bg text-amber",
    green: "bg-green-bg text-green",
    red: "bg-red-bg text-red",
    neutral: "bg-surface-2 text-ink-600",
  };

  return (
    <div className="rounded-lg border border-line bg-white p-3.5 transition-colors hover:border-ink-300">
      <div className="text-xs font-medium text-ink-500">{label}</div>
      <div className={`mt-1 text-xl font-bold ${colorMap[tone]}`}>{value}</div>
    </div>
  );
}