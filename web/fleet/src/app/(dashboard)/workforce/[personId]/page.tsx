// src/app/(dashboard)/workforce/[personId]/page.tsx

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
  Edit3,
  RefreshCw,
  Calendar,
  Activity,
  Shield,
  FileText,
  Briefcase,
  Users,
  ArrowRight,
  MoreVertical,
  Gauge,
  Wrench,
  Zap,
  Award,
  TrendingUp,
  TrendingDown,
  Circle,
  Sparkles,
  Car,
  Bike,
  Truck,
  GraduationCap,
  CalendarDays,
  Phone,
  Mail,
  MessageSquare,
  Star,
  Clock as ClockIcon,
  Building,
  UserCheck,
  UserX,
} from "lucide-react";

type PersonDetail = {
  id: string;
  initials: string;
  name: string;
  relationship: "Employee" | "Provider" | "Contractor";
  role: string;
  branch: string;
  status: "Active" | "On Leave" | "Suspended" | "Inactive";
  eligibility: "Eligible" | "Warning" | "Blocked";
  phone: string;
  email: string;
  joinedAt: string;
  lastActive: string;
  currentShift: string | null;
  supervisor: string;
  rating: number;
  completedJobs: number;
  attendanceRate: number;
  qualifications: {
    id: string;
    name: string;
    status: "Verified" | "Expiring" | "Missing";
    expiry: string;
  }[];
  shifts: {
    id: string;
    date: string;
    time: string;
    status: "Scheduled" | "Completed" | "Missed";
  }[];
  leaveHistory: {
    id: string;
    type: string;
    startDate: string;
    endDate: string;
    status: "Approved" | "Pending" | "Rejected";
  }[];
};

