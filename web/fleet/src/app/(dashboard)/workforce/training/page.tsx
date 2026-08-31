// src/app/(dashboard)/workforce/training/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  Search,
  Wrench,
  Plus,
  GraduationCap,
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
  Activity,
  Users,
  UserCheck,
  UserX,
  Send,
  BookOpen,
  Video,
  Monitor,
  Users as UsersIcon,
  CalendarDays,
} from "lucide-react";

type TrainingCourse = {
  id: string;
  name: string;
  description: string;
  category: "Safety" | "Technical" | "Compliance" | "Leadership" | "Soft Skills";
  type: "Online" | "In-Person" | "Hybrid";
  duration: string;
  status: "Active" | "Inactive" | "Archived";
  enrolled: number;
  completed: number;
  requiredRoles: string[];
  validity: string;
  createdAt: string;
  lastUpdated: string;
  createdBy: string;
};

// Mock data
const TRAINING_COURSES: TrainingCourse[] = [
  {
    id: "TR-001",
    name: "Defensive Riding - Advanced",
    description: "Advanced defensive riding techniques for experienced riders. Covers emergency maneuvers, hazard perception, and risk assessment.",
    category: "Safety",
    type: "In-Person",
    duration: "2 days (16 hours)",
    status: "Active",
    enrolled: 24,
    completed: 18,
    requiredRoles: ["Dispatch Rider", "Hauling Driver"],
    validity: "2 years",
    createdAt: "2026-01-15",
    lastUpdated: "2 days ago",
    createdBy: "T. Adeyemi",
  },
  {
    id: "TR-002",
    name: "Forklift Certification",
    description: "Certified forklift operation training including safety, maintenance, and practical assessment.",
    category: "Technical",
    type: "In-Person",
    duration: "1 day (8 hours)",
    status: "Active",
    enrolled: 12,
    completed: 10,
    requiredRoles: ["Warehouse Supervisor", "Maintenance Tech"],
    validity: "2 years",
    createdAt: "2026-02-10",
    lastUpdated: "1 week ago",
    createdBy: "F. Daniel",
  },
  {
    id: "TR-003",
    name: "First Aid & Emergency Response",
    description: "Comprehensive first aid training including CPR, wound care, and emergency response procedures.",
    category: "Safety",
    type: "Hybrid",
    duration: "1 day (8 hours)",
    status: "Active",
    enrolled: 32,
    completed: 28,
    requiredRoles: ["All"],
    validity: "3 years",
    createdAt: "2026-03-01",
    lastUpdated: "3 days ago",
    createdBy: "A. Yusuf",
  },
  {
    id: "TR-004",
    name: "Leadership for Supervisors",
    description: "Essential leadership skills for supervisors including team management, communication, and conflict resolution.",
    category: "Leadership",
    type: "Online",
    duration: "4 weeks (self-paced)",
    status: "Active",
    enrolled: 8,
    completed: 6,
    requiredRoles: ["Dispatch Supervisor", "Fleet Manager", "Warehouse Supervisor"],
    validity: "2 years",
    createdAt: "2026-04-15",
    lastUpdated: "4 days ago",
    createdBy: "M. Bello",
  },
  {
    id: "TR-005",
    name: "Hazardous Materials Handling",
    description: "Safe handling, storage, and transportation of hazardous materials according to industry standards.",
    category: "Safety",
    type: "In-Person",
    duration: "2 days (16 hours)",
    status: "Inactive",
    enrolled: 6,
    completed: 4,
    requiredRoles: ["Hauling Driver", "Warehouse Supervisor"],
    validity: "2 years",
    createdAt: "2025-11-15",
    lastUpdated: "1 month ago",
    createdBy: "K. Ahmed",
  },
  {
    id: "TR-006",
    name: "Customer Service Excellence",
    description: "Building exceptional customer service skills for frontline staff and operations teams.",
    category: "Soft Skills",
    type: "Online",
    duration: "2 weeks (self-paced)",
    status: "Active",
    enrolled: 15,
    completed: 12,
    requiredRoles: ["All"],
    validity: "2 years",
    createdAt: "2026-05-10",
    lastUpdated: "5 days ago",
    createdBy: "T. Adeyemi",
  },
  {
    id: "TR-007",
    name: "Fleet Maintenance Fundamentals",
    description: "Basic fleet maintenance, inspection techniques, and preventive maintenance planning.",
    category: "Technical",
    type: "Hybrid",
    duration: "3 days (24 hours)",
    status: "Archived",
    enrolled: 0,
    completed: 0,
    requiredRoles: ["Fleet Manager", "Maintenance Tech"],
    validity: "2 years",
    createdAt: "2025-09-20",
    lastUpdated: "2 months ago",
    createdBy: "F. Daniel",
  },
];

