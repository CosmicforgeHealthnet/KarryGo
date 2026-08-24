// src/app/(dashboard)/documents/reconciliation/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  FileText,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Search,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  Users,
  Building2,
  Tag,
  Calendar,
  Info,
  ListChecks,
  Download,
  Printer,
  Edit3,
  Trash2,
  Check,
  MoreHorizontal,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Send,
  Paperclip,
  Link2,
  ExternalLink,
  File,
  FileArchive,
  FileImage,
  FileJson,
  FilePdf,
  FileSpreadsheet,
  FileText as FileTextIcon,
  FolderOpen,
  Shield,
  Lock,
  EyeOff,
  Clock as ClockIcon,
  CheckCircle,
  XCircle,
  AlertOctagon,
  FileCheck,
  FileSignature,
  Folder,
  HardDrive,
  Database,
  Cloud,
  History,
  GitBranch,
  FileUser,
  FileKey,
  FileClock,
  FileX,
  FileMinus,
  FilePlus,
  FileCog,
  FileCheck2,
  RefreshCcw,
  RefreshCw as RefreshCwIcon,
  GitPullRequest,
  GitCommit,
  GitBranch as GitBranchIcon,
  GitMerge,
  GitPullRequestClosed,
  AlertTriangle as AlertTriangleIcon,
  CheckCircle as CheckCircleIcon,
  XCircle as XCircleIcon,
  Info as InfoIcon,
} from "lucide-react";

// ==================== TYPES ====================

