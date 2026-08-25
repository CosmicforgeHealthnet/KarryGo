// src/app/(dashboard)/comms/escalations/page.tsx

"use client";

export const dynamic = "force-dynamic";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
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
  Check,
  Link2,
  Flag,
  Shield,
  Gauge,
  AlertOctagon as AlertOctagonIcon,
  ArrowUp,
  Settings,
  HelpCircle,
  ClipboardList,
} from "lucide-react";

// ==================== TYPES ====================

type Escalation = {
  id: string;
  reference: string;
  source: string;
  sourceType: "Incident" | "SafetyReport" | "SupportCase" | "QualityCase" | "SLA" | "Compliance" | "System";
  severity: "Critical" | "High" | "Medium" | "Low";
  level: number;
  maxLevel: number;
  title: string;
  description: string;
  recipient: string | null;
  owner: string | null;
  dueTime: string | null;
  status: "Active" | "Acknowledged" | "InProgress" | "Resolved" | "Escalated" | "Closed";
  sla: string | null;
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  branch: string;
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_ESCALATIONS: Escalation[] = [
  {
    id: "ESC-001",
    reference: "ESC-2026-001",
    source: "INC-771",
    sourceType: "Incident",
    severity: "Critical",
    level: 2,
    maxLevel: 3,
    title: "Critical incident escalation - Vehicle collision",
    description: "Provider P-044 involved in collision. Investigation in progress. Multiple departments need coordination.",
    recipient: "Operations Director",
    owner: "F. Daniel",
    dueTime: "2026-08-23T17:00:00Z",
    status: "Active",
    sla: "24 hours",
    createdAt: "2026-08-22T06:30:00Z",
    updatedAt: "2026-08-22T10:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    branch: "Ikeja",
    tags: ["Incident", "Critical", "Coordination"],
    sourceFreshness: "2026-08-22T10:00:00Z",
  },
  {
    id: "ESC-002",
    reference: "ESC-2026-002",
    source: "SR-003",
    sourceType: "SafetyReport",
    severity: "High",
    level: 1,
    maxLevel: 3,
    title: "Safety report escalation - Unsafe vehicle",
    description: "TR-112 safety report requires fleet manager attention. Vehicle out of service.",
    recipient: "M. Bello",
    owner: "F. Daniel",
    dueTime: "2026-08-23T09:00:00Z",
    status: "Acknowledged",
    sla: "12 hours",
    createdAt: "2026-08-19T16:30:00Z",
    updatedAt: "2026-08-20T08:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    branch: "Apapa",
    tags: ["Safety", "Fleet", "Vehicle"],
    sourceFreshness: "2026-08-20T08:00:00Z",
  },
  {
    id: "ESC-003",
    reference: "ESC-2026-003",
    source: "SUP-002",
    sourceType: "SupportCase",
    severity: "High",
    level: 2,
    maxLevel: 3,
    title: "Support escalation - Provider payment delay",
    description: "Provider payment issue escalated to finance. Multiple follow-ups required.",
    recipient: "Finance Manager",
    owner: "F. Daniel",
    dueTime: "2026-08-23T12:00:00Z",
    status: "InProgress",
    sla: "24 hours",
    createdAt: "2026-08-21T14:30:00Z",
    updatedAt: "2026-08-22T08:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    branch: "Ikeja",
    tags: ["Support", "Billing", "Finance"],
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
  {
    id: "ESC-004",
    reference: "ESC-2026-004",
    source: "SLA-001",
    sourceType: "SLA",
    severity: "Medium",
    level: 1,
    maxLevel: 2,
    title: "SLA breach escalation - Charitos Hospital",
    description: "SLA breach identified for Charitos Hospital. Account manager notified.",
    recipient: "O. Adebayo",
    owner: "F. Daniel",
    dueTime: "2026-08-24T10:00:00Z",
    status: "Resolved",
    sla: "48 hours",
    createdAt: "2026-08-22T09:00:00Z",
    updatedAt: "2026-08-22T14:00:00Z",
    resolvedAt: "2026-08-22T14:00:00Z",
    resolvedBy: "O. Adebayo",
    branch: "Ikeja",
    tags: ["SLA", "Customer", "Breach"],
    sourceFreshness: "2026-08-22T14:00:00Z",
  },
  {
    id: "ESC-005",
    reference: "ESC-2026-005",
    source: "QC-004",
    sourceType: "QualityCase",
    severity: "Critical",
    level: 3,
    maxLevel: 3,
    title: "Critical quality escalation - Safety non-conformance",
    description: "Critical safety non-conformance with TR-112. Multiple teams involved. Executive notification required.",
    recipient: "Operations Director",
    owner: "M. Bello",
    dueTime: "2026-08-22T17:00:00Z",
    status: "Escalated",
    sla: "24 hours",
    createdAt: "2026-08-19T16:30:00Z",
    updatedAt: "2026-08-21T14:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    branch: "Apapa",
    tags: ["Quality", "Critical", "Safety"],
    sourceFreshness: "2026-08-21T14:00:00Z",
  },
  {
    id: "ESC-006",
    reference: "ESC-2026-006",
    source: "SYS-001",
    sourceType: "System",
    severity: "Low",
    level: 1,
    maxLevel: 2,
    title: "System alert - Webhook delivery failures",
    description: "Webhook deliveries failing for order updates. Investigation in progress.",
    recipient: "F. Daniel",
    owner: "F. Daniel",
    dueTime: "2026-08-23T08:00:00Z",
    status: "InProgress",
    sla: "48 hours",
    createdAt: "2026-08-21T09:30:00Z",
    updatedAt: "2026-08-22T07:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    branch: "All branches",
    tags: ["System", "Integration", "Webhook"],
    sourceFreshness: "2026-08-22T07:00:00Z",
  },
  {
    id: "ESC-007",
    reference: "ESC-2026-007",
    source: "COM-001",
    sourceType: "Compliance",
    severity: "High",
    level: 2,
    maxLevel: 3,
    title: "Compliance escalation - Document expiry",
    description: "Multiple provider documents expiring. Compliance team needs to action.",
    recipient: "O. Adebayo",
    owner: "F. Daniel",
    dueTime: "2026-08-25T17:00:00Z",
    status: "Active",
    sla: "72 hours",
    createdAt: "2026-08-20T09:00:00Z",
    updatedAt: "2026-08-22T11:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    branch: "Ikeja",
    tags: ["Compliance", "Documents", "Expiry"],
    sourceFreshness: "2026-08-22T11:00:00Z",
  },
  {
    id: "ESC-008",
    reference: "ESC-2026-008",
    source: "CMP-001",
    sourceType: "Incident",
    severity: "Medium",
    level: 1,
    maxLevel: 2,
    title: "Incident escalation - Warehouse slip hazard",
    description: "Slip hazard reported in warehouse. Safety team notified.",
    recipient: "F. Daniel",
    owner: "F. Daniel",
    dueTime: "2026-08-23T14:00:00Z",
    status: "Closed",
    sla: "24 hours",
    createdAt: "2026-08-21T14:30:00Z",
    updatedAt: "2026-08-22T09:00:00Z",
    resolvedAt: "2026-08-22T09:00:00Z",
    resolvedBy: "F. Daniel",
    branch: "Yaba",
    tags: ["Safety", "Warehouse", "Resolved"],
    sourceFreshness: "2026-08-22T09:00:00Z",
  },
];

const SEVERITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagonIcon className="h-3.5 w-3.5" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertOctagonIcon className="h-3.5 w-3.5" /> },
  Acknowledged: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Check className="h-3.5 w-3.5" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  Escalated: { bg: "bg-purple-bg", text: "text-purple", dot: "bg-purple", icon: <ArrowUp className="h-3.5 w-3.5" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
};

const SOURCE_TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Incident: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagonIcon className="h-3.5 w-3.5" /> },
  SafetyReport: { bg: "bg-amber-bg", text: "text-amber", icon: <Shield className="h-3.5 w-3.5" /> },
  SupportCase: { bg: "bg-blue-bg", text: "text-blue", icon: <HelpCircle className="h-3.5 w-3.5" /> },
  QualityCase: { bg: "bg-purple-bg", text: "text-purple", icon: <ClipboardList className="h-3.5 w-3.5" /> },
  SLA: { bg: "bg-green-bg", text: "text-green", icon: <Gauge className="h-3.5 w-3.5" /> },
  Compliance: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-3.5 w-3.5" /> },
  System: { bg: "bg-surface-2", text: "text-ink-500", icon: <Settings className="h-3.5 w-3.5" /> },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function EscalationsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Escalation[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedSource, setSelectedSource] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_ESCALATIONS);
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
      item.title.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      (item.recipient && item.recipient.toLowerCase().includes(search.toLowerCase())) ||
      (item.owner && item.owner.toLowerCase().includes(search.toLowerCase()));

    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesSource = selectedSource === "All" || item.sourceType === selectedSource;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesSeverity && matchesStatus && matchesSource && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading escalations...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedSeverity !== "All" ||
    selectedStatus !== "All" ||
    selectedSource !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedSeverity("All");
    setSelectedStatus("All");
    setSelectedSource("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedSeverity !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedSource !== "All" ? 1 : 0) +
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
  const totalActive = data.filter((d) => d.status === "Active" || d.status === "Acknowledged" || d.status === "InProgress").length;
  const totalCritical = data.filter((d) => d.severity === "Critical").length;
  const totalEscalated = data.filter((d) => d.status === "Escalated").length;

  const goToEscalationDetail = (id: string) => {
    router.push(`/comms/escalations/${id}`);
  };

  const goToCreateEscalation = () => {
    router.push("/comms/escalations/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Escalations
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Source, severity/level, recipient/owner, due time, state and SLA.
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
            onClick={goToCreateEscalation}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Escalation
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Critical</div>
          <div className="text-lg font-bold text-red">{totalCritical}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Escalated</div>
          <div className="text-lg font-bold text-purple">{totalEscalated}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, title, recipient..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const severities = ["All", "Critical", "High", "Medium", "Low"];
            const currentIndex = severities.indexOf(selectedSeverity);
            const nextIndex = (currentIndex + 1) % severities.length;
            setSelectedSeverity(severities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertOctagonIcon className="h-4 w-4 text-ink-400" />
          {selectedSeverity}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Acknowledged", "InProgress", "Escalated", "Resolved", "Closed"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Flag className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const sources = ["All", "Incident", "SafetyReport", "SupportCase", "QualityCase", "SLA", "Compliance", "System"];
            const currentIndex = sources.indexOf(selectedSource);
            const nextIndex = (currentIndex + 1) % sources.length;
            setSelectedSource(sources[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Link2 className="h-4 w-4 text-ink-400" />
          {selectedSource}
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

      {/* ===== ESCALATIONS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <AlertOctagonIcon className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No escalations found</h3>
          <p className="mt-1 text-sm text-ink-500">No escalations match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[1100px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Escalation</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Source</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Severity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Level</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Recipient</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const severityStyle = SEVERITY_STYLES[item.severity] || SEVERITY_STYLES.Medium;
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
                  const sourceStyle = SOURCE_TYPE_STYLES[item.sourceType] || SOURCE_TYPE_STYLES.System;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToEscalationDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.title}</div>
                        <div className="font-mono text-[10px] text-ink-500 mt-0.5">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${sourceStyle.bg} ${sourceStyle.text}`}>
                          {sourceStyle.icon}
                          {item.sourceType}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                          {severityStyle.icon}
                          {item.severity}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className="text-xs font-medium text-ink-950">
                          {item.level}/{item.maxLevel}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">
                        {item.recipient || "—"}
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
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
          <AlertOctagonIcon className="h-4 w-4" />
          COM-09 · Escalations
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} escalations shown
        </span>
      </div>
    </div>
  );
}