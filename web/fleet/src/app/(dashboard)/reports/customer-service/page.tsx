// src/app/(dashboard)/reports/customer-service/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
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
  Users as UsersIcon,
  Building2 as Building2Icon,
  Tag as TagIcon,
  Calendar as CalendarIcon,
  Info as InfoIcon,
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
  Calendar as CalendarIcon2,
  Clock as ClockIcon2,
  UserCheck as UserCheckIcon,
  UserX as UserXIcon,
  UserPlus as UserPlusIcon,
  UserMinus as UserMinusIcon,
  Gem,
  Package,
  Wallet,
  Briefcase as BriefcaseIcon2,
  FileCheck as FileCheckIcon2,
  Shield as ShieldIcon2,
  Lock as LockIcon2,
  EyeOff as EyeOffIcon2,
  AlertOctagon as AlertOctagonIcon2,
  FileCheck as FileCheckIcon3,
  FileSignature as FileSignatureIcon2,
  Folder as FolderIcon2,
  HardDrive as HardDriveIcon2,
  Database as DatabaseIcon2,
  Cloud as CloudIcon2,
  History as HistoryIcon2,
  GitBranch as GitBranchIcon2,
  FileUser as FileUserIcon2,
  FileKey as FileKeyIcon2,
  FileClock as FileClockIcon2,
  FileX as FileXIcon2,
  FileMinus as FileMinusIcon2,
  FilePlus as FilePlusIcon2,
  FileCog as FileCogIcon2,
  FileCheck2 as FileCheck2Icon2,
  TrendingUp as TrendingUpIcon2,
  TrendingDown as TrendingDownIcon2,
  Gauge as GaugeIcon2,
  Target as TargetIcon2,
  Activity as ActivityIcon2,
  BarChart as BarChartIcon2,
  LineChart as LineChartIcon2,
  PieChart as PieChartIcon2,
  Percent as PercentIcon2,
  Timer as TimerIcon2,
  Award as AwardIcon2,
  Sparkles as SparklesIcon2,
  Globe as GlobeIcon2,
  FilterX as FilterXIcon2,
  LayoutGrid as LayoutGridIcon2,
  LayoutList as LayoutListIcon2,
  Sliders as SlidersIcon2,
  ArrowUp as ArrowUpIcon2,
  ArrowDown as ArrowDownIcon2,
  Zap as ZapIcon2,
  Flame as FlameIcon2,
  Medal as MedalIcon2,
  Star as StarIcon2,
  Gem as GemIcon,
  Package as PackageIcon,
  Wallet as WalletIcon,
  Briefcase as BriefcaseIcon3,
  FileCheck as FileCheckIcon4,
  Shield as ShieldIcon3,
  Lock as LockIcon3,
  EyeOff as EyeOffIcon3,
  AlertOctagon as AlertOctagonIcon3,
  FileCheck as FileCheckIcon5,
  FileSignature as FileSignatureIcon3,
  Folder as FolderIcon3,
  HardDrive as HardDriveIcon3,
  Database as DatabaseIcon3,
  Cloud as CloudIcon3,
  History as HistoryIcon3,
  GitBranch as GitBranchIcon3,
  FileUser as FileUserIcon3,
  FileKey as FileKeyIcon3,
  FileClock as FileClockIcon3,
  FileX as FileXIcon3,
  FileMinus as FileMinusIcon3,
  FilePlus as FilePlusIcon3,
  FileCog as FileCogIcon3,
  FileCheck2 as FileCheck2Icon3,
  TrendingUp as TrendingUpIcon3,
  TrendingDown as TrendingDownIcon3,
  Gauge as GaugeIcon3,
  Target as TargetIcon3,
  Activity as ActivityIcon3,
  BarChart as BarChartIcon3,
  LineChart as LineChartIcon3,
  PieChart as PieChartIcon3,
  Percent as PercentIcon3,
  Timer as TimerIcon3,
  Award as AwardIcon3,
  Sparkles as SparklesIcon3,
  Globe as GlobeIcon3,
  FilterX as FilterXIcon3,
  LayoutGrid as LayoutGridIcon3,
  LayoutList as LayoutListIcon3,
  Sliders as SlidersIcon3,
  ArrowUp as ArrowUpIcon3,
  ArrowDown as ArrowDownIcon3,
  Zap as ZapIcon3,
  Flame as FlameIcon3,
  Medal as MedalIcon3,
  Star as StarIcon3,
} from "lucide-react";

