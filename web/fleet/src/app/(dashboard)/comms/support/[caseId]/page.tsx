// src/app/(dashboard)/comms/support/[caseId]/page.tsx

"use client";

import { useState, useEffect, useRef } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  LifeBuoy,
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
  Car,
  Briefcase as BriefcaseIcon,
  AlertCircle as AlertCircleIcon,
  Ticket,
  HelpCircle,
  Shield,
  Clock as ClockIcon2,
  CheckCircle,
  XCircle as XCircleIcon,
  UserCog as UserCogIcon,
} from "lucide-react";

// ==================== TYPES ====================

type SupportCaseDetail = {
  id: string;
  reference: string;
  title: string;
  description: string;
  category: "Technical" | "Operational" | "Billing" | "Provider" | "Customer" | "Fleet" | "Compliance" | "Other";
  priority: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "InProgress" | "Escalated" | "Resolved" | "Closed" | "AwaitingResponse";
  source: "Email" | "Phone" | "InApp" | "API" | "Internal";
  organizationId: string;
  organizationName: string;
  branch: string;
  relatedTo: {
    type: "Provider" | "Customer" | "Trip" | "Order" | "Asset" | "Incident" | "Other";
    reference: string | null;
    id: string | null;
  } | null;
  assignedTo: string | null;
  assignedAt: string | null;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  resolution: string | null;
  messages: {
    id: string;
    content: string;
    sender: {
      id: string;
      name: string;
      role: string;
    };
    timestamp: string;
    isInternal: boolean;
    attachments: {
      id: string;
      name: string;
      size: string;
      type: string;
    }[];
  }[];
  attachments: {
    id: string;
    name: string;
    size: string;
    type: string;
    uploadedAt: string;
    uploadedBy: string;
  }[];
  timeline: {
    id: string;
    type: "Created" | "StatusChange" | "Assignment" | "Note" | "Attachment" | "Resolution";
    description: string;
    createdBy: string;
    createdAt: string;
  }[];
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_CASE_DATA: Record<string, SupportCaseDetail> = {
  "SUP-001": {
    id: "SUP-001",
    reference: "SUP-2026-001",
    title: "Unable to create new dispatch plan",
    description: "Dispatch plan creation is failing with an error. Users cannot schedule new routes. The error occurs when attempting to save a new dispatch plan after selecting providers and routes. This is blocking all new dispatch operations.",
    category: "Technical",
    priority: "High",
    status: "InProgress",
    source: "InApp",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Ikeja",
    relatedTo: {
      type: "Other",
      reference: null,
      id: null,
    },
    assignedTo: "F. Daniel",
    assignedAt: "2026-08-22T09:00:00Z",
    createdBy: "O. Adebayo",
    createdAt: "2026-08-22T08:30:00Z",
    updatedAt: "2026-08-22T10:15:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    messages: [
      {
        id: "MSG-012",
        content: "I'm unable to create a new dispatch plan. The system shows an error after I click save.",
        sender: { id: "S-003", name: "O. Adebayo", role: "Operations Manager" },
        timestamp: "2026-08-22T08:30:00Z",
        isInternal: false,
        attachments: [
          { id: "ATT-002", name: "error_screenshot.png", size: "1.2 MB", type: "image" },
        ],
      },
      {
        id: "MSG-013",
        content: "We've identified the issue. A recent update to the provider assignment logic is causing a validation error. We're working on a fix.",
        sender: { id: "S-001", name: "F. Daniel", role: "Support Engineer" },
        timestamp: "2026-08-22T09:00:00Z",
        isInternal: false,
        attachments: [],
      },
      {
        id: "MSG-014",
        content: "Internal note: The issue appears to be with the provider eligibility check. We need to update the validation logic.",
        sender: { id: "S-001", name: "F. Daniel", role: "Support Engineer" },
        timestamp: "2026-08-22T09:15:00Z",
        isInternal: true,
        attachments: [],
      },
      {
        id: "MSG-015",
        content: "Fix has been deployed to staging. We'll push to production after verification.",
        sender: { id: "S-001", name: "F. Daniel", role: "Support Engineer" },
        timestamp: "2026-08-22T10:15:00Z",
        isInternal: false,
        attachments: [],
      },
    ],
    attachments: [
      {
        id: "ATT-002",
        name: "error_screenshot.png",
        size: "1.2 MB",
        type: "image",
        uploadedAt: "2026-08-22T08:30:00Z",
        uploadedBy: "O. Adebayo",
      },
    ],
    timeline: [
      {
        id: "T-012",
        type: "Created",
        description: "Case created by O. Adebayo",
        createdBy: "O. Adebayo",
        createdAt: "2026-08-22T08:30:00Z",
      },
      {
        id: "T-013",
        type: "Attachment",
        description: "Screenshot uploaded",
        createdBy: "O. Adebayo",
        createdAt: "2026-08-22T08:30:00Z",
      },
      {
        id: "T-014",
        type: "Assignment",
        description: "Assigned to F. Daniel",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T09:00:00Z",
      },
      {
        id: "T-015",
        type: "StatusChange",
        description: "Status changed from Open to InProgress",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T09:00:00Z",
      },
      {
        id: "T-016",
        type: "Note",
        description: "Internal note added about validation logic",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T09:15:00Z",
      },
    ],
    tags: ["Technical", "Dispatch", "Bug"],
    sourceFreshness: "2026-08-22T10:15:00Z",
  },
  "SUP-002": {
    id: "SUP-002",
    reference: "SUP-2026-002",
    title: "Provider payment delay - P-044",
    description: "Provider P-044 has not received payment for completed trips. The provider has completed 3 trips this week and payment is overdue by 4 days.",
    category: "Billing",
    priority: "Critical",
    status: "Escalated",
    source: "Email",
    organizationId: "ORG-001",
    organizationName: "Apex Logistics",
    branch: "Ikeja",
    relatedTo: {
      type: "Provider",
      reference: "P-044",
      id: "P-044",
    },
    assignedTo: "F. Daniel",
    assignedAt: "2026-08-21T16:00:00Z",
    createdBy: "P. Okafor",
    createdAt: "2026-08-21T14:30:00Z",
    updatedAt: "2026-08-22T08:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    messages: [
      {
        id: "MSG-016",
        content: "I haven't received payment for my trips this week. Can you please check?",
        sender: { id: "P-044", name: "Peter Okafor", role: "Provider" },
        timestamp: "2026-08-21T14:30:00Z",
        isInternal: false,
        attachments: [],
      },
      {
        id: "MSG-017",
        content: "We've escalated this to finance. Let me check the payment status.",
        sender: { id: "S-001", name: "F. Daniel", role: "Support Engineer" },
        timestamp: "2026-08-21T16:00:00Z",
        isInternal: false,
        attachments: [],
      },
      {
        id: "MSG-018",
        content: "Finance team is investigating. The payment appears to be stuck in the approval workflow.",
        sender: { id: "S-001", name: "F. Daniel", role: "Support Engineer" },
        timestamp: "2026-08-21T17:00:00Z",
        isInternal: true,
        attachments: [],
      },
      {
        id: "MSG-019",
        content: "We've identified the issue. The approval threshold was incorrectly applied. We're processing the payment now.",
        sender: { id: "S-001", name: "F. Daniel", role: "Support Engineer" },
        timestamp: "2026-08-22T08:00:00Z",
        isInternal: false,
        attachments: [],
      },
      {
        id: "MSG-020",
        content: "Thank you. Please confirm when payment is completed.",
        sender: { id: "P-044", name: "Peter Okafor", role: "Provider" },
        timestamp: "2026-08-22T08:15:00Z",
        isInternal: false,
        attachments: [],
      },
    ],
    attachments: [],
    timeline: [
      {
        id: "T-017",
        type: "Created",
        description: "Case created by P. Okafor",
        createdBy: "P. Okafor",
        createdAt: "2026-08-21T14:30:00Z",
      },
      {
        id: "T-018",
        type: "Assignment",
        description: "Assigned to F. Daniel",
        createdBy: "F. Daniel",
        createdAt: "2026-08-21T16:00:00Z",
      },
      {
        id: "T-019",
        type: "StatusChange",
        description: "Status changed from Open to Escalated",
        createdBy: "F. Daniel",
        createdAt: "2026-08-21T16:00:00Z",
      },
      {
        id: "T-020",
        type: "Note",
        description: "Internal note about finance investigation",
        createdBy: "F. Daniel",
        createdAt: "2026-08-21T17:00:00Z",
      },
    ],
    tags: ["Billing", "Provider", "Escalated"],
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
};