// Mock data
const PERSON_DETAIL: PersonDetail = {
  id: "P-044",
  initials: "PO",
  name: "Peter Okafor",
  relationship: "Provider",
  role: "Dispatch Rider",
  branch: "Ikeja",
  status: "Active",
  eligibility: "Eligible",
  phone: "+234 802 345 6789",
  email: "peter.okafor@example.com",
  joinedAt: "Jan 2024",
  lastActive: "12 sec ago",
  currentShift: "06:00 - 10:00",
  supervisor: "T. Adeyemi",
  rating: 4.8,
  completedJobs: 342,
  attendanceRate: 97,
  qualifications: [
    { id: "q-001", name: "Rider Identity Verification", status: "Verified", expiry: "—" },
    { id: "q-002", name: "Defensive Riding Training", status: "Expiring", expiry: "In 6 days" },
    { id: "q-003", name: "Vehicle Roadworthiness", status: "Verified", expiry: "—" },
    { id: "q-004", name: "Background Check", status: "Verified", expiry: "—" },
  ],
  shifts: [
    { id: "sh-001", date: "2026-08-17", time: "06:00 - 10:00", status: "Completed" },
    { id: "sh-002", date: "2026-08-18", time: "06:00 - 10:00", status: "Scheduled" },
    { id: "sh-003", date: "2026-08-19", time: "06:00 - 10:00", status: "Scheduled" },
    { id: "sh-004", date: "2026-08-16", time: "06:00 - 10:00", status: "Missed" },
  ],
  leaveHistory: [
    { id: "lv-001", type: "Annual Leave", startDate: "2026-07-10", endDate: "2026-07-12", status: "Approved" },
    { id: "lv-002", type: "Sick Leave", startDate: "2026-06-05", endDate: "2026-06-06", status: "Approved" },
    { id: "lv-003", type: "Annual Leave", startDate: "2026-09-01", endDate: "2026-09-03", status: "Pending" },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  "On Leave": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-4 w-4" />,
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

const ELIGIBILITY_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Eligible: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Warning: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Blocked: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const QUALIFICATION_STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Verified: {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Expiring: {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Missing: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
};

const SHIFT_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Scheduled: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Missed: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const LEAVE_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Approved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Rejected: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function PersonProfilePage() {
  const params = useParams();
  const router = useRouter();
  const personId = params.personId as string;

  const [loading, setLoading] = useState(true);
  const [person, setPerson] = useState<PersonDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [activeTab, setActiveTab] = useState<"overview" | "shifts" | "leave" | "qualifications">("overview");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setPerson(PERSON_DETAIL);
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
    router.push("/workforce");
  };

  const goToEdit = () => {
    router.push(`/workforce/${personId}/edit`);
  };

  const goToShifts = () => {
    router.push("/workforce/shifts");
  };

  const goToLeave = () => {
    router.push("/workforce/leave");
  };

  const goToQualifications = () => {
    router.push("/workforce/qualifications");
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading profile...</span>
        </div>
      </div>
    );
  }

  if (!person) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <User className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Person not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The person you're looking for doesn't exist or has been removed.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to directory
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[person.status] || STATUS_STYLES.Active;
  const eligibilityStyle = ELIGIBILITY_STYLES[person.eligibility] || ELIGIBILITY_STYLES.Eligible;

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
                  {person.name}
                </h1>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                  {statusStyle.icon}
                  {person.status}
                </span>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${eligibilityStyle.bg} ${eligibilityStyle.text}`}>
                  <span className={`h-1.5 w-1.5 rounded-full ${eligibilityStyle.dot}`} />
                  {person.eligibility}
                </span>
              </div>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {person.role} · {person.branch} branch · ID: {person.id}
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
          <button
            onClick={goToEdit}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
        </div>
      </div>

      {/* ===== STATS ROW ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-primary-bg p-1.5">
              <Star className="h-4 w-4 text-primary fill-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Rating</div>
              <div className="text-[16px] font-semibold text-ink-950">{person.rating}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-green-bg p-1.5">
              <Briefcase className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Jobs Completed</div>
              <div className="text-[16px] font-semibold text-ink-950">{person.completedJobs}</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-blue-bg p-1.5">
              <CalendarDays className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Attendance</div>
              <div className="text-[16px] font-semibold text-ink-950">{person.attendanceRate}%</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-amber-bg p-1.5">
              <Clock className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Last Active</div>
              <div className="text-[16px] font-semibold text-ink-950">{person.lastActive}</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== TABS ===== */}
      <div className="flex gap-1 border-b border-line overflow-x-auto scrollbar-hide">
        {[
          { key: "overview", label: "Overview", icon: User },
          { key: "shifts", label: "Shifts", icon: Clock },
          { key: "leave", label: "Leave", icon: CalendarDays },
          { key: "qualifications", label: "Qualifications", icon: GraduationCap },
        ].map((tab) => {
          const isActive = activeTab === tab.key as any;
          const Icon = tab.icon;
          return (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key as any)}
              className={`flex items-center gap-1.5 px-3.5 py-2.5 text-[12.5px] font-medium transition whitespace-nowrap border-b-2 ${
                isActive
                  ? "text-primary border-primary"
                  : "text-ink-600 hover:text-ink-950 border-transparent hover:border-ink-300"
              }`}
            >
              <Icon className="h-4 w-4" />
              {tab.label}
            </button>
          );
        })}
      </div>

      {/* ===== TAB CONTENT ===== */}
      <div>
        {/* OVERVIEW TAB */}
        {activeTab === "overview" && (
          <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
            {/* LEFT COLUMN */}
            <div className="space-y-5">
              {/* Profile Details */}
              <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
                <div className="flex items-center gap-2.5 mb-3">
                  <User className="h-5 w-5 text-ink-400" />
                  <h3 className="text-[15px] font-semibold text-ink-950">Profile Details</h3>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Full Name</div>
                    <div className="mt-0.5 text-[13px] text-ink-950">{person.name}</div>
                  </div>
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">ID</div>
                    <div className="mt-0.5 font-mono text-[13px] text-ink-950">{person.id}</div>
                  </div>
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Role</div>
                    <div className="mt-0.5 text-[13px] text-ink-950">{person.role}</div>
                  </div>
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Relationship</div>
                    <div className="mt-0.5 text-[13px] text-ink-950">{person.relationship}</div>
                  </div>
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
                    <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-ink-950">
                      <MapPin className="h-4 w-4 text-ink-400" />
                      {person.branch}
                    </div>
                  </div>
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Supervisor</div>
                    <div className="mt-0.5 text-[13px] text-ink-950">{person.supervisor}</div>
                  </div>
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Phone</div>
                    <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-ink-950">
                      <Phone className="h-4 w-4 text-ink-400" />
                      {person.phone}
                    </div>
                  </div>
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Email</div>
                    <div className="mt-0.5 flex items-center gap-1.5 text-[13px] text-ink-950">
                      <Mail className="h-4 w-4 text-ink-400" />
                      {person.email}
                    </div>
                  </div>
                </div>
              </div>

              {/* Current Status */}
              <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
                <div className="flex items-center gap-2.5 mb-3">
                  <Activity className="h-5 w-5 text-ink-400" />
                  <h3 className="text-[15px] font-semibold text-ink-950">Current Status</h3>
                </div>
                <div className="space-y-3">
                  <div className="flex items-center justify-between text-[13px] border-b border-line pb-2">
                    <span className="text-ink-500">Status</span>
                    <span className={`font-semibold ${statusStyle.text}`}>{person.status}</span>
                  </div>
                  <div className="flex items-center justify-between text-[13px] border-b border-line pb-2">
                    <span className="text-ink-500">Eligibility</span>
                    <span className={`font-semibold ${eligibilityStyle.text}`}>{person.eligibility}</span>
                  </div>
                  <div className="flex items-center justify-between text-[13px]">
                    <span className="text-ink-500">Current Shift</span>
                    <span className="font-medium text-ink-950">{person.currentShift || "Not assigned"}</span>
                  </div>
                </div>
              </div>
            </div>

            {/* RIGHT COLUMN */}
            <div className="space-y-5">
              {/* Quick Actions */}
              <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
                <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Actions</h3>
                <div className="space-y-2">
                  <button
                    onClick={goToShifts}
                    className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                  >
                    <span className="flex items-center gap-2">
                      <Clock className="h-4 w-4" />
                      View Shifts
                    </span>
                    <ArrowRight className="h-4 w-4 text-ink-400" />
                  </button>
                  <button
                    onClick={goToLeave}
                    className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                  >
                    <span className="flex items-center gap-2">
                      <CalendarDays className="h-4 w-4" />
                      View Leave
                    </span>
                    <ArrowRight className="h-4 w-4 text-ink-400" />
                  </button>
                  <button
                    onClick={goToQualifications}
                    className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                  >
                    <span className="flex items-center gap-2">
                      <GraduationCap className="h-4 w-4" />
                      View Qualifications
                    </span>
                    <ArrowRight className="h-4 w-4 text-ink-400" />
                  </button>
                  <button
  onClick={() => router.push("/workforce/training")}
  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
>
  <span className="flex items-center gap-2">
    <GraduationCap className="h-4 w-4" />
    View Training
  </span>
  <ArrowRight className="h-4 w-4 text-ink-400" />
</button>

<button
  onClick={() => router.push("/workforce/performance")}
  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
>
  <span className="flex items-center gap-2">
    <Activity className="h-4 w-4" />
    View Performance
  </span>
  <ArrowRight className="h-4 w-4 text-ink-400" />
</button>
                </div>
              </div>

              {/* Contact */}
              <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
                <div className="flex items-center gap-2.5 mb-3">
                  <MessageSquare className="h-5 w-5 text-ink-400" />
                  <h3 className="text-[15px] font-semibold text-ink-950">Contact</h3>
                </div>
                <div className="flex flex-wrap gap-2">
                  <button className="flex items-center gap-2 rounded-xl border border-line bg-white/70 px-4 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white">
                    <Phone className="h-4 w-4" />
                    Call
                  </button>
                  <button className="flex items-center gap-2 rounded-xl border border-line bg-white/70 px-4 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white">
                    <MessageSquare className="h-4 w-4" />
                    Message
                  </button>
                  <button className="flex items-center gap-2 rounded-xl border border-line bg-white/70 px-4 py-2 text-[12px] font-medium text-ink-700 transition hover:bg-white">
                    <Mail className="h-4 w-4" />
                    Email
                  </button>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* SHIFTS TAB */}
        {activeTab === "shifts" && (
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2.5">
                <Clock className="h-5 w-5 text-ink-400" />
                <h3 className="text-[15px] font-semibold text-ink-950">Shift History</h3>
                <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                  {person.shifts.length}
                </span>
              </div>
              <button
                onClick={goToShifts}
                className="text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
              >
                View All →
              </button>
            </div>
            <div className="space-y-2.5">
              {person.shifts.map((shift) => {
                const shiftStyle = SHIFT_STATUS_STYLES[shift.status] || SHIFT_STATUS_STYLES.Scheduled;
                return (
                  <div key={shift.id} className="flex items-center justify-between rounded-lg border border-line p-3 hover:bg-surface-2 transition-colors">
                    <div>
                      <div className="text-[13px] font-medium text-ink-950">{shift.date}</div>
                      <div className="text-[12px] text-ink-500">{shift.time}</div>
                    </div>
                    <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${shiftStyle.bg} ${shiftStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${shiftStyle.dot}`} />
                      {shift.status}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* LEAVE TAB */}
        {activeTab === "leave" && (
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2.5">
                <CalendarDays className="h-5 w-5 text-ink-400" />
                <h3 className="text-[15px] font-semibold text-ink-950">Leave History</h3>
                <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                  {person.leaveHistory.length}
                </span>
              </div>
              <button
                onClick={goToLeave}
                className="text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
              >
                View All →
              </button>
            </div>
            <div className="space-y-2.5">
              {person.leaveHistory.map((leave) => {
                const leaveStyle = LEAVE_STATUS_STYLES[leave.status] || LEAVE_STATUS_STYLES.Pending;
                return (
                  <div key={leave.id} className="flex items-center justify-between rounded-lg border border-line p-3 hover:bg-surface-2 transition-colors">
                    <div>
                      <div className="text-[13px] font-medium text-ink-950">{leave.type}</div>
                      <div className="text-[12px] text-ink-500">{leave.startDate} → {leave.endDate}</div>
                    </div>
                    <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${leaveStyle.bg} ${leaveStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${leaveStyle.dot}`} />
                      {leave.status}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* QUALIFICATIONS TAB */}
        {activeTab === "qualifications" && (
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-4">
              <div className="flex items-center gap-2.5">
                <GraduationCap className="h-5 w-5 text-ink-400" />
                <h3 className="text-[15px] font-semibold text-ink-950">Qualifications</h3>
                <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                  {person.qualifications.length}
                </span>
              </div>
              <button
                onClick={goToQualifications}
                className="text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
              >
                View All →
              </button>
            </div>
            <div className="space-y-2.5">
              {person.qualifications.map((qual) => {
                const qualStyle = QUALIFICATION_STATUS_STYLES[qual.status] || QUALIFICATION_STATUS_STYLES.Verified;
                return (
                  <div key={qual.id} className="flex items-center justify-between rounded-lg border border-line p-3 hover:bg-surface-2 transition-colors">
                    <div>
                      <div className="text-[13px] font-medium text-ink-950">{qual.name}</div>
                      {qual.expiry && qual.expiry !== "—" && (
                        <div className="text-[11px] text-ink-500">Expires: {qual.expiry}</div>
                      )}
                    </div>
                    <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${qualStyle.bg} ${qualStyle.text}`}>
                      {qualStyle.icon}
                      {qual.status}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        )}
      </div>

      {/* ===== NAVIGATION FOOTER ===== */}
      <div className="flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:justify-between">
        <button
          onClick={goBack}
          className="flex items-center gap-1.5 text-[12px] font-medium text-ink-500 hover:text-ink-900 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to directory
        </button>
        <button
          onClick={goToEdit}
          className="flex items-center gap-1.5 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
        >
          Edit Profile
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}