// ==================== TYPES ====================

type CustomerMetric = {
  label: string;
  value: string;
  change: number;
  trend: "up" | "down" | "neutral";
  icon: React.ReactNode;
  color: "blue" | "green" | "amber" | "red" | "purple";
  subtitle: string;
};

type CustomerActivity = {
  id: string;
  name: string;
  segment: string;
  orders: number;
  revenue: string;
  lastActivity: string;
  slaStatus: "MET" | "BREACHED" | "UNKNOWN";
};

type SLAStatus = {
  metric: string;
  target: string;
  actual: string;
  result: "MET" | "BREACHED" | "UNKNOWN";
  customer: string;
  period: string;
};

// ==================== MOCK DATA ====================

const MOCK_CUSTOMER_METRICS: CustomerMetric[] = [
  {
    label: "Active Customers",
    value: "428",
    change: 19,
    trend: "up",
    icon: <Users className="h-5 w-5" />,
    color: "blue",
    subtitle: "Company commercial relationships",
  },
  {
    label: "Open Quotations",
    value: "31",
    change: 31,
    trend: "neutral",
    icon: <FileText className="h-5 w-5" />,
    color: "green",
    subtitle: "₦12.4m quoted value",
  },
  {
    label: "Renewals Due",
    value: "14",
    change: 14,
    trend: "up",
    icon: <Calendar className="h-5 w-5" />,
    color: "amber",
    subtitle: "Next 30 days",
  },
  {
    label: "SLA Risks",
    value: "7",
    change: 7,
    trend: "down",
    icon: <AlertTriangle className="h-5 w-5" />,
    color: "red",
    subtitle: "Commercial attention",
  },
];

const MOCK_CUSTOMER_ACTIVITY: CustomerActivity[] = [
  {
    id: "C-003",
    name: "Charitos Hospital",
    segment: "Corporate",
    orders: 46,
    revenue: "₦4.2m",
    lastActivity: "2026-08-22T11:00:00Z",
    slaStatus: "BREACHED",
  },
  {
    id: "C-004",
    name: "Greenline Foods",
    segment: "Enterprise",
    orders: 74,
    revenue: "₦6.9m",
    lastActivity: "2026-08-22T10:00:00Z",
    slaStatus: "MET",
  },
  {
    id: "C-002",
    name: "Oakfield Stores",
    segment: "SME",
    orders: 27,
    revenue: "₦1.8m",
    lastActivity: "2026-08-21T16:00:00Z",
    slaStatus: "BREACHED",
  },
  {
    id: "C-005",
    name: "Solaris Construction",
    segment: "Corporate",
    orders: 18,
    revenue: "₦3.1m",
    lastActivity: "2026-08-20T14:00:00Z",
    slaStatus: "UNKNOWN",
  },
];

const MOCK_SLA_STATUS: SLAStatus[] = [
  {
    metric: "On-Time Delivery",
    target: "98%",
    actual: "96.4%",
    result: "BREACHED",
    customer: "Charitos Hospital",
    period: "Q3 2026",
  },
  {
    metric: "Damage Rate",
    target: "<1%",
    actual: "0.8%",
    result: "MET",
    customer: "Charitos Hospital",
    period: "Q3 2026",
  },
  {
    metric: "On-Time Delivery",
    target: "97%",
    actual: "91.2%",
    result: "BREACHED",
    customer: "Oakfield Stores",
    period: "Q3 2026",
  },
  {
    metric: "Customer Satisfaction",
    target: "4.5/5",
    actual: "4.2/5",
    result: "BREACHED",
    customer: "Oakfield Stores",
    period: "Q3 2026",
  },
  {
    metric: "On-Time Delivery",
    target: "99%",
    actual: "99.1%",
    result: "MET",
    customer: "Greenline Foods",
    period: "Q3 2026",
  },
];

