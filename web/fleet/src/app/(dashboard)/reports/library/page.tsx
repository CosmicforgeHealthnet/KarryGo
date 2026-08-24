// src/app/(dashboard)/reports/library/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Library,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Users,
  Building2,
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
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Send,
  Paperclip,
  Link2,
  ExternalLink,
  FileText,
  File,
  FileArchive,
  FileImage,
  FolderOpen,
  Shield,
  Lock,
  EyeOff,
  Clock as ClockIcon,
  CheckCircle,
  XCircle,
  AlertOctagon,
  FileCheck,
  FileSignature,
  Folder,
  HardDrive,
  Database,
  Cloud,
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
  Car,
  Wallet,
} from "lucide-react";

type Report = {
  id: string;
  reference: string;
  name: string;
  description: string;
  category: "Operations" | "Fleet" | "Workforce" | "Finance" | "Customer" | "Compliance" | "Custom";
  format: "PDF" | "Excel" | "CSV" | "JSON" | "HTML";
  lastRun: string | null;
  lastRunBy: string | null;
  runCount: number;
  scope: "All" | "Branch" | "Department" | "User";
  status: "Available" | "Generating" | "Failed" | "Expired";
  tags: string[];
};

const MOCK_REPORTS: Report[] = [
  {
    id: "RPT-001",
    reference: "RPT-2026-001",
    name: "Operations Summary",
    description: "Daily operations overview including active work, exceptions, and branch performance.",
    category: "Operations",
    format: "PDF",
    lastRun: "2026-08-22T08:00:00Z",
    lastRunBy: "F. Daniel",
    runCount: 124,
    scope: "All",
    status: "Available",
    tags: ["Operations", "Daily", "Summary"],
  },
  {
    id: "RPT-002",
    reference: "RPT-2026-002",
    name: "Fleet Utilization",
    description: "Asset utilization, maintenance status, and fleet readiness metrics.",
    category: "Fleet",
    format: "Excel",
    lastRun: "2026-08-21T14:00:00Z",
    lastRunBy: "M. Bello",
    runCount: 87,
    scope: "Branch",
    status: "Available",
    tags: ["Fleet", "Utilization", "Maintenance"],
  },
  {
    id: "RPT-003",
    reference: "RPT-2026-003",
    name: "Workforce Performance",
    description: "Provider utilization, attendance, acceptance/decline categories, and training status.",
    category: "Workforce",
    format: "PDF",
    lastRun: "2026-08-20T10:00:00Z",
    lastRunBy: "O. Adebayo",
    runCount: 56,
    scope: "All",
    status: "Available",
    tags: ["Workforce", "Performance", "Training"],
  },
  {
    id: "RPT-004",
    reference: "RPT-2026-004",
    name: "SLA Compliance Report",
    description: "SLA results by customer, metric, and period. Includes MET/BREACHED/UNKNOWN status.",
    category: "Customer",
    format: "Excel",
    lastRun: "2026-08-22T09:00:00Z",
    lastRunBy: "F. Daniel",
    runCount: 42,
    scope: "All",
    status: "Available",
    tags: ["SLA", "Customer", "Compliance"],
  },
  {
    id: "RPT-005",
    reference: "RPT-2026-005",
    name: "Financial Summary",
    description: "Revenue, receivables, expenses, and profitability metrics by branch.",
    category: "Finance",
    format: "Excel",
    lastRun: "2026-08-19T16:00:00Z",
    lastRunBy: "F. Daniel",
    runCount: 38,
    scope: "All",
    status: "Available",
    tags: ["Finance", "Revenue", "Profitability"],
  },
  {
    id: "RPT-006",
    reference: "RPT-2026-006",
    name: "Compliance Overview",
    description: "Active restrictions, incidents, safety reports, and compliance status.",
    category: "Compliance",
    format: "PDF",
    lastRun: "2026-08-18T11:00:00Z",
    lastRunBy: "O. Adebayo",
    runCount: 29,
    scope: "All",
    status: "Available",
    tags: ["Compliance", "Safety", "Incidents"],
  },
  {
    id: "RPT-007",
    reference: "RPT-2026-007",
    name: "Branch Performance",
    description: "Branch-level performance metrics including completion rates and active work.",
    category: "Operations",
    format: "HTML",
    lastRun: "2026-08-21T08:00:00Z",
    lastRunBy: "F. Daniel",
    runCount: 67,
    scope: "Branch",
    status: "Available",
    tags: ["Branch", "Performance", "Operations"],
  },
  {
    id: "RPT-008",
    reference: "RPT-2026-008",
    name: "Customer Activity Report",
    description: "Customer order volume, revenue, and SLA performance by segment.",
    category: "Customer",
    format: "PDF",
    lastRun: null,
    lastRunBy: null,
    runCount: 0,
    scope: "All",
    status: "Available",
    tags: ["Customer", "Activity", "SLA"],
  },
  {
    id: "RPT-009",
    reference: "RPT-2026-009",
    name: "Maintenance Schedule",
    description: "Upcoming maintenance items by asset, priority, and status.",
    category: "Fleet",
    format: "Excel",
    lastRun: "2026-08-22T07:00:00Z",
    lastRunBy: "M. Bello",
    runCount: 15,
    scope: "Branch",
    status: "Generating",
    tags: ["Maintenance", "Schedule", "Fleet"],
  },
];

