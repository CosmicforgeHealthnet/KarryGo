// src/app/(dashboard)/settings/staff/invite/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  UserPlus,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  ArrowLeft,
  Save,
  X,
  User,
  Mail,
  Phone,
  Briefcase,
  Building2,
  Shield,
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
  UserPlus as UserPlusIcon,
  UserMinus,
  HelpCircle,
  Car,
  Truck,
  Package as PackageIcon,
  Warehouse,
  Shield as ShieldIcon,
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
  UserCheck,
  UserX,
  AlertOctagon as AlertOctagonIcon,
} from "lucide-react";

// ==================== TYPES ====================

type InviteFormData = {
  email: string;
  name: string;
  role: string;
  scope: string;
  branch: string;
  department: string;
  message: string;
  sendEmail: boolean;
};

// ==================== CONSTANTS ====================

const ROLES = [
  "Organization Admin",
  "Operations Manager",
  "Fleet Manager",
  "Dispatch Supervisor",
  "Warehouse Manager",
  "Compliance Officer",
  "Finance Manager",
  "HR Manager",
  "Support Staff",
  "Read-only User",
];

const SCOPES = ["Company-wide", "Branch", "Department", "User"];

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

const DEPARTMENTS = ["Executive", "Operations", "Fleet", "Warehouse", "Compliance", "Finance", "HR", "Support"];

export default function InviteStaffPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [formData, setFormData] = useState<InviteFormData>({
    email: "",
    name: "",
    role: "Operations Manager",
    scope: "Branch",
    branch: "Ikeja",
    department: "Operations",
    message: "",
    sendEmail: true,
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const validateForm = () => {
    const newErrors: Record<string, string> = {};
    if (!formData.email.trim()) newErrors.email = "Email is required";
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.email)) {
      newErrors.email = "Invalid email address";
    }
    if (!formData.name.trim()) newErrors.name = "Name is required";
    if (!formData.role) newErrors.role = "Role is required";
    if (formData.scope === "Branch" && !formData.branch) newErrors.branch = "Branch is required";
    if (formData.scope === "Department" && !formData.department) newErrors.department = "Department is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async () => {
    if (!validateForm()) return;

    setIsSubmitting(true);
    setLoading(true);

    await new Promise((resolve) => setTimeout(resolve, 1500));

    setLoading(false);
    setIsSubmitting(false);

    toast("Invitation sent successfully! (mock)");
    router.push("/settings/staff");
  };

  const handleCancel = () => {
    router.push("/settings/staff");
  };

  return (
    <div className="space-y-5 max-w-3xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <div className="flex items-center gap-2 text-sm text-ink-500">
          <button
            onClick={() => router.push("/settings/staff")}
            className="hover:text-primary transition-colors"
          >
            Staff
          </button>
          <span>/</span>
          <span className="text-ink-700 font-medium">Invite Staff</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950">
          Invite Staff
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Send an invitation to a new staff member.
        </p>
      </div>

      {/* ===== FORM ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Email */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Email Address <span className="text-red">*</span>
              </label>
              <input
                type="email"
                value={formData.email}
                onChange={(e) => setFormData((prev) => ({ ...prev, email: e.target.value }))}
                placeholder="colleague@company.com"
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.email ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.email && <p className="mt-1 text-sm text-red">{errors.email}</p>}
            </div>

            {/* Name */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Full Name <span className="text-red">*</span>
              </label>
              <input
                type="text"
                value={formData.name}
                onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                placeholder="e.g. John Doe"
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.name ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.name && <p className="mt-1 text-sm text-red">{errors.name}</p>}
            </div>

            {/* Role */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Role <span className="text-red">*</span>
              </label>
              <select
                value={formData.role}
                onChange={(e) => setFormData((prev) => ({ ...prev, role: e.target.value }))}
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.role ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              >
                {ROLES.map((r) => (
                  <option key={r} value={r}>{r}</option>
                ))}
              </select>
              {errors.role && <p className="mt-1 text-sm text-red">{errors.role}</p>}
            </div>

            {/* Scope */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Scope
              </label>
              <select
                value={formData.scope}
                onChange={(e) => setFormData((prev) => ({ ...prev, scope: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {SCOPES.map((s) => (
                  <option key={s} value={s}>{s}</option>
                ))}
              </select>
            </div>

            {/* Branch */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Branch
              </label>
              <select
                value={formData.branch}
                onChange={(e) => setFormData((prev) => ({ ...prev, branch: e.target.value }))}
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.branch ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              >
                {BRANCHES.map((b) => (
                  <option key={b} value={b}>{b}</option>
                ))}
              </select>
              {errors.branch && <p className="mt-1 text-sm text-red">{errors.branch}</p>}
            </div>

            {/* Department */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Department
              </label>
              <select
                value={formData.department}
                onChange={(e) => setFormData((prev) => ({ ...prev, department: e.target.value }))}
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.department ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              >
                {DEPARTMENTS.map((d) => (
                  <option key={d} value={d}>{d}</option>
                ))}
              </select>
              {errors.department && <p className="mt-1 text-sm text-red">{errors.department}</p>}
            </div>

            {/* Message */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Personal Message
              </label>
              <textarea
                value={formData.message}
                onChange={(e) => setFormData((prev) => ({ ...prev, message: e.target.value }))}
                placeholder="Add a personal message to the invitation..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary resize-none min-h-[80px]"
                rows={3}
              />
            </div>

            {/* Send Email */}
            <div className="sm:col-span-2">
              <label className="flex items-center gap-3 cursor-pointer">
                <input
                  type="checkbox"
                  checked={formData.sendEmail}
                  onChange={(e) => setFormData((prev) => ({ ...prev, sendEmail: e.target.checked }))}
                  className="h-4 w-4 rounded border-line text-primary focus:ring-primary"
                />
                <span className="text-sm text-ink-700">Send invitation email</span>
                <span className="text-xs text-ink-400">(The recipient will receive login instructions)</span>
              </label>
            </div>
          </div>

          {/* Submit Buttons */}
          <div className="mt-6 flex flex-wrap gap-3">
            <button
              onClick={handleSubmit}
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
                  Sending...
                </>
              ) : (
                <>
                  <Send className="h-4 w-4" />
                  Send Invitation
                </>
              )}
            </button>
            <button
              onClick={handleCancel}
              className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <UserPlus className="h-4 w-4" />
          IAM-02 · Invite Staff
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