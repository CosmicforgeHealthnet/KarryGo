// src/app/(dashboard)/workforce/[personId]/edit/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  User,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Save,
  RefreshCw,
  Calendar,
  Activity,
  Shield,
  FileText,
  Briefcase,
  Users,
  ArrowRight,
  MoreVertical,
  Sparkles,
  Phone,
  Mail,
  Building,
  UserCheck,
  UserX,
  History,
  Edit3,
  ChevronDown,
  X,
  Plus,
  Trash2,
} from "lucide-react";

type StaffAccess = {
  id: string;
  name: string;
  initials: string;
  role: string;
  branch: string;
  status: "Active" | "Suspended" | "Inactive";
  scope: "Company" | "Branch" | "Department";
  permissions: string[];
  assignedRoles: {
    id: string;
    name: string;
    scope: string;
  }[];
  history: {
    id: string;
    action: string;
    by: string;
    timestamp: string;
  }[];
  phone: string;
  email: string;
  joinedAt: string;
  lastActive: string;
};

// Mock data
const STAFF_ACCESS: StaffAccess = {
  id: "P-044",
  name: "Peter Okafor",
  initials: "PO",
  role: "Dispatch Rider",
  branch: "Ikeja",
  status: "Active",
  scope: "Branch",
  permissions: [
    "View assigned trips",
    "Accept/Decline assignments",
    "View route details",
    "Update trip status",
    "View own profile",
  ],
  assignedRoles: [
    { id: "role-001", name: "Dispatch Rider", scope: "Ikeja Branch" },
    { id: "role-002", name: "Provider", scope: "Company" },
  ],
  history: [
    { id: "hist-001", action: "Role assigned: Dispatch Rider", by: "T. Adeyemi", timestamp: "2026-08-15T10:30:00Z" },
    { id: "hist-002", action: "Branch access updated: Ikeja", by: "F. Daniel", timestamp: "2026-08-10T14:20:00Z" },
    { id: "hist-003", action: "Account activated", by: "System", timestamp: "2026-08-01T09:00:00Z" },
    { id: "hist-004", action: "Invitation sent", by: "M. Bello", timestamp: "2026-07-28T11:15:00Z" },
  ],
  phone: "+234 802 345 6789",
  email: "peter.okafor@example.com",
  joinedAt: "Jan 2024",
  lastActive: "12 sec ago",
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Suspended: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-4 w-4" />,
  },
  Inactive: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <AlertCircle className="h-4 w-4" />,
  },
};

const SCOPE_OPTIONS = ["Company", "Branch", "Department"];
const BRANCH_OPTIONS = ["Ikeja", "Lekki", "Yaba", "Apapa"];
const ROLE_OPTIONS = [
  "Dispatch Rider",
  "Hauling Driver",
  "Dispatch Supervisor",
  "Fleet Manager",
  "Warehouse Supervisor",
  "Maintenance Tech",
  "Provider",
];

