// src/app/(dashboard)/comms/quality/[caseId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  User,
  Info,
  XCircle,
  ClipboardList,
  ChevronDown,
  ChevronUp,
  Download,
  ExternalLink,
  MoreHorizontal,
  Edit3,
  Check,
  Flag,
  UserCog,
  MessageSquare,
  Paperclip,
  AlertOctagon,
  Shield,
  Plus,
  FileText,
  ClipboardCheck,
} from "lucide-react";

// ==================== TYPES ====================

type QualityCaseDetail = {
  id: string;
  reference: string;
  title: string;
  description: string;
  category: "Complaint" | "NonConformance" | "ProcessFailure" | "ServiceFailure" | "CustomerExperience" | "Safety" | "Other";
  severity: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "Investigating" | "UnderReview" | "Resolved" | "Closed";
  owner: string | null;
  priority: "Urgent" | "High" | "Normal" | "Low";
  rootCauseStatus: "Unknown" | "Identified" | "Confirmed" | "Resolved";
  rootCause: string | null;
  relatedOperation: {
    type: "Trip" | "Order" | "Customer" | "Provider" | "Asset" | "Other";
    reference: string | null;
    id: string | null;
  } | null;
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  resolution: string | null;
  correctiveActions: {
    id: string;
    description: string;
    status: "Pending" | "InProgress" | "Completed" | "Verified";
    assignedTo: string | null;
    dueDate: string | null;
    completedAt: string | null;
    verifiedBy: string | null;
    verifiedAt: string | null;
  }[];
  investigation: {
    status: "NotStarted" | "InProgress" | "Completed";
    lead: string | null;
    startedAt: string | null;
    completedAt: string | null;
    findings: string | null;
    recommendations: string[];
  } | null;
  evidence: {
    id: string;
    name: string;
    type: "Image" | "Document" | "Video" | "Audio";
    url: string;
    uploadedAt: string;
    uploadedBy: string;
  }[];
  timeline: {
    id: string;
    type: "Created" | "StatusChange" | "Assignment" | "Note" | "Evidence" | "Resolution" | "RootCause";
    description: string;
    createdBy: string;
    createdAt: string;
  }[];
  tags: string[];
  branch: string;
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_QUALITY_CASE_DATA: Record<string, QualityCaseDetail> = {
  "QC-001": {
    id: "QC-001",
    reference: "QC-2026-001",
    title: "Delivery quality complaint - Damaged goods",
    description: "Customer reported damaged goods from delivery KG-8491. The package arrived with visible damage and contents were broken. Customer has provided photos and is requesting compensation.",
    category: "Complaint",
    severity: "High",
    status: "Investigating",
    owner: "F. Daniel",
    priority: "High",
    rootCauseStatus: "Identified",
    rootCause: "Improper packaging during loading. Items were not secured properly in the vehicle.",
    relatedOperation: {
      type: "Trip",
      reference: "KG-8491",
      id: "KG-8491",
    },
    createdAt: "2026-08-22T11:00:00Z",
    updatedAt: "2026-08-22T14:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    correctiveActions: [
      {
        id: "CA-009",
        description: "Review packaging procedures with loading team",
        status: "InProgress",
        assignedTo: "F. Daniel",
        dueDate: "2026-08-25T17:00:00Z",
        completedAt: null,
        verifiedBy: null,
        verifiedAt: null,
      },
      {
        id: "CA-010",
        description: "Implement additional packaging checks",
        status: "Pending",
        assignedTo: null,
        dueDate: "2026-08-28T17:00:00Z",
        completedAt: null,
        verifiedBy: null,
        verifiedAt: null,
      },
      {
        id: "CA-011",
        description: "Compensate customer for damaged goods",
        status: "Completed",
        assignedTo: "F. Daniel",
        dueDate: "2026-08-23T17:00:00Z",
        completedAt: "2026-08-23T14:00:00Z",
        verifiedBy: "F. Daniel",
        verifiedAt: "2026-08-23T14:00:00Z",
      },
    ],
    investigation: {
      status: "InProgress",
      lead: "F. Daniel",
      startedAt: "2026-08-22T12:00:00Z",
      completedAt: null,
      findings: "Preliminary review shows packaging was insufficient for the item type. Loading procedures not followed.",
      recommendations: ["Update packaging guidelines", "Add inspection step during loading", "Monitor compliance for 30 days"],
    },
    evidence: [
      {
        id: "E-010",
        name: "Damaged Package 1.jpg",
        type: "Image",
        url: "#",
        uploadedAt: "2026-08-22T11:30:00Z",
        uploadedBy: "C. Adeyemi",
      },
      {
        id: "E-011",
        name: "Damaged Package 2.jpg",
        type: "Image",
        url: "#",
        uploadedAt: "2026-08-22T11:30:00Z",
        uploadedBy: "C. Adeyemi",
      },
    ],
    timeline: [
      {
        id: "T-021",
        type: "Created",
        description: "Case created by C. Adeyemi",
        createdBy: "C. Adeyemi",
        createdAt: "2026-08-22T11:00:00Z",
      },
      {
        id: "T-022",
        type: "Evidence",
        description: "Damage photos uploaded",
        createdBy: "C. Adeyemi",
        createdAt: "2026-08-22T11:30:00Z",
      },
      {
        id: "T-023",
        type: "Assignment",
        description: "Assigned to F. Daniel",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T12:00:00Z",
      },
      {
        id: "T-024",
        type: "StatusChange",
        description: "Status changed from Open to Investigating",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T12:00:00Z",
      },
      {
        id: "T-025",
        type: "RootCause",
        description: "Root cause identified: Improper packaging",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T14:00:00Z",
      },
    ],
    tags: ["Complaint", "Damage", "Packaging"],
    branch: "Lekki",
    sourceFreshness: "2026-08-22T14:00:00Z",
  },
  "QC-004": {
    id: "QC-004",
    reference: "QC-2026-004",
    title: "Safety non-conformance - Vehicle TR-112",
    description: "TR-112 operated with faulty brakes. Pre-trip inspection not properly completed. Critical safety breach requiring immediate corrective action.",
    category: "NonConformance",
    severity: "Critical",
    status: "Investigating",
    owner: "M. Bello",
    priority: "Urgent",
    rootCauseStatus: "Identified",
    rootCause: "Inspection procedure not followed. Pre-trip checklist not completed.",
    relatedOperation: {
      type: "Asset",
      reference: "TR-112",
      id: "TR-112",
    },
    createdAt: "2026-08-19T16:30:00Z",
    updatedAt: "2026-08-21T14:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    correctiveActions: [
      {
        id: "CA-012",
        description: "Review and update pre-trip inspection checklist",
        status: "Completed",
        assignedTo: "M. Bello",
        dueDate: "2026-08-21T17:00:00Z",
        completedAt: "2026-08-21T16:00:00Z",
        verifiedBy: "F. Daniel",
        verifiedAt: "2026-08-21T16:00:00Z",
      },
      {
        id: "CA-013",
        description: "Retrain all drivers on inspection procedures",
        status: "InProgress",
        assignedTo: "M. Bello",
        dueDate: "2026-08-25T17:00:00Z",
        completedAt: null,
        verifiedBy: null,
        verifiedAt: null,
      },
    ],
    investigation: {
      status: "InProgress",
      lead: "M. Bello",
      startedAt: "2026-08-19T17:00:00Z",
      completedAt: null,
      findings: "Pre-trip inspection was skipped by driver. Brake issue not identified before departure.",
      recommendations: ["Mandatory pre-trip inspection check", "Digital sign-off required", "Random audit of inspections"],
    },
    evidence: [
      {
        id: "E-012",
        name: "Inspection Report.pdf",
        type: "Document",
        url: "#",
        uploadedAt: "2026-08-20T09:00:00Z",
        uploadedBy: "M. Bello",
      },
    ],
    timeline: [
      {
        id: "T-026",
        type: "Created",
        description: "Case created by M. Bello",
        createdBy: "M. Bello",
        createdAt: "2026-08-19T16:30:00Z",
      },
      {
        id: "T-027",
        type: "StatusChange",
        description: "Status changed from Open to Investigating",
        createdBy: "M. Bello",
        createdAt: "2026-08-19T17:00:00Z",
      },
      {
        id: "T-028",
        type: "Assignment",
        description: "Assigned to M. Bello",
        createdBy: "M. Bello",
        createdAt: "2026-08-19T17:00:00Z",
      },
      {
        id: "T-029",
        type: "RootCause",
        description: "Root cause identified: Inspection procedure not followed",
        createdBy: "M. Bello",
        createdAt: "2026-08-21T14:00:00Z",
      },
    ],
    tags: ["Safety", "NonConformance", "Vehicle"],
    branch: "Apapa",
    sourceFreshness: "2026-08-21T14:00:00Z",
  },
};

