// src/app/(dashboard)/settings/roles/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Plus,
  Search,
  CheckCircle2,
  ChevronDown,
  Shield,
  SlidersHorizontal,
  X,
  Clock,
  Eye,
  Edit3,
} from "lucide-react";

// ==================== TYPES ====================

type Role = {
  id: string;
  reference: string;
  name: string;
  description: string;
  type: "System" | "Custom";
  scope: "Company-wide" | "Branch" | "Department";
  status: "Active" | "Inactive" | "Archived";
  permissions: number;
  assignedUsers: number;
  createdAt: string;
  updatedAt: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_ROLES: Role[] = [
  {
    id: "R-001",
    reference: "ROLE-2026-001",
    name: "Organization Admin",
    description: "Full access to all organization settings and data.",
    type: "System",
    scope: "Company-wide",
    status: "Active",
    permissions: 42,
    assignedUsers: 1,
    createdAt: "2026-06-01T09:00:00Z",
    updatedAt: "2026-08-23T10:00:00Z",
    tags: ["Admin", "System"],
  },
  {
    id: "R-002",
    reference: "ROLE-2026-002",
    name: "Operations Manager",
    description: "Manage operations, dispatch, and provider workflows.",
    type: "Custom",
    scope: "Branch",
    status: "Active",
    permissions: 28,
    assignedUsers: 2,
    createdAt: "2026-06-15T10:00:00Z",
    updatedAt: "2026-08-22T16:00:00Z",
    tags: ["Operations", "Manager"],
  },
  {
    id: "R-003",
    reference: "ROLE-2026-003",
    name: "Fleet Manager",
    description: "Manage fleet assets, maintenance, and inspections.",
    type: "Custom",
    scope: "Branch",
    status: "Active",
    permissions: 22,
    assignedUsers: 1,
    createdAt: "2026-06-20T14:00:00Z",
    updatedAt: "2026-08-21T11:00:00Z",
    tags: ["Fleet", "Manager"],
  },
  {
    id: "R-004",
    reference: "ROLE-2026-004",
    name: "Dispatch Supervisor",
    description: "Manage dispatch board, assignments, and trips.",
    type: "Custom",
    scope: "Branch",
    status: "Active",
    permissions: 18,
    assignedUsers: 1,
    createdAt: "2026-07-01T11:00:00Z",
    updatedAt: "2026-08-20T09:00:00Z",
    tags: ["Dispatch", "Supervisor"],
  },
  {
    id: "R-005",
    reference: "ROLE-2026-005",
    name: "Read-only User",
    description: "View-only access to all data.",
    type: "System",
    scope: "Company-wide",
    status: "Active",
    permissions: 8,
    assignedUsers: 0,
    createdAt: "2026-07-15T09:00:00Z",
    updatedAt: "2026-08-19T14:00:00Z",
    tags: ["Read-only", "System"],
  },
  {
    id: "R-006",
    reference: "ROLE-2026-006",
    name: "Warehouse Manager",
    description: "Manage warehouse operations and inventory.",
    type: "Custom",
    scope: "Branch",
    status: "Inactive",
    permissions: 16,
    assignedUsers: 0,
    createdAt: "2026-08-01T10:00:00Z",
    updatedAt: "2026-08-18T08:00:00Z",
    tags: ["Warehouse", "Manager"],
  },
];

type StatusStyle = {
  bg: string;
  text: string;
  dot: string;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

type TypeStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const TYPE_STYLES: Record<string, TypeStyle> = {
  System: { bg: "bg-purple-bg", text: "text-purple", icon: <Shield className="h-3.5 w-3.5" /> },
  Custom: { bg: "bg-blue-bg", text: "text-blue", icon: <Edit3 className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Active", "Inactive", "Archived"];
const TYPE_FILTERS = ["All", "System", "Custom"];

export default function RolesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<Role[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [search, setSearch] = useState<string>("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [selectedType, setSelectedType] = useState<string>("All");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_ROLES);
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
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesType = selectedType === "All" || item.type === selectedType;

    return matchesSearch && matchesStatus && matchesType;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading roles...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedType !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedType("All");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0) + (selectedType !== "All" ? 1 : 0);

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

  const goToRoleDetail = (id: string) => {
    router.push(`/settings/roles/${id}`);
  };

  const goToCreateRole = () => {
    router.push("/settings/roles/create");
  };

  // Stats
  const totalActive = data.filter((d) => d.status === "Active").length;
  const totalSystem = data.filter((d) => d.type === "System").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Roles
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            System roles and company-created roles with scope and status.
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
            onClick={goToCreateRole}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Role
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">System</div>
          <div className="text-lg font-bold text-purple">{totalSystem}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Custom</div>
          <div className="text-lg font-bold text-blue">{data.length - totalSystem}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, reference..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Inactive", "Archived"];
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
          onClick={() => {
            const types = ["All", "System", "Custom"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Shield className="h-4 w-4 text-ink-400" />
          {selectedType}
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

      {/* ===== ROLES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Shield className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No roles found</h3>
          <p className="mt-1 text-sm text-ink-500">No roles match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Role</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Scope</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Permissions</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Users</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Custom;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToRoleDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.name}</div>
                        <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.icon}
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.scope}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.permissions}</td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.assignedUsers}</td>
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
          IAM-04 · Roles
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} roles shown
        </span>
      </div>
    </div>
  );
}