// src/app/(setup)/setup/verification/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  ArrowLeft,
  X,
  ShieldCheck,
  FileText,
  Building2,
  User,
  Briefcase,
  Wallet,
  FileCheck,
  ListChecks,
  Check,
  Info,
  Tag,
  Calendar,
  Download,
  Printer,
  Edit3,
  Trash2,
  MoreHorizontal,
  Mail,
  Phone,
  MapPin,
  Send,
  Paperclip,
  Link2,
  ExternalLink,
  FolderOpen,
  Lock,
  EyeOff,
  AlertOctagon,
  FileSignature,
  Folder,
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
  TrendingUp,
  TrendingDown,
  Gauge,
  Target,
  Activity,
  BarChart,
  LineChart,
  PieChart,
  Percent,
  Timer,
  Award,
  Sparkles,
  Globe,
  FilterX,
  LayoutGrid,
  LayoutList,
  Sliders,
  ArrowUp,
  ArrowDown,
  Zap,
  Flame,
  Medal,
  Star,
  BookOpen,
  Book,
  DownloadCloud,
  RotateCw,
  Undo2,
  Redo2,
  SkipForward,
  SkipBack,
  FastForward,
  Rewind,
  PlayCircle,
  PauseCircle,
  StopCircle,
  Inbox,
  MailOpen,
  MailX,
  MailCheck,
  MailWarning,
  Archive,
  ArchiveRestore,
  Trash,
  Trash2 as TrashIcon,
  Server,
  Cpu,
  Home,
  Settings as SettingsIcon,
  UserPlus,
  UserMinus,
  HelpCircle,
  Car,
  Truck,
  Package as PackageIcon,
  Warehouse,
  Shield,
  Users,
  Building2 as Building2Icon,
  MapPin as MapPinIcon,
  HardDrive,
  Database,
  Cloud,
  Ship,
  Plane,
  Train,
  Bike,
  Save,
} from "lucide-react";

// ==================== TYPES ====================

type VerificationItem = {
  id: string;
  name: string;
  status: "Pending" | "InProgress" | "Verified" | "Rejected" | "NotStarted";
  description: string;
  required: boolean;
  updatedAt: string;
  notes: string | null;
};

type VerificationState = {
  status: "Pending" | "InProgress" | "Verified" | "Rejected" | "NotStarted";
  submittedAt: string | null;
  reviewedAt: string | null;
  reviewer: string | null;
  overallNotes: string | null;
  items: VerificationItem[];
};

// ==================== MOCK DATA ====================

const MOCK_VERIFICATION: VerificationState = {
  status: "Pending",
  submittedAt: "2026-08-22T10:00:00Z",
  reviewedAt: null,
  reviewer: null,
  overallNotes: "Awaiting document verification.",
  items: [
    {
      id: "V-001",
      name: "Business Registration",
      status: "Verified",
      description: "Company registration certificate verification.",
      required: true,
      updatedAt: "2026-08-22T10:30:00Z",
      notes: "Verified against government database.",
    },
    {
      id: "V-002",
      name: "Tax ID Verification",
      status: "Pending",
      description: "Tax identification number verification.",
      required: true,
      updatedAt: "2026-08-22T09:00:00Z",
      notes: "Awaiting verification.",
    },
    {
      id: "V-003",
      name: "Operating License",
      status: "Rejected",
      description: "Business operating license verification.",
      required: true,
      updatedAt: "2026-08-22T08:00:00Z",
      notes: "License number mismatch. Please re-upload document.",
    },
    {
      id: "V-004",
      name: "Insurance Policy",
      status: "NotStarted",
      description: "Business insurance policy verification.",
      required: false,
      updatedAt: "2026-08-21T14:30:00Z",
      notes: "Not yet verified.",
    },
    {
      id: "V-005",
      name: "Director Verification",
      status: "InProgress",
      description: "Director identity and background verification.",
      required: true,
      updatedAt: "2026-08-22T11:00:00Z",
      notes: "Background check in progress.",
    },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-4 w-4" /> },
  NotStarted: { bg: "bg-surface-2", text: "text-ink-500", icon: <div className="h-4 w-4" /> },
};

const OVERALL_STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-5 w-5" /> },
  InProgress: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-5 w-5" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-5 w-5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-5 w-5" /> },
  NotStarted: { bg: "bg-surface-2", text: "text-ink-500", icon: <div className="h-5 w-5" /> },
};