const CATEGORY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Safety: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <Shield className="h-3.5 w-3.5" />,
  },
  Technical: {
    bg: "bg-blue-bg",
    text: "text-blue",
    icon: <Wrench className="h-3.5 w-3.5" />,
  },
  Compliance: {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Leadership: {
    bg: "bg-purple-bg",
    text: "text-purple",
    icon: <Briefcase className="h-3.5 w-3.5" />,
  },
  "Soft Skills": {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <Users className="h-3.5 w-3.5" />,
  },
};

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  "Online": {
    bg: "bg-blue-bg",
    text: "text-blue",
    icon: <Monitor className="h-3.5 w-3.5" />,
  },
  "In-Person": {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <UsersIcon className="h-3.5 w-3.5" />,
  },
  "Hybrid": {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <Video className="h-3.5 w-3.5" />,
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Inactive: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const CATEGORY_FILTERS = ["All", "Safety", "Technical", "Compliance", "Leadership", "Soft Skills"];
const TYPE_FILTERS = ["All", "Online", "In-Person", "Hybrid"];
const STATUS_FILTERS = ["All", "Active", "Inactive", "Archived"];

export default function TrainingCoursesPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<TrainingCourse[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(TRAINING_COURSES);
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
      item.description.toLowerCase().includes(search.toLowerCase());

    const matchesCategory =
      selectedCategory === "All" || item.category === selectedCategory;
    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesCategory && matchesType && matchesStatus;
  });

  const stats = {
    total: data.length,
    active: data.filter((d) => d.status === "Active").length,
    enrolled: data.reduce((acc, d) => acc + d.enrolled, 0),
    completed: data.reduce((acc, d) => acc + d.completed, 0),
  };

  // ===== FIXED: Go to completions page, not course detail =====
  const goToCourseCompletions = (courseId: string) => {
    router.push(`/workforce/training/${courseId}/completions`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading training courses...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedCategory !== "All" ||
    selectedType !== "All" ||
    selectedStatus !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedCategory("All");
    setSelectedType("All");
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
                Training Courses
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
              {stats.active} active
            </div>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-600">
            Training courses, enrollments, and completion tracking for workforce development.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Plus className="h-3.5 w-3.5" />
            Create Course
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5">
              <BookOpen className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Total Courses</div>
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
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Active</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.active}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+2 this month</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Users className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Enrolled</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.enrolled}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-blue">
            <TrendingUp className="h-3 w-3" />
            <span>+8 this week</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5">
              <Award className="h-4 w-4 text-green" />
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
            placeholder="Search by name, ID, or description..."
            className="w-full bg-transparent text-[13px] text-ink-950 outline-none placeholder:text-ink-400 py-2"
          />
        </div>

        <button
          onClick={() => {
            const categories = ["All", ...new Set(data.map((d) => d.category))];
            const currentIndex = categories.indexOf(selectedCategory);
            const nextIndex = (currentIndex + 1) % categories.length;
            setSelectedCategory(categories[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <GraduationCap className="h-4 w-4 text-ink-400" />
          {selectedCategory}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const types = ["All", "Online", "In-Person", "Hybrid"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Monitor className="h-4 w-4 text-ink-400" />
          {selectedType}
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
              {(selectedCategory !== "All" ? 1 : 0) +
                (selectedType !== "All" ? 1 : 0) +
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Category</div>
              <div className="flex gap-1.5">
                {CATEGORY_FILTERS.map((category) => (
                  <button
                    key={category}
                    onClick={() => setSelectedCategory(category)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedCategory === category
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {category}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Type</div>
              <div className="flex gap-1.5">
                {TYPE_FILTERS.map((type) => (
                  <button
                    key={type}
                    onClick={() => setSelectedType(type)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedType === type
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {type}
                  </button>
                ))}
              </div>
            </div>
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

      {/* ===== COURSE CARDS WITH FIXED NAVIGATION ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <GraduationCap className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No training courses found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            No courses match your current filters.
          </p>
          <button
            onClick={clearFilters}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const categoryStyle = CATEGORY_STYLES[item.category] || CATEGORY_STYLES.Safety;
            const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Online;
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
            const isActive = item.status === "Active";
            const completionRate = item.enrolled > 0 ? Math.round((item.completed / item.enrolled) * 100) : 0;

            return (
              <div
                key={item.id}
                onClick={() => goToCourseCompletions(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  isActive
                    ? "border-green/20 bg-white hover:border-green/40"
                    : item.status === "Inactive"
                    ? "border-amber/20 bg-amber-bg/10 hover:border-amber/40"
                    : "border-line bg-white/60 hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  isActive ? "bg-green-500/5" : item.status === "Inactive" ? "bg-amber-500/5" : "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-xl ${categoryStyle.bg} ${categoryStyle.text}`}
                    >
                      {categoryStyle.icon}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.name}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {item.id}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-0.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}
                        >
                          {categoryStyle.icon}
                          {item.category}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}
                        >
                          {typeStyle.icon}
                          {item.type}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </div>
                    </div>
                  </div>
                  <button className="flex-shrink-0 text-ink-300 hover:text-ink-600 transition-colors">
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                </div>

                <div className="relative z-10 mt-2">
                  <p className="text-[13px] text-ink-600 line-clamp-2">{item.description}</p>
                </div>

                <div className="relative z-10 mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.duration}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <CalendarDays className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>Valid: {item.validity}</span>
                  </div>
                </div>

                {/* Enrollment Stats */}
                <div className="relative z-10 mt-3 border-t border-line pt-2.5">
                  <div className="flex items-center justify-between text-[12px]">
                    <span className="text-ink-500">Enrollment</span>
                    <span className="font-medium text-ink-950">
                      {item.completed}/{item.enrolled} completed
                    </span>
                  </div>
                  <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                    <div
                      className={`h-full rounded-full transition-all ${
                        completionRate >= 80 ? "bg-green" :
                        completionRate >= 50 ? "bg-amber" :
                        "bg-red"
                      }`}
                      style={{ width: `${completionRate}%` }}
                    />
                  </div>
                  <div className="mt-1 flex justify-between text-[10px] text-ink-400">
                    <span>{completionRate}% completion rate</span>
                    <span>{item.enrolled} enrolled</span>
                  </div>
                </div>

                {/* Required Roles */}
                <div className="relative z-10 mt-2 flex flex-wrap gap-1">
                  {item.requiredRoles.slice(0, 3).map((role) => (
                    <span
                      key={role}
                      className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500"
                    >
                      {role}
                    </span>
                  ))}
                  {item.requiredRoles.length > 3 && (
                    <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-400">
                      +{item.requiredRoles.length - 3}
                    </span>
                  )}
                </div>

                <div className="relative z-10 mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="text-[10px] text-ink-400">
                    Created: {item.createdAt}
                  </div>
                  <div className="flex items-center gap-1 text-[11px] font-medium text-ink-400 group-hover:text-primary transition-colors">
                    View Completions <ArrowRight className="h-3.5 w-3.5 group-hover:translate-x-0.5 transition-transform" />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}