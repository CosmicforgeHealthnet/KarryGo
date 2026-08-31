// src/app/(dashboard)/settings/approvals/delegation/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  UserCheck,
  RefreshCw,
  Clock,
  Eye,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
} from "lucide-react";

// ==================== TYPES ====================

type Delegation = {
  id: string;
  reference: string;
  delegator: {
    id: string;
    name: string;
    email: string;
  };
  delegate: {
    id: string;
    name: string;
    email: string;
  };
  scope: string;
  validity: {
    start: string;
    end: string;
  };
  status: "Active" | "Expired" | "Revoked";
  createdBy: string;
  createdAt: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_DELEGATIONS: Delegation[] = [
  {
    id: "D-001",
    reference: "DEL-2026-001",
    delegator: {
      id: "S-001",
      name: "Favour Daniel",
      email: "favour@apexlogistics.com",
    },
    delegate: {
      id: "S-002",
      name: "Oluwaseun Adebayo",
      email: "seun@apexlogistics.com",
    },
    scope: "All Approvals",
    validity: {
      start: "2026-08-01",
      end: "2026-08-31",
    },
    status: "Active",
    createdBy: "F. Daniel",
    createdAt: "2026-08-01T09:00:00Z",
    tags: ["Approval", "Full"],
  },
  {
    id: "D-002",
    reference: "DEL-2026-002",
    delegator: {
      id: "S-002",
      name: "Oluwaseun Adebayo",
      email: "seun@apexlogistics.com",
    },
    delegate: {
      id: "S-004",
      name: "Amina Yusuf",
      email: "amina@apexlogistics.com",
    },
    scope: "Operations Approvals",
    validity: {
      start: "2026-08-15",
      end: "2026-09-15",
    },
    status: "Active",
    createdBy: "O. Adebayo",
    createdAt: "2026-08-15T10:00:00Z",
    tags: ["Approval", "Operations"],
  },
  {
    id: "D-003",
    reference: "DEL-2026-003",
    delegator: {
      id: "S-003",
      name: "Musa Bello",
      email: "musa@apexlogistics.com",
    },
    delegate: {
      id: "S-006",
      name: "Esther Obi",
      email: "esther@apexlogistics.com",
    },
    scope: "Fleet Approvals",
    validity: {
      start: "2026-07-01",
      end: "2026-07-31",
    },
    status: "Expired",
    createdBy: "M. Bello",
    createdAt: "2026-07-01T14:00:00Z",
    tags: ["Approval", "Fleet"],
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Revoked: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const STATUS_FILTERS = ["All", "Active", "Expired", "Revoked"];

export default function ApprovalDelegationPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Delegation[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_DELEGATIONS);
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
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.delegator.name.toLowerCase().includes(search.toLowerCase()) ||
      item.delegate.name.toLowerCase().includes(search.toLowerCase()) ||
      item.scope.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading delegations...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0);

  const formatTime = (date: string) => {
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

  const formatDate = (date: string) => {
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  const goToDelegationDetail = (id: string) => {
    router.push(`/settings/approvals/delegation/${id}`);
  };

  const goToCreateDelegation = () => {
    router.push("/settings/approvals/delegation/create");
  };

  // Stats
  const totalActive = data.filter((d) => d.status === "Active").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Approval Delegation
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Delegator, delegate, permitted scope and validity window.
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
            onClick={goToCreateDelegation}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Delegation
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-green">{totalActive}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expired / Revoked</div>
          <div className="text-lg font-bold text-ink-500">{data.length - totalActive}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by delegator, delegate, scope..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Expired", "Revoked"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <UserCheck className="h-4 w-4 text-ink-400" />
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

      {/* ===== DELEGATIONS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <UserCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No delegations found</h3>
          <p className="mt-1 text-sm text-ink-500">No delegations match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[900px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Delegation</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Delegator</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Delegate</th>
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
                      onClick={() => goToDelegationDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-mono text-[11px] font-medium text-ink-950">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs font-medium text-ink-950">{item.delegator.name}</div>
                        <div className="text-[10px] text-ink-500">{item.delegator.email}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs font-medium text-ink-950">{item.delegate.name}</div>
                        <div className="text-[10px] text-ink-500">{item.delegate.email}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.scope}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">
                        {formatDate(item.validity.start)} → {formatDate(item.validity.end)}
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
          <UserCheck className="h-4 w-4" />
          IAM-11 · Approval Delegation
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} delegations shown
        </span>
      </div>
    </div>
  );
}