// ==================== STYLES ====================

const CATEGORY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Complaint: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-4 w-4" /> },
  NonConformance: { bg: "bg-purple-bg", text: "text-purple", icon: <XCircle className="h-4 w-4" /> },
  ProcessFailure: { bg: "bg-amber-bg", text: "text-amber", icon: <RefreshCw className="h-4 w-4" /> },
  ServiceFailure: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertTriangle className="h-4 w-4" /> },
  CustomerExperience: { bg: "bg-green-bg", text: "text-green", icon: <User className="h-4 w-4" /> },
  Safety: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-4 w-4" /> },
  Other: { bg: "bg-surface-2", text: "text-ink-500", icon: <MoreHorizontal className="h-4 w-4" /> },
};

const SEVERITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-4 w-4" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-4 w-4" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-4 w-4" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-4 w-4" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-4 w-4" /> },
  Investigating: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <FileText className="h-4 w-4" /> },
  UnderReview: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <ClipboardList className="h-4 w-4" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <XCircle className="h-4 w-4" /> },
};

const ROOT_CAUSE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Unknown: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-4 w-4" /> },
  Identified: { bg: "bg-amber-bg", text: "text-amber", icon: <Flag className="h-4 w-4" /> },
  Confirmed: { bg: "bg-blue-bg", text: "text-blue", icon: <Check className="h-4 w-4" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
};

