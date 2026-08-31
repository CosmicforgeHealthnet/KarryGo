// src/app/(dashboard)/workforce/attendance/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Search,
  Plus,
  Users,
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
  Calendar,
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
  Car,
  Bike,
  Briefcase,
  Shield,
  GraduationCap,
  Activity,
  Clock as ClockIcon,
  CalendarDays,
  UserCheck,
  UserX,
  Phone,
  Mail,
  MessageSquare,
} from "lucide-react";

type AttendanceRecord = {
  id: string;
  personId: string;
  name: string;
  initials: string;
  role: string;
  branch: string;
  shift: string;
  date: string;
  checkIn: string | null;
  checkOut: string | null;
  status: "Present" | "Late" | "Absent" | "Excused";
  hours: string;
  overtime: string;
  notes?: string;
};

// Mock data
const ATTENDANCE_RECORDS: AttendanceRecord[] = [
  {
    id: "ATT-001",
    personId: "P-044",
    name: "Peter Okafor",
    initials: "PO",
    role: "Dispatch Rider",
    branch: "Ikeja",
    shift: "06:00 - 10:00",
    date: "2026-08-20",
    checkIn: "06:02",
    checkOut: "10:05",
    status: "Present",
    hours: "4.0",
    overtime: "0.0",
    notes: "",
  },
  {
    id: "ATT-002",
    personId: "P-038",
    name: "Musa Bello",
    initials: "MB",
    role: "Hauling Driver",
    branch: "Apapa",
    shift: "12:00 - 18:00",
    date: "2026-08-20",
    checkIn: "12:10",
    checkOut: "18:00",
    status: "Late",
    hours: "4.0",
    overtime: "0.0",
    notes: "Traffic delay",
  },
  {
    id: "ATT-003",
    personId: "P-052",
    name: "Amina Yusuf",
    initials: "AY",
    role: "Dispatch Supervisor",
    branch: "Yaba",
    shift: "08:00 - 17:00",
    date: "2026-08-20",
    checkIn: null,
    checkOut: null,
    status: "Absent",
    hours: "0.0",
    overtime: "0.0",
    notes: "Sick leave approved",
  },
  {
    id: "ATT-004",
    personId: "P-061",
    name: "Chinedu Uche",
    initials: "CU",
    role: "Dispatch Rider",
    branch: "Lekki",
    shift: "16:00 - 20:00",
    date: "2026-08-20",
    checkIn: "15:58",
    checkOut: "20:00",
    status: "Present",
    hours: "4.0",
    overtime: "0.0",
    notes: "",
  },
  {
    id: "ATT-005",
    personId: "P-029",
    name: "Kabiru Ahmed",
    initials: "KA",
    role: "Hauling Driver",
    branch: "Apapa",
    shift: "12:00 - 18:00",
    date: "2026-08-20",
    checkIn: "12:00",
    checkOut: "19:30",
    status: "Present",
    hours: "6.0",
    overtime: "1.5",
    notes: "Extended trip completion",
  },
  {
    id: "ATT-006",
    personId: "P-073",
    name: "Esther Obi",
    initials: "EO",
    role: "Fleet Manager",
    branch: "Ikeja",
    shift: "08:00 - 17:00",
    date: "2026-08-20",
    checkIn: "08:05",
    checkOut: "17:00",
    status: "Present",
    hours: "8.0",
    overtime: "0.0",
    notes: "",
  },
  {
    id: "ATT-007",
    personId: "P-088",
    name: "Tunde Fatoki",
    initials: "TF",
    role: "Maintenance Tech",
    branch: "Ikeja",
    shift: "10:00 - 18:00",
    date: "2026-08-20",
    checkIn: "10:00",
    checkOut: "17:30",
    status: "Excused",
    hours: "6.5",
    overtime: "0.0",
    notes: "Left early - personal matter",
  },
  {
    id: "ATT-008",
    personId: "P-091",
    name: "Ngozi Akpan",
    initials: "NA",
    role: "Warehouse Supervisor",
    branch: "Apapa",
    shift: "08:00 - 17:00",
    date: "2026-08-20",
    checkIn: "08:00",
    checkOut: "17:00",
    status: "Present",
    hours: "8.0",
    overtime: "0.0",
    notes: "",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Present: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Late: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Absent: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
  Excused: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Present", "Late", "Absent", "Excused"];

export default function AttendancePage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedDate, setSelectedDate] = useState(new Date().toISOString().split("T")[0]);
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<AttendanceRecord[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(ATTENDANCE_RECORDS);
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
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.role.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus =
      selectedStatus === "All" || item.status === selectedStatus;
    const matchesDate = item.date === selectedDate;

    return matchesSearch && matchesBranch && matchesStatus && matchesDate;
  });

  // Stats
  const stats = {
    total: data.length,
    present: data.filter((d) => d.status === "Present").length,
    late: data.filter((d) => d.status === "Late").length,
    absent: data.filter((d) => d.status === "Absent").length,
  };

  const attendanceRate = Math.round((stats.present / stats.total) * 100);

  // Navigation
  const goToPerson = (personId: string) => {
    router.push(`/workforce/${personId}`);
  };

  const handleDateChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setSelectedDate(e.target.value);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading attendance records...</span>
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

  return (
    <div className="mx-auto max-w-[1400px] space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <div className="relative">
              <div className="absolute -top-1 -right-1">
                <Sparkles className="h-4 w-4 text-amber animate-pulse" />
              </div>
              <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                Attendance
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
              {stats.present} present
            </div>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-600">
            Track attendance, check-ins, check-outs, and manage exceptions.
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
          <input
            type="date"
            value={selectedDate}
            onChange={handleDateChange}
            className="h-9 rounded-xl border border-line bg-white/70 px-3 text-[12px] font-medium text-ink-700 shadow-sm outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5 group-hover:scale-110 transition-transform">
              <Users className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.total}</div>
            </div>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5 group-hover:scale-110 transition-transform">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Present</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.present}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+3 vs yesterday</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5 group-hover:scale-110 transition-transform">
              <Clock className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Late</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.late}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-amber">
            <AlertCircle className="h-3 w-3" />
            <span>Requires attention</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-red-bg p-1.5 group-hover:scale-110 transition-transform">
              <XCircle className="h-4 w-4 text-red" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Absent</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.absent}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-red">
            <AlertCircle className="h-3 w-3" />
            <span>Follow up needed</span>
          </div>
        </div>
      </div>

      {/* ===== ATTENDANCE RATE BAR ===== */}
      <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
        <div className="flex items-center justify-between mb-2">
          <span className="text-[12px] font-medium text-ink-600">Attendance Rate</span>
          <span className={`text-[12px] font-semibold ${
            attendanceRate >= 90 ? "text-green" :
            attendanceRate >= 70 ? "text-amber" :
            "text-red"
          }`}>
            {attendanceRate}%
          </span>
        </div>
        <div className="h-2 w-full rounded-full bg-surface-3 overflow-hidden">
          <div
            className={`h-full rounded-full transition-all ${
              attendanceRate >= 90 ? "bg-green" :
              attendanceRate >= 70 ? "bg-amber" :
              "bg-red"
            }`}
            style={{ width: `${attendanceRate}%` }}
          />
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[240px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, ID, or role..."
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

      {/* ===== ATTENDANCE TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <UserCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No attendance records</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            No attendance records match your current filters for {selectedDate}.
          </p>
          <button
            onClick={clearFilters}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-[13px] min-w-[800px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Person</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Role</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Check In</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Check Out</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Hours</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((record) => {
                  const statusStyle = STATUS_STYLES[record.status] || STATUS_STYLES.Present;

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
                            <div className="text-[11px] text-ink-500">{record.id}</div>
                          </div>
                        </div>
                      </td>
                      <td className="border-b border-line px-4 py-3 text-ink-600">{record.role}</td>
                      <td className="border-b border-line px-4 py-3">
                        <span className="font-mono text-ink-950">
                          {record.checkIn || "—"}
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className="font-mono text-ink-950">
                          {record.checkOut || "—"}
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className="font-semibold text-ink-950">{record.hours}h</span>
                        {parseFloat(record.overtime) > 0 && (
                          <span className="ml-1 text-[11px] text-amber">(+{record.overtime}h OT)</span>
                        )}
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
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
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}