const CATEGORY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Technical: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-4 w-4" /> },
  Operational: { bg: "bg-amber-bg", text: "text-amber", icon: <Briefcase className="h-4 w-4" /> },
  Billing: { bg: "bg-green-bg", text: "text-green", icon: <Tag className="h-4 w-4" /> },
  Provider: { bg: "bg-purple-bg", text: "text-purple", icon: <Users className="h-4 w-4" /> },
  Customer: { bg: "bg-red-bg", text: "text-red", icon: <User className="h-4 w-4" /> },
  Fleet: { bg: "bg-amber-bg", text: "text-amber", icon: <Car className="h-4 w-4" /> },
  Compliance: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-4 w-4" /> },
  Other: { bg: "bg-surface-2", text: "text-ink-500", icon: <MoreHorizontal className="h-4 w-4" /> },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-4 w-4" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-4 w-4" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-4 w-4" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-4 w-4" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-4 w-4" /> },
  InProgress: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <RefreshCw className="h-4 w-4" /> },
  Escalated: { bg: "bg-purple-bg", text: "text-purple", dot: "bg-purple", icon: <AlertOctagon className="h-4 w-4" /> },
  AwaitingResponse: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <ClockIcon2 className="h-4 w-4" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <XCircleIcon className="h-4 w-4" /> },
};

