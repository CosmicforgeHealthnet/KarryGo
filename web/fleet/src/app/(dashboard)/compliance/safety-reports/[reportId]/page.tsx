// src/app/(dashboard)/compliance/safety-reports/[reportId]/page.tsx

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
  Flag,
  EyeOff,
  UserCog,
  MessageSquare,
  Paperclip,
  Link2,
  Send,
  Phone as PhoneIcon,
} from "lucide-react";

// ==================== TYPES ====================

type SafetyReportDetail = {
  id: string;
  reference: string;
  type: "Hazard" | "NearMiss" | "UnsafeCondition" | "Incident" | "SafetyConcern";
  severity: "Critical" | "High" | "Medium" | "Low";
  title: string;
  description: string;
  location: string;
  branch: string;
  workReference: string | null;
  reportedBy: string | null;
  isAnonymous: boolean;
  status: "Open" | "UnderReview" | "Assigned" | "Resolved" | "Closed";
  assignedTo: string | null;
  assignedAt: string | null;
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  evidence: {
    id: string;
    name: string;
    type: "Image" | "Document" | "Video" | "Audio";
    url: string;
    uploadedAt: string;
    uploadedBy: string;
  }[];
  followUpActions: {
    id: string;
    description: string;
    status: "Pending" | "InProgress" | "Completed";
    assignedTo: string | null;
    dueDate: string | null;
    completedAt: string | null;
  }[];
  tags: string[];
  notes: {
    id: string;
    content: string;
    createdBy: string;
    createdAt: string;
    isInternal: boolean;
  }[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_REPORT_DATA: Record<string, SafetyReportDetail> = {
  "SR-001": {
    id: "SR-001",
    reference: "SR-2026-001",
    type: "Hazard",
    severity: "High",
    title: "Slippery floor at Ikeja depot",
    description: "Oil spill near fuel pump area. Multiple providers reported near-misses. The spill appears to be from a leaking vehicle that was not properly contained. The area is frequented by providers and staff, creating a significant slip hazard.",
    location: "Ikeja Depot - Fuel Bay",
    branch: "Ikeja",
    workReference: "KG-8491",
    reportedBy: "A. Yusuf",
    isAnonymous: false,
    status: "Assigned",
    assignedTo: "F. Daniel",
    assignedAt: "2026-08-21T16:00:00Z",
    createdAt: "2026-08-21T14:30:00Z",
    updatedAt: "2026-08-22T09:15:00Z",
    resolvedAt: null,
    resolvedBy: null,
    evidence: [
      {
        id: "E-001",
        name: "Spill Photo 1.jpg",
        type: "Image",
        url: "#",
        uploadedAt: "2026-08-21T15:00:00Z",
        uploadedBy: "A. Yusuf",
      },
      {
        id: "E-002",
        name: "Spill Photo 2.jpg",
        type: "Image",
        url: "#",
        uploadedAt: "2026-08-21T15:05:00Z",
        uploadedBy: "A. Yusuf",
      },
    ],
    followUpActions: [
      {
        id: "FA-001",
        description: "Clean spill area immediately",
        status: "Completed",
        assignedTo: "M. Bello",
        dueDate: "2026-08-21T18:00:00Z",
        completedAt: "2026-08-21T17:30:00Z",
      },
      {
        id: "FA-002",
        description: "Apply anti-slip coating to area",
        status: "InProgress",
        assignedTo: "F. Daniel",
        dueDate: "2026-08-23T17:00:00Z",
        completedAt: null,
      },
      {
        id: "FA-003",
        description: "Review and update safety protocol for fuel handling",
        status: "Pending",
        assignedTo: null,
        dueDate: "2026-08-28T17:00:00Z",
        completedAt: null,
      },
    ],
    tags: ["Spill", "SlipHazard", "Depot"],
    notes: [
      {
        id: "N-001",
        content: "Initial report received from provider. Spill area cordoned off.",
        createdBy: "F. Daniel",
        createdAt: "2026-08-21T15:30:00Z",
        isInternal: true,
      },
      {
        id: "N-002",
        content: "Cleanup crew dispatched. Estimated completion by 6pm.",
        createdBy: "M. Bello",
        createdAt: "2026-08-21T16:15:00Z",
        isInternal: false,
      },
    ],
    sourceFreshness: "2026-08-22T09:15:00Z",
  },
  "SR-003": {
    id: "SR-003",
    reference: "SR-2026-003",
    type: "UnsafeCondition",
    severity: "Critical",
    title: "Unsafe vehicle - TR-112",
    description: "Truck TR-112 reported with brake issues during pre-trip inspection. Vehicle should not operate until repairs are completed and verified.",
    location: "Apapa Depot",
    branch: "Apapa",
    workReference: "KG-8512",
    reportedBy: "K. Ahmed",
    isAnonymous: false,
    status: "Closed",
    assignedTo: "M. Bello",
    assignedAt: "2026-08-19T17:00:00Z",
    createdAt: "2026-08-19T16:20:00Z",
    updatedAt: "2026-08-21T11:00:00Z",
    resolvedAt: "2026-08-21T11:00:00Z",
    resolvedBy: "M. Bello",
    evidence: [
      {
        id: "E-003",
        name: "Brake Inspection Report.pdf",
        type: "Document",
        url: "#",
        uploadedAt: "2026-08-19T17:30:00Z",
        uploadedBy: "M. Bello",
      },
      {
        id: "E-004",
        name: "Repair Invoice.pdf",
        type: "Document",
        url: "#",
        uploadedAt: "2026-08-20T14:00:00Z",
        uploadedBy: "M. Bello",
      },
      {
        id: "E-005",
        name: "Post-Repair Inspection.jpg",
        type: "Image",
        url: "#",
        uploadedAt: "2026-08-21T10:30:00Z",
        uploadedBy: "M. Bello",
      },
    ],
    followUpActions: [
      {
        id: "FA-004",
        description: "Inspect brake system",
        status: "Completed",
        assignedTo: "M. Bello",
        dueDate: "2026-08-20T09:00:00Z",
        completedAt: "2026-08-20T08:30:00Z",
      },
      {
        id: "FA-005",
        description: "Repair brake pads and rotors",
        status: "Completed",
        assignedTo: "M. Bello",
        dueDate: "2026-08-20T17:00:00Z",
        completedAt: "2026-08-20T16:45:00Z",
      },
      {
        id: "FA-006",
        description: "Pass post-repair inspection",
        status: "Completed",
        assignedTo: "M. Bello",
        dueDate: "2026-08-21T12:00:00Z",
        completedAt: "2026-08-21T10:30:00Z",
      },
    ],
    tags: ["Vehicle", "Brakes", "Maintenance"],
    notes: [
      {
        id: "N-003",
        content: "Vehicle flagged during pre-trip inspection. Brake pads worn below minimum threshold.",
        createdBy: "K. Ahmed",
        createdAt: "2026-08-19T16:25:00Z",
        isInternal: false,
      },
      {
        id: "N-004",
        content: "Work order WO-1917 created for brake repair.",
        createdBy: "M. Bello",
        createdAt: "2026-08-19T17:45:00Z",
        isInternal: true,
      },
      {
        id: "N-005",
        content: "All repairs completed. Vehicle passed inspection. Cleared for operation.",
        createdBy: "M. Bello",
        createdAt: "2026-08-21T11:00:00Z",
        isInternal: false,
      },
    ],
    sourceFreshness: "2026-08-21T11:00:00Z",
  },
};

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Hazard: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-4 w-4" /> },
  NearMiss: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-4 w-4" /> },
  UnsafeCondition: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-4 w-4" /> },
  Incident: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-4 w-4" /> },
  SafetyConcern: { bg: "bg-purple-bg", text: "text-purple", icon: <Flag className="h-4 w-4" /> },
};

const SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Critical: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertOctagon className="h-3.5 w-3.5" />,
  },
  High: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
  },
  Medium: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Low: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Info className="h-3.5 w-3.5" />,
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  UnderReview: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Assigned: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const ACTION_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
};

