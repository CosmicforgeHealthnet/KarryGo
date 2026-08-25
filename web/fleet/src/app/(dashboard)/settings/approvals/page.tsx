// src/app/(dashboard)/settings/approvals/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Inbox,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertTriangle,
  Eye,
  Check,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Download,
} from "lucide-react";

// ==================== TYPES ====================

type ApprovalRequest = {
  id: string;
  reference: string;
  action: string;
  resource: string;
  requester: {
    id: string;
    name: string;
  };
  status: "Pending" | "Approved" | "Rejected" | "Cancelled" | "Expired" | "Executed";
  urgency: "Urgent" | "High" | "Normal" | "Low";
  approvalsRequired: number;
  approvalsReceived: number;
  createdAt: string;
  expiresAt: string | null;
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_APPROVALS: ApprovalRequest[] = [
  {
    id: "AR-001",
    reference: "APR-2026-001",
    action: "Staff Role Change",
    resource: "S-005 · Chinedu Uche",
    requester: { id: "S-001", name: "Favour Daniel" },
    status: "Pending",
    urgency: "High",
    approvalsRequired: 2,
    approvalsReceived: 1,
    createdAt: "2026-08-23T09:00:00Z",
    expiresAt: "2026-08-26T09:00:00Z",
    branch: "Ikeja",
    tags: ["Staff", "Role"],
  },
  {
    id: "AR-002",
    reference: "APR-2026-002",
    action: "Wallet Withdrawal",
    resource: "₦1,200,000 · UBA ••3901",
    requester: { id: "S-002", name: "Oluwaseun Adebayo" },
    status: "Pending",
    urgency: "Urgent",
    approvalsRequired: 2,
    approvalsReceived: 0,
    createdAt: "2026-08-23T08:30:00Z",
    expiresAt: "2026-08-25T08:30:00Z",
    branch: "Ikeja",
    tags: ["Wallet", "Finance"],
  },
  {
    id: "AR-003",
    reference: "APR-2026-003",
    action: "Allocation",
    resource: "P-044 · ₦120,000",
    requester: { id: "S-003", name: "Musa Bello" },
    status: "Pending",
    urgency: "Normal",
    approvalsRequired: 1,
    approvalsReceived: 0,
    createdAt: "2026-08-23T08:00:00Z",
    expiresAt: "2026-08-24T08:00:00Z",
    branch: "Lekki",
    tags: ["Allocation", "Finance"],
  },
  {
    id: "AR-004",
    reference: "APR-2026-004",
    action: "Staff Suspension",
    resource: "S-007 · Kabiru Ahmed",
    requester: { id: "S-001", name: "Favour Daniel" },
    status: "Approved",
    urgency: "High",
    approvalsRequired: 2,
    approvalsReceived: 2,
    createdAt: "2026-08-22T16:00:00Z",
    expiresAt: "2026-08-24T16:00:00Z",
    branch: "Apapa",
    tags: ["Staff", "Suspension"],
  },
  {
    id: "AR-005",
    reference: "APR-2026-005",
    action: "Return to Service",
    resource: "TR-112 · Mercedes Actros",
    requester: { id: "S-003", name: "Musa Bello" },
    status: "Rejected",
    urgency: "Normal",
    approvalsRequired: 1,
    approvalsReceived: 1,
    createdAt: "2026-08-22T14:00:00Z",
    expiresAt: "2026-08-23T14:00:00Z",
    branch: "Apapa",
    tags: ["Fleet", "Maintenance"],
  },
  {
    id: "AR-006",
    reference: "APR-2026-006",
    action: "API Client Access",
    resource: "Production API Key",
    requester: { id: "S-006", name: "Esther Obi" },
    status: "Expired",
    urgency: "Normal",
    approvalsRequired: 2,
    approvalsReceived: 1,
    createdAt: "2026-08-20T10:00:00Z",
    expiresAt: "2026-08-22T10:00:00Z",
    branch: "All branches",
    tags: ["API", "Security"],
  },
  {
    id: "AR-007",
    reference: "APR-2026-007",
    action: "Staff Invitation",
    resource: "C. Adeyemi · daniel@company.com",
    requester: { id: "S-001", name: "Favour Daniel" },
    status: "Executed",
    urgency: "Normal",
    approvalsRequired: 1,
    approvalsReceived: 1,
    createdAt: "2026-08-21T11:00:00Z",
    expiresAt: "2026-08-22T11:00:00Z",
    branch: "Ikeja",
    tags: ["Staff", "Invite"],
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Approved: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-3.5 w-3.5" /> },
  Cancelled: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-3.5 w-3.5" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-3.5 w-3.5" /> },
  Executed: { bg: "bg-blue-bg", text: "text-blue", icon: <Check className="h-3.5 w-3.5" /> },
};

const URGENCY_STYLES: Record<string, { bg: string; text: string }> = {
  Urgent: { bg: "bg-red-bg", text: "text-red" },
  High: { bg: "bg-amber-bg", text: "text-amber" },
  Normal: { bg: "bg-blue-bg", text: "text-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATUS_FILTERS = ["All", "Pending", "Approved", "Rejected", "Cancelled", "Expired", "Executed"];
const URGENCY_FILTERS = ["All", "Urgent", "High", "Normal", "Low"];

export default function ApprovalsInboxPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ApprovalRequest[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedUrgency, setSelectedUrgency] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_APPROVALS);
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
      item.action.toLowerCase().includes(search.toLowerCase()) ||
      item.resource.toLowerCase().includes(search.toLowerCase()) ||
      item.requester.name.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesUrgency = selectedUrgency === "All" || item.urgency === selectedUrgency;

    return matchesSearch && matchesStatus && matchesUrgency;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading approvals...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedUrgency !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedUrgency("All");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0) + (selectedUrgency !== "All" ? 1 : 0);

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

  const formatDate = (date: string | null) => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  const goToRequestDetail = (id: string) => {
    router.push(`/settings/approvals/${id}`);
  };

  // Stats
  const totalPending = data.filter((d) => d.status === "Pending").length;
  const totalUrgent = data.filter((d) => d.urgency === "Urgent").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Approvals Inbox
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Pending, approved, rejected, cancelled, expired and executed requests with requester/action/resource.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            Export
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{totalPending}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Urgent</div>
          <div className="text-lg font-bold text-red">{totalUrgent}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Executed</div>
          <div className="text-lg font-bold text-blue">{data.filter((d) => d.status === "Executed").length}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, action, resource..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "Approved", "Rejected", "Cancelled", "Expired", "Executed"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Inbox className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const urgencies = ["All", "Urgent", "High", "Normal", "Low"];
            const currentIndex = urgencies.indexOf(selectedUrgency);
            const nextIndex = (currentIndex + 1) % urgencies.length;
            setSelectedUrgency(urgencies[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedUrgency}
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

      {/* ===== APPROVALS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Inbox className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No approval requests found</h3>
          <p className="mt-1 text-sm text-ink-500">No requests match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Request</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Action</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Urgency</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Approvals</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Pending;
                  const urgencyStyle = URGENCY_STYLES[item.urgency] || URGENCY_STYLES.Normal;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToRequestDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.resource}</div>
                        <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                        <div className="text-[9px] text-ink-400">by {item.requester.name}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.action}</td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${urgencyStyle.bg} ${urgencyStyle.text}`}>
                          {item.urgency}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">
                        {item.approvalsReceived}/{item.approvalsRequired}
                      </td>
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
          <Inbox className="h-4 w-4" />
          IAM-09 · Approvals Inbox
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} requests shown
        </span>
      </div>
    </div>
  );
}