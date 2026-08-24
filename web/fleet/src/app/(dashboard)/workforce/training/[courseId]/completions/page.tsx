// src/app/(dashboard)/workforce/training/[courseId]/completions/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  GraduationCap,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Clock,
  Calendar,
  User,
  MapPin,
  FileText,
  Search,
  RefreshCw,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Eye,
  ArrowRight,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Award,
  Users,
} from "lucide-react";

type CourseCompletion = {
  id: string;
  personId: string;
  personName: string;
  initials: string;
  status: "Completed" | "In Progress" | "Failed" | "Enrolled";
  score: number;
  completionDate: string | null;
  expiryDate: string | null;
  branch: string;
  role: string;
};

type Course = {
  id: string;
  name: string;
  category: string;
  totalEnrolled: number;
  completedCount: number;
};

// Mock data
const COURSE: Course = {
  id: "TR-001",
  name: "Defensive Riding - Advanced",
  category: "Safety",
  totalEnrolled: 24,
  completedCount: 18,
};

const COMPLETIONS: CourseCompletion[] = [
  {
    id: "TC-001",
    personId: "P-044",
    personName: "Peter Okafor",
    initials: "PO",
    status: "Completed",
    score: 92,
    completionDate: "2026-08-15",
    expiryDate: "2028-08-15",
    branch: "Ikeja",
    role: "Dispatch Rider",
  },
  {
    id: "TC-002",
    personId: "P-061",
    personName: "Chinedu Uche",
    initials: "CU",
    status: "Completed",
    score: 88,
    completionDate: "2026-08-14",
    expiryDate: "2028-08-14",
    branch: "Lekki",
    role: "Dispatch Rider",
  },
  {
    id: "TC-003",
    personId: "P-073",
    personName: "Esther Obi",
    initials: "EO",
    status: "In Progress",
    score: 65,
    completionDate: null,
    expiryDate: null,
    branch: "Ikeja",
    role: "Fleet Manager",
  },
  {
    id: "TC-004",
    personId: "P-052",
    personName: "Amina Yusuf",
    initials: "AY",
    status: "Completed",
    score: 94,
    completionDate: "2026-08-12",
    expiryDate: "2028-08-12",
    branch: "Yaba",
    role: "Dispatch Supervisor",
  },
  {
    id: "TC-005",
    personId: "P-038",
    personName: "Musa Bello",
    initials: "MB",
    status: "Failed",
    score: 55,
    completionDate: null,
    expiryDate: null,
    branch: "Apapa",
    role: "Hauling Driver",
  },
  {
    id: "TC-006",
    personId: "P-088",
    personName: "Tunde Fatoki",
    initials: "TF",
    status: "Enrolled",
    score: 0,
    completionDate: null,
    expiryDate: null,
    branch: "Ikeja",
    role: "Maintenance Tech",
  },
  {
    id: "TC-007",
    personId: "P-029",
    personName: "Kabiru Ahmed",
    initials: "KA",
    status: "Completed",
    score: 86,
    completionDate: "2026-08-10",
    expiryDate: "2028-08-10",
    branch: "Apapa",
    role: "Hauling Driver",
  },
  {
    id: "TC-008",
    personId: "P-091",
    personName: "Ngozi Akpan",
    initials: "NA",
    status: "Enrolled",
    score: 0,
    completionDate: null,
    expiryDate: null,
    branch: "Apapa",
    role: "Warehouse Supervisor",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Completed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  "In Progress": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
  Enrolled: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <GraduationCap className="h-3.5 w-3.5" />,
  },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Completed", "In Progress", "Failed", "Enrolled"];

export default function CourseCompletionsPage() {
  const params = useParams();
  const router = useRouter();
  const courseId = params.courseId as string;

  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [course, setCourse] = useState<Course | null>(null);
  const [completions, setCompletions] = useState<CourseCompletion[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setCourse(COURSE);
      setCompletions(COMPLETIONS);
      setLoading(false);
    };
    loadData();
  }, [courseId]);

  const filteredData = completions.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.personName.toLowerCase().includes(search.toLowerCase()) ||
      item.personId.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus =
      selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesBranch && matchesStatus;
  });

  // Navigation
  const goBack = () => {
    router.push("/workforce/training");
  };

  const goToCompletionDetail = (personId: string) => {
    router.push(`/workforce/training/${courseId}/completions/${personId}`);
  };

  const goToPerson = (personId: string) => {
    router.push(`/workforce/${personId}`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading completions...</span>
        </div>
      </div>
    );
  }

  if (!course) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <GraduationCap className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Course not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The course you're looking for doesn't exist.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to training
          </button>
        </div>
      </div>
    );
  }

  const stats = {
    total: completions.length,
    completed: completions.filter((c) => c.status === "Completed").length,
    inProgress: completions.filter((c) => c.status === "In Progress").length,
    failed: completions.filter((c) => c.status === "Failed").length,
    enrolled: completions.filter((c) => c.status === "Enrolled").length,
  };

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
            <button
              onClick={goBack}
              className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-white/70 text-ink-600 transition hover:bg-white hover:text-ink-900"
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <div>
              <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                {course.name}
              </h1>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {course.category} · {course.totalEnrolled} enrolled · {course.completedCount} completed
              </p>
            </div>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <span className="rounded-full bg-green-bg px-2.5 py-0.5 text-[11px] font-semibold text-green">
            {stats.completed} completed
          </span>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5">
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
            <div className="rounded-full bg-green-bg p-1.5">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Completed</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.completed}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+3 this week</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5">
              <Clock className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">In Progress</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.inProgress}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-amber">
            <Clock className="h-3 w-3" />
            <span>Active</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-red-bg p-1.5">
              <XCircle className="h-4 w-4 text-red" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Failed</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.failed}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-red">
            <AlertCircle className="h-3 w-3" />
            <span>Retake needed</span>
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
            placeholder="Search by name or ID..."
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

      {/* ===== COMPLETIONS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No completions found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            No completions match your current filters.
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
            <table className="w-full border-collapse text-[13px] min-w-[700px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Person</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Role</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Score</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Completed</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Enrolled;
                  const isCompleted = item.status === "Completed";

                  return (
                    <tr
                      key={item.id}
                      className="cursor-pointer hover:bg-surface-2 transition-colors group"
                    >
                      <td
                        className="border-b border-line px-4 py-3"
                        onClick={() => goToPerson(item.personId)}
                      >
                        <div className="flex items-center gap-3">
                          <div
                            className={`flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-full text-[10px] font-bold text-white ${
                              item.status === "Completed" ? "bg-primary" :
                              item.status === "In Progress" ? "bg-amber" :
                              item.status === "Failed" ? "bg-red" :
                              "bg-blue"
                            }`}
                          >
                            {item.initials}
                          </div>
                          <div>
                            <div className="font-medium text-ink-950">{item.personName}</div>
                            <div className="text-[11px] text-ink-500">{item.personId}</div>
                          </div>
                        </div>
                      </td>
                      <td className="border-b border-line px-4 py-3 text-ink-600">{item.role}</td>
                      <td className="border-b border-line px-4 py-3 text-ink-600">{item.branch}</td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`font-semibold ${
                          item.score >= 80 ? "text-green" :
                          item.score >= 60 ? "text-amber" :
                          "text-red"
                        }`}>
                          {item.score}%
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </td>
                      <td className="border-b border-line px-4 py-3 text-[12px] text-ink-600">
                        {item.completionDate || "—"}
                      </td>
                      <td className="border-b border-line px-4 py-3 text-right">
                        <button
                          onClick={() => goToCompletionDetail(item.personId)}
                          className="text-[11px] font-medium text-ink-400 group-hover:text-primary transition-colors"
                        >
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