export default function SafetyReportDetailPage() {
  const params = useParams();
  const router = useRouter();
  const reportId = params.reportId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SafetyReportDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showFollowUp, setShowFollowUp] = useState(true);
  const [showEvidence, setShowEvidence] = useState(true);
  const [showNotes, setShowNotes] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_REPORT_DATA[reportId] || MOCK_REPORT_DATA["SR-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [reportId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading safety report details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileSearch className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Report not found</h3>
        <p className="mt-1 text-sm text-ink-500">No safety report found with this ID.</p>
        <button
          onClick={() => router.push("/compliance/safety-reports")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Safety Reports
        </button>
      </div>
    );
  }

  const typeStyle = TYPE_STYLES[data.type] || TYPE_STYLES.Hazard;
  const severityStyle = SEVERITY_STYLES[data.severity] || SEVERITY_STYLES.Medium;
  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Open;

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

  const getStatusIcon = (status: string) => {
    switch (status) {
      case "Open":
        return <AlertCircle className="h-4 w-4" />;
      case "UnderReview":
        return <Eye className="h-4 w-4" />;
      case "Assigned":
        return <UserCog className="h-4 w-4" />;
      case "Resolved":
        return <CheckCircle2 className="h-4 w-4" />;
      case "Closed":
        return <Check className="h-4 w-4" />;
      default:
        return <Info className="h-4 w-4" />;
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/compliance/safety-reports")}
              className="hover:text-primary transition-colors"
            >
              Safety Reports
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {data.title}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
              {typeStyle.icon}
              {data.type}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
              {severityStyle.icon}
              {data.severity}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
              {data.status}
            </span>
            {data.isAnonymous && (
              <span className="flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold bg-purple-bg text-purple">
                <EyeOff className="h-3 w-3" />
                Anonymous
              </span>
            )}
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
            <UserCog className="h-3.5 w-3.5" />
            Assign
          </button>
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Location</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <MapPin className="h-4 w-4 text-ink-400" />
            {data.location}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">{data.branch} branch</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Reported</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <Clock className="h-4 w-4 text-ink-400" />
            {formatTime(data.createdAt)}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">
            {data.isAnonymous ? "Anonymous report" : `by ${data.reportedBy}`}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Assigned To</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            {data.assignedTo ? (
              <>
                <User className="h-4 w-4 text-ink-400" />
                {data.assignedTo}
              </>
            ) : (
              <span className="text-ink-400">Unassigned</span>
            )}
          </div>
          {data.assignedAt && (
            <div className="mt-0.5 text-xs text-ink-500">Assigned {formatTime(data.assignedAt)}</div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Work Reference</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            {data.workReference ? (
              <button className="text-primary hover:underline flex items-center gap-1">
                {data.workReference}
                <ExternalLink className="h-3.5 w-3.5" />
              </button>
            ) : (
              <span className="text-ink-400">None</span>
            )}
          </div>
          {data.resolvedAt && (
            <div className="mt-0.5 text-xs text-green-500">
              Resolved {formatTime(data.resolvedAt)} by {data.resolvedBy}
            </div>
          )}
        </div>
      </div>

      {/* ===== DESCRIPTION ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Description</h3>
        <p className="mt-2 text-sm text-ink-700 leading-relaxed">{data.description}</p>
      </div>

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
              <div className="text-center py-6 text-sm text-ink-500">No evidence attached to this report.</div>
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

      {/* ===== FOLLOW-UP ACTIONS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowFollowUp(!showFollowUp)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-amber-bg p-2">
              <ClipboardList className="h-4 w-4 text-amber" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Follow-up Actions</span>
              <span className="ml-2 text-sm text-ink-500">
                {data.followUpActions.filter(a => a.status === "Completed").length}/{data.followUpActions.length} completed
              </span>
            </div>
          </div>
          {showFollowUp ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showFollowUp && (
          <div className="border-t border-line p-4">
            <div className="space-y-3">
              {data.followUpActions.map((action) => {
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

      {/* ===== NOTES ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowNotes(!showNotes)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-purple-bg p-2">
              <MessageSquare className="h-4 w-4 text-purple" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Notes</span>
              <span className="ml-2 text-sm text-ink-500">{data.notes.length} notes</span>
            </div>
          </div>
          {showNotes ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showNotes && (
          <div className="border-t border-line p-4">
            <div className="space-y-3">
              {data.notes.map((note) => (
                <div
                  key={note.id}
                  className={`rounded-xl border p-3 ${note.isInternal ? "border-purple/20 bg-purple-bg/20" : "border-line"}`}
                >
                  <div className="flex items-start justify-between">
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-ink-950">{note.content}</div>
                      <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-ink-500">
                        <span>by {note.createdBy}</span>
                        <span>· {formatTime(note.createdAt)}</span>
                        {note.isInternal && (
                          <span className="rounded-full bg-purple-bg px-2 py-0.5 text-[9px] font-medium text-purple">
                            Internal
                          </span>
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
            <div className="mt-3 flex gap-2">
              <input
                type="text"
                placeholder="Add a note..."
                className="flex-1 rounded-xl border border-line px-4 py-2 text-sm text-ink-950 outline-none placeholder:text-ink-400 focus:border-primary"
              />
              <button className="flex-shrink-0 rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
                <Send className="h-4 w-4" />
              </button>
            </div>
          </div>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          CMP-07 · Safety Report Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}