type ReconciliationCase = {
  id: string;
  reference: string;
  documentId: string;
  documentReference: string;
  documentName: string;
  authority: string;
  discrepancyType: "Missing" | "Mismatch" | "Stale" | "Corrupted" | "VerificationFailed";
  description: string;
  state: "Open" | "Investigating" | "PendingResolution" | "Resolved" | "Closed" | "Failed";
  severity: "Critical" | "High" | "Medium" | "Low";
  attempts: number;
  lastAttemptAt: string | null;
  lastAttemptStatus: "Success" | "Failed" | "Pending" | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  resolution: string | null;
  sourceFreshness: string;
  branch: string;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_RECONCILIATION_CASES: ReconciliationCase[] = [
  {
    id: "RC-001",
    reference: "RC-2026-001",
    documentId: "DOC-002",
    documentReference: "DOC-2026-002",
    documentName: "Provider ID - P-044",
    authority: "Verification Service",
    discrepancyType: "Stale",
    description: "Document metadata mismatch between local and authority records. Version dates do not align.",
    state: "Open",
    severity: "High",
    attempts: 2,
    lastAttemptAt: "2026-08-21T10:00:00Z",
    lastAttemptStatus: "Failed",
    createdBy: "System",
    createdAt: "2026-08-20T09:00:00Z",
    updatedAt: "2026-08-21T10:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    sourceFreshness: "2026-08-21T10:00:00Z",
    branch: "Ikeja",
    tags: ["Verification", "Stale", "Authority"],
  },
  {
    id: "RC-002",
    reference: "RC-2026-002",
    documentId: "DOC-005",
    documentReference: "DOC-2026-005",
    documentName: "Driver License - P-044",
    authority: "Document Service",
    discrepancyType: "Mismatch",
    description: "Document checksum mismatch. Local file differs from authoritative source.",
    state: "Investigating",
    severity: "High",
    attempts: 1,
    lastAttemptAt: "2026-08-22T08:30:00Z",
    lastAttemptStatus: "Pending",
    createdBy: "System",
    createdAt: "2026-08-22T08:00:00Z",
    updatedAt: "2026-08-22T08:30:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    sourceFreshness: "2026-08-22T08:30:00Z",
    branch: "Ikeja",
    tags: ["Mismatch", "Checksum", "Authority"],
  },
  {
    id: "RC-003",
    reference: "RC-2026-003",
    documentId: "DOC-007",
    documentReference: "DOC-2026-007",
    documentName: "Financial Report Q3 2026",
    authority: "Media Service",
    discrepancyType: "Corrupted",
    description: "Media file appears corrupted. Content hash does not match expected value.",
    state: "PendingResolution",
    severity: "Medium",
    attempts: 3,
    lastAttemptAt: "2026-08-22T09:00:00Z",
    lastAttemptStatus: "Failed",
    createdBy: "System",
    createdAt: "2026-08-22T07:00:00Z",
    updatedAt: "2026-08-22T09:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    sourceFreshness: "2026-08-22T09:00:00Z",
    branch: "All branches",
    tags: ["Corrupted", "Media", "Hash"],
  },
  {
    id: "RC-004",
    reference: "RC-2026-004",
    documentId: "DOC-001",
    documentReference: "DOC-2026-001",
    documentName: "Vehicle Registration - BR-204",
    authority: "Government Database",
    discrepancyType: "Missing",
    description: "Document not found in authoritative government database. May require re-upload.",
    state: "Resolved",
    severity: "High",
    attempts: 4,
    lastAttemptAt: "2026-08-16T09:00:00Z",
    lastAttemptStatus: "Success",
    createdBy: "System",
    createdAt: "2026-08-14T10:00:00Z",
    updatedAt: "2026-08-16T09:00:00Z",
    resolvedAt: "2026-08-16T09:00:00Z",
    resolvedBy: "F. Daniel",
    resolution: "Document re-uploaded and verified against government database.",
    sourceFreshness: "2026-08-16T09:00:00Z",
    branch: "Ikeja",
    tags: ["Missing", "Government", "Resolved"],
  },
  {
    id: "RC-005",
    reference: "RC-2026-005",
    documentId: "DOC-009",
    documentReference: "DOC-2026-009",
    documentName: "W9 Form - Provider P-071",
    authority: "Tax Service",
    discrepancyType: "VerificationFailed",
    description: "External verification failed. Tax ID validation returned error.",
    state: "Open",
    severity: "Critical",
    attempts: 2,
    lastAttemptAt: "2026-08-22T15:30:00Z",
    lastAttemptStatus: "Failed",
    createdBy: "System",
    createdAt: "2026-08-22T15:00:00Z",
    updatedAt: "2026-08-22T15:30:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    sourceFreshness: "2026-08-22T15:30:00Z",
    branch: "All branches",
    tags: ["Verification", "Tax", "Critical"],
  },
  {
    id: "RC-006",
    reference: "RC-2026-006",
    documentId: "DOC-003",
    documentReference: "DOC-2026-003",
    documentName: "Insurance Certificate - VN-041",
    authority: "Insurance Service",
    discrepancyType: "Stale",
    description: "Insurance policy status may be stale. Last update from authority was 5 days ago.",
    state: "Closed",
    severity: "Low",
    attempts: 1,
    lastAttemptAt: "2026-08-01T14:00:00Z",
    lastAttemptStatus: "Success",
    createdBy: "System",
    createdAt: "2026-08-01T10:00:00Z",
    updatedAt: "2026-08-01T14:00:00Z",
    resolvedAt: "2026-08-01T14:00:00Z",
    resolvedBy: "System",
    resolution: "Authority confirmed policy is active. Stale indicator was false positive.",
    sourceFreshness: "2026-08-01T14:00:00Z",
    branch: "Lekki",
    tags: ["Stale", "Insurance", "Closed"],
  },
  {
    id: "RC-007",
    reference: "RC-2026-007",
    documentId: "DOC-008",
    documentReference: "DOC-2026-008",
    documentName: "Incident Report - INC-771",
    authority: "Compliance Service",
    discrepancyType: "Mismatch",
    description: "Compliance status mismatch. Local and authority states differ.",
    state: "Investigating",
    severity: "Medium",
    attempts: 2,
    lastAttemptAt: "2026-08-22T12:00:00Z",
    lastAttemptStatus: "Pending",
    createdBy: "System",
    createdAt: "2026-08-22T11:00:00Z",
    updatedAt: "2026-08-22T12:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    sourceFreshness: "2026-08-22T12:00:00Z",
    branch: "Ikeja",
    tags: ["Mismatch", "Compliance", "Pending"],
  },
];

const DISCREPANCY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Missing: { bg: "bg-red-bg", text: "text-red", icon: <FileX className="h-3.5 w-3.5" /> },
  Mismatch: { bg: "bg-amber-bg", text: "text-amber", icon: <GitPullRequest className="h-3.5 w-3.5" /> },
  Stale: { bg: "bg-blue-bg", text: "text-blue", icon: <ClockIcon className="h-3.5 w-3.5" /> },
  Corrupted: { bg: "bg-purple-bg", text: "text-purple", icon: <AlertTriangleIcon className="h-3.5 w-3.5" /> },
  VerificationFailed: { bg: "bg-red-bg", text: "text-red", icon: <XCircleIcon className="h-3.5 w-3.5" /> },
};

const STATE_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Investigating: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <FileSearch className="h-3.5 w-3.5" /> },
  PendingResolution: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <ClockIcon className="h-3.5 w-3.5" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <CheckCircle className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <XCircle className="h-3.5 w-3.5" /> },
};

const SEVERITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-3.5 w-3.5" /> },
};