export default function VerificationPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<VerificationState | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_VERIFICATION);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleResubmit = async () => {
    setIsSubmitting(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setIsSubmitting(false);
    toast("Verification resubmitted successfully! (mock)");
    router.push("/setup/verification");
  };

  const handleBack = () => {
    router.push("/setup");
  };

  const handleNext = () => {
    router.push("/setup/wallet-provisioning");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading verification status...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <ShieldCheck className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No verification data found</h3>
        <p className="mt-1 text-sm text-ink-500">Unable to load verification status.</p>
        <button
          onClick={() => router.push("/setup")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Setup
        </button>
      </div>
    );
  }

  const overallStatusStyle = OVERALL_STATUS_STYLES[data.status] || OVERALL_STATUS_STYLES.NotStarted;

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

  const isComplete = data.status === "Verified";
  const isRejected = data.status === "Rejected";

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <div className="flex items-center gap-2 text-sm text-ink-500">
          <button onClick={handleBack} className="hover:text-primary transition-colors">
            Onboarding
          </button>
          <span>/</span>
          <span className="text-ink-700 font-medium">Business Verification</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Business Verification
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Track the verification status of your company and documents.
        </p>
      </div>

      {/* ===== OVERALL STATUS ===== */}
      <div className={`rounded-2xl border p-5 shadow-sm ${
        isComplete ? "border-green/20 bg-green-bg/30" :
        isRejected ? "border-red/20 bg-red-bg/30" :
        "border-amber/20 bg-amber-bg/30"
      }`}>
        <div className="flex items-start gap-4">
          <div className={`rounded-full p-3 flex-shrink-0 ${
            isComplete ? "bg-green-bg" :
            isRejected ? "bg-red-bg" :
            "bg-amber-bg"
          }`}>
            {overallStatusStyle.icon}
          </div>
          <div className="flex-1">
            <div className="flex items-center gap-3">
              <span className={`text-lg font-semibold ${
                isComplete ? "text-green" :
                isRejected ? "text-red" :
                "text-amber"
              }`}>
                {data.status}
              </span>
              {data.submittedAt && (
                <span className="text-xs text-ink-500">
                  Submitted {formatTime(data.submittedAt)}
                </span>
              )}
            </div>
            {data.overallNotes && (
              <p className="mt-1 text-sm text-ink-600">{data.overallNotes}</p>
            )}
            {data.reviewedAt && data.reviewer && (
              <p className="mt-1 text-xs text-ink-500">
                Reviewed {formatTime(data.reviewedAt)} by {data.reviewer}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* ===== VERIFICATION ITEMS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <ListChecks className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Verification Items</span>
            <span className="text-xs text-ink-400">
              {data.items.filter((i) => i.status === "Verified").length}/{data.items.length} complete
            </span>
          </div>
        </div>
        <div className="divide-y divide-line">
          {data.items.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.NotStarted;

            return (
              <div key={item.id} className="flex items-center gap-4 p-4">
                <div className="flex-shrink-0">
                  <div className={`rounded-full p-2 ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                  </div>
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium text-ink-950">{item.name}</span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                      {item.status}
                    </span>
                    {item.required && (
                      <span className="text-[9px] text-red font-medium">Required</span>
                    )}
                  </div>
                  <div className="mt-0.5 text-xs text-ink-500">{item.description}</div>
                  {item.notes && (
                    <div className="mt-0.5 text-xs text-ink-400">{item.notes}</div>
                  )}
                  <div className="mt-0.5 text-[10px] text-ink-400">
                    Updated {formatTime(item.updatedAt)}
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {/* ===== ACTIONS ===== */}
      <div className="flex flex-wrap gap-3">
        {isRejected && (
          <button
            onClick={handleResubmit}
            disabled={isSubmitting}
            className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
              isSubmitting
                ? "bg-ink-400 cursor-not-allowed"
                : "bg-primary shadow-primary/20 hover:bg-primary/90"
            }`}
          >
            {isSubmitting ? (
              <>
                <RefreshCw className="h-4 w-4 animate-spin" />
                Resubmitting...
              </>
            ) : (
              <>
                <FileCheck className="h-4 w-4" />
                Resubmit for Review
              </>
            )}
          </button>
        )}
        {isComplete && (
          <button
            onClick={handleNext}
            className="flex items-center gap-2 rounded-xl bg-green px-6 py-2.5 text-sm font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90"
          >
            Next Step <ArrowRight className="h-4 w-4" />
          </button>
        )}
        <button
          onClick={handleBack}
          className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
        >
          Back
        </button>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <ShieldCheck className="h-4 w-4" />
          ORG-06 · Business Verification
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}