const CATEGORY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Operations: { bg: "bg-primary-bg", text: "text-primary", icon: <Activity className="h-3.5 w-3.5" /> },
  Fleet: { bg: "bg-amber-bg", text: "text-amber", icon: <Car className="h-3.5 w-3.5" /> },
  Workforce: { bg: "bg-purple-bg", text: "text-purple", icon: <Users className="h-3.5 w-3.5" /> },
  Finance: { bg: "bg-green-bg", text: "text-green", icon: <Wallet className="h-3.5 w-3.5" /> },
  Customer: { bg: "bg-primary-bg", text: "text-primary", icon: <Users className="h-3.5 w-3.5" /> },
  Compliance: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-3.5 w-3.5" /> },
  Custom: { bg: "bg-surface-2", text: "text-ink-500", icon: <FileText className="h-3.5 w-3.5" /> },
};

const FORMAT_ICONS: Record<string, React.ReactNode> = {
  PDF: <File className="h-3.5 w-3.5 text-red" />,
  Excel: <File className="h-3.5 w-3.5 text-green" />,
  CSV: <File className="h-3.5 w-3.5 text-blue" />,
  JSON: <File className="h-3.5 w-3.5 text-amber" />,
  HTML: <FileText className="h-3.5 w-3.5 text-purple" />,
};

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Available: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Generating: { bg: "bg-amber-bg", text: "text-amber", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  Failed: { bg: "bg-red-bg", text: "text-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <ClockIcon className="h-3.5 w-3.5" /> },
};

const CATEGORY_FILTERS = ["All", "Operations", "Fleet", "Workforce", "Finance", "Customer", "Compliance", "Custom"];
const FORMAT_FILTERS = ["All", "PDF", "Excel", "CSV", "JSON", "HTML"];
const STATUS_FILTERS = ["All", "Available", "Generating", "Failed", "Expired"];

export default function ReportsLibraryPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Report[]>([]);
  const [search, setSearch] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [selectedFormat, setSelectedFormat] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_REPORTS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.tags.some(t => t.toLowerCase().includes(search.toLowerCase()));

    const matchesCategory = selectedCategory === "All" || item.category === selectedCategory;
    const matchesFormat = selectedFormat === "All" || item.format === selectedFormat;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesCategory && matchesFormat && matchesStatus;
  });

  const hasActiveFilters =
    selectedCategory !== "All" ||
    selectedFormat !== "All" ||
    selectedStatus !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedCategory("All");
    setSelectedFormat("All");
    setSelectedStatus("All");
  };

  const filterCount =
    (selectedCategory !== "All" ? 1 : 0) +
    (selectedFormat !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0);

  const formatTime = (date: string | null) => {
    if (!date) return "Never";
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

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading reports...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Reports Library</h1>
          <p className="mt-1.5">Available ordinary reports, description, last run and permitted scope.</p>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Library className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total Reports</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Available</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {data.filter(d => d.status === "Available").length}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <RefreshCw className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Generating</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {data.filter(d => d.status === "Generating").length}
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <XCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Failed / Expired</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">
            {data.filter(d => d.status === "Failed" || d.status === "Expired").length}
          </div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search reports..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <BookOpen className="h-3.5 w-3.5 text-ink-400" />
          {selectedCategory}
          <span className="text-ink-300">·</span>
          <FileText className="h-3.5 w-3.5 text-ink-400" />
          {selectedFormat}
          <span className="text-ink-300">·</span>
          <CheckCircle2 className="h-3.5 w-3.5 text-ink-400" />
          {selectedStatus}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {filterCount}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filteredData.length} reports
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Category</span>
              <div className="flex flex-wrap gap-1.5">
                {CATEGORY_FILTERS.map((cat) => (
                  <button
                    key={cat}
                    onClick={() => setSelectedCategory(cat)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedCategory === cat
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {cat}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Format</span>
              <div className="flex flex-wrap gap-1.5">
                {FORMAT_FILTERS.map((fmt) => (
                  <button
                    key={fmt}
                    onClick={() => setSelectedFormat(fmt)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedFormat === fmt
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {fmt}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((st) => (
                  <button
                    key={st}
                    onClick={() => setSelectedStatus(st)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === st
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {st}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Reports Grid ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Library className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No reports found</h3>
          <p className="mt-1 text-sm text-ink-500">No reports match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((report) => {
            const categoryStyle = CATEGORY_STYLES[report.category] || CATEGORY_STYLES.Custom;
            const formatIcon = FORMAT_ICONS[report.format] || FORMAT_ICONS.PDF;
            const statusStyle = STATUS_STYLES[report.status] || STATUS_STYLES.Available;

            return (
              <div
                key={report.id}
                className="group rounded-lg border border-line bg-white p-4 transition-colors hover:bg-surface-2 cursor-pointer"
                onClick={() => router.push(`/reports/library/${report.id}/run`)}
              >
                <div className="flex items-start justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-1.5">
                      <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                        {categoryStyle.icon}
                        {report.category}
                      </span>
                      <span className="text-[10px] text-ink-400">{formatIcon}</span>
                      <span className={`flex items-center gap-0.5 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        {statusStyle.icon}
                        {report.status}
                      </span>
                    </div>
                    <div className="mt-1.5 font-semibold text-ink-950">{report.name}</div>
                    <div className="text-xs text-ink-500 line-clamp-2">{report.description}</div>
                    <div className="mt-1.5 flex flex-wrap items-center gap-1 text-[10px] text-ink-400">
                      <span className="font-mono">{report.reference}</span>
                      <span>·</span>
                      <span>{report.scope}</span>
                      <span>·</span>
                      <span>{report.runCount} runs</span>
                    </div>
                    {report.tags.length > 0 && (
                      <div className="mt-1 flex flex-wrap gap-1">
                        {report.tags.map((tag) => (
                          <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[8px] font-medium text-ink-500">
                            {tag}
                          </span>
                        ))}
                      </div>
                    )}
                  </div>
                  <ArrowRight className="h-4 w-4 text-ink-400 transition-colors group-hover:text-primary flex-shrink-0" />
                </div>
                <div className="mt-2.5 flex items-center gap-2 text-[10px] text-ink-400 border-t border-line pt-2.5">
                  {report.lastRun ? (
                    <>
                      <Clock className="h-3 w-3" />
                      Last run: {formatTime(report.lastRun)}
                      {report.lastRunBy && ` by ${report.lastRunBy}`}
                    </>
                  ) : (
                    <span className="text-ink-400">Never run</span>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}