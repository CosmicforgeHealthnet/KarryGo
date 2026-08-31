// src/app/(dashboard)/workforce/shifts/[shiftId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Calendar,
  Clock,
  MapPin,
  Users,
  Briefcase,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Edit3,
  RefreshCw,
  User,
  UserPlus,
  UserX,
  Send,
  X,
  MoreVertical,
  ArrowRight,
  Sparkles,
  Activity,
  Sun,
  Moon,
  Cloud,
  TrendingUp,
  FileText,
  UserCheck,
  Eye,
} from "lucide-react";

type ShiftDetail = {
  id: string;
  date: string;
  dayName: string;
  time: string;
  branch: string;
  role: string;
  capacity: number;
  assigned: number;
  status: "Scheduled" | "In Progress" | "Completed" | "Cancelled";
  people: {
    id: string;
    name: string;
    initials: string;
    status: "Confirmed" | "Pending" | "Absent";
  }[];
  createdBy: string;
  createdAt: string;
  notes: string;
};

type AttendanceRecord = {
  id: string;
  personId: string;
  name: string;
  initials: string;
  checkIn: string | null;
  checkOut: string | null;
  status: "Present" | "Late" | "Absent" | "Excused";
  hours: string;
};

// Mock data
const SHIFT_DETAIL: ShiftDetail = {
  id: "SH-001",
  date: "2026-08-20",
  dayName: "Today",
  time: "06:00 - 10:00",
  branch: "Ikeja",
  role: "Dispatch Rider",
  capacity: 8,
  assigned: 6,
  status: "Scheduled",
  people: [
    { id: "P-044", name: "Peter Okafor", initials: "PO", status: "Confirmed" },
    { id: "P-061", name: "Chinedu Uche", initials: "CU", status: "Confirmed" },
    { id: "P-073", name: "Esther Obi", initials: "EO", status: "Pending" },
    { id: "P-052", name: "Amina Yusuf", initials: "AY", status: "Confirmed" },
    { id: "P-038", name: "Musa Bello", initials: "MB", status: "Pending" },
    { id: "P-088", name: "Tunde Fatoki", initials: "TF", status: "Confirmed" },
  ],
  createdBy: "T. Adeyemi",
  createdAt: "2026-08-15T10:30:00Z",
  notes: "Morning rush hour shift. Priority deliveries. Ensure all riders have proper documentation.",
};

// Mock attendance data for this shift
const SHIFT_ATTENDANCE: AttendanceRecord[] = [
  {
    id: "ATT-001",
    personId: "P-044",
    name: "Peter Okafor",
    initials: "PO",
    checkIn: "06:02",
    checkOut: "10:05",
    status: "Present",
    hours: "4.0",
  },
  {
    id: "ATT-002",
    personId: "P-061",
    name: "Chinedu Uche",
    initials: "CU",
    checkIn: "06:00",
    checkOut: "10:00",
    status: "Present",
    hours: "4.0",
  },
  {
    id: "ATT-003",
    personId: "P-073",
    name: "Esther Obi",
    initials: "EO",
    checkIn: null,
    checkOut: null,
    status: "Absent",
    hours: "0.0",
  },
  {
    id: "ATT-004",
    personId: "P-052",
    name: "Amina Yusuf",
    initials: "AY",
    checkIn: "06:10",
    checkOut: "10:00",
    status: "Late",
    hours: "4.0",
  },
  {
    id: "ATT-005",
    personId: "P-038",
    name: "Musa Bello",
    initials: "MB",
    checkIn: null,
    checkOut: null,
    status: "Absent",
    hours: "0.0",
  },
  {
    id: "ATT-006",
    personId: "P-088",
    name: "Tunde Fatoki",
    initials: "TF",
    checkIn: "06:00",
    checkOut: "09:30",
    status: "Excused",
    hours: "3.5",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Scheduled: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Calendar className="h-4 w-4" />,
  },
  "In Progress": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Activity className="h-4 w-4" />,
  },
  Completed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Cancelled: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-4 w-4" />,
  },
};

const ATTENDANCE_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Present: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Late: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Absent: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Excused: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
};

const PERSON_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Confirmed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Absent: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

function TimeIndicator({ time }: { time: string }) {
  const hour = parseInt(time.split(":")[0]);
  const isMorning = hour < 12;
  const isAfternoon = hour >= 12 && hour < 17;
  const isEvening = hour >= 17;

  if (isMorning) return <Sun className="h-4 w-4 text-amber" />;
  if (isAfternoon) return <Cloud className="h-4 w-4 text-blue" />;
  return <Moon className="h-4 w-4 text-indigo" />;
}

