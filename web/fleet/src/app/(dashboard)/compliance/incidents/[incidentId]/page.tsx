// src/app/(dashboard)/compliance/incidents/[incidentId]/page.tsx

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
  Car,
  Briefcase as BriefcaseIcon,
  AlertCircle as AlertCircleIcon,
} from "lucide-react";

// ==================== TYPES ====================

type IncidentDetail = {
  id: string;
  reference: string;
  type: "Accident" | "NearMiss" | "PropertyDamage" | "SafetyIncident" | "SecurityIncident" | "VehicleIncident" | "Other";
  severity: "Critical" | "High" | "Medium" | "Low";
  title: string;
  description: string;
  location: string;
  branch: string;
  affectedPeople: {
    id: string;
    name: string;
    role: string;
    impact: string;
  }[];
  affectedAssets: {
    id: string;
    name: string;
    type: string;
    impact: string;
  }[];
  affectedTrips: {
    id: string;
    reference: string;
    status: string;
  }[];
  status: "Open" | "Investigating" | "UnderReview" | "Resolved" | "Closed";
  owner: string | null;
  priority: "Urgent" | "High" | "Normal" | "Low";
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  triage: {
    category: string;
    assignedTo: string | null;
    assignedAt: string | null;
    notes: string | null;
  } | null;
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
    type: "Created" | "StatusChange" | "Assignment" | "Note" | "Evidence" | "Resolution";
    description: string;
    createdBy: string;
    createdAt: string;
  }[];
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
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_INCIDENT_DATA: Record<string, IncidentDetail> = {
  "INC-771": {
    id: "INC-771",
    reference: "INC-2026-771",
    type: "VehicleIncident",
    severity: "High",
    title: "Road incident - Provider P-044",
    description: "Provider P-044 was involved in a collision with another vehicle on Opebi Road, Ikeja. The incident occurred during a scheduled dispatch trip. No injuries were reported to the provider or third parties. Vehicle BR-204 sustained front-end damage including bumper, headlight, and hood. The provider reported that the other vehicle failed to yield at an intersection.",
    location: "Opebi Road, Ikeja",
    branch: "Ikeja",
    affectedPeople: [
      {
        id: "P-044",
        name: "Peter Okafor",
        role: "Dispatch Rider",
        impact: "Provider involved in collision. No injuries.",
      },
    ],
    affectedAssets: [
      {
        id: "BR-204",
        name: "Honda ACE 110",
        type: "Motorcycle",
        impact: "Front-end damage. Vehicle out of service.",
      },
    ],
    affectedTrips: [
      {
        id: "KG-8479",
        reference: "KG-8479",
        status: "Disrupted",
      },
    ],
    status: "Investigating",
    owner: "F. Daniel",
    priority: "High",
    createdAt: "2026-08-22T06:30:00Z",
    updatedAt: "2026-08-22T09:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    triage: {
      category: "Vehicle Collision",
      assignedTo: "F. Daniel",
      assignedAt: "2026-08-22T07:00:00Z",
      notes: "Provider reported incident via app. Police report pending.",
    },
    investigation: {
      status: "InProgress",
      lead: "F. Daniel",
      startedAt: "2026-08-22T08:00:00Z",
      completedAt: null,
      findings: "Preliminary review suggests other vehicle at fault. Waiting for police report and dashcam footage.",
      recommendations: ["Obtain police report", "Review dashcam footage", "Assess vehicle damage"],
    },
    evidence: [
      {
        id: "E-006",
        name: "Incident Photo 1.jpg",
        type: "Image",
        url: "#",
        uploadedAt: "2026-08-22T06:45:00Z",
        uploadedBy: "P. Okafor",
      },
      {
        id: "E-007",
        name: "Incident Photo 2.jpg",
        type: "Image",
        url: "#",
        uploadedAt: "2026-08-22T06:50:00Z",
        uploadedBy: "P. Okafor",
      },
      {
        id: "E-008",
        name: "Police Report.pdf",
        type: "Document",
        url: "#",
        uploadedAt: "2026-08-22T08:30:00Z",
        uploadedBy: "F. Daniel",
      },
      {
        id: "E-009",
        name: "Dashcam Footage.mp4",
        type: "Video",
        url: "#",
        uploadedAt: "2026-08-22T09:00:00Z",
        uploadedBy: "F. Daniel",
      },
    ],
    timeline: [
      {
        id: "T-001",
        type: "Created",
        description: "Incident reported by provider via app",
        createdBy: "P. Okafor",
        createdAt: "2026-08-22T06:30:00Z",
      },
      {
        id: "T-002",
        type: "StatusChange",
        description: "Status changed from Open to Investigating",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T07:00:00Z",
      },
      {
        id: "T-003",
        type: "Assignment",
        description: "Assigned to F. Daniel for investigation",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T07:00:00Z",
      },
      {
        id: "T-004",
        type: "Evidence",
        description: "Incident photos uploaded",
        createdBy: "P. Okafor",
        createdAt: "2026-08-22T06:50:00Z",
      },
      {
        id: "T-005",
        type: "Evidence",
        description: "Police report uploaded",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T08:30:00Z",
      },
      {
        id: "T-006",
        type: "Note",
        description: "Dashcam footage requested from provider",
        createdBy: "F. Daniel",
        createdAt: "2026-08-22T08:45:00Z",
      },
    ],
    correctiveActions: [
      {
        id: "CA-001",
        description: "Obtain police report for incident",
        status: "Completed",
        assignedTo: "F. Daniel",
        dueDate: "2026-08-23T17:00:00Z",
        completedAt: "2026-08-22T08:30:00Z",
        verifiedBy: null,
        verifiedAt: null,
      },
      {
        id: "CA-002",
        description: "Review dashcam footage",
        status: "InProgress",
        assignedTo: "F. Daniel",
        dueDate: "2026-08-24T17:00:00Z",
        completedAt: null,
        verifiedBy: null,
        verifiedAt: null,
      },
      {
        id: "CA-003",
        description: "Assess vehicle damage and create work order",
        status: "Pending",
        assignedTo: null,
        dueDate: "2026-08-25T17:00:00Z",
        completedAt: null,
        verifiedBy: null,
        verifiedAt: null,
      },
    ],
    tags: ["Collision", "Vehicle", "Provider", "Police"],
    sourceFreshness: "2026-08-22T09:00:00Z",
  },
  "INC-777": {
    id: "INC-777",
    reference: "INC-2026-777",
    type: "SafetyIncident",
    severity: "Critical",
    title: "Unsafe vehicle - TR-112",
    description: "Pre-trip inspection revealed critical brake issues on TR-112. Vehicle placed out of service immediately. Maintenance work order created.",
    location: "Apapa Depot",
    branch: "Apapa",
    affectedPeople: [
      {
        id: "P-071",
        name: "Kabiru Ahmed",
        role: "Hauling Driver",
        impact: "Unable to complete scheduled trip. Reassignment required.",
      },
    ],
    affectedAssets: [
      {
        id: "TR-112",
        name: "Mercedes Actros",
        type: "Truck",
        impact: "Brake failure risk. Out of service until repaired.",
      },
    ],
    affectedTrips: [
      {
        id: "KG-8512",
        reference: "KG-8512",
        status: "Reassigned",
      },
    ],
    status: "Closed",
    owner: "M. Bello",
    priority: "Urgent",
    createdAt: "2026-08-19T16:20:00Z",
    updatedAt: "2026-08-21T11:00:00Z",
    resolvedAt: "2026-08-21T11:00:00Z",
    resolvedBy: "M. Bello",
    triage: {
      category: "Vehicle Safety",
      assignedTo: "M. Bello",
      assignedAt: "2026-08-19T17:00:00Z",
      notes: "Critical safety issue identified. Vehicle immediately placed out of service.",
    },
    investigation: {
      status: "Completed",
      lead: "M. Bello",
      startedAt: "2026-08-19T17:30:00Z",
      completedAt: "2026-08-21T10:30:00Z",
      findings: "Brake pads worn below minimum threshold. Rotors damaged. All components replaced. Vehicle passed post-repair inspection.",
      recommendations: ["Increase inspection frequency", "Implement brake pad monitoring system"],
    },
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
    timeline: [
      {
        id: "T-007",
        type: "Created",
        description: "Safety incident reported by driver",
        createdBy: "K. Ahmed",
        createdAt: "2026-08-19T16:20:00Z",
      },
      {
        id: "T-008",
        type: "StatusChange",
        description: "Status changed to Investigating",
        createdBy: "M. Bello",
        createdAt: "2026-08-19T17:00:00Z",
      },
      {
        id: "T-009",
        type: "Assignment",
        description: "Assigned to M. Bello",
        createdBy: "M. Bello",
        createdAt: "2026-08-19T17:00:00Z",
      },
      {
        id: "T-010",
        type: "Evidence",
        description: "Inspection report uploaded",
        createdBy: "M. Bello",
        createdAt: "2026-08-19T17:30:00Z",
      },
      {
        id: "T-011",
        type: "Resolution",
        description: "Repairs completed. Vehicle passed inspection. Incident resolved.",
        createdBy: "M. Bello",
        createdAt: "2026-08-21T11:00:00Z",
      },
    ],
    correctiveActions: [
      {
        id: "CA-004",
        description: "Inspect brake system",
        status: "Completed",
        assignedTo: "M. Bello",
        dueDate: "2026-08-20T09:00:00Z",
        completedAt: "2026-08-20T08:30:00Z",
        verifiedBy: "F. Daniel",
        verifiedAt: "2026-08-21T10:00:00Z",
      },
      {
        id: "CA-005",
        description: "Repair brake pads and rotors",
        status: "Completed",
        assignedTo: "M. Bello",
        dueDate: "2026-08-20T17:00:00Z",
        completedAt: "2026-08-20T16:45:00Z",
        verifiedBy: "F. Daniel",
        verifiedAt: "2026-08-21T10:00:00Z",
      },
      {
        id: "CA-006",
        description: "Pass post-repair inspection",
        status: "Completed",
        assignedTo: "M. Bello",
        dueDate: "2026-08-21T12:00:00Z",
        completedAt: "2026-08-21T10:30:00Z",
        verifiedBy: "F. Daniel",
        verifiedAt: "2026-08-21T10:30:00Z",
      },
    ],
    tags: ["Vehicle", "Brakes", "Maintenance", "Safety"],
    sourceFreshness: "2026-08-21T11:00:00Z",
  },
};

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Accident: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircleIcon className="h-4 w-4" /> },
  NearMiss: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-4 w-4" /> },
  PropertyDamage: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-4 w-4" /> },
  SafetyIncident: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-4 w-4" /> },
  SecurityIncident: { bg: "bg-purple-bg", text: "text-purple", icon: <Shield className="h-4 w-4" /> },
  VehicleIncident: { bg: "bg-blue-bg", text: "text-blue", icon: <Car className="h-4 w-4" /> },
  Other: { bg: "bg-surface-2", text: "text-ink-500", icon: <MoreHorizontal className="h-4 w-4" /> },
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
  Investigating: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  UnderReview: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string }> = {
  Urgent: { bg: "bg-red-bg", text: "text-red" },
  High: { bg: "bg-amber-bg", text: "text-amber" },
  Normal: { bg: "bg-blue-bg", text: "text-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500" },
};

