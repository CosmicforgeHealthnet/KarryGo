// src/app/(dashboard)/reports/fleet/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Car,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Search,
  Filter,
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
  FileJson,
  FilePdf,
  FileSpreadsheet,
  FileText as FileTextIcon,
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
  Truck,
  Bike,
  Wrench,
  Fuel,
  AlertTriangle as AlertTriangleIcon,
  CheckCircle as CheckCircleIcon,
  XCircle as XCircleIcon,
  Info as InfoIcon,
  Calendar as CalendarIcon,
  Building2 as Building2Icon,
  Users as UsersIcon,
  Tag as TagIcon,
  ListChecks as ListChecksIcon,
  Download as DownloadIcon,
  Printer as PrinterIcon,
  Edit3 as Edit3Icon,
  Trash2 as Trash2Icon,
  Check as CheckIcon,
  MoreHorizontal as MoreHorizontalIcon,
  User as UserIcon,
  Mail as MailIcon,
  Phone as PhoneIcon,
  MapPin as MapPinIcon,
  Briefcase as BriefcaseIcon,
  Send as SendIcon,
  Paperclip as PaperclipIcon,
  Link2 as Link2Icon,
  ExternalLink as ExternalLinkIcon,
  FileText as FileTextIcon2,
  File as FileIcon,
  FileArchive as FileArchiveIcon,
  FileImage as FileImageIcon,
  FileJson as FileJsonIcon,
  FilePdf as FilePdfIcon,
  FileSpreadsheet as FileSpreadsheetIcon,
  FolderOpen as FolderOpenIcon,
  Shield as ShieldIcon,
  Lock as LockIcon,
  EyeOff as EyeOffIcon,
  AlertOctagon as AlertOctagonIcon,
  FileCheck as FileCheckIcon,
  FileSignature as FileSignatureIcon,
  Folder as FolderIcon,
  HardDrive as HardDriveIcon,
  Database as DatabaseIcon,
  Cloud as CloudIcon,
  History as HistoryIcon,
  GitBranch as GitBranchIcon,
  FileUser as FileUserIcon,
  FileKey as FileKeyIcon,
  FileClock as FileClockIcon,
  FileX as FileXIcon,
  FileMinus as FileMinusIcon,
  FilePlus as FilePlusIcon,
  FileCog as FileCogIcon,
  FileCheck2 as FileCheck2Icon,
  TrendingUp as TrendingUpIcon,
  TrendingDown as TrendingDownIcon,
  Gauge as GaugeIcon,
  Target as TargetIcon,
  Activity as ActivityIcon,
  BarChart as BarChartIcon,
  LineChart as LineChartIcon,
  PieChart as PieChartIcon,
  Percent as PercentIcon,
  Timer as TimerIcon,
  Award as AwardIcon,
  Sparkles as SparklesIcon,
  Globe as GlobeIcon,
  FilterX as FilterXIcon,
  LayoutGrid as LayoutGridIcon,
  LayoutList as LayoutListIcon,
  Sliders as SlidersIcon,
  ArrowUp as ArrowUpIcon,
  ArrowDown as ArrowDownIcon,
  Zap as ZapIcon,
  Flame as FlameIcon,
  Medal as MedalIcon,
  Star as StarIcon,
  Wrench as WrenchIcon,
  Fuel as FuelIcon,
  Truck as TruckIcon,
  Bike as BikeIcon,
} from "lucide-react";

// ==================== TYPES ====================

type FleetMetric = {
  label: string;
  value: string;
  change: number;
  trend: "up" | "down" | "neutral";
  icon: React.ReactNode;
  color: "blue" | "green" | "amber" | "red" | "purple";
  subtitle: string;
};

type AssetStatus = {
  id: string;
  name: string;
  type: string;
  branch: string;
  status: "InService" | "SafetyBlocked" | "Maintenance" | "Reserved" | "Workshop";
  provider: string | null;
  lastInspection: string;
  nextMaintenance: string;
};