export default function StaffAccessEditPage() {
  const params = useParams();
  const router = useRouter();
  const personId = params.personId as string;

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [staff, setStaff] = useState<StaffAccess | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);

  // Form state
  const [formData, setFormData] = useState({
    role: "",
    branch: "",
    scope: "",
    status: "",
  });

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setStaff(STAFF_ACCESS);
      setFormData({
        role: STAFF_ACCESS.role,
        branch: STAFF_ACCESS.branch,
        scope: STAFF_ACCESS.scope,
        status: STAFF_ACCESS.status,
      });
      setLoading(false);
    };
    loadData();
  }, [personId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  // Navigation handlers
  const goBack = () => {
    router.push(`/workforce/${personId}`);
  };

  const goToDirectory = () => {
    router.push("/workforce");
  };

  const handleSave = async () => {
    setSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 1200));
    setSaving(false);
    setEditMode(false);
    // In production, this would save to the backend
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading access details...</span>
        </div>
      </div>
    );
  }

  if (!staff) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <User className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Staff not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The staff member you're looking for doesn't exist.
          </p>
          <button
            onClick={goToDirectory}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to directory
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[staff.status] || STATUS_STYLES.Active;

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <button
              onClick={goBack}
              className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-white/70 text-ink-600 transition hover:bg-white hover:text-ink-900"
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <div>
              <div className="flex items-center gap-2.5">
                <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                  Staff Access
                </h1>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                  {statusStyle.icon}
                  {staff.status}
                </span>
              </div>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {staff.name} · {staff.id} · {staff.role}
              </p>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          {!editMode ? (
            <button
              onClick={() => setEditMode(true)}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit Access
            </button>
          ) : (
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex h-9 items-center gap-1.5 rounded-xl bg-green px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-green/20 transition hover:bg-green/90 disabled:opacity-60"
            >
              {saving ? (
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
              ) : (
                <Save className="h-3.5 w-3.5" />
              )}
              Save Changes
            </button>
          )}
        </div>
      </div>

      {/* ===== EDIT MODE BANNER ===== */}
      {editMode && (
        <div className="rounded-xl border border-primary/30 bg-primary-bg/30 p-4">
          <div className="flex items-center gap-3">
            <Edit3 className="h-5 w-5 text-primary" />
            <div>
              <div className="text-[13px] font-semibold text-ink-950">Edit Mode</div>
              <p className="text-[12px] text-ink-600">
                Update role, branch, scope, or status. Changes will be logged in the access history.
              </p>
            </div>
            <button
              onClick={() => setEditMode(false)}
              className="ml-auto rounded-lg border border-line bg-white/70 px-3 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* LEFT COLUMN */}
        <div className="space-y-5">
          {/* Access Details */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-4">
              <Shield className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Access Details</h3>
            </div>

            <div className="space-y-4">
              {/* Role */}
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Role</div>
                {editMode ? (
                  <div className="relative mt-1">
                    <select
                      value={formData.role}
                      onChange={(e) => setFormData((prev) => ({ ...prev, role: e.target.value }))}
                      className="w-full appearance-none rounded-xl border border-line px-3.5 py-2.5 text-[13px] text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    >
                      {ROLE_OPTIONS.map((role) => (
                        <option key={role} value={role}>
                          {role}
                        </option>
                      ))}
                    </select>
                    <ChevronDown className="absolute right-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400 pointer-events-none" />
                  </div>
                ) : (
                  <div className="mt-1 text-[14px] font-semibold text-ink-950">{staff.role}</div>
                )}
              </div>

              {/* Branch */}
              <div className="border-t border-line pt-4">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
                {editMode ? (
                  <div className="relative mt-1">
                    <select
                      value={formData.branch}
                      onChange={(e) => setFormData((prev) => ({ ...prev, branch: e.target.value }))}
                      className="w-full appearance-none rounded-xl border border-line px-3.5 py-2.5 text-[13px] text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    >
                      {BRANCH_OPTIONS.map((branch) => (
                        <option key={branch} value={branch}>
                          {branch}
                        </option>
                      ))}
                    </select>
                    <ChevronDown className="absolute right-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400 pointer-events-none" />
                  </div>
                ) : (
                  <div className="mt-1 flex items-center gap-1.5 text-[14px] font-semibold text-ink-950">
                    <MapPin className="h-4 w-4 text-ink-400" />
                    {staff.branch}
                  </div>
                )}
              </div>

              {/* Scope */}
              <div className="border-t border-line pt-4">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Scope</div>
                {editMode ? (
                  <div className="relative mt-1">
                    <select
                      value={formData.scope}
                      onChange={(e) => setFormData((prev) => ({ ...prev, scope: e.target.value }))}
                      className="w-full appearance-none rounded-xl border border-line px-3.5 py-2.5 text-[13px] text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    >
                      {SCOPE_OPTIONS.map((scope) => (
                        <option key={scope} value={scope}>
                          {scope}
                        </option>
                      ))}
                    </select>
                    <ChevronDown className="absolute right-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400 pointer-events-none" />
                  </div>
                ) : (
                  <div className="mt-1 text-[14px] font-semibold text-ink-950">{staff.scope}</div>
                )}
              </div>

              {/* Status */}
              <div className="border-t border-line pt-4">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
                {editMode ? (
                  <div className="relative mt-1">
                    <select
                      value={formData.status}
                      onChange={(e) => setFormData((prev) => ({ ...prev, status: e.target.value }))}
                      className="w-full appearance-none rounded-xl border border-line px-3.5 py-2.5 text-[13px] text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    >
                      <option value="Active">Active</option>
                      <option value="Suspended">Suspended</option>
                      <option value="Inactive">Inactive</option>
                    </select>
                    <ChevronDown className="absolute right-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400 pointer-events-none" />
                  </div>
                ) : (
                  <div className="mt-1 text-[14px] font-semibold text-ink-950">{staff.status}</div>
                )}
              </div>
            </div>
          </div>

          {/* Permissions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-4">
              <CheckCircle2 className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Effective Permissions</h3>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {staff.permissions.length}
              </span>
            </div>
            <div className="space-y-2">
              {staff.permissions.map((perm, index) => (
                <div key={index} className="flex items-center gap-2.5 rounded-lg border border-line p-2.5">
                  <CheckCircle2 className="h-4 w-4 text-green" />
                  <span className="text-[13px] text-ink-700">{perm}</span>
                </div>
              ))}
            </div>
            {editMode && (
              <button className="mt-3 flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-line py-2.5 text-[12px] font-medium text-ink-500 hover:border-primary hover:text-primary transition-colors">
                <Plus className="h-4 w-4" />
                Add Permission
              </button>
            )}
          </div>
        </div>

        {/* RIGHT COLUMN */}
        <div className="space-y-5">
          {/* Assigned Roles */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-4">
              <Briefcase className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Assigned Roles</h3>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {staff.assignedRoles.length}
              </span>
            </div>
            <div className="space-y-2.5">
              {staff.assignedRoles.map((role) => (
                <div key={role.id} className="flex items-center justify-between rounded-lg border border-line p-3">
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">{role.name}</div>
                    <div className="text-[11px] text-ink-500">Scope: {role.scope}</div>
                  </div>
                  {editMode && (
                    <button className="rounded-lg p-1 text-ink-400 hover:text-red transition-colors">
                      <Trash2 className="h-4 w-4" />
                    </button>
                  )}
                </div>
              ))}
            </div>
            {editMode && (
              <button className="mt-3 flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-line py-2.5 text-[12px] font-medium text-ink-500 hover:border-primary hover:text-primary transition-colors">
                <Plus className="h-4 w-4" />
                Assign Role
              </button>
            )}
          </div>

          {/* Access History */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-4">
              <History className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Access History</h3>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {staff.history.length}
              </span>
            </div>
            <div className="space-y-3">
              {staff.history.map((event, index) => (
                <div key={event.id} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className={`h-2 w-2 rounded-full ${
                      index === 0 ? "bg-primary" : "bg-ink-300"
                    }`} />
                    {index < staff.history.length - 1 && (
                      <div className="h-6 w-px bg-line" />
                    )}
                  </div>
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">{event.action}</div>
                    <div className="text-[11px] text-ink-500">
                      {event.by} · {new Date(event.timestamp).toLocaleString()}
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Profile Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <User className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Profile</h3>
            </div>
            <div className="space-y-2 text-[13px]">
              <div className="flex items-center gap-2">
                <Phone className="h-4 w-4 text-ink-400" />
                <span className="text-ink-700">{staff.phone}</span>
              </div>
              <div className="flex items-center gap-2">
                <Mail className="h-4 w-4 text-ink-400" />
                <span className="text-ink-700">{staff.email}</span>
              </div>
              <div className="flex items-center gap-2">
                <Calendar className="h-4 w-4 text-ink-400" />
                <span className="text-ink-700">Joined: {staff.joinedAt}</span>
              </div>
              <div className="flex items-center gap-2">
                <Clock className="h-4 w-4 text-ink-400" />
                <span className="text-ink-700">Last active: {staff.lastActive}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== NAVIGATION FOOTER ===== */}
      <div className="flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:justify-between">
        <button
          onClick={goBack}
          className="flex items-center gap-1.5 text-[12px] font-medium text-ink-500 hover:text-ink-900 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to profile
        </button>
        <button
          onClick={goToDirectory}
          className="flex items-center gap-1.5 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
        >
          View All Staff
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}