// src/app/(dashboard)/comms/threads/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  MessageSquare,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  Users,
  Tag,
  Info,
  Download,
  MoreHorizontal,
  User,
  Link2,
  Lock,
  Shield,
} from "lucide-react";

// ==================== TYPES ====================

type CommunicationThread = {
  id: string;
  reference: string;
  subject: string;
  participants: {
    id: string;
    name: string;
    role: string;
    avatar?: string;
  }[];
  visibility: "Internal" | "External" | "Restricted";
  unreadCount: number;
  lastActivity: string;
  lastMessage: string;
  lastMessageBy: string;
  branch: string;
  relatedTo: {
    type: "Customer" | "Provider" | "Order" | "Trip" | "Incident" | "Support";
    reference: string;
    id: string;
  } | null;
  status: "Active" | "Archived" | "Closed";
  createdAt: string;
  updatedAt: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_THREADS: CommunicationThread[] = [
  {
    id: "THR-001",
    reference: "THR-2026-001",
    subject: "Delivery issue - KG-8491",
    participants: [
      { id: "P-044", name: "Peter Okafor", role: "Provider" },
      { id: "C-001", name: "Adeyemi Stores", role: "Customer" },
      { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
    ],
    visibility: "External",
    unreadCount: 3,
    lastActivity: "2026-08-22T14:30:00Z",
    lastMessage: "I'll reschedule for tomorrow morning.",
    lastMessageBy: "F. Daniel",
    branch: "Ikeja",
    relatedTo: {
      type: "Trip",
      reference: "KG-8491",
      id: "KG-8491",
    },
    status: "Active",
    createdAt: "2026-08-22T10:15:00Z",
    updatedAt: "2026-08-22T14:30:00Z",
    tags: ["Delivery", "Customer", "Urgent"],
  },
  {
    id: "THR-002",
    reference: "THR-2026-002",
    subject: "Vehicle maintenance request - BR-204",
    participants: [
      { id: "P-044", name: "Peter Okafor", role: "Provider" },
      { id: "S-002", name: "M. Bello", role: "Fleet Manager" },
    ],
    visibility: "Internal",
    unreadCount: 0,
    lastActivity: "2026-08-22T11:45:00Z",
    lastMessage: "Work order created. Scheduled for tomorrow.",
    lastMessageBy: "M. Bello",
    branch: "Ikeja",
    relatedTo: {
      type: "Incident",
      reference: "INC-771",
      id: "INC-771",
    },
    status: "Active",
    createdAt: "2026-08-22T08:30:00Z",
    updatedAt: "2026-08-22T11:45:00Z",
    tags: ["Maintenance", "Vehicle"],
  },
  {
    id: "THR-003",
    reference: "THR-2026-003",
    subject: "SLA breach notification - Oakfield Stores",
    participants: [
      { id: "C-002", name: "Oakfield Stores", role: "Customer" },
      { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
      { id: "S-003", name: "O. Adebayo", role: "Account Manager" },
    ],
    visibility: "External",
    unreadCount: 1,
    lastActivity: "2026-08-22T09:20:00Z",
    lastMessage: "We apologize for the delay. Compensation has been processed.",
    lastMessageBy: "O. Adebayo",
    branch: "Lekki",
    relatedTo: {
      type: "Order",
      reference: "ORD-2026-884",
      id: "ORD-884",
    },
    status: "Active",
    createdAt: "2026-08-21T16:00:00Z",
    updatedAt: "2026-08-22T09:20:00Z",
    tags: ["SLA", "Customer", "Compensation"],
  },
  {
    id: "THR-004",
    reference: "THR-2026-004",
    subject: "Incident investigation - INC-777",
    participants: [
      { id: "P-071", name: "Kabiru Ahmed", role: "Provider" },
      { id: "S-002", name: "M. Bello", role: "Fleet Manager" },
      { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
    ],
    visibility: "Internal",
    unreadCount: 0,
    lastActivity: "2026-08-21T11:00:00Z",
    lastMessage: "Incident closed. Vehicle cleared for operation.",
    lastMessageBy: "M. Bello",
    branch: "Apapa",
    relatedTo: {
      type: "Incident",
      reference: "INC-777",
      id: "INC-777",
    },
    status: "Closed",
    createdAt: "2026-08-19T16:30:00Z",
    updatedAt: "2026-08-21T11:00:00Z",
    tags: ["Incident", "Investigation"],
  },
  {
    id: "THR-005",
    reference: "THR-2026-005",
    subject: "New service request - Charitos Hospital",
    participants: [
      { id: "C-003", name: "Charitos Hospital", role: "Customer" },
      { id: "S-003", name: "O. Adebayo", role: "Account Manager" },
    ],
    visibility: "External",
    unreadCount: 2,
    lastActivity: "2026-08-22T13:00:00Z",
    lastMessage: "We'll send a quotation by end of day.",
    lastMessageBy: "O. Adebayo",
    branch: "Yaba",
    relatedTo: {
      type: "Support",
      reference: "SUP-2026-442",
      id: "SUP-442",
    },
    status: "Active",
    createdAt: "2026-08-22T09:00:00Z",
    updatedAt: "2026-08-22T13:00:00Z",
    tags: ["Quotation", "New Business"],
  },
  {
    id: "THR-006",
    reference: "THR-2026-006",
    subject: "Provider feedback - Dispatch performance",
    participants: [
      { id: "P-044", name: "Peter Okafor", role: "Provider" },
      { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
    ],
    visibility: "Internal",
    unreadCount: 0,
    lastActivity: "2026-08-20T15:30:00Z",
    lastMessage: "Feedback noted. Will review in monthly meeting.",
    lastMessageBy: "F. Daniel",
    branch: "Ikeja",
    relatedTo: null,
    status: "Archived",
    createdAt: "2026-08-18T11:00:00Z",
    updatedAt: "2026-08-20T15:30:00Z",
    tags: ["Feedback", "Performance"],
  },
];

const VISIBILITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Internal: { bg: "bg-purple-bg", text: "text-purple", icon: <Lock className="h-3.5 w-3.5" /> },
  External: { bg: "bg-blue-bg", text: "text-blue", icon: <Users className="h-3.5 w-3.5" /> },
  Restricted: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-3.5 w-3.5" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Archived: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const VISIBILITY_FILTERS = ["All", "Internal", "External", "Restricted"];
const STATUS_FILTERS = ["All", "Active", "Archived", "Closed"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function CommunicationThreadsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<CommunicationThread[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedVisibility, setSelectedVisibility] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_THREADS);
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
      item.subject.toLowerCase().includes(search.toLowerCase()) ||
      item.lastMessage.toLowerCase().includes(search.toLowerCase()) ||
      item.participants.some(p => p.name.toLowerCase().includes(search.toLowerCase()));

    const matchesVisibility = selectedVisibility === "All" || item.visibility === selectedVisibility;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesVisibility && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading communication threads...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedVisibility !== "All" ||
    selectedStatus !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedVisibility("All");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedVisibility !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

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

  // Stats
  const totalActive = data.filter((d) => d.status === "Active").length;
  const totalUnread = data.reduce((sum, d) => sum + d.unreadCount, 0);
  const totalInternal = data.filter((d) => d.visibility === "Internal").length;
  const totalExternal = data.filter((d) => d.visibility === "External").length;

  const goToThread = (id: string) => {
    router.push(`/comms/threads/${id}`);
  };

  const goToCreateThread = () => {
    router.push("/comms/threads/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Communication Threads
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Subject, participants, visibility, unread/activity state and branch.
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
          <button
            onClick={goToCreateThread}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            New Thread
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Unread</div>
          <div className="text-lg font-bold text-red">{totalUnread}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">External</div>
          <div className="text-lg font-bold text-blue">{totalExternal}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by subject, reference, participant..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Internal", "External", "Restricted"];
            const currentIndex = types.indexOf(selectedVisibility);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedVisibility(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Users className="h-4 w-4 text-ink-400" />
          {selectedVisibility}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Archived", "Closed"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <MessageSquare className="h-4 w-4 text-ink-400" />
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

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
              <div className="flex gap-1.5 flex-wrap">
                {BRANCHES.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedBranch === branch
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
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
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== THREADS LIST ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <MessageSquare className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No threads found</h3>
          <p className="mt-1 text-sm text-ink-500">No communication threads match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          {filteredData.map((item) => {
            const visibilityStyle = VISIBILITY_STYLES[item.visibility] || VISIBILITY_STYLES.External;
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;

            return (
              <div
                key={item.id}
                onClick={() => goToThread(item.id)}
                className="rounded-2xl border border-line bg-white p-4 shadow-sm hover:shadow-md transition-all cursor-pointer group"
              >
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-ink-950">{item.subject}</span>
                      <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${visibilityStyle.bg} ${visibilityStyle.text}`}>
                        {visibilityStyle.icon}
                        {item.visibility}
                      </span>
                      <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                        {item.status}
                      </span>
                      {item.unreadCount > 0 && (
                        <span className="rounded-full bg-red-bg px-2 py-0.5 text-[9px] font-semibold text-red">
                          {item.unreadCount} unread
                        </span>
                      )}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-3 text-sm text-ink-600">
                      <span className="text-xs truncate max-w-[300px]">
                        <span className="text-ink-400">Last: </span>
                        {item.lastMessage}
                      </span>
                      <span className="text-xs text-ink-400">— {item.lastMessageBy}</span>
                    </div>
                    <div className="mt-1.5 flex flex-wrap items-center gap-2 text-xs text-ink-500">
                      <span className="font-mono text-[10px]">{item.reference}</span>
                      <span>·</span>
                      <span>{item.branch}</span>
                      {item.relatedTo && (
                        <>
                          <span>·</span>
                          <span className="flex items-center gap-1 text-blue">
                            <Link2 className="h-3 w-3" />
                            {item.relatedTo.reference}
                          </span>
                        </>
                      )}
                      {item.tags.map((tag) => (
                        <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                          {tag}
                        </span>
                      ))}
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-1.5">
                      {item.participants.map((p, idx) => (
                        <span key={p.id} className="flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[9px] text-ink-600">
                          <div className="h-4 w-4 rounded-full bg-primary-bg flex items-center justify-center text-[8px] font-bold text-primary">
                            {p.name.charAt(0)}
                          </div>
                          {p.name}
                          {idx < item.participants.length - 1 && ","}
                        </span>
                      ))}
                    </div>
                  </div>
                  <div className="flex flex-col items-end gap-1 flex-shrink-0">
                    <span className="text-xs text-ink-400">{formatTime(item.lastActivity)}</span>
                    <button className="text-ink-400 group-hover:text-primary transition-colors">
                      <Eye className="h-4 w-4" />
                    </button>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <MessageSquare className="h-4 w-4" />
          COM-01 · Communication Threads
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} threads shown
        </span>
      </div>
    </div>
  );
}