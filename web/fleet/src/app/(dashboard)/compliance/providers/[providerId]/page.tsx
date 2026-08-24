// src/app/(dashboard)/compliance/providers/[providerId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Users,
  FileText,
  Plus,
  Building2,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Tag,
  Info,
  FileCheck,
  Calendar,
  XCircle,
  Activity,
  ListChecks,
  ShieldCheck,
  ShieldAlert,
  ShieldQuestion,
  FileSearch,
  ClipboardList,
  BookOpen,
  FileSignature,
  FolderOpen,
  ChevronDown,
  ChevronUp,
  Download,
  Printer,
  ExternalLink,
  MoreHorizontal,
  Edit3,
  Trash2,
  UserCheck,
  UserX,
  AlertOctagon,
  Check,
  X,
} from "lucide-react";

// ==================== TYPES ====================

type ComplianceRequirement = {
  id: string;
  name: string;
  reference: string;
  category: "Document" | "Qualification" | "Training" | "Inspection" | "Certification";
  status: "Compliant" | "Warning" | "NonCompliant" | "Pending" | "Expired";
  dueDate: string | null;
  verified: boolean;
  verificationSource: "Internal" | "External" | "Pending";
  evidenceRef: string | null;
  evidenceStatus: "Uploaded" | "Missing" | "Rejected" | "Pending";
  restrictionImpact: boolean;
  notes: string | null;
};

type ProviderRestriction = {
  id: string;
  type: "Safety" | "Document" | "Training" | "Qualification" | "Compliance";
  severity: "Critical" | "High" | "Medium" | "Low";
  reason: string;
  source: string;
  effectiveDate: string;
  clearanceState: "Active" | "PendingClearance" | "Cleared" | "Expired";
  clearanceAction: string | null;
};

