// src/app/(dashboard)/settings/legal-holds/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Scale,
  RefreshCw,
  Plus,
  Search,
  ChevronDown,
  Tag,
  SlidersHorizontal,
  X,
  Clock,
  FileText,
  User,
  Users,
  Car,
  MapPin,
  Briefcase,
  Globe,
  Eye,
} from "lucide-react";

// ==================== TYPES ====================

type LegalHold = {
  id: string;
  reference: string;
  scopeType: "Document" | "Staff" | "Provider" | "Asset" | "Trip" | "Order" | "All";
  scopeId: string | null;
  scopeName: string | null;
  reason: string;
  status: "Active" | "Released" | "Expired";
  creator: {
    id: string;
    name: string;
  };
  approver: {
    id: string;
    name: string;
  } | null;
  createdAt: string;
  releasedAt: string | null;
  releasedBy: string | null;
  releaseReason: string | null;
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_LEGAL_HOLDS: LegalHold[] = [
  {
    id: "LH-001",
    reference: "LH-2026-001",
    scopeType: "Document",
    scopeId: "DOC-007",
    scopeName: "Financial Report Q3 2026",
    reason: "Under audit investigation",
    status: "Active",
    creator: { id: "S-001", name: "Favour Daniel" },
    approver: { id: "S-006", name: "Esther Obi" },
    createdAt: "2026-08-20T09:00:00Z",
    releasedAt: null,
    releasedBy: null,
    releaseReason: null,
    branch: "Ikeja",
    tags: ["Audit", "Financial"],
  },
  {
    id: "LH-002",
    reference: "LH-2026-002",
    scopeType: "Staff",
    scopeId: "S-007",
    scopeName: "Kabiru Ahmed",
    reason: "Employment dispute investigation",
    status: "Active",
    creator: { id: "S-001", name: "Favour Daniel" },
    approver: { id: "S-006", name: "Esther Obi" },
    createdAt: "2026-08-22T10:00:00Z",
    releasedAt: null,
    releasedBy: null,
    releaseReason: null,
    branch: "Apapa",
    tags: ["Staff", "Dispute"],
  },
  {
    id: "LH-003",
    reference: "LH-2026-003",
    scopeType: "Provider",
    scopeId: "P-044",
    scopeName: "Peter Okafor",
    reason: "Complaint investigation",
    status: "Active",
    creator: { id: "S-002", name: "Oluwaseun Adebayo" },
    approver: { id: "S-001", name: "Favour Daniel" },
    createdAt: "2026-08-21T14:00:00Z",
    releasedAt: null,
    releasedBy: null,
    releaseReason: null,
    branch: "Ikeja",
    tags: ["Provider", "Complaint"],
  },
  {
    id: "LH-004",
    reference: "LH-2026-004",
    scopeType: "All",
    scopeId: null,
    scopeName: "All Organization Data",
    reason: "Regulatory compliance review",
    status: "Released",
    creator: { id: "S-001", name: "Favour Daniel" },
    approver: { id: "S-006", name: "Esther Obi" },
    createdAt: "2026-07-01T09:00:00Z",
    releasedAt: "2026-08-15T10:00:00Z",
    releasedBy: "F. Daniel",
    releaseReason: "Review completed, no further holds required",
    branch: "All branches",
    tags: ["Regulatory", "Compliance"],
  },
  {
    id: "LH-005",
    reference: "LH-2026-005",
    scopeType: "Asset",
    scopeId: "TR-112",
    scopeName: "Mercedes Actros TR-112",
    reason: "Safety incident investigation",
    status: "Expired",
    creator: { id: "S-003", name: "Musa Bello" },
    approver: { id: "S-001", name: "Favour Daniel" },
    createdAt: "2026-07-15T11:00:00Z",
    releasedAt: "2026-08-15T11:00:00Z",
    releasedBy: "System",
    releaseReason: "Hold expired after 30 days",
    branch: "Apapa",
    tags: ["Asset", "Safety"],
  },
];

type StatusStyle = {
  bg: string;
  text: string;
  dot: string;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Active: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Released: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

type ScopeStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const SCOPE_STYLES: Record<string, ScopeStyle> = {
  Document: { bg: "bg-blue-bg", text: "text-blue", icon: <FileText className="h-3.5 w-3.5" /> },
  Staff: { bg: "bg-purple-bg", text: "text-purple", icon: <User className="h-3.5 w-3.5" /> },
  Provider: { bg: "bg-amber-bg", text: "text-amber", icon: <Users className="h-3.5 w-3.5" /> },
  Asset: { bg: "bg-green-bg", text: "text-green", icon: <Car className="h-3.5 w-3.5" /> },
  Trip: { bg: "bg-blue-bg", text: "text-blue", icon: <MapPin className="h-3.5 w-3.5" /> },
  Order: { bg: "bg-green-bg", text: "text-green", icon: <Briefcase className="h-3.5 w-3.5" /> },
  All: { bg: "bg-red-bg", text: "text-red", icon: <Globe className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "Active", "Released", "Expired"];
const SCOPE_FILTERS = ["All", "Document", "Staff", "Provider", "Asset", "Trip", "Order"];

export default function LegalHoldsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<LegalHold[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [search, setSearch] = useState<string>("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [selectedScope, setSelectedScope] = useState<string>("All");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_LEGAL_HOLDS);
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
      item.scopeName?.toLowerCase().includes(search.toLowerCase()) ||
      item.reason.toLowerCase().includes(search.toLowerCase()) ||
      item.creator.name.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesScope = selectedScope === "All" || item.scopeType === selectedScope;

    return matchesSearch && matchesStatus && matchesScope;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading legal holds...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedScope !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedScope("All");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0) + (selectedScope !== "All" ? 1 : 0);

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

  const goToHoldDetail = (id: string) => {
    router.push(`/settings/legal-holds/${id}`);
  };

  const goToCreateHold = () => {
    router.push("/settings/legal-holds/create");
  };

  // Stats
  const totalActive = data.filter((d) => d.status === "Active").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Legal Holds
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Scope type/reference, reason, status, creator/approver and created time.
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
            onClick={goToCreateHold}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Hold
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
          <div className="text-lg font-bold text-red">{totalActive}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Released</div>
          <div className="text-lg font-bold text-green">{data.filter((d) => d.status === "Released").length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expired</div>
          <div className="text-lg font-bold text-ink-500">{data.filter((d) => d.status === "Expired").length}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, scope, reason..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Released", "Expired"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Scale className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const scopes = ["All", "Document", "Staff", "Provider", "Asset", "Trip", "Order"];
            const currentIndex = scopes.indexOf(selectedScope);
            const nextIndex = (currentIndex + 1) % scopes.length;
            setSelectedScope(scopes[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Tag className="h-4 w-4 text-ink-400" />
          {selectedScope}
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

      {/* ===== HOLDS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Scale className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No legal holds found</h3>
          <p className="mt-1 text-sm text-ink-500">No holds match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Hold</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Scope</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Creator</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Created</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
                  const scopeStyle = SCOPE_STYLES[item.scopeType] || SCOPE_STYLES.All;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToHoldDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.reason}</div>
                        <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${scopeStyle.bg} ${scopeStyle.text}`}>
                          {scopeStyle.icon}
                          {item.scopeType}
                        </span>
                        {item.scopeName && (
                          <div className="text-[10px] text-ink-500 mt-0.5">{item.scopeName}</div>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.creator.name}</td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
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
          <Scale className="h-4 w-4" />
          SEC-05 · Legal Holds
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} holds shown
        </span>
      </div>
    </div>
  );
}