type MaintenanceItem = {
  id: string;
  asset: string;
  type: string;
  dueDate: string;
  priority: "Critical" | "High" | "Medium" | "Low";
  status: "Pending" | "InProgress" | "Completed";
  workOrder: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_FLEET_METRICS: FleetMetric[] = [
  {
    label: "Total Assets",
    value: "121",
    change: 4,
    trend: "up",
    icon: <Car className="h-5 w-5" />,
    color: "blue",
    subtitle: "98 currently serviceable",
  },
  {
    label: "In Service",
    value: "96",
    change: 79,
    trend: "neutral",
    icon: <CheckCircle className="h-5 w-5" />,
    color: "green",
    subtitle: "79% fleet readiness",
  },
  {
    label: "Maintenance Due",
    value: "13",
    change: 13,
    trend: "up",
    icon: <Wrench className="h-5 w-5" />,
    color: "amber",
    subtitle: "Within next 14 days",
  },
  {
    label: "Safety Blocked",
    value: "7",
    change: 7,
    trend: "down",
    icon: <AlertTriangle className="h-5 w-5" />,
    color: "red",
    subtitle: "Cannot be assigned",
  },
];

const MOCK_ASSETS: AssetStatus[] = [
  {
    id: "BR-204",
    name: "Honda ACE 110",
    type: "Motorcycle",
    branch: "Ikeja",
    status: "SafetyBlocked",
    provider: "P. Okafor",
    lastInspection: "2026-08-20",
    nextMaintenance: "2026-08-27",
  },
  {
    id: "VN-041",
    name: "Toyota Hiace",
    type: "Van",
    branch: "Lekki",
    status: "InService",
    provider: "M. Bello",
    lastInspection: "2026-08-10",
    nextMaintenance: "2026-09-10",
  },
  {
    id: "TR-112",
    name: "Mercedes Actros",
    type: "Truck",
    branch: "Apapa",
    status: "Workshop",
    provider: null,
    lastInspection: "2026-08-05",
    nextMaintenance: "2026-09-05",
  },
  {
    id: "BR-188",
    name: "Honda ACE 110",
    type: "Motorcycle",
    branch: "Yaba",
    status: "InService",
    provider: "A. Yusuf",
    lastInspection: "2026-08-15",
    nextMaintenance: "2026-09-15",
  },
  {
    id: "VN-029",
    name: "Toyota Hiace",
    type: "Van",
    branch: "Lekki",
    status: "Reserved",
    provider: "C. Adeyemi",
    lastInspection: "2026-08-12",
    nextMaintenance: "2026-09-12",
  },
];

const MOCK_MAINTENANCE: MaintenanceItem[] = [
  {
    id: "MT-001",
    asset: "BR-204",
    type: "Brake Inspection",
    dueDate: "2026-08-27",
    priority: "Critical",
    status: "Pending",
    workOrder: null,
  },
  {
    id: "MT-002",
    asset: "TR-112",
    type: "Engine Service",
    dueDate: "2026-09-05",
    priority: "High",
    status: "InProgress",
    workOrder: "WO-1917",
  },
  {
    id: "MT-003",
    asset: "VN-041",
    type: "Oil Change",
    dueDate: "2026-09-10",
    priority: "Medium",
    status: "Pending",
    workOrder: null,
  },
  {
    id: "MT-004",
    asset: "BR-188",
    type: "Tyre Replacement",
    dueDate: "2026-09-15",
    priority: "Low",
    status: "Pending",
    workOrder: null,
  },
];

export default function FleetDashboardPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [selectedBranch, setSelectedBranch] = useState("All branches");

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading fleet dashboard...</p>
      </div>
    );
  }

  const formatDate = (date: string) => {
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  const getStatusStyle = (status: string) => {
    switch (status) {
      case "InService":
        return { bg: "bg-green-bg", text: "text-green", label: "In Service" };
      case "SafetyBlocked":
        return { bg: "bg-red-bg", text: "text-red", label: "Safety Blocked" };
      case "Maintenance":
        return { bg: "bg-amber-bg", text: "text-amber", label: "Maintenance" };
      case "Reserved":
        return { bg: "bg-blue-bg", text: "text-blue", label: "Reserved" };
      case "Workshop":
        return { bg: "bg-surface-2", text: "text-ink-500", label: "Workshop" };
      default:
        return { bg: "bg-surface-2", text: "text-ink-500", label: status };
    }
  };

  const getPriorityStyle = (priority: string) => {
    switch (priority) {
      case "Critical":
        return { bg: "bg-red-bg", text: "text-red" };
      case "High":
        return { bg: "bg-amber-bg", text: "text-amber" };
      case "Medium":
        return { bg: "bg-blue-bg", text: "text-blue" };
      case "Low":
        return { bg: "bg-surface-2", text: "text-ink-500" };
      default:
        return { bg: "bg-surface-2", text: "text-ink-500" };
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Fleet Dashboard
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Asset availability, safety blocks, maintenance due/overdue, breakdowns, downtime and fuel/energy summaries.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Building2 className="h-3.5 w-3.5" />
            {selectedBranch} ▾
          </button>
        </div>
      </div>

      {/* ===== METRICS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        {MOCK_FLEET_METRICS.map((metric) => {
          const colorClasses = {
            blue: "bg-blue-bg text-blue",
            green: "bg-green-bg text-green",
            amber: "bg-amber-bg text-amber",
            red: "bg-red-bg text-red",
            purple: "bg-purple-bg text-purple",
          };

          return (
            <div key={metric.label} className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
              <div className="flex items-center justify-between">
                <div className={`rounded-full p-2 ${colorClasses[metric.color]}`}>
                  {metric.icon}
                </div>
                <div className={`flex items-center gap-0.5 text-xs font-semibold ${
                  metric.trend === "up" ? "text-green" :
                  metric.trend === "down" ? "text-red" :
                  "text-ink-500"
                }`}>
                  {metric.trend === "up" && <ArrowUp className="h-3 w-3" />}
                  {metric.trend === "down" && <ArrowDown className="h-3 w-3" />}
                  {metric.change}%
                </div>
              </div>
              <div className="mt-2 text-lg font-bold text-ink-950">{metric.value}</div>
              <div className="text-[10px] font-medium text-ink-500">{metric.label}</div>
              <div className="text-[9px] text-ink-400">{metric.subtitle}</div>
            </div>
          );
        })}
      </div>

      {/* ===== ASSETS TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Car className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">Asset Status</span>
            <span className="text-xs text-ink-500">Operational fleet assets</span>
          </div>
          <button
            onClick={() => router.push("/fleet/assets")}
            className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Asset</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Provider</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Last Inspection</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Next Maintenance</th>
              </tr>
            </thead>
            <tbody>
              {MOCK_ASSETS.map((asset) => {
                const statusStyle = getStatusStyle(asset.status);
                return (
                  <tr
                    key={asset.id}
                    onClick={() => router.push(`/fleet/assets/${asset.id}`)}
                    className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                  >
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink-950 text-xs">{asset.name}</div>
                      <div className="font-mono text-[10px] text-ink-500">{asset.id}</div>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-600">{asset.type}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{asset.branch}</td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        {statusStyle.label}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-600">{asset.provider || "—"}</td>
                    <td className="px-4 py-3 text-xs text-ink-500">{formatDate(asset.lastInspection)}</td>
                    <td className="px-4 py-3 text-xs text-ink-500">{formatDate(asset.nextMaintenance)}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== MAINTENANCE QUEUE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Wrench className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">Maintenance Queue</span>
            <span className="text-xs text-ink-500">Due maintenance items</span>
          </div>
          <button
            onClick={() => router.push("/fleet/maintenance/schedules")}
            className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="divide-y divide-line">
          {MOCK_MAINTENANCE.map((item) => {
            const priorityStyle = getPriorityStyle(item.priority);
            const statusColor = item.status === "Completed" ? "bg-green-bg text-green" :
                              item.status === "InProgress" ? "bg-blue-bg text-blue" :
                              "bg-amber-bg text-amber";

            return (
              <div
                key={item.id}
                className="flex items-center justify-between p-3 hover:bg-surface-2 transition-colors cursor-pointer"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium text-ink-950">{item.asset}</span>
                    <span className="text-[10px] text-ink-500">{item.type}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${priorityStyle.bg} ${priorityStyle.text}`}>
                      {item.priority}
                    </span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusColor}`}>
                      {item.status}
                    </span>
                  </div>
                  <div className="mt-0.5 text-[10px] text-ink-500">
                    Due: {formatDate(item.dueDate)}
                    {item.workOrder && ` · Work Order: ${item.workOrder}`}
                  </div>
                </div>
                <button className="text-ink-400 hover:text-primary transition-colors">
                  <Eye className="h-4 w-4" />
                </button>
              </div>
            );
          })}
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Car className="h-4 w-4" />
          RPT-02 · Fleet Dashboard
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Branch: {selectedBranch}
        </span>
      </div>
    </div>
  );
}