type ProviderComplianceData = {
  providerId: string;
  providerName: string;
  providerType: "Dispatch" | "Hauling" | "Both";
  branch: string;
  serviceRole: string;
  overallStatus: "Compliant" | "Warning" | "NonCompliant" | "Pending";
  complianceScore: number;
  lastAssessment: string;
  requirements: ComplianceRequirement[];
  restrictions: ProviderRestriction[];
  summary: {
    total: number;
    compliant: number;
    warning: number;
    nonCompliant: number;
    pending: number;
    expiringSoon: number;
    expired: number;
  };
  documentCount: number;
  verifiedCount: number;
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_PROVIDER_DATA: Record<string, ProviderComplianceData> = {
  "P-044": {
    providerId: "P-044",
    providerName: "Peter Okafor",
    providerType: "Dispatch",
    branch: "Ikeja",
    serviceRole: "Dispatch Rider",
    overallStatus: "Warning",
    complianceScore: 78,
    lastAssessment: "2026-08-22T10:30:00Z",
    requirements: [
      {
        id: "REQ-001",
        name: "Vehicle Safety Inspection",
        reference: "REQ-2026-001",
        category: "Inspection",
        status: "Compliant",
        dueDate: "2026-09-15",
        verified: true,
        verificationSource: "Internal",
        evidenceRef: "DOC-8812",
        evidenceStatus: "Uploaded",
        restrictionImpact: false,
        notes: null,
      },
      {
        id: "REQ-002",
        name: "Provider Certification",
        reference: "REQ-2026-002",
        category: "Certification",
        status: "Warning",
        dueDate: "2026-09-30",
        verified: true,
        verificationSource: "External",
        evidenceRef: "DOC-7721",
        evidenceStatus: "Uploaded",
        restrictionImpact: false,
        notes: "Certification expires in 38 days. Renewal process started.",
      },
      {
        id: "REQ-003",
        name: "Document Renewal",
        reference: "REQ-2026-003",
        category: "Document",
        status: "NonCompliant",
        dueDate: "2026-08-25",
        verified: false,
        verificationSource: "Pending",
        evidenceRef: null,
        evidenceStatus: "Missing",
        restrictionImpact: true,
        notes: "ID card expired. New document required before 2026-08-25.",
      },
      {
        id: "REQ-004",
        name: "Safety Training",
        reference: "REQ-2026-004",
        category: "Training",
        status: "Compliant",
        dueDate: "2026-10-15",
        verified: true,
        verificationSource: "Internal",
        evidenceRef: "DOC-6643",
        evidenceStatus: "Uploaded",
        restrictionImpact: false,
        notes: null,
      },
      {
        id: "REQ-005",
        name: "Provider Drug Test",
        reference: "REQ-2026-007",
        category: "Qualification",
        status: "Expired",
        dueDate: "2026-07-01",
        verified: false,
        verificationSource: "External",
        evidenceRef: null,
        evidenceStatus: "Pending",
        restrictionImpact: true,
        notes: "Drug test expired. New test required before reassignment.",
      },
      {
        id: "REQ-006",
        name: "Vehicle Registration",
        reference: "REQ-2026-005",
        category: "Document",
        status: "Pending",
        dueDate: "2026-12-31",
        verified: false,
        verificationSource: "Pending",
        evidenceRef: null,
        evidenceStatus: "Pending",
        restrictionImpact: false,
        notes: "Awaiting registration certificate from government agency.",
      },
    ],
    restrictions: [
      {
        id: "RST-044-01",
        type: "Document",
        severity: "Critical",
        reason: "Expired ID card - Provider P-044",
        source: "Compliance Requirements",
        effectiveDate: "2026-08-20T00:00:00Z",
        clearanceState: "Active",
        clearanceAction: "Upload valid ID card for verification",
      },
      {
        id: "RST-044-02",
        type: "Qualification",
        severity: "High",
        reason: "Drug test expired",
        source: "Provider Qualification",
        effectiveDate: "2026-07-02T00:00:00Z",
        clearanceState: "Active",
        clearanceAction: "Complete new drug test and upload results",
      },
    ],
    summary: {
      total: 6,
      compliant: 2,
      warning: 1,
      nonCompliant: 1,
      pending: 1,
      expiringSoon: 1,
      expired: 1,
    },
    documentCount: 4,
    verifiedCount: 3,
    sourceFreshness: "2026-08-22T10:30:00Z",
  },
  "P-052": {
    providerId: "P-052",
    providerName: "Amina Yusuf",
    providerType: "Dispatch",
    branch: "Yaba",
    serviceRole: "Dispatch Rider",
    overallStatus: "NonCompliant",
    complianceScore: 45,
    lastAssessment: "2026-08-20T14:15:00Z",
    requirements: [
      {
        id: "REQ-001",
        name: "Vehicle Safety Inspection",
        reference: "REQ-2026-001",
        category: "Inspection",
        status: "Compliant",
        dueDate: "2026-10-10",
        verified: true,
        verificationSource: "Internal",
        evidenceRef: "DOC-9923",
        evidenceStatus: "Uploaded",
        restrictionImpact: false,
        notes: null,
      },
      {
        id: "REQ-003",
        name: "Document Renewal",
        reference: "REQ-2026-003",
        category: "Document",
        status: "NonCompliant",
        dueDate: "2026-08-10",
        verified: false,
        verificationSource: "Pending",
        evidenceRef: null,
        evidenceStatus: "Missing",
        restrictionImpact: true,
        notes: "Licence expired on 2026-08-10. Provider cannot operate.",
      },
      {
        id: "REQ-004",
        name: "Safety Training",
        reference: "REQ-2026-004",
        category: "Training",
        status: "Expired",
        dueDate: "2026-07-01",
        verified: false,
        verificationSource: "External",
        evidenceRef: null,
        evidenceStatus: "Missing",
        restrictionImpact: true,
        notes: "Annual safety training overdue. Must complete before reinstatement.",
      },
    ],
    restrictions: [
      {
        id: "RST-052-01",
        type: "Document",
        severity: "Critical",
        reason: "Expired licence - Provider P-052",
        source: "Compliance Requirements",
        effectiveDate: "2026-08-10T00:00:00Z",
        clearanceState: "Active",
        clearanceAction: "Upload valid licence for verification",
      },
      {
        id: "RST-052-02",
        type: "Training",
        severity: "Critical",
        reason: "Safety training overdue",
        source: "Training Compliance",
        effectiveDate: "2026-07-01T00:00:00Z",
        clearanceState: "Active",
        clearanceAction: "Complete safety training and upload certificate",
      },
    ],
    summary: {
      total: 3,
      compliant: 1,
      warning: 0,
      nonCompliant: 1,
      pending: 0,
      expiringSoon: 0,
      expired: 2,
    },
    documentCount: 1,
    verifiedCount: 1,
    sourceFreshness: "2026-08-20T14:15:00Z",
  },
};

const REQUIREMENT_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Compliant: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Warning: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
  },
  NonCompliant: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
  Pending: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Expired: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
};