export default function SupportCaseDetailPage() {
  const params = useParams();
  const router = useRouter();
  const caseId = params.caseId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SupportCaseDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [newMessage, setNewMessage] = useState("");
  const [showMessages, setShowMessages] = useState(true);
  const [showTimeline, setShowTimeline] = useState(true);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_CASE_DATA[caseId] || MOCK_CASE_DATA["SUP-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [caseId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (messagesEndRef.current) {
      messagesEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [data]);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading support case...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <LifeBuoy className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Case not found</h3>
        <p className="mt-1 text-sm text-ink-500">No support case found with this ID.</p>
        <button
          onClick={() => router.push("/comms/support")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Support Cases
        </button>
      </div>
    );
  }

  const categoryStyle = CATEGORY_STYLES[data.category] || CATEGORY_STYLES.Other;
  const priorityStyle = PRIORITY_STYLES[data.priority] || PRIORITY_STYLES.Medium;
  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Open;

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

  const formatTimestamp = (date: string) => {
    const d = new Date(date);
    return d.toLocaleString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const handleSendMessage = () => {
    if (!newMessage.trim()) return;
    toast("Message sent (mock)");
    setNewMessage("");
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSendMessage();
    }
  };

  const getTimelineIcon = (type: string) => {
    switch (type) {
      case "Created":
        return <AlertCircle className="h-4 w-4 text-blue" />;
      case "StatusChange":
        return <RefreshCw className="h-4 w-4 text-amber" />;
      case "Assignment":
        return <UserCogIcon className="h-4 w-4 text-purple" />;
      case "Note":
        return <MessageSquare className="h-4 w-4 text-blue" />;
      case "Attachment":
        return <Paperclip className="h-4 w-4 text-green" />;
      case "Resolution":
        return <CheckCircle2 className="h-4 w-4 text-green" />;
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
              onClick={() => router.push("/comms/support")}
              className="hover:text-primary transition-colors"
            >
              Support Cases
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
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${priorityStyle.bg} ${priorityStyle.text}`}>
              {priorityStyle.icon}
              {data.priority}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.status}
            </span>
            <span className="text-[10px] text-ink-500">{data.organizationName}</span>
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
            <UserCogIcon className="h-3.5 w-3.5" />
            Assign
          </button>
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Source</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            {data.source === "Email" && <Mail className="h-4 w-4 text-ink-400" />}
            {data.source === "Phone" && <PhoneIcon className="h-4 w-4 text-ink-400" />}
            {data.source === "InApp" && <MessageSquare className="h-4 w-4 text-ink-400" />}
            {data.source === "API" && <Link2 className="h-4 w-4 text-ink-400" />}
            {data.source === "Internal" && <UserCogIcon className="h-4 w-4 text-ink-400" />}
            {data.source}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">Created {formatTime(data.createdAt)}</div>
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Related To</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            {data.relatedTo ? (
              <button className="text-primary hover:underline flex items-center gap-1">
                {data.relatedTo.type}: {data.relatedTo.reference || data.relatedTo.id}
                <ExternalLink className="h-3.5 w-3.5" />
              </button>
            ) : (
              <span className="text-ink-400">None</span>
            )}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">Branch: {data.branch}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Resolution</div>
          <div className="mt-1 text-sm text-ink-950">
            {data.resolvedAt ? (
              <span className="flex items-center gap-1 text-green">
                <CheckCircle2 className="h-4 w-4" />
                Resolved {formatTime(data.resolvedAt)}
              </span>
            ) : (
              <span className="text-ink-400">Not resolved</span>
            )}
          </div>
          {data.resolvedBy && (
            <div className="mt-0.5 text-xs text-ink-500">By: {data.resolvedBy}</div>
          )}
        </div>
      </div>

      {/* ===== DESCRIPTION ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Description</h3>
        <p className="mt-2 text-sm text-ink-700 leading-relaxed">{data.description}</p>
      </div>

      {/* ===== MESSAGES ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowMessages(!showMessages)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-blue-bg p-2">
              <MessageSquare className="h-4 w-4 text-blue" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Messages</span>
              <span className="ml-2 text-sm text-ink-500">{data.messages.length} messages</span>
            </div>
          </div>
          {showMessages ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showMessages && (
          <div className="border-t border-line">
            <div className="max-h-[400px] overflow-y-auto p-4 space-y-4">
              {data.messages.map((message) => {
                const isOwn = message.sender.id === "S-001";
                return (
                  <div
                    key={message.id}
                    className={`flex ${isOwn ? "justify-end" : "justify-start"}`}
                  >
                    <div
                      className={`max-w-[80%] rounded-2xl px-4 py-3 ${
                        isOwn
                          ? "bg-primary text-white"
                          : message.isInternal
                          ? "bg-purple-bg border border-purple/20"
                          : "bg-surface-2"
                      }`}
                    >
                      <div className="flex items-center gap-2 mb-1">
                        <span className={`text-xs font-medium ${isOwn ? "text-white/90" : "text-ink-950"}`}>
                          {message.sender.name}
                        </span>
                        {message.isInternal && (
                          <span className="rounded-full bg-purple-bg px-1.5 py-0.5 text-[8px] font-medium text-purple">
                            Internal
                          </span>
                        )}
                      </div>
                      <div className={`text-sm ${isOwn ? "text-white" : "text-ink-950"}`}>
                        {message.content}
                      </div>
                      {message.attachments.length > 0 && (
                        <div className="mt-2 flex flex-wrap gap-1.5">
                          {message.attachments.map((att) => (
                            <div
                              key={att.id}
                              className={`flex items-center gap-1 rounded-lg px-2 py-1 text-[10px] ${
                                isOwn ? "bg-white/10 text-white/80" : "bg-white text-ink-700"
                              }`}
                            >
                              <Paperclip className="h-3 w-3" />
                              {att.name}
                              <span className="opacity-50">({att.size})</span>
                            </div>
                          ))}
                        </div>
                      )}
                      <div className={`flex items-center gap-1.5 mt-1.5 text-[10px] ${
                        isOwn ? "text-white/60" : "text-ink-400"
                      }`}>
                        <span>{formatTimestamp(message.timestamp)}</span>
                      </div>
                    </div>
                  </div>
                );
              })}
              <div ref={messagesEndRef} />
            </div>

            {/* ===== MESSAGE INPUT ===== */}
            <div className="border-t border-line p-3">
              <div className="flex items-end gap-2">
                <button className="flex-shrink-0 rounded-lg p-2 hover:bg-surface-2 transition-colors text-ink-400 hover:text-ink-600">
                  <Paperclip className="h-5 w-5" />
                </button>
                <div className="flex-1 min-w-0">
                  <textarea
                    value={newMessage}
                    onChange={(e) => setNewMessage(e.target.value)}
                    onKeyDown={handleKeyDown}
                    placeholder="Add a message..."
                    className="w-full resize-none rounded-xl border border-line px-3 py-2 text-sm text-ink-950 outline-none placeholder:text-ink-400 focus:border-primary min-h-[44px] max-h-[120px]"
                    rows={1}
                  />
                </div>
                <button
                  onClick={handleSendMessage}
                  disabled={!newMessage.trim()}
                  className={`flex-shrink-0 rounded-xl px-4 py-2.5 text-sm font-semibold transition ${
                    newMessage.trim()
                      ? "bg-primary text-white shadow-sm shadow-primary/20 hover:bg-primary/90"
                      : "bg-surface-2 text-ink-400 cursor-not-allowed"
                  }`}
                >
                  <Send className="h-4 w-4" />
                </button>
              </div>
              <div className="mt-1.5 text-[10px] text-ink-400 flex items-center gap-2">
                <span>Press Enter to send</span>
                <span>·</span>
                <span className="flex items-center gap-1">
                  <Lock className="h-3 w-3" />
                  Messages are secure
                </span>
              </div>
            </div>
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
          <LifeBuoy className="h-4 w-4" />
          COM-05 · Support Case Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}