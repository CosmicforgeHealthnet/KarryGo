// src/app/(dashboard)/comms/escalations/[escalationId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  AlertOctagon,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  User,
  Mail,
  Phone,
  Info,
  XCircle,
  ClipboardList,
  ChevronDown,
  ChevronUp,
  ExternalLink,
  Check,
  UserCog,
  MessageSquare,
  Link2,
  Shield,
  Settings,
  BellRing,
  HelpCircle,
  Gauge,
  ArrowUp,
} from "lucide-react";

// ==================== TYPES ====================

type EscalationDetail = {
  id: string;
  reference: string;
  source: string;
  sourceType: "Incident" | "SafetyReport" | "SupportCase" | "QualityCase" | "SLA" | "Compliance" | "System";
  sourceUrl: string | null;
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
  resolution: string | null;
  branch: string;
  notifications: {
    id: string;
    channel: "Email" | "SMS" | "Push" | "InApp";
    recipient: string;
    sentAt: string;
    status: "Sent" | "Delivered" | "Read" | "Failed";
  }[];
  timeline: {
    id: string;
    type: "Created" | "StatusChange" | "Assignment" | "Notification" | "Acknowledgment" | "Resolution" | "Escalation";
    description: string;
    createdBy: string;
    createdAt: string;
  }[];
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_ESCALATION_DATA: Record<string, EscalationDetail> = {
  "ESC-001": {
    id: "ESC-001",
    reference: "ESC-2026-001",
    source: "INC-771",
    sourceType: "Incident",
    sourceUrl: "/compliance/incidents/INC-771",
    severity: "Critical",
    level: 2,
    maxLevel: 3,
    title: "Critical incident escalation - Vehicle collision",
    description: "Provider P-044 was involved in a collision on Opebi Road, Ikeja. Multiple departments need to coordinate for investigation, vehicle repair, provider support, and customer communication. The incident has been escalated to level 2 requiring Operations Director attention.",
    recipient: "Operations Director",
    owner: "F. Daniel",
    dueTime: "2026-08-23T17:00:00Z",
    status: "Active",
    sla: "24 hours",
    createdAt: "2026-08-22T06:30:00Z",
    updatedAt: "2026-08-22T10:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    branch: "Ikeja",
    notifications: [
      {
        id: "N-001",
        channel: "Email",
        recipient: "Operations Director",
        sentAt: "2026-08-22T06:35:00Z",
        status: "Delivered",
      },
      {
        id: "N-002",
        channel: "SMS",
        recipient: "Operations Director",
        sentAt: "2026-08-22T06:35:00Z",
        status: "Delivered",
      },
      {
        id: "N-003",
        channel: "Push",
        recipient: "F. Daniel",
        sentAt: "2026-08-22T06:40:00Z",
        status: "Read",
      },
    ],
    timeline: [
      {
        id: "T-030",
        type: "Created",
        description: "Escalation created from incident INC-771",
        createdBy: "System",
        createdAt: "2026-08-22T06:30:00Z",
      },
      {
        id: "T-031",
        type: "Notification",
        description: "Email sent to Operations Director",
        createdBy: "System",
        createdAt: "2026-08-22T06:35:00Z",
      },
      {
        id: "T-032",
        type: "Notification",
        description: "SMS sent to Operations Director",
        createdBy: "System",
        createdAt: "2026-08-22T06:35:00Z",
      },
      {
        id: "T-033",
        type: "Notification",
        description: "Push notification sent to F. Daniel",
        createdBy: "System",
        createdAt: "2026-08-22T06:40:00Z",
      },
      {
        id: "T-034",
        type: "Acknowledgment",
        description: "Escalation acknowledged by F. Daniel",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T07:00:00Z",
      },
      {
        id: "T-035",
        type: "Assignment",
        description: "Incident investigation assigned to F. Daniel",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T07:00:00Z",
      },
    ],
    tags: ["Incident", "Critical", "Coordination"],
    sourceFreshness: "2026-08-22T10:00:00Z",
  },
  "ESC-005": {
    id: "ESC-005",
    reference: "ESC-2026-005",
    source: "QC-004",
    sourceType: "QualityCase",
    sourceUrl: "/comms/quality/QC-004",
    severity: "Critical",
    level: 3,
    maxLevel: 3,
    title: "Critical quality escalation - Safety non-conformance",
    description: "Critical safety non-conformance identified with TR-112. Vehicle operated with faulty brakes. Pre-trip inspection not completed. This is a level 3 escalation requiring Executive notification.",
    recipient: "Operations Director",
    owner: "M. Bello",
    dueTime: "2026-08-22T17:00:00Z",
    status: "Escalated",
    sla: "24 hours",
    createdAt: "2026-08-19T16:30:00Z",
    updatedAt: "2026-08-21T14:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    resolution: null,
    branch: "Apapa",
    notifications: [
      {
        id: "N-004",
        channel: "Email",
        recipient: "Operations Director",
        sentAt: "2026-08-19T16:35:00Z",
        status: "Read",
      },
      {
        id: "N-005",
        channel: "SMS",
        recipient: "Operations Director",
        sentAt: "2026-08-19T16:35:00Z",
        status: "Delivered",
      },
      {
        id: "N-006",
        channel: "Push",
        recipient: "M. Bello",
        sentAt: "2026-08-19T16:40:00Z",
        status: "Read",
      },
      {
        id: "N-007",
        channel: "Email",
        recipient: "Safety Manager",
        sentAt: "2026-08-19T16:45:00Z",
        status: "Delivered",
      },
    ],
    timeline: [
      {
        id: "T-036",
        type: "Created",
        description: "Escalation created from quality case QC-004",
        createdBy: "System",
        createdAt: "2026-08-19T16:30:00Z",
      },
      {
        id: "T-037",
        type: "Notification",
        description: "Email sent to Operations Director",
        createdBy: "System",
        createdAt: "2026-08-19T16:35:00Z",
      },
      {
        id: "T-038",
        type: "Notification",
        description: "SMS sent to Operations Director",
        createdBy: "System",
        createdAt: "2026-08-19T16:35:00Z",
      },
      {
        id: "T-039",
        type: "Notification",
        description: "Email sent to Safety Manager",
        createdBy: "System",
        createdAt: "2026-08-19T16:45:00Z",
      },
      {
        id: "T-040",
        type: "Acknowledgment",
        description: "Escalation acknowledged by M. Bello",
        createdBy: "M. Bello",
        createdAt: "2026-08-19T17:00:00Z",
      },
      {
        id: "T-041",
        type: "Escalation",
        description: "Escalated to level 3 - Executive notification",
        createdBy: "M. Bello",
        createdAt: "2026-08-21T14:00:00Z",
      },
      {
        id: "T-042",
        type: "StatusChange",
        description: "Status changed to Escalated",
        createdBy: "M. Bello",
        createdAt: "2026-08-21T14:00:00Z",
      },
    ],
    tags: ["Quality", "Critical", "Safety"],
    sourceFreshness: "2026-08-21T14:00:00Z",
  },
  "ESC-004": {
    id: "ESC-004",
    reference: "ESC-2026-004",
    source: "SLA-001",
    sourceType: "SLA",
    sourceUrl: "/comms/sla/SLA-001",
    severity: "Medium",
    level: 1,
    maxLevel: 2,
    title: "SLA breach escalation - Charitos Hospital",
    description: "SLA breach identified for Charitos Hospital. On-time delivery rate fell below target. Account manager notified for customer communication.",
    recipient: "O. Adebayo",
    owner: "F. Daniel",
    dueTime: "2026-08-24T10:00:00Z",
    status: "Resolved",
    sla: "48 hours",
    createdAt: "2026-08-22T09:00:00Z",
    updatedAt: "2026-08-22T14:00:00Z",
    resolvedAt: "2026-08-22T14:00:00Z",
    resolvedBy: "O. Adebayo",
    resolution: "Customer notified. Action plan implemented to improve on-time delivery rate.",
    branch: "Ikeja",
    notifications: [
      {
        id: "N-008",
        channel: "Email",
        recipient: "O. Adebayo",
        sentAt: "2026-08-22T09:05:00Z",
        status: "Read",
      },
      {
        id: "N-009",
        channel: "InApp",
        recipient: "O. Adebayo",
        sentAt: "2026-08-22T09:05:00Z",
        status: "Read",
      },
    ],
    timeline: [
      {
        id: "T-043",
        type: "Created",
        description: "Escalation created from SLA measurement SLA-001",
        createdBy: "System",
        createdAt: "2026-08-22T09:00:00Z",
      },
      {
        id: "T-044",
        type: "Notification",
        description: "Email sent to O. Adebayo",
        createdBy: "System",
        createdAt: "2026-08-22T09:05:00Z",
      },
      {
        id: "T-045",
        type: "Acknowledgment",
        description: "Escalation acknowledged by O. Adebayo",
        createdBy: "O. Adebayo",
        createdAt: "2026-08-22T09:30:00Z",
      },
      {
        id: "T-046",
        type: "Resolution",
        description: "Customer notified. Action plan implemented.",
        createdBy: "O. Adebayo",
        createdAt: "2026-08-22T14:00:00Z",
      },
      {
        id: "T-047",
        type: "StatusChange",
        description: "Status changed to Resolved",
        createdBy: "O. Adebayo",
        createdAt: "2026-08-22T14:00:00Z",
      },
    ],
    tags: ["SLA", "Customer", "Breach"],
    sourceFreshness: "2026-08-22T14:00:00Z",
  },
};

// ==================== STYLES ====================

const SEVERITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-4 w-4" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-4 w-4" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-4 w-4" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-4 w-4" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertOctagon className="h-4 w-4" /> },
  Acknowledged: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Check className="h-4 w-4" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Escalated: { bg: "bg-purple-bg", text: "text-purple", dot: "bg-purple", icon: <ArrowUp className="h-4 w-4" /> },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <XCircle className="h-4 w-4" /> },
};

