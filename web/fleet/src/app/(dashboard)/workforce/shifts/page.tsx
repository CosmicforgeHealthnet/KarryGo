// src/app/(dashboard)/workforce/shifts/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Search,
  Plus,
  Calendar,
  RefreshCw,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Eye,
  Filter,
  ArrowRight,
  CalendarDays,
  User,
  Truck,
  FileText,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Award,
  Zap,
  Gauge,
  AlertTriangle,
  MoreHorizontal,
  Edit3,
  Trash2,
  Users,
  Briefcase,
  Shield,
  GraduationCap,
  Activity,
  Clock as ClockIcon,
  ChevronLeft,
  ChevronRight,
  Circle,
  Sun,
  Moon,
  Cloud,
  Star,
  Zap as ZapIcon,
} from "lucide-react";

type Shift = {
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
};

// Mock data
const SHIFTS: Shift[] = [
  {
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
    ],
  },
  {
    id: "SH-002",
    date: "2026-08-20",
    dayName: "Today",
    time: "12:00 - 18:00",
    branch: "Apapa",
    role: "Hauling Driver",
    capacity: 6,
    assigned: 4,
    status: "Scheduled",
    people: [
      { id: "P-038", name: "Musa Bello", initials: "MB", status: "Confirmed" },
      { id: "P-029", name: "Kabiru Ahmed", initials: "KA", status: "Pending" },
    ],
  },
  {
    id: "SH-003",
    date: "2026-08-20",
    dayName: "Today",
    time: "16:00 - 20:00",
    branch: "Lekki",
    role: "Dispatch Rider",
    capacity: 6,
    assigned: 3,
    status: "Scheduled",
    people: [
      { id: "P-052", name: "Amina Yusuf", initials: "AY", status: "Confirmed" },
    ],
  },
  {
    id: "SH-004",
    date: "2026-08-19",
    dayName: "Yesterday",
    time: "06:00 - 10:00",
    branch: "Ikeja",
    role: "Dispatch Rider",
    capacity: 8,
    assigned: 7,
    status: "In Progress",
    people: [
      { id: "P-044", name: "Peter Okafor", initials: "PO", status: "Confirmed" },
      { id: "P-061", name: "Chinedu Uche", initials: "CU", status: "Confirmed" },
    ],
  },
  {
    id: "SH-005",
    date: "2026-08-19",
    dayName: "Yesterday",
    time: "12:00 - 18:00",
    branch: "Yaba",
    role: "Warehouse Ops",
    capacity: 4,
    assigned: 4,
    status: "Completed",
    people: [
      { id: "P-091", name: "Ngozi Akpan", initials: "NA", status: "Confirmed" },
    ],
  },
  {
    id: "SH-006",
    date: "2026-08-21",
    dayName: "Tomorrow",
    time: "06:00 - 10:00",
    branch: "Ikeja",
    role: "Dispatch Rider",
    capacity: 8,
    assigned: 5,
    status: "Scheduled",
    people: [
      { id: "P-044", name: "Peter Okafor", initials: "PO", status: "Confirmed" },
    ],
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Scheduled: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  "In Progress": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Activity className="h-3.5 w-3.5" />,
  },
  Completed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Cancelled: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
};

const PERSON_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Confirmed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Absent: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Scheduled", "In Progress", "Completed", "Cancelled"];

// ===== TIME SLOT INDICATOR =====
function TimeIndicator({ time }: { time: string }) {
  const hour = parseInt(time.split(":")[0]);
  const isMorning = hour < 12;
  const isAfternoon = hour >= 12 && hour < 17;
  const isEvening = hour >= 17;

  if (isMorning) return <Sun className="h-4 w-4 text-amber" />;
  if (isAfternoon) return <Cloud className="h-4 w-4 text-blue" />;
  return <Moon className="h-4 w-4 text-indigo" />;
}