export default function ShiftDetailPage() {
  const params = useParams();
  const router = useRouter();
  const shiftId = params.shiftId as string;

  const [loading, setLoading] = useState(true);
  const [shift, setShift] = useState<ShiftDetail | null>(null);
  const [attendance, setAttendance] = useState<AttendanceRecord[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showAssignModal, setShowAssignModal] = useState(false);
  const [selectedPerson, setSelectedPerson] = useState("");
  const [activeTab, setActiveTab] = useState<"overview" | "attendance">("overview");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setShift(SHIFT_DETAIL);
      setAttendance(SHIFT_ATTENDANCE);
      setLoading(false);
    };
    loadData();
  }, [shiftId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  // Navigation handlers
  const goBack = () => {
    router.push("/workforce/shifts");
  };

  const goToEdit = () => {
    router.push(`/workforce/shifts/${shiftId}/edit`);
  };

  const goToPerson = (personId: string) => {
    router.push(`/workforce/${personId}`);
  };

  // ===== GO TO GLOBAL ATTENDANCE PAGE =====
  const goToGlobalAttendance = () => {
    router.push("/workforce/attendance");
  };

  // Action handlers
  const handlePublish = () => {
    if (!shift) return;
    const updated = { ...shift, status: "Scheduled" as const };
    setShift(updated);
  };

  const handleCancel = () => {
    if (!shift) return;
    if (window.confirm(`Cancel shift ${shift.id}? This will notify all assigned people.`)) {
      const updated = { ...shift, status: "Cancelled" as const };
      setShift(updated);
    }
  };

  const handleAssign = (personId: string) => {
    if (!shift) return;
    if (shift.people.some((p) => p.id === personId)) return;

    const newPerson = {
      id: personId,
      name: "New Person",
      initials: "NP",
      status: "Pending" as const,
    };

    const updated = {
      ...shift,
      people: [...shift.people, newPerson],
      assigned: shift.assigned + 1,
    };
    setShift(updated);
    setShowAssignModal(false);
    setSelectedPerson("");
  };

  const handleRemovePerson = (personId: string) => {
    if (!shift) return;
    if (window.confirm("Remove this person from the shift?")) {
      const updated = {
        ...shift,
        people: shift.people.filter((p) => p.id !== personId),
        assigned: shift.assigned - 1,
      };
      setShift(updated);
    }
  };

  const handleNudge = (personId: string) => {
    alert(`Nudge sent to ${shift?.people.find(p => p.id === personId)?.name}`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading shift...</span>
        </div>
      </div>
    );
  }

  if (!shift) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Calendar className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Shift not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The shift you're looking for doesn't exist or has been removed.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to shifts
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[shift.status] || STATUS_STYLES.Scheduled;
  const fillRate = Math.round((shift.assigned / shift.capacity) * 100);
  const isActive = shift.status === "In Progress" || shift.status === "Scheduled";
  const isCancelled = shift.status === "Cancelled";

  const attendanceStats = {
    total: attendance.length,
    present: attendance.filter((a) => a.status === "Present").length,
    late: attendance.filter((a) => a.status === "Late").length,
    absent: attendance.filter((a) => a.status === "Absent").length,
  };

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
                  {shift.id}
                </h1>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                  {statusStyle.icon}
                  {shift.status}
                </span>
                {!isCancelled && isActive && (
                  <span className="flex items-center gap-1 rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
                    <span className="relative flex h-1.5 w-1.5">
                      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                      <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                    </span>
                    Active
                  </span>
                )}
              </div>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {shift.branch} · {shift.role} · {shift.dayName}
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
          {!isCancelled && (
            <>
              <button
                onClick={goToEdit}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
              >
                <Edit3 className="h-3.5 w-3.5" />
                Edit
              </button>
              {shift.status === "Scheduled" && (
                <button
                  onClick={handlePublish}
                  className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
                >
                  <Send className="h-3.5 w-3.5" />
                  Publish
                </button>
              )}
              <button
                onClick={handleCancel}
                className="flex h-9 items-center gap-1.5 rounded-xl border border-red/20 bg-red-bg/50 px-3 text-[11.5px] font-medium text-red shadow-sm transition hover:bg-red-bg"
              >
                <XCircle className="h-3.5 w-3.5" />
                Cancel
              </button>
            </>
          )}
        </div>
      </div>

      {/* ===== STATS ROW ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-primary-bg p-1.5">
              <Clock className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Time</div>
              <div className="flex items-center gap-1.5 text-[16px] font-semibold text-ink-950">
                <TimeIndicator time={shift.time} />
                {shift.time}
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Users className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Assigned</div>
              <div className="text-[16px] font-semibold text-ink-950">
                {shift.assigned}/{shift.capacity}
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-amber-bg p-1.5">
              <TrendingUp className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Fill Rate</div>
              <div className="text-[16px] font-semibold text-ink-950">{fillRate}%</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-green-bg p-1.5">
              <Calendar className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Date</div>
              <div className="text-[16px] font-semibold text-ink-950">{shift.date}</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== CAPACITY BAR ===== */}
      <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
        <div className="flex items-center justify-between mb-2">
          <span className="text-[12px] font-medium text-ink-600">Capacity Utilization</span>
          <span className={`text-[12px] font-semibold ${
            fillRate >= 80 ? "text-green" :
            fillRate >= 50 ? "text-amber" :
            "text-red"
          }`}>
            {fillRate}% filled
          </span>
        </div>
        <div className="h-2 w-full rounded-full bg-surface-3 overflow-hidden">
          <div
            className={`h-full rounded-full transition-all ${
              fillRate >= 80 ? "bg-green" :
              fillRate >= 50 ? "bg-amber" :
              "bg-red"
            }`}
            style={{ width: `${fillRate}%` }}
          />
        </div>
      </div>

      {/* ===== TABS ===== */}
      <div className="flex gap-1 border-b border-line overflow-x-auto scrollbar-hide">
        <button
          onClick={() => setActiveTab("overview")}
          className={`flex items-center gap-1.5 px-3.5 py-2.5 text-[12.5px] font-medium transition whitespace-nowrap border-b-2 ${
            activeTab === "overview"
              ? "text-primary border-primary"
              : "text-ink-600 hover:text-ink-950 border-transparent hover:border-ink-300"
          }`}
        >
          <FileText className="h-4 w-4" />
          Overview
        </button>
        <button
          onClick={() => setActiveTab("attendance")}
          className={`flex items-center gap-1.5 px-3.5 py-2.5 text-[12.5px] font-medium transition whitespace-nowrap border-b-2 ${
            activeTab === "attendance"
              ? "text-primary border-primary"
              : "text-ink-600 hover:text-ink-950 border-transparent hover:border-ink-300"
          }`}
        >
          <UserCheck className="h-4 w-4" />
          Attendance
          <span className="rounded-full bg-surface-2 px-1.5 py-0.5 text-[9px] font-medium text-ink-500">
            {attendance.length}
          </span>
        </button>
      </div>

      {/* ===== TAB CONTENT ===== */}
      {activeTab === "overview" && (
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
          {/* LEFT COLUMN - People List */}
          <div className="space-y-5">
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-2.5">
                  <Users className="h-5 w-5 text-ink-400" />
                  <h3 className="text-[15px] font-semibold text-ink-950">Assigned People</h3>
                  <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                    {shift.people.length}
                  </span>
                </div>
                {!isCancelled && (
                  <button
                    onClick={() => setShowAssignModal(true)}
                    className="flex items-center gap-1 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
                  >
                    <UserPlus className="h-3.5 w-3.5" />
                    Assign
                  </button>
                )}
              </div>

              <div className="space-y-2.5">
                {shift.people.map((person) => {
                  const personStyle = PERSON_STATUS_STYLES[person.status] || PERSON_STATUS_STYLES.Pending;
                  return (
                    <div
                      key={person.id}
                      className="flex items-center justify-between rounded-lg border border-line p-3 hover:bg-surface-2 transition-colors"
                    >
                      <div
                        onClick={() => goToPerson(person.id)}
                        className="flex cursor-pointer items-center gap-3 flex-1 min-w-0"
                      >
                        <div
                          className={`flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-white ${
                            person.status === "Confirmed" ? "bg-primary" :
                            person.status === "Pending" ? "bg-amber" :
                            "bg-red"
                          }`}
                        >
                          {person.initials}
                        </div>
                        <div className="min-w-0">
                          <div className="text-[13px] font-medium text-ink-950 truncate">{person.name}</div>
                          <div className="text-[11px] text-ink-500">{person.id}</div>
                        </div>
                      </div>
                      <div className="flex items-center gap-1.5">
                        <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${personStyle.bg} ${personStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${personStyle.dot}`} />
                          {person.status}
                        </span>
                        {!isCancelled && (
                          <>
                            <button
                              onClick={() => handleNudge(person.id)}
                              className="rounded-lg p-1 text-ink-400 hover:text-primary transition-colors"
                              title="Send nudge"
                            >
                              <Send className="h-3.5 w-3.5" />
                            </button>
                            <button
                              onClick={() => handleRemovePerson(person.id)}
                              className="rounded-lg p-1 text-ink-400 hover:text-red transition-colors"
                              title="Remove from shift"
                            >
                              <UserX className="h-3.5 w-3.5" />
                            </button>
                          </>
                        )}
                      </div>
                    </div>
                  );
                })}

                {shift.people.length === 0 && (
                  <div className="flex flex-col items-center justify-center py-8 text-center">
                    <div className="rounded-full bg-surface-2 p-3">
                      <Users className="h-6 w-6 text-ink-400" />
                    </div>
                    <p className="mt-2 text-[13px] text-ink-500">No people assigned yet</p>
                    {!isCancelled && (
                      <button
                        onClick={() => setShowAssignModal(true)}
                        className="mt-2 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
                      >
                        + Assign someone
                      </button>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* Notes */}
            {shift.notes && (
              <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
                <div className="flex items-center gap-2.5 mb-2">
                  <FileText className="h-5 w-5 text-ink-400" />
                  <h3 className="text-[15px] font-semibold text-ink-950">Notes</h3>
                </div>
                <p className="text-[13px] text-ink-700">{shift.notes}</p>
              </div>
            )}
          </div>

          {/* RIGHT COLUMN */}
          <div className="space-y-5">
            {/* Quick Actions */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Actions</h3>
              <div className="space-y-2">
                {!isCancelled && (
                  <>
                    <button
                      onClick={() => setShowAssignModal(true)}
                      className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                    >
                      <span className="flex items-center gap-2">
                        <UserPlus className="h-4 w-4" />
                        Assign People
                      </span>
                      <ArrowRight className="h-4 w-4 text-ink-400" />
                    </button>
                    <button
                      onClick={goToEdit}
                      className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                    >
                      <span className="flex items-center gap-2">
                        <Edit3 className="h-4 w-4" />
                        Edit Shift
                      </span>
                      <ArrowRight className="h-4 w-4 text-ink-400" />
                    </button>
                    {shift.status === "Scheduled" && (
                      <button
                        onClick={handlePublish}
                        className="flex w-full items-center justify-between rounded-xl bg-primary px-4 py-2.5 text-[12.5px] font-medium text-white transition hover:bg-primary/90"
                      >
                        <span className="flex items-center gap-2">
                          <Send className="h-4 w-4" />
                          Publish Shift
                        </span>
                        <ArrowRight className="h-4 w-4 text-white/70" />
                      </button>
                    )}
                  </>
                )}

                {/* ===== FIXED: NAVIGATES TO GLOBAL ATTENDANCE ===== */}
                <button
                  onClick={goToGlobalAttendance}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <UserCheck className="h-4 w-4" />
                    View All Attendance
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>

                <button
                  onClick={goBack}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <ArrowLeft className="h-4 w-4" />
                    Back to Shifts
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              </div>
            </div>

            {/* Metadata */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Metadata</h3>
              <div className="space-y-2 text-[13px]">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Created By</span>
                  <span className="font-medium text-ink-950">{shift.createdBy}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Created</span>
                  <span className="font-medium text-ink-950">
                    {new Date(shift.createdAt).toLocaleString()}
                  </span>
                </div>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ===== ATTENDANCE TAB ===== */}
      {activeTab === "attendance" && (
        <div className="space-y-5">
          {/* Attendance Stats */}
          <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
            <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
              <div className="flex items-center gap-2.5">
                <div className="rounded-full bg-primary-bg p-1.5">
                  <Users className="h-4 w-4 text-primary" />
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total</div>
                  <div className="font-display text-[18px] font-bold text-ink-950">{attendanceStats.total}</div>
                </div>
              </div>
            </div>
            <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
              <div className="flex items-center gap-2.5">
                <div className="rounded-full bg-green-bg p-1.5">
                  <CheckCircle2 className="h-4 w-4 text-green" />
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Present</div>
                  <div className="font-display text-[18px] font-bold text-ink-950">{attendanceStats.present}</div>
                </div>
              </div>
            </div>
            <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
              <div className="flex items-center gap-2.5">
                <div className="rounded-full bg-amber-bg p-1.5">
                  <Clock className="h-4 w-4 text-amber" />
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Late</div>
                  <div className="font-display text-[18px] font-bold text-ink-950">{attendanceStats.late}</div>
                </div>
              </div>
            </div>
            <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
              <div className="flex items-center gap-2.5">
                <div className="rounded-full bg-red-bg p-1.5">
                  <XCircle className="h-4 w-4 text-red" />
                </div>
                <div>
                  <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Absent</div>
                  <div className="font-display text-[18px] font-bold text-ink-950">{attendanceStats.absent}</div>
                </div>
              </div>
            </div>
          </div>

          {/* Attendance Table */}
          <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
            <div className="border-b border-line px-5 py-3.5">
              <div className="flex items-center gap-2">
                <UserCheck className="h-4 w-4 text-ink-400" />
                <span className="text-[14px] font-semibold text-ink-950">Shift Attendance</span>
                <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                  {attendance.length} records
                </span>
              </div>
            </div>

            <div className="overflow-x-auto">
              <table className="w-full border-collapse text-[13px] min-w-[600px]">
                <thead>
                  <tr className="bg-surface-2/70">
                    <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Person</th>
                    <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Check In</th>
                    <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Check Out</th>
                    <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Hours</th>
                    <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                    <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                  </tr>
                </thead>
                <tbody>
                  {attendance.length === 0 ? (
                    <tr>
                      <td colSpan={6} className="px-4 py-8 text-center text-ink-500">
                        No attendance records for this shift
                      </td>
                    </tr>
                  ) : (
                    attendance.map((record) => {
                      const attStatusStyle = ATTENDANCE_STATUS_STYLES[record.status] || ATTENDANCE_STATUS_STYLES.Present;

                      return (
                        <tr
                          key={record.id}
                          onClick={() => goToPerson(record.personId)}
                          className="cursor-pointer hover:bg-surface-2 transition-colors group"
                        >
                          <td className="border-b border-line px-4 py-3">
                            <div className="flex items-center gap-3">
                              <div
                                className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full text-[10px] font-bold text-white ${
                                  record.status === "Present" ? "bg-primary" :
                                  record.status === "Late" ? "bg-amber" :
                                  record.status === "Absent" ? "bg-red" :
                                  "bg-blue"
                                }`}
                              >
                                {record.initials}
                              </div>
                              <div>
                                <div className="font-medium text-ink-950">{record.name}</div>
                                <div className="text-[11px] text-ink-500">{record.personId}</div>
                              </div>
                            </div>
                          </td>
                          <td className="border-b border-line px-4 py-3 font-mono text-ink-950">
                            {record.checkIn || "—"}
                          </td>
                          <td className="border-b border-line px-4 py-3 font-mono text-ink-950">
                            {record.checkOut || "—"}
                          </td>
                          <td className="border-b border-line px-4 py-3 font-semibold text-ink-950">
                            {record.hours}h
                          </td>
                          <td className="border-b border-line px-4 py-3">
                            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${attStatusStyle.bg} ${attStatusStyle.text}`}>
                              <span className={`h-1.5 w-1.5 rounded-full ${attStatusStyle.dot}`} />
                              {record.status}
                            </span>
                          </td>
                          <td className="border-b border-line px-4 py-3">
                            <button className="text-[11px] font-medium text-ink-400 group-hover:text-primary transition-colors">
                              <Eye className="h-4 w-4" />
                            </button>
                          </td>
                        </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      )}

      {/* ===== ASSIGN MODAL ===== */}
      {showAssignModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 backdrop-blur-sm p-4">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-[18px] font-semibold text-ink-950">Assign Person</h3>
              <button
                onClick={() => setShowAssignModal(false)}
                className="rounded-lg p-1 text-ink-400 hover:bg-surface-2 transition-colors"
              >
                <X className="h-5 w-5" />
              </button>
            </div>
            <p className="text-[13px] text-ink-600 mb-4">
              Search for a person to assign to this shift.
            </p>
            <div className="space-y-3">
              <input
                type="text"
                placeholder="Search by name or ID..."
                className="w-full rounded-xl border border-line px-3.5 py-2.5 text-[13px] text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                value={selectedPerson}
                onChange={(e) => setSelectedPerson(e.target.value)}
              />
              <div className="flex gap-2">
                <button
                  onClick={() => {
                    if (selectedPerson.trim()) {
                      handleAssign("P-" + Date.now().toString().slice(-6));
                    }
                  }}
                  className="flex-1 rounded-xl bg-primary py-2.5 text-[13px] font-semibold text-white hover:bg-primary/90 transition-colors"
                >
                  Assign
                </button>
                <button
                  onClick={() => setShowAssignModal(false)}
                  className="flex-1 rounded-xl border border-line py-2.5 text-[13px] font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ===== NAVIGATION FOOTER ===== */}
      <div className="flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:justify-between">
        <button
          onClick={goBack}
          className="flex items-center gap-1.5 text-[12px] font-medium text-ink-500 hover:text-ink-900 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to shifts
        </button>
        <button
          onClick={goToGlobalAttendance}
          className="flex items-center gap-1.5 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
        >
          View All Attendance
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}