const SOURCE_TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Incident: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-4 w-4" /> },
  SafetyReport: { bg: "bg-amber-bg", text: "text-amber", icon: <Shield className="h-4 w-4" /> },
  SupportCase: { bg: "bg-blue-bg", text: "text-blue", icon: <HelpCircle className="h-4 w-4" /> },
  QualityCase: { bg: "bg-purple-bg", text: "text-purple", icon: <ClipboardList className="h-4 w-4" /> },
  SLA: { bg: "bg-green-bg", text: "text-green", icon: <Gauge className="h-4 w-4" /> },
  Compliance: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-4 w-4" /> },
  System: { bg: "bg-surface-2", text: "text-ink-500", icon: <Settings className="h-4 w-4" /> },
};

const NOTIFICATION_STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Sent: { bg: "bg-blue-bg", text: "text-blue", icon: <Clock className="h-3.5 w-3.5" /> },
  Delivered: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Read: { bg: "bg-purple-bg", text: "text-purple", icon: <Eye className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
};

const CHANNEL_ICONS: Record<string, React.ReactNode> = {
  Email: <Mail className="h-3.5 w-3.5" />,
  SMS: <Phone className="h-3.5 w-3.5" />,
  Push: <BellRing className="h-3.5 w-3.5" />,
  InApp: <MessageSquare className="h-3.5 w-3.5" />,
};