export default function ShiftsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Shift[]>([]);
  const [viewMode, setViewMode] = useState<"list" | "calendar">("list");
  const [currentDate, setCurrentDate] = useState(new Date());

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(SHIFTS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.role.toLowerCase().includes(search.toLowerCase()) ||
      item.branch.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesBranch && matchesStatus;
  });

  const stats = {
    total: data.length,
    scheduled: data.filter((d) => d.status === "Scheduled").length,
    inProgress: data.filter((d) => d.status === "In Progress").length,
    completed: data.filter((d) => d.status === "Completed").length,
  };

  // Navigation
  const goToShiftDetail = (shiftId: string) => {
    router.push(`/workforce/shifts/${shiftId}`);
  };

  const goToCreateShift = () => {
    router.push("/workforce/shifts/create");
  };

  const handlePreviousWeek = () => {
    const newDate = new Date(currentDate);
    newDate.setDate(newDate.getDate() - 7);
    setCurrentDate(newDate);
  };

  const handleNextWeek = () => {
    const newDate = new Date(currentDate);
    newDate.setDate(newDate.getDate() + 7);
    setCurrentDate(newDate);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading shifts...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedStatus !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
  };

  // Get week days
  const getWeekDays = () => {
    const start = new Date(currentDate);
    start.setDate(start.getDate() - start.getDay());
    const days = [];
    for (let i = 0; i < 7; i++) {
      const d = new Date(start);
      d.setDate(d.getDate() + i);
      days.push(d);
    }
    return days;
  };

  const weekDays = getWeekDays();

  return (
    <div className="mx-auto max-w-[1400px] space-y-5">
      {/* ===== HEADER WITH COSMIC TOUCH ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <div className="relative">
              <div className="absolute -top-2 -right-2">
                <Sparkles className="h-5 w-5 text-amber animate-pulse" />
              </div>
              <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                Shift Operations
              </h1>
            </div>
            <span className="rounded-full bg-primary-bg px-2.5 py-0.5 text-[11px] font-semibold text-primary">
              {data.length}
            </span>
            <div className="flex items-center gap-1 rounded-full bg-green-bg px-2.5 py-0.5 text-[10px] font-semibold text-green">
              <span className="relative flex h-1.5 w-1.5">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
              </span>
              {stats.inProgress} active
            </div>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-600">
            Shift scheduling, capacity management, and real-time assignment tracking.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          {/* View toggle */}
          <div className="flex rounded-xl border border-line bg-white/70 p-0.5 shadow-sm">
            <button
              onClick={() => setViewMode("list")}
              className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-[11px] font-medium transition ${
                viewMode === "list"
                  ? "bg-primary text-white shadow-sm shadow-primary/20"
                  : "text-ink-600 hover:bg-surface-2"
              }`}
            >
              <FileText className="h-3.5 w-3.5" />
              List
            </button>
            <button
              onClick={() => setViewMode("calendar")}
              className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-[11px] font-medium transition ${
                viewMode === "calendar"
                  ? "bg-primary text-white shadow-sm shadow-primary/20"
                  : "text-ink-600 hover:bg-surface-2"
              }`}
            >
              <Calendar className="h-3.5 w-3.5" />
              Calendar
            </button>
          </div>
          <button
            onClick={goToCreateShift}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Shift
          </button>
        </div>
      </div>

      {/* ===== STATS WITH TRENDS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5 group-hover:scale-110 transition-transform">
              <Calendar className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total Shifts</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.total}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-primary">
            <TrendingUp className="h-3 w-3" />
            <span>+4 this week</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-blue-bg p-1.5 group-hover:scale-110 transition-transform">
              <Clock className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Scheduled</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.scheduled}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-blue">
            <Clock className="h-3 w-3" />
            <span>Upcoming</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5 group-hover:scale-110 transition-transform">
              <Activity className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">In Progress</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.inProgress}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-amber">
            <Activity className="h-3 w-3" />
            <span>Live</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5 group-hover:scale-110 transition-transform">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Completed</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.completed}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+5 this month</span>
          </div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[240px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, role, or branch..."
            className="w-full bg-transparent text-[13px] text-ink-950 outline-none placeholder:text-ink-400 py-2"
          />
        </div>

        <button
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <MapPin className="h-4 w-4 text-ink-400" />
          {selectedBranch}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-[12px] font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedBranch !== "All branches" ? 1 : 0) +
                (selectedStatus !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-[12px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
              <div className="flex gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedStatus === status
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== LIST VIEW ===== */}
      {viewMode === "list" && (
        <div className="space-y-3">
          {filteredData.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
              <div className="rounded-full bg-surface-2 p-4">
                <Calendar className="h-8 w-8 text-ink-400" />
              </div>
              <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No shifts found</h3>
              <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
                No shifts match your current filters. Create a new shift to get started.
              </p>
              <button
                onClick={goToCreateShift}
                className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
              >
                <Plus className="inline h-3.5 w-3.5 mr-1" />
                Create Shift
              </button>
            </div>
          ) : (
            filteredData.map((shift) => {
              const statusStyle = STATUS_STYLES[shift.status] || STATUS_STYLES.Scheduled;
              const isActive = shift.status === "In Progress";
              const isScheduled = shift.status === "Scheduled";
              const fillRate = Math.round((shift.assigned / shift.capacity) * 100);

              return (
                <div
                  key={shift.id}
                  onClick={() => goToShiftDetail(shift.id)}
                  className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-0.5 ${
                    isActive
                      ? "border-amber/30 bg-amber-bg/5 hover:border-amber/50"
                      : isScheduled
                      ? "border-blue/20 bg-white hover:border-blue/40"
                      : "border-green/20 bg-white hover:border-green/40"
                  }`}
                >
                  <div className="absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 pointer-events-none bg-gradient-to-r from-primary/5 to-transparent" />

                  <div className="relative z-10 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    {/* Left */}
                    <div className="flex-1 min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-display text-[15px] font-semibold text-ink-950">
                          {shift.id}
                        </span>
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {shift.status}
                        </span>
                        {isActive && (
                          <span className="flex items-center gap-1 rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
                            <span className="relative flex h-1.5 w-1.5">
                              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                              <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                            </span>
                            Live
                          </span>
                        )}
                      </div>
                      <div className="mt-1 flex flex-wrap items-center gap-3 text-[12.5px] text-ink-500">
                        <span className="flex items-center gap-1">
                          <TimeIndicator time={shift.time} />
                          {shift.time}
                        </span>
                        <span className="text-ink-300">·</span>
                        <span className="flex items-center gap-1">
                          <MapPin className="h-3.5 w-3.5" />
                          {shift.branch}
                        </span>
                        <span className="text-ink-300">·</span>
                        <span className="flex items-center gap-1">
                          <Briefcase className="h-3.5 w-3.5" />
                          {shift.role}
                        </span>
                      </div>
                    </div>

                    {/* Right */}
                    <div className="flex flex-wrap items-center gap-4 sm:flex-nowrap">
                      {/* Capacity */}
                      <div className="flex items-center gap-2">
                        <div className="text-right">
                          <div className="text-[11px] text-ink-500">Capacity</div>
                          <div className="text-[14px] font-semibold text-ink-950">
                            {shift.assigned}/{shift.capacity}
                          </div>
                        </div>
                        <div className="h-8 w-1.5 rounded-full bg-surface-3 overflow-hidden">
                          <div
                            className={`w-full rounded-full transition-all ${
                              fillRate >= 80 ? "bg-green" :
                              fillRate >= 50 ? "bg-amber" :
                              "bg-red"
                            }`}
                            style={{ height: `${fillRate}%` }}
                          />
                        </div>
                      </div>

                      {/* People avatars */}
                      <div className="flex -space-x-1.5">
                        {shift.people.slice(0, 4).map((person) => (
                          <div
                            key={person.id}
                            className={`flex h-7 w-7 items-center justify-center rounded-full border-2 border-white text-[9px] font-bold text-white ${
                              person.status === "Confirmed" ? "bg-primary" :
                              person.status === "Pending" ? "bg-amber" :
                              "bg-red"
                            }`}
                            title={person.name}
                          >
                            {person.initials}
                          </div>
                        ))}
                        {shift.people.length > 4 && (
                          <div className="flex h-7 w-7 items-center justify-center rounded-full border-2 border-white bg-surface-2 text-[9px] font-bold text-ink-500">
                            +{shift.people.length - 4}
                          </div>
                        )}
                      </div>

                      <ArrowRight className="h-4 w-4 text-ink-300 group-hover:text-primary group-hover:translate-x-0.5 transition-all" />
                    </div>
                  </div>
                </div>
              );
            })
          )}
        </div>
      )}

      {/* ===== CALENDAR VIEW ===== */}
      {viewMode === "calendar" && (
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          {/* Calendar Header */}
          <div className="flex items-center justify-between border-b border-line px-5 py-3.5">
            <div className="flex items-center gap-4">
              <button
                onClick={handlePreviousWeek}
                className="rounded-lg p-1.5 text-ink-400 hover:bg-surface-2 transition-colors"
              >
                <ChevronLeft className="h-5 w-5" />
              </button>
              <span className="text-[15px] font-semibold text-ink-950">
                {weekDays[0].toLocaleDateString("en-US", { month: "short", day: "numeric" })} —{" "}
                {weekDays[6].toLocaleDateString("en-US", { month: "short", day: "numeric", year: "numeric" })}
              </span>
              <button
                onClick={handleNextWeek}
                className="rounded-lg p-1.5 text-ink-400 hover:bg-surface-2 transition-colors"
              >
                <ChevronRight className="h-5 w-5" />
              </button>
            </div>
            <button
              onClick={() => setCurrentDate(new Date())}
              className="rounded-lg border border-line px-3 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-surface-2 transition-colors"
            >
              Today
            </button>
          </div>

          {/* Calendar Grid */}
          <div className="overflow-x-auto">
            <div className="grid grid-cols-7 min-w-[700px]">
              {weekDays.map((day, index) => {
                const isToday = day.toDateString() === new Date().toDateString();
                const dayShifts = data.filter(
                  (s) => new Date(s.date).toDateString() === day.toDateString()
                );

                return (
                  <div key={index} className="border-r border-line last:border-r-0">
                    <div className={`border-b border-line px-3 py-2.5 text-center ${
                      isToday ? "bg-primary-bg/30" : ""
                    }`}>
                      <div className="text-[10px] font-medium uppercase text-ink-500">
                        {day.toLocaleDateString("en-US", { weekday: "short" })}
                      </div>
                      <div className={`text-[16px] font-semibold ${
                        isToday ? "text-primary" : "text-ink-950"
                      }`}>
                        {day.getDate()}
                      </div>
                    </div>
                    <div className="min-h-[180px] p-2 space-y-1.5">
                      {dayShifts.length === 0 ? (
                        <div className="flex h-[160px] items-center justify-center text-[11px] text-ink-400">
                          No shifts
                        </div>
                      ) : (
                        dayShifts.slice(0, 4).map((shift) => {
                          const statusStyle = STATUS_STYLES[shift.status] || STATUS_STYLES.Scheduled;
                          return (
                            <div
                              key={shift.id}
                              onClick={() => goToShiftDetail(shift.id)}
                              className={`cursor-pointer rounded-lg border p-2 transition hover:shadow-md ${
                                statusStyle.bg
                              } border-transparent hover:border-ink-200`}
                            >
                              <div className="flex items-center justify-between gap-1">
                                <span className="text-[11px] font-medium text-ink-950 truncate">
                                  {shift.time}
                                </span>
                                <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                              </div>
                              <div className="text-[10px] text-ink-500 truncate">{shift.role}</div>
                              <div className="mt-0.5 flex items-center gap-1 text-[9px] text-ink-400">
                                <Users className="h-3 w-3" />
                                {shift.assigned}/{shift.capacity}
                              </div>
                            </div>
                          );
                        })
                      )}
                      {dayShifts.length > 4 && (
                        <div className="text-center text-[10px] text-ink-400">
                          +{dayShifts.length - 4} more
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}