const ACTION_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Verified: { bg: "bg-purple-bg", text: "text-purple", dot: "bg-purple" },
};

export default function IncidentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const incidentId = params.incidentId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<IncidentDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showTimeline, setShowTimeline] = useState(true);
  const [showEvidence, setShowEvidence] = useState(true);
  const [showActions, setShowActions] = useState(true);
  const [showAffected, setShowAffected] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_INCIDENT_DATA[incidentId] || MOCK_INCIDENT_DATA["INC-771"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [incidentId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading incident details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <AlertCircle className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Incident not found</h3>
        <p className="mt-1 text-sm text-ink-500">No incident found with this ID.</p>
        <button
          onClick={() => router.push("/compliance/incidents")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Incidents
        </button>
      </div>
    );
  }

  const typeStyle = TYPE_STYLES[data.type] || TYPE_STYLES.Other;
  const severityStyle = SEVERITY_STYLES[data.severity] || SEVERITY_STYLES.Medium;
  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Open;
  const priorityStyle = PRIORITY_STYLES[data.priority] || PRIORITY_STYLES.Normal;

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
              onClick={() => router.push("/compliance/incidents")}
              className="hover:text-primary transition-colors"
            >
              Incidents
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
            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${priorityStyle.bg} ${priorityStyle.text}`}>
              {data.priority} Priority
            </span>
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Location</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <MapPin className="h-4 w-4 text-ink-400" />
            {data.location}
          </div>
          <div className="mt-0.5 text-xs text-ink-500">{data.branch} branch</div>
        </div>
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Triage</div>
          <div className="mt-1 text-sm font-medium text-ink-950">
            {data.triage?.category || "Not triaged"}
          </div>
          {data.triage?.assignedTo && (
            <div className="mt-0.5 text-xs text-ink-500">
              Assigned to {data.triage.assignedTo} · {formatTime(data.triage.assignedAt || "")}
            </div>
          )}
        </div>
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Investigation</div>
          <div className="mt-1 flex items-center gap-1.5 text-sm font-medium text-ink-950">
            <FileSearch className="h-4 w-4 text-ink-400" />
            {data.investigation?.status || "Not started"}
          </div>
          {data.investigation?.lead && (
            <div className="mt-0.5 text-xs text-ink-500">
              Lead: {data.investigation.lead}
            </div>
          )}
        </div>
      </div>

      {/* ===== DESCRIPTION ===== */}
      <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
        <h3 className="text-[15px] font-semibold text-ink-950">Description</h3>
        <p className="mt-2 text-sm text-ink-700 leading-relaxed">{data.description}</p>
      </div>

      {/* ===== AFFECTED RESOURCES ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowAffected(!showAffected)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-red-bg p-2">
              <Users className="h-4 w-4 text-red" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Affected Resources</span>
              <span className="ml-2 text-sm text-ink-500">
                {data.affectedPeople.length + data.affectedAssets.length + data.affectedTrips.length} items
              </span>
            </div>
          </div>
          {showAffected ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showAffected && (
          <div className="border-t border-line p-4">
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
              {/* People */}
              <div>
                <div className="text-xs font-medium text-ink-500 uppercase tracking-wide">People</div>
                {data.affectedPeople.length === 0 ? (
                  <div className="mt-2 text-sm text-ink-400">None</div>
                ) : (
                  <div className="mt-2 space-y-2">
                    {data.affectedPeople.map((person) => (
                      <div key={person.id} className="rounded-lg border border-line p-2.5">
                        <div className="flex items-center gap-2">
                          <div className="rounded-full bg-blue-bg p-1.5">
                            <User className="h-3.5 w-3.5 text-blue" />
                          </div>
                          <div>
                            <div className="text-xs font-medium text-ink-950">{person.name}</div>
                            <div className="text-[10px] text-ink-500">{person.role}</div>
                          </div>
                        </div>
                        <div className="mt-1 text-[10px] text-ink-500">{person.impact}</div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Assets */}
              <div>
                <div className="text-xs font-medium text-ink-500 uppercase tracking-wide">Assets</div>
                {data.affectedAssets.length === 0 ? (
                  <div className="mt-2 text-sm text-ink-400">None</div>
                ) : (
                  <div className="mt-2 space-y-2">
                    {data.affectedAssets.map((asset) => (
                      <div key={asset.id} className="rounded-lg border border-line p-2.5">
                        <div className="flex items-center gap-2">
                          <div className="rounded-full bg-amber-bg p-1.5">
                            <Car className="h-3.5 w-3.5 text-amber" />
                          </div>
                          <div>
                            <div className="text-xs font-medium text-ink-950">{asset.name}</div>
                            <div className="text-[10px] text-ink-500">{asset.type}</div>
                          </div>
                        </div>
                        <div className="mt-1 text-[10px] text-ink-500">{asset.impact}</div>
                      </div>
                    ))}
                  </div>
                )}
              </div>

              {/* Trips */}
              <div>
                <div className="text-xs font-medium text-ink-500 uppercase tracking-wide">Trips</div>
                {data.affectedTrips.length === 0 ? (
                  <div className="mt-2 text-sm text-ink-400">None</div>
                ) : (
                  <div className="mt-2 space-y-2">
                    {data.affectedTrips.map((trip) => (
                      <div key={trip.id} className="rounded-lg border border-line p-2.5">
                        <div className="flex items-center gap-2">
                          <div className="rounded-full bg-green-bg p-1.5">
                            <BriefcaseIcon className="h-3.5 w-3.5 text-green" />
                          </div>
                          <div>
                            <div className="text-xs font-medium text-ink-950 font-mono">{trip.reference}</div>
                            <div className="text-[10px] text-ink-500">Status: {trip.status}</div>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
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
              <div className="text-center py-6 text-sm text-ink-500">No evidence attached to this incident.</div>
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

      {/* ===== CORRECTIVE ACTIONS ===== */}
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
                        {action.verifiedBy && (
                          <span className="text-[9px] text-purple">Verified by: {action.verifiedBy}</span>
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

      {/* ===== INVESTIGATION ===== */}
      {data.investigation && (
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <h3 className="text-[15px] font-semibold text-ink-950">Investigation</h3>
          <div className="mt-3 grid grid-cols-1 gap-3 sm:grid-cols-2">
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

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          CMP-09 · Incident Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Source: {formatTime(data.sourceFreshness)}
        </span>
      </div>
    </div>
  );
}