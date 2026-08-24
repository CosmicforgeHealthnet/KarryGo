// src/app/(dashboard)/documents/[documentId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  FileText,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
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
  Upload,
  ChevronUp,
  ChevronDown,
  Minus,
} from "lucide-react";

// ==================== TYPES ====================

type DocumentDetail = {
  id: string;
  reference: string;
  name: string;
  type: "PDF" | "Image" | "Spreadsheet" | "Document" | "Archive" | "Other";
  category: "Business" | "Compliance" | "Provider" | "Fleet" | "Financial" | "Operational" | "Legal" | "Other";
  ownerType: "Provider" | "Asset" | "Customer" | "Staff" | "Company" | "Trip" | "Other";
  ownerId: string;
  ownerName: string;
  status: "Uploaded" | "Scanning" | "Verified" | "Rejected" | "Expired" | "Archived";
  visibility: "Public" | "Internal" | "Restricted" | "Confidential";
  verificationStatus: "Pending" | "Verified" | "Rejected" | "NotRequired";
  verifiedBy: string | null;
  verifiedAt: string | null;
  verificationNotes: string | null;
  retentionPeriod: string;
  expiryDate: string | null;
  size: string;
  version: number;
  uploadedBy: string;
  uploadedAt: string;
  updatedAt: string;
  description: string | null;
  downloadUrl: string;
  history: {
    id: string;
    action: "Uploaded" | "Updated" | "Verified" | "Rejected" | "Archived" | "Expired" | "Downloaded";
    performedBy: string;
    performedAt: string;
    notes: string | null;
  }[];
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_DOCUMENT_DATA: Record<string, DocumentDetail> = {
  "DOC-001": {
    id: "DOC-001",
    reference: "DOC-2026-001",
    name: "Vehicle Registration - BR-204",
    type: "PDF",
    category: "Fleet",
    ownerType: "Asset",
    ownerId: "BR-204",
    ownerName: "Honda ACE 110",
    status: "Verified",
    visibility: "Internal",
    verificationStatus: "Verified",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-15T10:00:00Z",
    verificationNotes: "Registration verified against government database. All details match.",
    retentionPeriod: "7 years",
    expiryDate: "2026-12-31",
    size: "2.4 MB",
    version: 1,
    uploadedBy: "M. Bello",
    uploadedAt: "2026-08-10T14:30:00Z",
    updatedAt: "2026-08-15T10:00:00Z",
    description: "Vehicle registration certificate for BR-204. Valid until December 2026.",
    downloadUrl: "#",
    history: [
      {
        id: "H-001",
        action: "Uploaded",
        performedBy: "M. Bello",
        performedAt: "2026-08-10T14:30:00Z",
        notes: "Initial upload of registration certificate.",
      },
      {
        id: "H-002",
        action: "Verified",
        performedBy: "F. Daniel",
        performedAt: "2026-08-15T10:00:00Z",
        notes: "Document verified against government database. All details match.",
      },
    ],
    tags: ["Registration", "Vehicle", "Fleet"],
    sourceFreshness: "2026-08-15T10:00:00Z",
  },
  "DOC-002": {
    id: "DOC-002",
    reference: "DOC-2026-002",
    name: "Provider ID - P-044",
    type: "PDF",
    category: "Provider",
    ownerType: "Provider",
    ownerId: "P-044",
    ownerName: "Peter Okafor",
    status: "Expired",
    visibility: "Internal",
    verificationStatus: "Rejected",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-20T09:00:00Z",
    verificationNotes: "ID card has expired. New document required immediately.",
    retentionPeriod: "3 years",
    expiryDate: "2026-08-20",
    size: "1.8 MB",
    version: 1,
    uploadedBy: "P. Okafor",
    uploadedAt: "2026-08-01T11:00:00Z",
    updatedAt: "2026-08-20T09:00:00Z",
    description: "National ID card for provider Peter Okafor.",
    downloadUrl: "#",
    history: [
      {
        id: "H-003",
        action: "Uploaded",
        performedBy: "P. Okafor",
        performedAt: "2026-08-01T11:00:00Z",
        notes: "ID card uploaded for verification.",
      },
      {
        id: "H-004",
        action: "Rejected",
        performedBy: "F. Daniel",
        performedAt: "2026-08-20T09:00:00Z",
        notes: "ID card has expired. New document required immediately.",
      },
      {
        id: "H-005",
        action: "Expired",
        performedBy: "System",
        performedAt: "2026-08-20T00:00:00Z",
        notes: "Document auto-expired based on expiry date.",
      },
    ],
    tags: ["Provider", "ID", "Expired"],
    sourceFreshness: "2026-08-20T09:00:00Z",
  },
};

const TYPE_ICONS: Record<string, React.ReactNode> = {
  PDF: <File className="h-8 w-8 text-red" />,
  Image: <FileImage className="h-8 w-8 text-blue" />,
  Spreadsheet: <FileSpreadsheet className="h-8 w-8 text-green" />,
  Document: <FileTextIcon className="h-8 w-8 text-blue" />,
  Archive: <FileArchive className="h-8 w-8 text-amber" />,
  Other: <File className="h-8 w-8 text-ink-400" />,
};

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Uploaded: { bg: "bg-blue-bg", text: "text-blue", icon: <Cloud className="h-4 w-4" /> },
  Scanning: { bg: "bg-amber-bg", text: "text-amber", icon: <RefreshCw className="h-4 w-4" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <ClockIcon className="h-4 w-4" /> },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", icon: <Folder className="h-4 w-4" /> },
};

