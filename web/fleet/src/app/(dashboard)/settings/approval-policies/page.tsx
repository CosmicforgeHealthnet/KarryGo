// src/app/(dashboard)/settings/approval-policies/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  FileCheck,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  Download,
  FileText,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Shield,
  Lock,
  EyeOff,
  Settings,
  Users,
  Tag,
  Info,
  ListChecks,
  Printer,
  Edit3,
  Trash2,
  MoreHorizontal,
  ExternalLink,
  FolderOpen,
  HardDrive,
  Database,
  Cloud,
  History,
  GitBranch,
  BookOpen,
  Book,
  ArrowRight,
} from "lucide-react";

// ==================== TYPES ====================

type ApprovalPolicy = {
  id: string;
  reference: string;
  name: string;
  description: string;
  action: string;
  approvalCount: number;
  approvalOrder: "Sequential" | "Parallel" | "Any";
  separationOfDuty: boolean;
  selfApproval: boolean;
  expiryHours: number;
  threshold: string | null;
  status: "Active" | "Inactive" | "Archived";
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_POLICIES: ApprovalPolicy[] = [
  {
    id: "AP-001",
    reference: "POL-2026-001",
    name: "Staff Role Change",
    description: "Requires approval for any staff role or scope changes.",
    action: "Role Change",
    approvalCount: 2,
    approvalOrder: "Sequential",
    separationOfDuty: true,
    selfApproval: false,
    expiryHours: 72,
    threshold: null,
    status: "Active",
    createdBy: "F. Daniel",
    createdAt: "2026-06-01T09:00:00Z",
    updatedAt: "2026-08-23T10:00:00Z",
    tags: ["Staff", "Role"],
  },
  {
    id: "AP-002",
    reference: "POL-2026-002",
    name: "Wallet Withdrawal",
    description: "Requires approval for company wallet withdrawals above ₦1,000,000.",
    action: "Withdrawal",
    approvalCount: 2,
    approvalOrder: "Sequential",
    separationOfDuty: true,
    selfApproval: false,
    expiryHours: 48,
    threshold: "₦1,000,000",
    status: "Active",
    createdBy: "F. Daniel",
    createdAt: "2026-06-15T10:00:00Z",
    updatedAt: "2026-08-22T16:00:00Z",
    tags: ["Wallet", "Finance"],
  },
  {
    id: "AP-003",
    reference: "POL-2026-003",
    name: "Allocation Approval",
    description: "Requires approval for provider allocations above ₦500,000.",
    action: "Allocation",
    approvalCount: 1,
    approvalOrder: "Parallel",
    separationOfDuty: false,
    selfApproval: true,
    expiryHours: 24,
    threshold: "₦500,000",
    status: "Active",
    createdBy: "F. Daniel",
    createdAt: "2026-07-01T11:00:00Z",
    updatedAt: "2026-08-21T14:00:00Z",
    tags: ["Allocation", "Finance"],
  },
  {
    id: "AP-004",
    reference: "POL-2026-004",
    name: "Staff Suspension",
    description: "Requires approval for staff suspension or reactivation.",
    action: "Staff Suspension",
    approvalCount: 2,
    approvalOrder: "Sequential",
    separationOfDuty: true,
    selfApproval: false,
    expiryHours: 48,
    threshold: null,
    status: "Active",
    createdBy: "F. Daniel",
    createdAt: "2026-07-15T09:00:00Z",
    updatedAt: "2026-08-20T11:00:00Z",
    tags: ["Staff", "Suspension"],
  },
  {
    id: "AP-005",
    reference: "POL-2026-005",
    name: "Return to Service",
    description: "Requires approval for asset return-to-service decisions.",
    action: "Return to Service",
    approvalCount: 1,
    approvalOrder: "Any",
    separationOfDuty: false,
    selfApproval: false,
    expiryHours: 24,
    threshold: null,
    status: "Inactive",
    createdBy: "M. Bello",
    createdAt: "2026-08-01T10:00:00Z",
    updatedAt: "2026-08-19T08:00:00Z",
    tags: ["Fleet", "Maintenance"],
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const ORDER_STYLES: Record<string, { bg: string; text: string }> = {
  Sequential: { bg: "bg-blue-bg", text: "text-blue" },
  Parallel: { bg: "bg-purple-bg", text: "text-purple" },
  Any: { bg: "bg-green-bg", text: "text-green" },
};

const STATUS_FILTERS = ["All", "Active", "Inactive", "Archived"];

export default function ApprovalPoliciesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ApprovalPolicy[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_POLICIES);
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
      item.action.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading approval policies...</p>
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

  const goToPolicyDetail = (id: string) => {
    router.push(`/settings/approval-policies/${id}`);
  };

  const goToCreatePolicy = () => {
    router.push("/settings/approval-policies/create");
  };

  // Stats
  const totalActive = data.filter((d) => d.status === "Active").length;
  const totalWithSOD = data.filter((d) => d.separationOfDuty).length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Approval Policies
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Controlled action, approval count/order, separation-of-duty rule, expiry and thresholds/references.
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
            onClick={goToCreatePolicy}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Policy
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Separation of Duty</div>
          <div className="text-lg font-bold text-purple">{totalWithSOD}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Self-Approval</div>
          <div className="text-lg font-bold text-amber">{data.filter((d) => d.selfApproval).length}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, action, reference..."
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

      {/* ===== POLICIES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No policies found</h3>
          <p className="mt-1 text-sm text-ink-500">No approval policies match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Policy</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Action</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Approvals</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Order</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">SOD</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
                  const orderStyle = ORDER_STYLES[item.approvalOrder] || ORDER_STYLES.Sequential;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToPolicyDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.name}</div>
                        <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.action}</td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.approvalCount}</td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${orderStyle.bg} ${orderStyle.text}`}>
                          {item.approvalOrder}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        {item.separationOfDuty ? (
                          <span className="text-green text-xs font-medium">Yes</span>
                        ) : (
                          <span className="text-ink-400 text-xs">No</span>
                        )}
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
          <FileCheck className="h-4 w-4" />
          IAM-08 · Approval Policies
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} policies shown
        </span>
      </div>
    </div>
  );
}