const ACTION_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Verified: { bg: "bg-purple-bg", text: "text-purple", dot: "bg-purple" },
};

// ==================== COMPONENT ====================

export default function QualityCaseDetailPage() {
  const params = useParams();
  const router = useRouter();
  const caseId = params.caseId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<QualityCaseDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showInvestigation, setShowInvestigation] = useState(true);
  const [showActions, setShowActions] = useState(true);
  const [showEvidence, setShowEvidence] = useState(true);
  const [showTimeline, setShowTimeline] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_QUALITY_CASE_DATA[caseId] || MOCK_QUALITY_CASE_DATA["QC-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [caseId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading quality case...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <ClipboardCheck className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Case not found</h3>
        <p className="mt-1 text-sm text-ink-500">No quality case found with this ID.</p>
        <button
          onClick={() => router.push("/comms/quality")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Quality Cases
        </button>
      </div>
    );
  }

  const categoryStyle = CATEGORY_STYLES[data.category] || CATEGORY_STYLES.Other;
  const severityStyle = SEVERITY_STYLES[data.severity] || SEVERITY_STYLES.Medium;
  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Open;
  const rootCauseStyle = ROOT_CAUSE_STYLES[data.rootCauseStatus] || ROOT_CAUSE_STYLES.Unknown;

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

  const formatDateTime = (date: string | null) => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const getTimelineIcon = (type: string) => {
    switch (type) {
      case "Created":
        return <AlertCircle className="h-4 w-4 text-blue" />;
      case "StatusChange":
        return <RefreshCw className="h-4 w-4 text-amber" />;
      case "Assignment":
        return <UserCog className="h-4 w-4 text-purple" />;
      case "Note":
        return <MessageSquare className="h-4 w-4 text-blue" />;
      case "Evidence":
        return <Paperclip className="h-4 w-4 text-green" />;
      case "Resolution":
        return <CheckCircle2 className="h-4 w-4 text-green" />;
      case "RootCause":
        return <Flag className="h-4 w-4 text-amber" />;
      default:
        return <Info className="h-4 w-4 text-ink-400" />;
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/comms/quality")}
              className="hover:text-primary transition-colors"
            >
              Quality Cases
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {data.title}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
              {categoryStyle.icon}
              {data.category}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
              {severityStyle.icon}
              {data.severity}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.status}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${rootCauseStyle.bg} ${rootCauseStyle.text}`}>
              {rootCauseStyle.icon}
              Root Cause: {data.rootCauseStatus}
            </span>
            <span className="text-[10px] text-ink-500">{data.branch}</span>
            {data.tags.map((tag) => (
              <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                {tag}
              </span>
            ))}
          </div>
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
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <ClipboardList className="h-3.5 w-3.5" />
            Create Action
          </button>
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Owner</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <User className="h-4 w-4 text-ink-400" />
            {data.owner || "Unassigned"}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">
            Created {formatTime(data.createdAt)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Priority</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.priority}</div>
          <div className="mt-0.5 text-xs text-ink-500">
            Updated {formatTime(data.updatedAt)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Related Operation</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            {data.relatedOperation ? (
              <button className="text-primary hover:underline flex items-center gap-1">
                {data.relatedOperation.type}: {data.relatedOperation.reference || data.relatedOperation.id}
                <ExternalLink className="h-3.5 w-3.5" />
              </button>
            ) : (
              <span className="text-ink-400">None</span>
            )}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Root Cause</div>
          <div className="mt-1 text-sm text-ink-950">
            {data.rootCause || <span className="text-ink-400">Not identified</span>}
          </div>
        </div>
      </div>

      {/* ===== DESCRIPTION ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Description</h3>
        <p className="mt-2 text-sm text-ink-700 leading-relaxed">{data.description}</p>
      </div>

      {/* ===== INVESTIGATION ===== */}
      {data.investigation && (
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          <button
            onClick={() => setShowInvestigation(!showInvestigation)}
            className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
          >
            <div className="flex items-center gap-3">
              <div className="rounded-full bg-blue-bg p-2">
                <FileText className="h-4 w-4 text-blue" />
              </div>
              <div>
                <span className="font-semibold text-ink-950">Investigation</span>
                <span className="ml-2 text-sm text-ink-500">
                  {data.investigation.status}
                  {data.investigation.lead && ` · Lead: ${data.investigation.lead}`}
                </span>
              </div>
            </div>
            {showInvestigation ? (
              <ChevronUp className="h-4 w-4 text-ink-400" />
            ) : (
              <ChevronDown className="h-4 w-4 text-ink-400" />
            )}
          </button>

          {showInvestigation && (
            <div className="border-t border-line p-4">
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
                  <div className="mt-1 text-sm font-medium text-ink-950">{data.investigation.status}</div>
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Lead</div>
                  <div className="mt-1 text-sm font-medium text-ink-950">{data.investigation.lead || "Not assigned"}</div>
                </div>
                {data.investigation.startedAt && (
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Started</div>
                    <div className="mt-1 text-sm text-ink-950">{formatDateTime(data.investigation.startedAt)}</div>
                  </div>
                )}
                {data.investigation.completedAt && (
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Completed</div>
                    <div className="mt-1 text-sm text-ink-950">{formatDateTime(data.investigation.completedAt)}</div>
                  </div>
                )}
              </div>
              {data.investigation.findings && (
                <div className="mt-3">
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Findings</div>
                  <div className="mt-1 text-sm text-ink-700">{data.investigation.findings}</div>
                </div>
              )}
              {data.investigation.recommendations.length > 0 && (
                <div className="mt-3">
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Recommendations</div>
                  <ul className="mt-1 list-disc list-inside text-sm text-ink-700">
                    {data.investigation.recommendations.map((rec, idx) => (
                      <li key={idx}>{rec}</li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* ===== CORRECTIVE ACTIONS ===== */}
      {data.correctiveActions.length > 0 && (
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          <button
            onClick={() => setShowActions(!showActions)}
            className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
          >
            <div className="flex items-center gap-3">
              <div className="rounded-full bg-green-bg p-2">
                <ClipboardList className="h-4 w-4 text-green" />
              </div>
              <div>
                <span className="font-semibold text-ink-950">Corrective Actions</span>
                <span className="ml-2 text-sm text-ink-500">
                  {data.correctiveActions.filter(a => a.status === "Completed" || a.status === "Verified").length}/{data.correctiveActions.length} completed
                </span>
              </div>
            </div>
            {showActions ? (
              <ChevronUp className="h-4 w-4 text-ink-400" />
            ) : (
              <ChevronDown className="h-4 w-4 text-ink-400" />
            )}
          </button>

          {showActions && (
            <div className="border-t border-line p-4">
              <div className="space-y-3">
                {data.correctiveActions.map((action) => {
                  const actionStatusStyle = ACTION_STATUS_STYLES[action.status] || ACTION_STATUS_STYLES.Pending;
                  return (
                    <div
                      key={action.id}
                      className="flex flex-col gap-2 rounded-xl border border-line p-3 hover:bg-surface-2 transition-colors sm:flex-row sm:items-center sm:justify-between"
                    >
                      <div className="flex-1 min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${actionStatusStyle.bg} ${actionStatusStyle.text}`}>
                            <span className={`h-1.5 w-1.5 rounded-full ${actionStatusStyle.dot}`} />
                            {action.status}
                          </span>
                          {action.assignedTo && (
                            <span className="text-[9px] text-ink-500">Assigned to: {action.assignedTo}</span>
                          )}
                        </div>
                        <div className="text-sm font-medium text-ink-950">{action.description}</div>
                        {action.dueDate && (
                          <div className="text-xs text-ink-500">Due: {formatDate(action.dueDate)}</div>
                        )}
                        {action.completedAt && (
                          <div className="text-xs text-green-500">Completed: {formatTime(action.completedAt)}</div>
                        )}
                      </div>
                      <button className="flex-shrink-0 rounded-lg border border-line px-3 py-1.5 text-[11px] font-medium text-ink-700 hover:bg-white transition-colors">
                        Update
                      </button>
                    </div>
                  );
                })}
              </div>
              <button className="mt-3 flex items-center gap-1.5 text-sm font-medium text-primary hover:underline">
                <Plus className="h-4 w-4" />
                Add Action
              </button>
            </div>
          )}
        </div>
      )}

      {/* ===== EVIDENCE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowEvidence(!showEvidence)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-blue-bg p-2">
              <Paperclip className="h-4 w-4 text-blue" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Evidence</span>
              <span className="ml-2 text-sm text-ink-500">{data.evidence.length} files</span>
            </div>
          </div>
          {showEvidence ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showEvidence && (
          <div className="border-t border-line p-4">
            {data.evidence.length === 0 ? (
              <div className="text-center py-6 text-sm text-ink-500">No evidence attached to this case.</div>
            ) : (
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                {data.evidence.map((file) => (
                  <div
                    key={file.id}
                    className="flex items-center gap-3 rounded-xl border border-line p-3 hover:bg-surface-2 transition-colors cursor-pointer"
                  >
                    <div className="flex-shrink-0 rounded-lg bg-surface-2 p-2">
                      <FileText className="h-5 w-5 text-ink-400" />
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-xs font-medium text-ink-950 truncate">{file.name}</div>
                      <div className="text-[10px] text-ink-500">
                        {file.type} · {formatTime(file.uploadedAt)} · by {file.uploadedBy}
                      </div>
                    </div>
                    <button className="flex-shrink-0 text-ink-400 hover:text-primary transition-colors">
                      <Download className="h-4 w-4" />
                    </button>
                  </div>
                ))}
              </div>
            )}
            <button className="mt-3 flex items-center gap-1.5 text-sm font-medium text-primary hover:underline">
              <Plus className="h-4 w-4" />
              Upload Evidence
            </button>
          </div>
        )}
      </div>

      {/* ===== TIMELINE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowTimeline(!showTimeline)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-purple-bg p-2">
              <Clock className="h-4 w-4 text-purple" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Timeline</span>
              <span className="ml-2 text-sm text-ink-500">{data.timeline.length} events</span>
            </div>
          </div>
          {showTimeline ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showTimeline && (
          <div className="border-t border-line p-4">
            <div className="relative">
              <div className="absolute left-[11px] top-0 bottom-0 w-0.5 bg-line" />
              <div className="space-y-4">
                {data.timeline.map((event) => (
                  <div key={event.id} className="flex gap-3">
                    <div className="relative flex-shrink-0">
                      <div className="h-6 w-6 rounded-full bg-white border border-line flex items-center justify-center z-10 relative">
                        {getTimelineIcon(event.type)}
                      </div>
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-ink-950">{event.description}</div>
                      <div className="flex flex-wrap items-center gap-2 text-xs text-ink-500">
                        <span>by {event.createdBy}</span>
                        <span>· {formatTime(event.createdAt)}</span>
                        <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                          {event.type}
                        </span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <ClipboardCheck className="h-4 w-4" />
          COM-07 · Quality Case Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}