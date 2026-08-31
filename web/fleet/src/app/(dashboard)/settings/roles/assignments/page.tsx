// src/app/(dashboard)/settings/roles/assignments/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Plus,
  ArrowLeft,
  Search,
  CheckCircle2,
  ChevronDown,
  SlidersHorizontal,
  X,
  Shield,
  Clock,
  Eye,
} from "lucide-react";

// ==================== TYPES ====================

type RoleAssignment = {
  id: string;
  user: {
    id: string;
    name: string;
    email: string;
  };
  role: {
    id: string;
    name: string;
  };
  scope: {
    type: "Company-wide" | "Branch" | "Department";
    value: string;
  };
  validity: {
    start: string | null;
    end: string | null;
  };
  status: "Active" | "Pending" | "Expired" | "Revoked";
  assignedBy: string;
  assignedAt: string;
};

// ==================== MOCK DATA ====================

const MOCK_ASSIGNMENTS: RoleAssignment[] = [
  {
    id: "RA-001",
    user: { id: "S-001", name: "Favour Daniel", email: "favour@apexlogistics.com" },
    role: { id: "R-001", name: "Organization Admin" },
    scope: { type: "Company-wide", value: "All" },
    validity: { start: "2026-06-01", end: null },
    status: "Active",
    assignedBy: "System",
    assignedAt: "2026-06-01T09:00:00Z",
  },
  {
    id: "RA-002",
    user: { id: "S-002", name: "Oluwaseun Adebayo", email: "seun@apexlogistics.com" },
    role: { id: "R-002", name: "Operations Manager" },
    scope: { type: "Branch", value: "Ikeja" },
    validity: { start: "2026-06-15", end: null },
    status: "Active",
    assignedBy: "F. Daniel",
    assignedAt: "2026-06-15T10:00:00Z",
  },
  {
    id: "RA-003",
    user: { id: "S-003", name: "Musa Bello", email: "musa@apexlogistics.com" },
    role: { id: "R-003", name: "Fleet Manager" },
    scope: { type: "Branch", value: "Lekki" },
    validity: { start: "2026-06-20", end: null },
    status: "Active",
    assignedBy: "F. Daniel",
    assignedAt: "2026-06-20T14:00:00Z",
  },
  {
    id: "RA-004",
    user: { id: "S-004", name: "Amina Yusuf", email: "amina@apexlogistics.com" },
    role: { id: "R-004", name: "Dispatch Supervisor" },
    scope: { type: "Branch", value: "Yaba" },
    validity: { start: "2026-07-01", end: null },
    status: "Active",
    assignedBy: "O. Adebayo",
    assignedAt: "2026-07-01T11:00:00Z",
  },
  {
    id: "RA-005",
    user: { id: "S-005", name: "Chinedu Uche", email: "chinedu@apexlogistics.com" },
    role: { id: "R-006", name: "Warehouse Manager" },
    scope: { type: "Branch", value: "Ikeja" },
    validity: { start: "2026-08-01", end: null },
    status: "Pending",
    assignedBy: "F. Daniel",
    assignedAt: "2026-08-01T10:00:00Z",
  },
  {
    id: "RA-006",
    user: { id: "S-006", name: "Esther Obi", email: "esther@apexlogistics.com" },
    role: { id: "R-005", name: "Read-only User" },
    scope: { type: "Company-wide", value: "All" },
    validity: { start: "2026-07-15", end: "2026-12-31" },
    status: "Active",
    assignedBy: "F. Daniel",
    assignedAt: "2026-07-15T09:00:00Z",
  },
];

type StatusStyle = {
  bg: string;
  text: string;
  dot: string;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Revoked: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const SCOPE_LABELS: Record<string, string> = {
  "Company-wide": "Company-wide",
  Branch: "Branch",
  Department: "Department",
};

const STATUS_FILTERS = ["All", "Active", "Pending", "Expired", "Revoked"];

export default function RoleAssignmentPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<RoleAssignment[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [search, setSearch] = useState<string>("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_ASSIGNMENTS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.user.name.toLowerCase().includes(search.toLowerCase()) ||
      item.user.email.toLowerCase().includes(search.toLowerCase()) ||
      item.role.name.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading role assignments...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0);

  const formatTime = (date: string): string => {
    const d = new Date(date);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${diffDay}d ago`;
  };

  const formatDate = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  const goBack = () => {
    router.push("/settings/roles");
  };

  const goToCreateAssignment = () => {
    router.push("/settings/roles/assignments/create");
  };

  // Stats
  const totalActive = data.filter((d) => d.status === "Active").length;
  const totalPending = data.filter((d) => d.status === "Pending").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/settings/roles")}
              className="hover:text-primary transition-colors"
            >
              Roles
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">Assignments</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Role Assignments
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            User, role, company/branch/department scope and validity period.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button
            onClick={goToCreateAssignment}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Assign Role
          </button>
          <button
            onClick={goBack}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-green">{totalActive}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{totalPending}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expired / Revoked</div>
          <div className="text-lg font-bold text-red">{data.length - totalActive - totalPending}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by user, email, role..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Pending", "Expired", "Revoked"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <CheckCircle2 className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {filterCount}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== ASSIGNMENTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Shield className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No assignments found</h3>
          <p className="mt-1 text-sm text-ink-500">No role assignments match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[1000px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">User</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Role</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Scope</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Validity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;

                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line hover:bg-surface-2 transition-colors group cursor-pointer"
                      onClick={() => router.push(`/settings/roles/assignments/${item.id}`)}
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.user.name}</div>
                        <div className="text-[10px] text-ink-500">{item.user.email}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-semibold text-ink-600">
                          {item.role.name}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs text-ink-950">{SCOPE_LABELS[item.scope.type]}</div>
                        <div className="text-[10px] text-ink-500">{item.scope.value}</div>
                      </td>
                      <td className="px-4 py-3 text-xs text-ink-500">
                        {item.validity.start ? formatDate(item.validity.start) : "—"}
                        {item.validity.end && ` → ${formatDate(item.validity.end)}`}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <button className="text-ink-400 group-hover:text-primary transition-colors">
                          <Eye className="h-4 w-4" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          IAM-07 · Role Assignments
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} assignments shown
        </span>
      </div>
    </div>
  );
}