const DISCREPANCY_FILTERS = ["All", "Missing", "Mismatch", "Stale", "Corrupted", "VerificationFailed"];
const STATE_FILTERS = ["All", "Open", "Investigating", "PendingResolution", "Resolved", "Closed", "Failed"];
const SEVERITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];

export default function DocumentReconciliationPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ReconciliationCase[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedDiscrepancy, setSelectedDiscrepancy] = useState("All");
  const [selectedState, setSelectedState] = useState("All");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_RECONCILIATION_CASES);
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
      item.documentName.toLowerCase().includes(search.toLowerCase()) ||
      item.documentReference.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase());

    const matchesDiscrepancy = selectedDiscrepancy === "All" || item.discrepancyType === selectedDiscrepancy;
    const matchesState = selectedState === "All" || item.state === selectedState;
    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;

    return matchesSearch && matchesDiscrepancy && matchesState && matchesSeverity;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading reconciliation cases...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedDiscrepancy !== "All" ||
    selectedState !== "All" ||
    selectedSeverity !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedDiscrepancy("All");
    setSelectedState("All");
    setSelectedSeverity("All");
  };

  const filterCount =
    (selectedDiscrepancy !== "All" ? 1 : 0) +
    (selectedState !== "All" ? 1 : 0) +
    (selectedSeverity !== "All" ? 1 : 0);

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
  const totalOpen = data.filter((d) => d.state === "Open" || d.state === "Investigating").length;
  const totalCritical = data.filter((d) => d.severity === "Critical").length;
  const totalResolved = data.filter((d) => d.state === "Resolved" || d.state === "Closed").length;
  const totalFailed = data.filter((d) => d.state === "Failed").length;

  const goToCaseDetail = (id: string) => {
    router.push(`/documents/reconciliation/${id}`);
  };

  const goToReconcile = () => {
    toast("Reconciliation requested (mock)");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Document Reconciliation Cases
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Document/source mismatches, safe metadata, state and reconciliation attempts.
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
            onClick={goToReconcile}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <RefreshCcw className="h-3.5 w-3.5" />
            Reconcile All
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Open</div>
          <div className="text-lg font-bold text-red">{totalOpen}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Critical</div>
          <div className="text-lg font-bold text-red">{totalCritical}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Resolved</div>
          <div className="text-lg font-bold text-green">{totalResolved}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, document, description..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Missing", "Mismatch", "Stale", "Corrupted", "VerificationFailed"];
            const currentIndex = types.indexOf(selectedDiscrepancy);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedDiscrepancy(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <GitPullRequest className="h-4 w-4 text-ink-400" />
          {selectedDiscrepancy}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const states = ["All", "Open", "Investigating", "PendingResolution", "Resolved", "Closed", "Failed"];
            const currentIndex = states.indexOf(selectedState);
            const nextIndex = (currentIndex + 1) % states.length;
            setSelectedState(states[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <GitBranchIcon className="h-4 w-4 text-ink-400" />
          {selectedState}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const severities = ["All", "Critical", "High", "Medium", "Low"];
            const currentIndex = severities.indexOf(selectedSeverity);
            const nextIndex = (currentIndex + 1) % severities.length;
            setSelectedSeverity(severities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedSeverity}
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

      {/* ===== CASES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <GitCommit className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No reconciliation cases found</h3>
          <p className="mt-1 text-sm text-ink-500">No cases match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Case</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Discrepancy</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">State</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Severity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Authority</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Attempts</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const discrepancyStyle = DISCREPANCY_STYLES[item.discrepancyType] || DISCREPANCY_STYLES.Mismatch;
                  const stateStyle = STATE_STYLES[item.state] || STATE_STYLES.Open;
                  const severityStyle = SEVERITY_STYLES[item.severity] || SEVERITY_STYLES.Medium;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToCaseDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.documentName}</div>
                        <div className="flex items-center gap-2 mt-0.5">
                          <span className="font-mono text-[10px] text-ink-500">{item.reference}</span>
                          <span className="text-[9px] text-ink-400 font-mono">{item.documentReference}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${discrepancyStyle.bg} ${discrepancyStyle.text}`}>
                          {discrepancyStyle.icon}
                          {item.discrepancyType}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${stateStyle.bg} ${stateStyle.text}`}>
                          {stateStyle.icon}
                          {item.state}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                          {severityStyle.icon}
                          {item.severity}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.authority}</td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.attempts}</td>
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
          <GitCommit className="h-4 w-4" />
          DAT-06 · Document Reconciliation Cases
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} cases shown
        </span>
      </div>
    </div>
  );
}