export default function CustomerServiceDashboardPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [selectedPeriod, setSelectedPeriod] = useState("Q3 2026");

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
        <p className="mt-3 text-sm text-ink-600">Loading dashboard...</p>
      </div>
    );
  }

  const formatTime = (date: string) => {
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

  const getSlaStatusStyle = (status: string) => {
    switch (status) {
      case "MET":
        return { bg: "bg-green-bg", text: "text-green", label: "MET" };
      case "BREACHED":
        return { bg: "bg-red-bg", text: "text-red", label: "BREACHED" };
      case "UNKNOWN":
        return { bg: "bg-amber-bg", text: "text-amber", label: "UNKNOWN" };
      default:
        return { bg: "bg-surface-2", text: "text-ink-500", label: status };
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Customer & Service Dashboard
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Volume, SLA results, complaints/quality, receivable references and service activity.
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
            <Calendar className="h-3.5 w-3.5" />
            {selectedPeriod} ▾
          </button>
        </div>
      </div>

      {/* ===== METRICS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        {MOCK_CUSTOMER_METRICS.map((metric) => {
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

      {/* ===== CUSTOMER ACTIVITY TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Users className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">Customer Activity</span>
            <span className="text-xs text-ink-500">Recent commercial activity</span>
          </div>
          <button
            onClick={() => router.push("/crm/customers")}
            className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Customer</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Segment</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Orders</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Revenue</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Last Activity</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">SLA</th>
              </tr>
            </thead>
            <tbody>
              {MOCK_CUSTOMER_ACTIVITY.map((customer) => {
                const slaStyle = getSlaStatusStyle(customer.slaStatus);
                return (
                  <tr
                    key={customer.id}
                    onClick={() => router.push(`/crm/customers/${customer.id}`)}
                    className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                  >
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink-950 text-xs">{customer.name}</div>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-600">{customer.segment}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{customer.orders}</td>
                    <td className="px-4 py-3 text-xs font-medium text-ink-950">{customer.revenue}</td>
                    <td className="px-4 py-3 text-xs text-ink-500">{formatTime(customer.lastActivity)}</td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${slaStyle.bg} ${slaStyle.text}`}>
                        {slaStyle.label}
                      </span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== SLA STATUS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="flex items-center justify-between p-4 border-b border-line">
          <div className="flex items-center gap-2">
            <Target className="h-4 w-4 text-ink-400" />
            <span className="font-semibold text-ink-950">SLA Status</span>
            <span className="text-xs text-ink-500">Service Level Agreement results</span>
          </div>
          <button
            onClick={() => router.push("/comms/sla")}
            className="text-xs font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>
        <div className="divide-y divide-line">
          {MOCK_SLA_STATUS.map((item, index) => {
            const resultStyle = getSlaStatusStyle(item.result);
            return (
              <div
                key={index}
                className="flex items-center justify-between p-3 hover:bg-surface-2 transition-colors cursor-pointer"
              >
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-medium text-ink-950">{item.customer}</span>
                    <span className="text-[10px] text-ink-500">{item.metric}</span>
                    <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${resultStyle.bg} ${resultStyle.text}`}>
                      {resultStyle.label}
                    </span>
                  </div>
                  <div className="mt-0.5 text-[10px] text-ink-500">
                    Target: {item.target} · Actual: {item.actual} · {item.period}
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
          <Users className="h-4 w-4" />
          RPT-04 · Customer & Service Dashboard
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Period: {selectedPeriod}
        </span>
      </div>
    </div>
  );
}