const CATEGORY_STYLES: Record<string, { bg: string; text: string }> = {
  Document: { bg: "bg-blue-bg", text: "text-blue" },
  Qualification: { bg: "bg-purple-bg", text: "text-purple" },
  Training: { bg: "bg-green-bg", text: "text-green" },
  Inspection: { bg: "bg-amber-bg", text: "text-amber" },
  Certification: { bg: "bg-red-bg", text: "text-red" },
};

const RESTRICTION_SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  High: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Medium: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const RESTRICTION_CLEARANCE_STYLES: Record<string, { bg: string; text: string }> = {
  Active: { bg: "bg-red-bg", text: "text-red" },
  PendingClearance: { bg: "bg-amber-bg", text: "text-amber" },
  Cleared: { bg: "bg-green-bg", text: "text-green" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATUS_FILTERS = ["All", "Compliant", "Warning", "NonCompliant", "Pending", "Expired"];
const CATEGORY_FILTERS = ["All", "Document", "Qualification", "Training", "Inspection", "Certification"];

export default function ProviderCompliancePage() {
  const params = useParams();
  const router = useRouter();
  const providerId = params.providerId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ProviderComplianceData | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [statusFilter, setStatusFilter] = useState("All");
  const [categoryFilter, setCategoryFilter] = useState("All");
  const [search, setSearch] = useState("");
  const [expandedRequirements, setExpandedRequirements] = useState<Set<string>>(new Set());
  const [showRestrictions, setShowRestrictions] = useState(true);
  const [showRequirements, setShowRequirements] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      // Use mock data based on providerId, fallback to P-044
      const mockData = MOCK_PROVIDER_DATA[providerId] || MOCK_PROVIDER_DATA["P-044"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [providerId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading provider compliance data...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Shield className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Provider not found</h3>
        <p className="mt-1 text-sm text-ink-500">No compliance data available for this provider.</p>
        <button
          onClick={() => router.push("/compliance")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Compliance
        </button>
      </div>
    );
  }

  const toggleRequirement = (id: string) => {
    const newSet = new Set(expandedRequirements);
    if (newSet.has(id)) {
      newSet.delete(id);
    } else {
      newSet.add(id);
    }
    setExpandedRequirements(newSet);
  };

  const filteredRequirements = data.requirements.filter((req) => {
    const matchesStatus = statusFilter === "All" || req.status === statusFilter;
    const matchesCategory = categoryFilter === "All" || req.category === categoryFilter;
    const matchesSearch =
      search.trim() === "" ||
      req.name.toLowerCase().includes(search.toLowerCase()) ||
      req.reference.toLowerCase().includes(search.toLowerCase());
    return matchesStatus && matchesCategory && matchesSearch;
  });

  const overallStatusStyle = {
    Compliant: { bg: "bg-green-bg", text: "text-green", icon: <ShieldCheck className="h-5 w-5" /> },
    Warning: { bg: "bg-amber-bg", text: "text-amber", icon: <ShieldAlert className="h-5 w-5" /> },
    NonCompliant: { bg: "bg-red-bg", text: "text-red", icon: <ShieldAlert className="h-5 w-5" /> },
    Pending: { bg: "bg-blue-bg", text: "text-blue", icon: <ShieldQuestion className="h-5 w-5" /> },
  }[data.overallStatus] || { bg: "bg-surface-2", text: "text-ink-500", icon: <Shield className="h-5 w-5" /> };

  const formatDate = (date: string | null) => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

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

  const getExpiryStatus = (dueDate: string | null) => {
    if (!dueDate) return { label: "No expiry", class: "text-ink-500" };
    const now = new Date();
    const due = new Date(dueDate);
    const diffDays = Math.ceil((due.getTime() - now.getTime()) / 86400000);

    if (diffDays < 0) return { label: "Expired", class: "text-red" };
    if (diffDays <= 7) return { label: `${diffDays} days`, class: "text-red" };
    if (diffDays <= 30) return { label: `${diffDays} days`, class: "text-amber" };
    return { label: `${diffDays} days`, class: "text-green" };
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/compliance")}
              className="hover:text-primary transition-colors"
            >
              Compliance
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.providerName}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Provider Compliance
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            {data.providerName} · {data.providerType} · {data.branch} branch
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2 flex-shrink-0">
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Printer className="h-3.5 w-3.5" />
            Print
          </button>
        </div>
      </div>

      {/* ===== OVERVIEW CARD ===== */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-4">
        {/* Status */}
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex items-center gap-3">
            <div className={`rounded-full p-2.5 ${overallStatusStyle.bg}`}>
              {overallStatusStyle.icon}
            </div>
            <div>
              <div className="text-sm text-ink-500">Overall Status</div>
              <div className={`text-lg font-semibold ${overallStatusStyle.text}`}>
                {data.overallStatus}
              </div>
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
            <Clock className="h-3.5 w-3.5" />
            <span>Last assessment: {formatTime(data.lastAssessment)}</span>
          </div>
        </div>

        {/* Compliance Score */}
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-ink-500">Compliance Score</div>
              <div className="text-2xl font-bold text-ink-950">{data.complianceScore}%</div>
            </div>
            <div className="relative h-14 w-14">
              <svg className="h-14 w-14 -rotate-90 transform" viewBox="0 0 56 56">
                <circle
                  cx="28"
                  cy="28"
                  r="22"
                  fill="none"
                  stroke="#E5E7EB"
                  strokeWidth="6"
                />
                <circle
                  cx="28"
                  cy="28"
                  r="22"
                  fill="none"
                  stroke={data.complianceScore >= 80 ? "#139a5d" : data.complianceScore >= 60 ? "#d97706" : "#df2f3f"}
                  strokeWidth="6"
                  strokeDasharray={`${data.complianceScore * 1.382} 138.2`}
                  strokeLinecap="round"
                />
              </svg>
              <span className="absolute inset-0 flex items-center justify-center text-[9px] font-bold text-ink-950">
                {data.complianceScore}%
              </span>
            </div>
          </div>
        </div>

        {/* Summary Stats */}
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm col-span-2">
          <div className="text-sm text-ink-500 mb-2">Requirements Summary</div>
          <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
            <div className="text-center">
              <div className="text-lg font-bold text-ink-950">{data.summary.total}</div>
              <div className="text-[10px] text-ink-500">Total</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-green">{data.summary.compliant}</div>
              <div className="text-[10px] text-ink-500">Compliant</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-amber">{data.summary.warning}</div>
              <div className="text-[10px] text-ink-500">Warning</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-red">{data.summary.nonCompliant}</div>
              <div className="text-[10px] text-ink-500">Non-Compliant</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-blue">{data.summary.pending}</div>
              <div className="text-[10px] text-ink-500">Pending</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-red">{data.summary.expired}</div>
              <div className="text-[10px] text-ink-500">Expired</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== RESTRICTIONS SECTION ===== */}
      {data.restrictions.length > 0 && (
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          <button
            onClick={() => setShowRestrictions(!showRestrictions)}
            className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
          >
            <div className="flex items-center gap-3">
              <div className="rounded-full bg-red-bg p-2">
                <ShieldAlert className="h-4 w-4 text-red" />
              </div>
              <div>
                <span className="font-semibold text-ink-950">Active Restrictions</span>
                <span className="ml-2 rounded-full bg-red-bg px-2 py-0.5 text-[10px] font-semibold text-red">
                  {data.restrictions.filter(r => r.clearanceState === "Active").length} active
                </span>
              </div>
            </div>
            {showRestrictions ? (
              <ChevronUp className="h-4 w-4 text-ink-400" />
            ) : (
              <ChevronDown className="h-4 w-4 text-ink-400" />
            )}
          </button>

          {showRestrictions && (
            <div className="border-t border-line p-4">
              <div className="space-y-3">
                {data.restrictions.map((restriction) => {
                  const severityStyle = RESTRICTION_SEVERITY_STYLES[restriction.severity] || RESTRICTION_SEVERITY_STYLES.Medium;
                  const clearanceStyle = RESTRICTION_CLEARANCE_STYLES[restriction.clearanceState] || RESTRICTION_CLEARANCE_STYLES.Active;

                  return (
                    <div
                      key={restriction.id}
                      className="rounded-xl border border-line p-4 hover:bg-surface-2 transition-colors"
                    >
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div className="flex-1 min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                              {restriction.severity}
                            </span>
                            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${clearanceStyle.bg} ${clearanceStyle.text}`}>
                              {restriction.clearanceState}
                            </span>
                            <span className="text-[10px] text-ink-500">{restriction.type}</span>
                          </div>
                          <div className="mt-1 font-medium text-ink-950">{restriction.reason}</div>
                          <div className="mt-0.5 text-xs text-ink-500">
                            Source: {restriction.source} · Effective: {formatDate(restriction.effectiveDate)}
                          </div>
                          {restriction.clearanceAction && (
                            <div className="mt-2 flex items-start gap-1.5 text-xs text-amber">
                              <Info className="h-3.5 w-3.5 flex-shrink-0 mt-0.5" />
                              <span>{restriction.clearanceAction}</span>
                            </div>
                          )}
                        </div>
                        <button className="flex-shrink-0 rounded-lg border border-line px-3 py-1.5 text-[11px] font-medium text-ink-700 hover:bg-white transition-colors">
                          Resolve
                        </button>
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}

      {/* ===== REQUIREMENTS SECTION ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowRequirements(!showRequirements)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-blue-bg p-2">
              <ListChecks className="h-4 w-4 text-blue" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Compliance Requirements</span>
              <span className="ml-2 text-sm text-ink-500">
                {filteredRequirements.length} of {data.requirements.length}
              </span>
            </div>
          </div>
          {showRequirements ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showRequirements && (
          <div className="border-t border-line">
            {/* Filters */}
            <div className="flex flex-wrap items-center gap-2 p-4 border-b border-line">
              <div className="flex flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-3.5 py-2 shadow-sm">
                <Search className="h-4 w-4 text-ink-400" />
                <input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search requirements..."
                  className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
                />
              </div>

              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setStatusFilter(status)}
                    className={`rounded-full px-3 py-1 text-[10px] font-semibold transition ${
                      statusFilter === status
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>

              <div className="flex flex-wrap gap-1.5">
                {CATEGORY_FILTERS.map((category) => (
                  <button
                    key={category}
                    onClick={() => setCategoryFilter(category)}
                    className={`rounded-full px-3 py-1 text-[10px] font-semibold transition ${
                      categoryFilter === category
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {category}
                  </button>
                ))}
              </div>

              {(statusFilter !== "All" || categoryFilter !== "All" || search !== "") && (
                <button
                  onClick={() => {
                    setStatusFilter("All");
                    setCategoryFilter("All");
                    setSearch("");
                  }}
                  className="rounded-lg border border-line px-3 py-1 text-[10px] font-medium text-ink-600 hover:bg-white transition-colors"
                >
                  Clear all
                </button>
              )}
            </div>

            {/* Requirement List */}
            {filteredRequirements.length === 0 ? (
              <div className="py-12 text-center">
                <div className="inline-block rounded-full bg-surface-2 p-4">
                  <ListChecks className="h-6 w-6 text-ink-400" />
                </div>
                <p className="mt-3 text-sm text-ink-500">No requirements match your filters.</p>
              </div>
            ) : (
              <div className="divide-y divide-line">
                {filteredRequirements.map((req) => {
                  const statusStyle = REQUIREMENT_STATUS_STYLES[req.status] || REQUIREMENT_STATUS_STYLES.Pending;
                  const categoryStyle = CATEGORY_STYLES[req.category] || CATEGORY_STYLES.Document;
                  const expiryStatus = getExpiryStatus(req.dueDate);
                  const isExpanded = expandedRequirements.has(req.id);

                  return (
                    <div key={req.id} className="transition-colors hover:bg-surface-2/50">
                      <button
                        onClick={() => toggleRequirement(req.id)}
                        className="flex w-full items-center gap-3 p-4 text-left"
                      >
                        <div className="flex-shrink-0">
                          {statusStyle.icon}
                        </div>

                        <div className="flex-1 min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-medium text-ink-950">{req.name}</span>
                            <span className="text-[10px] text-ink-500 font-mono">{req.reference}</span>
                            <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                              {req.category}
                            </span>
                          </div>
                          <div className="mt-0.5 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                            <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                              {req.status}
                            </span>
                            {req.dueDate && (
                              <span className={`flex items-center gap-1 ${expiryStatus.class}`}>
                                <Calendar className="h-3 w-3" />
                                Due: {formatDate(req.dueDate)} ({expiryStatus.label})
                              </span>
                            )}
                            {req.verified && (
                              <span className="flex items-center gap-1 text-green">
                                <CheckCircle2 className="h-3 w-3" />
                                Verified
                              </span>
                            )}
                            {req.restrictionImpact && (
                              <span className="flex items-center gap-1 text-red">
                                <AlertCircle className="h-3 w-3" />
                                Blocks work
                              </span>
                            )}
                          </div>
                        </div>

                        <div className="flex-shrink-0">
                          <button className="rounded-lg border border-line px-3 py-1 text-[10px] font-medium text-ink-700 hover:bg-white transition-colors">
                            {isExpanded ? "Collapse" : "Expand"}
                          </button>
                        </div>
                      </button>

                      {isExpanded && (
                        <div className="border-t border-line px-4 pb-4 pt-3 bg-surface-2/30">
                          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                            <div>
                              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Verification</div>
                              <div className="mt-1 text-sm text-ink-950">
                                {req.verified ? (
                                  <span className="flex items-center gap-1 text-green">
                                    <CheckCircle2 className="h-4 w-4" />
                                    Verified by {req.verificationSource}
                                  </span>
                                ) : (
                                  <span className="flex items-center gap-1 text-amber">
                                    <Clock className="h-4 w-4" />
                                    {req.verificationSource}
                                  </span>
                                )}
                              </div>
                            </div>

                            <div>
                              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Evidence</div>
                              <div className="mt-1 text-sm text-ink-950">
                                {req.evidenceRef ? (
                                  <button className="flex items-center gap-1 text-primary hover:underline">
                                    <FileText className="h-4 w-4" />
                                    {req.evidenceRef}
                                    <ExternalLink className="h-3 w-3" />
                                  </button>
                                ) : (
                                  <span className="text-ink-400">No evidence attached</span>
                                )}
                              </div>
                              {req.evidenceStatus && req.evidenceStatus !== "Uploaded" && (
                                <div className="mt-1 text-xs text-ink-500">
                                  Status: {req.evidenceStatus}
                                </div>
                              )}
                            </div>

                            <div>
                              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Notes</div>
                              <div className="mt-1 text-sm text-ink-950">
                                {req.notes || <span className="text-ink-400">No notes</span>}
                              </div>
                            </div>
                          </div>

                          <div className="mt-3 flex flex-wrap gap-2">
                            {req.restrictionImpact && (
                              <button className="rounded-lg border border-red/20 bg-red-bg px-3 py-1.5 text-[11px] font-medium text-red transition hover:bg-red/10">
                                <ShieldAlert className="inline h-3.5 w-3.5 mr-1" />
                                Resolve Block
                              </button>
                            )}
                            {!req.verified && (
                              <button className="rounded-lg border border-primary/20 bg-primary-bg px-3 py-1.5 text-[11px] font-medium text-primary transition hover:bg-primary/10">
                                <CheckCircle2 className="inline h-3.5 w-3.5 mr-1" />
                                Verify Now
                              </button>
                            )}
                            {!req.evidenceRef && (
                              <button className="rounded-lg border border-line px-3 py-1.5 text-[11px] font-medium text-ink-700 hover:bg-white transition-colors">
                                <FileText className="inline h-3.5 w-3.5 mr-1" />
                                Upload Evidence
                              </button>
                            )}
                            <button className="rounded-lg border border-line px-3 py-1.5 text-[11px] font-medium text-ink-700 hover:bg-white transition-colors">
                              <Edit3 className="inline h-3.5 w-3.5 mr-1" />
                              Edit
                            </button>
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          CMP-03 · Provider Compliance · {data.providerId}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.sourceFreshness ? `Source: ${formatTime(data.sourceFreshness)}` : ""}
        </span>
      </div>
    </div>
  );
}