// ==================== COMPONENT ====================

export default function EscalationDetailPage() {
  const params = useParams();
  const router = useRouter();
  const escalationId = params.escalationId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<EscalationDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showNotifications, setShowNotifications] = useState(true);
  const [showTimeline, setShowTimeline] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_ESCALATION_DATA[escalationId] || MOCK_ESCALATION_DATA["ESC-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [escalationId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading escalation details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <AlertOctagon className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Escalation not found</h3>
        <p className="mt-1 text-sm text-ink-500">No escalation found with this ID.</p>
        <button
          onClick={() => router.push("/comms/escalations")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Escalations
        </button>
      </div>
    );
  }

  const severityStyle = SEVERITY_STYLES[data.severity] || SEVERITY_STYLES.Medium;
  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Active;
  const sourceStyle = SOURCE_TYPE_STYLES[data.sourceType] || SOURCE_TYPE_STYLES.System;

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

  const getTimelineIcon = (type: string) => {
    switch (type) {
      case "Created":
        return <AlertCircle className="h-4 w-4 text-blue" />;
      case "StatusChange":
        return <RefreshCw className="h-4 w-4 text-amber" />;
      case "Assignment":
        return <UserCog className="h-4 w-4 text-purple" />;
      case "Notification":
        return <BellRing className="h-4 w-4 text-blue" />;
      case "Acknowledgment":
        return <Check className="h-4 w-4 text-green" />;
      case "Resolution":
        return <CheckCircle2 className="h-4 w-4 text-green" />;
      case "Escalation":
        return <ArrowUp className="h-4 w-4 text-red" />;
      default:
        return <Info className="h-4 w-4 text-ink-400" />;
    }
  };

  const handleAcknowledge = () => {
    console.log("Escalation acknowledged (mock)");
  };

  const handleResolve = () => {
    console.log("Escalation resolved (mock)");
  };

  const handleEscalate = () => {
    console.log("Escalation escalated to next level (mock)");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/comms/escalations")}
              className="hover:text-primary transition-colors"
            >
              Escalations
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {data.title}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${sourceStyle.bg} ${sourceStyle.text}`}>
              {sourceStyle.icon}
              {data.sourceType}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
              {severityStyle.icon}
              {data.severity}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              {statusStyle.icon}
              {data.status}
            </span>
            <span className="text-[10px] text-ink-500">
              Level {data.level}/{data.maxLevel}
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
          {data.status !== "Resolved" && data.status !== "Closed" && (
            <>
              <button
                onClick={handleAcknowledge}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
              >
                <Check className="h-3.5 w-3.5" />
                Acknowledge
              </button>
              <button
                onClick={handleEscalate}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-red/20 bg-red-bg px-3.5 text-[11.5px] font-medium text-red shadow-sm transition hover:bg-red/10"
              >
                <ArrowUp className="h-3.5 w-3.5" />
                Escalate
              </button>
            </>
          )}
          {data.status !== "Resolved" && data.status !== "Closed" && (
            <button
              onClick={handleResolve}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
            >
              <CheckCircle2 className="h-3.5 w-3.5" />
              Resolve
            </button>
          )}
        </div>
      </div>

      {/* ===== OVERVIEW GRID ===== */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Source</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <Link2 className="h-4 w-4 text-ink-400" />
            {data.source}
            {data.sourceUrl && (
              <button
                onClick={() => router.push(data.sourceUrl!)}
                className="text-primary hover:underline flex items-center gap-1 text-xs"
              >
                View
                <ExternalLink className="h-3.5 w-3.5" />
              </button>
            )}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">Created {formatTime(data.createdAt)}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Owner / Recipient</div>
          <div className="mt-1 flex flex-col">
            <div className="flex items-center gap-1.5 text-sm font-medium text-ink-950">
              <User className="h-4 w-4 text-ink-400" />
              {data.owner || "Unassigned"}
            </div>
            {data.recipient && (
              <div className="text-xs text-ink-500">Recipient: {data.recipient}</div>
            )}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">SLA / Due Time</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.sla || "No SLA"}
          </div>
          {data.dueTime && (
            <div className="mt-0.5 text-xs text-ink-500">
              Due: {formatDateTime(data.dueTime)}
            </div>
          )}
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
              <span className="text-ink-400">Pending</span>
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

      {/* ===== RESOLUTION ===== */}
      {data.resolution && (
        <div className="rounded-2xl border border-green/20 bg-green-bg/30 p-5 shadow-sm">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-green-bg p-2 flex-shrink-0">
              <CheckCircle2 className="h-5 w-5 text-green" />
            </div>
            <div>
              <h4 className="text-sm font-semibold text-green">Resolution</h4>
              <p className="mt-1 text-sm text-green/80">{data.resolution}</p>
              {data.resolvedAt && (
                <div className="mt-1 text-xs text-green/60">
                  Resolved {formatTime(data.resolvedAt)} by {data.resolvedBy}
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* ===== NOTIFICATIONS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowNotifications(!showNotifications)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-blue-bg p-2">
              <BellRing className="h-4 w-4 text-blue" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Notifications</span>
              <span className="ml-2 text-sm text-ink-500">{data.notifications.length} sent</span>
            </div>
          </div>
          {showNotifications ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showNotifications && (
          <div className="border-t border-line p-4">
            <div className="space-y-2">
              {data.notifications.map((notification) => {
                const statusStyle = NOTIFICATION_STATUS_STYLES[notification.status] || NOTIFICATION_STATUS_STYLES.Sent;
                const channelIcon = CHANNEL_ICONS[notification.channel] || <BellRing className="h-3.5 w-3.5" />;

                return (
                  <div
                    key={notification.id}
                    className="flex items-center gap-3 rounded-xl border border-line p-3 hover:bg-surface-2 transition-colors"
                  >
                    <div className="flex-shrink-0 rounded-lg bg-surface-2 p-2">
                      {channelIcon}
                    </div>
                    <div className="flex-1 min-w-0">
                      <div className="text-sm font-medium text-ink-950">
                        {notification.channel} to {notification.recipient}
                      </div>
                      <div className="text-xs text-ink-500">
                        Sent {formatTime(notification.sentAt)}
                      </div>
                    </div>
                    <div className="flex-shrink-0">
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        {statusStyle.icon}
                        {notification.status}
                      </span>
                    </div>
                  </div>
                );
              })}
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

      {/* ===== ACTIONS ===== */}
      {data.status !== "Resolved" && data.status !== "Closed" && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950">Actions</h3>
          <div className="mt-3 flex flex-wrap gap-3">
            <button
              onClick={handleAcknowledge}
              className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
            >
              <Check className="h-4 w-4" />
              Acknowledge
            </button>
            <button
              onClick={handleEscalate}
              className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg px-4 py-2.5 text-sm font-medium text-red hover:bg-red/10 transition-colors"
            >
              <ArrowUp className="h-4 w-4" />
              Escalate to Level {data.level + 1}
            </button>
            <button
              onClick={handleResolve}
              className="flex items-center gap-2 rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 hover:bg-primary/90 transition-colors"
            >
              <CheckCircle2 className="h-4 w-4" />
              Resolve
            </button>
            <button className="flex items-center gap-2 rounded-xl border border-line px-4 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors">
              <MessageSquare className="h-4 w-4" />
              Add Note
            </button>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <AlertOctagon className="h-4 w-4" />
          COM-10 · Escalation Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}