const VERIFICATION_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <ClockIcon className="h-4 w-4" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-4 w-4" /> },
  NotRequired: { bg: "bg-surface-2", text: "text-ink-500", icon: <Minus className="h-4 w-4" /> },
};

// Simple toast function
const toast = (message: string) => {
  if (typeof window !== "undefined") {
    const toastEl = document.createElement("div");
    toastEl.className = "fixed bottom-4 right-4 bg-ink-950 text-white px-4 py-2 rounded-xl text-sm shadow-lg z-50 animate-in slide-in-from-bottom-5";
    toastEl.textContent = message;
    document.body.appendChild(toastEl);
    setTimeout(() => {
      toastEl.classList.add("opacity-0", "transition-opacity", "duration-300");
      setTimeout(() => toastEl.remove(), 300);
    }, 3000);
  }
};

export default function DocumentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const documentId = params.documentId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<DocumentDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showHistory, setShowHistory] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_DOCUMENT_DATA[documentId] || MOCK_DOCUMENT_DATA["DOC-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [documentId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading document details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileText className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Document not found</h3>
        <p className="mt-1 text-sm text-ink-500">No document found with this ID.</p>
        <button
          onClick={() => router.push("/documents")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Documents
        </button>
      </div>
    );
  }

  const typeIcon = TYPE_ICONS[data.type] || TYPE_ICONS.Other;
  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Uploaded;
  const verificationStyle = VERIFICATION_STYLES[data.verificationStatus] || VERIFICATION_STYLES.Pending;

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

  const getActionIcon = (action: string) => {
    switch (action) {
      case "Uploaded": return <Upload className="h-4 w-4 text-blue" />;
      case "Updated": return <Edit3 className="h-4 w-4 text-amber" />;
      case "Verified": return <CheckCircle2 className="h-4 w-4 text-green" />;
      case "Rejected": return <XCircle className="h-4 w-4 text-red" />;
      case "Archived": return <Folder className="h-4 w-4 text-ink-400" />;
      case "Expired": return <ClockIcon className="h-4 w-4 text-ink-400" />;
      case "Downloaded": return <Download className="h-4 w-4 text-blue" />;
      default: return <Info className="h-4 w-4 text-ink-400" />;
    }
  };

  const handleDownload = () => {
    toast("Document download started (mock)");
  };

  const handleRequestVerification = () => {
    toast("Verification requested (mock)");
  };

  const handleReplace = () => {
    router.push(`/documents/upload?replace=${data.id}`);
  };

  const handleArchive = () => {
    toast("Document archived (mock)");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/documents")}
              className="hover:text-primary transition-colors"
            >
              Documents
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {data.name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.status}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${verificationStyle.bg} ${verificationStyle.text}`}>
              {verificationStyle.icon}
              {data.verificationStatus}
            </span>
            <span className="text-[10px] text-ink-500">{data.type}</span>
            <span className="text-[10px] text-ink-500">{data.category}</span>
            <span className="text-[10px] text-ink-500">v{data.version}</span>
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
          <button
            onClick={handleDownload}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Download className="h-3.5 w-3.5" />
            Download
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <MoreHorizontal className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Owner</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <User className="h-4 w-4 text-ink-400" />
            {data.ownerName}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">{data.ownerType} · {data.ownerId}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Retention & Expiry</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.retentionPeriod}</div>
          <div className="mt-0.5 text-xs text-ink-500">
            {data.expiryDate ? `Expires: ${formatDate(data.expiryDate)}` : "No expiry"}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Upload Info</div>
          <div className="mt-1 text-sm font-medium text-ink-950">{data.size}</div>
          <div className="mt-0.5 text-xs text-ink-500">
            By {data.uploadedBy} · {formatTime(data.uploadedAt)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Visibility</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <Lock className="h-4 w-4 text-ink-400" />
            {data.visibility}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">
            Updated {formatTime(data.updatedAt)}
          </div>
        </div>
      </div>

      {/* ===== DESCRIPTION ===== */}
      {data.description && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950">Description</h3>
          <p className="mt-2 text-sm text-ink-700 leading-relaxed">{data.description}</p>
        </div>
      )}

      {/* ===== VERIFICATION DETAILS ===== */}
      {data.verificationStatus !== "NotRequired" && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950">Verification</h3>
          <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
              <div className="mt-1 flex items-center gap-2">
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${verificationStyle.bg} ${verificationStyle.text}`}>
                  {verificationStyle.icon}
                  {data.verificationStatus}
                </span>
              </div>
            </div>
            {data.verifiedBy && (
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Verified By</div>
                <div className="mt-1 text-sm font-medium text-ink-950">{data.verifiedBy}</div>
                {data.verifiedAt && (
                  <div className="text-xs text-ink-500">{formatDateTime(data.verifiedAt)}</div>
                )}
              </div>
            )}
            {data.verificationNotes && (
              <div className="sm:col-span-2">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Notes</div>
                <div className="mt-1 text-sm text-ink-700">{data.verificationNotes}</div>
              </div>
            )}
          </div>
          {data.verificationStatus === "Pending" && (
            <button
              onClick={handleRequestVerification}
              className="mt-3 rounded-lg border border-primary/20 bg-primary-bg px-4 py-1.5 text-[11.5px] font-semibold text-primary transition hover:bg-primary/10"
            >
              <CheckCircle2 className="inline h-3.5 w-3.5 mr-1" />
              Request Verification
            </button>
          )}
        </div>
      )}

      {/* ===== ACTIONS ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Actions</h3>
        <div className="mt-3 flex flex-wrap gap-3">
          <button
            onClick={handleDownload}
            className="flex items-center gap-2 rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 hover:bg-primary/90 transition-colors"
          >
            <Download className="h-4 w-4" />
            Download
          </button>
          {data.verificationStatus === "Pending" && (
            <button
              onClick={handleRequestVerification}
              className="flex items-center gap-2 rounded-xl border border-primary/20 bg-primary-bg px-4 py-2.5 text-sm font-medium text-primary hover:bg-primary/10 transition-colors"
            >
              <CheckCircle2 className="h-4 w-4" />
              Request Verification
            </button>
          )}
          <button
            onClick={handleReplace}
            className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
          >
            <FilePlus className="h-4 w-4" />
            Replace Document
          </button>
          {data.status !== "Archived" && (
            <button
              onClick={handleArchive}
              className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
            >
              <Folder className="h-4 w-4" />
              Archive
            </button>
          )}
          <button className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors">
            <Printer className="h-4 w-4" />
            Print
          </button>
        </div>
      </div>

      {/* ===== HISTORY ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowHistory(!showHistory)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-purple-bg p-2">
              <History className="h-4 w-4 text-purple" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">History</span>
              <span className="ml-2 text-sm text-ink-500">{data.history.length} events</span>
            </div>
          </div>
          {showHistory ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showHistory && (
          <div className="border-t border-line p-4">
            <div className="relative">
              <div className="absolute left-[11px] top-0 bottom-0 w-0.5 bg-line" />
              <div className="space-y-4">
                {data.history.map((event) => (
                  <div key={event.id} className="flex gap-3">
                    <div className="relative flex-shrink-0">
                      <div className="h-6 w-6 rounded-full bg-white border border-line flex items-center justify-center z-10 relative">
                        {getActionIcon(event.action)}
                      </div>
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm text-ink-950">
                        <span className="font-medium">{event.action}</span>
                        {event.notes && (
                          <span className="text-ink-600"> — {event.notes}</span>
                        )}
                      </div>
                      <div className="flex flex-wrap items-center gap-2 text-xs text-ink-500">
                        <span>by {event.performedBy}</span>
                        <span>· {formatTime(event.performedAt)}</span>
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
          <FileText className="h-4 w-4" />
          DAT-03 · Document Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}