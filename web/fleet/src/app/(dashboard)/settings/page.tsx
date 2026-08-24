// src/app/(dashboard)/settings/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Settings,
  RefreshCw,
  Monitor,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  ArrowRight,
  Users,
  Building2,
  Link2,
  Shield,
  Lock,
  Database,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  FileText,
  File,
  FolderOpen,
  EyeOff,
  XCircle,
  FileCheck,
  FileSignature,
  Folder,
  HardDrive,
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

type SettingSection = {
  id: string;
  label: string;
  description: string;
  path: string;
  icon: React.ReactNode;
  color: "blue" | "green" | "amber" | "red" | "purple" | "gray";
};

const SETTING_SECTIONS: SettingSection[] = [
  {
    id: "staff",
    label: "Staff Directory",
    description: "Manage staff, roles, permissions and access.",
    path: "/settings/staff",
    icon: <Users className="h-5 w-5" />,
    color: "blue",
  },
  {
    id: "roles",
    label: "Roles & Permissions",
    description: "Create and manage roles and permissions.",
    path: "/settings/roles",
    icon: <Shield className="h-5 w-5" />,
    color: "purple",
  },
  {
    id: "approvals",
    label: "Approvals",
    description: "View and manage approval requests and policies.",
    path: "/settings/approvals",
    icon: <CheckCircle2 className="h-5 w-5" />,
    color: "green",
  },
  {
    id: "security",
    label: "Security",
    description: "Security settings, audit logs and legal holds.",
    path: "/settings/security",
    icon: <Lock className="h-5 w-5" />,
    color: "red",
  },
  {
    id: "branches",
    label: "Branches & Locations",
    description: "Manage branches, sites and operating locations.",
    path: "/settings/branches",
    icon: <Building2 className="h-5 w-5" />,
    color: "amber",
  },
  {
    id: "retention",
    label: "Retention",
    description: "Retention policies and data management.",
    path: "/settings/retention",
    icon: <Database className="h-5 w-5" />,
    color: "gray",
  },
  {
    id: "integrations",
    label: "Integrations",
    description: "API clients and webhook subscriptions.",
    path: "/settings/integrations",
    icon: <Link2 className="h-5 w-5" />,
    color: "blue",
  },
  {
    id: "sessions",
    label: "Sessions & Devices",
    description: "Manage active sessions and devices.",
    path: "/settings/sessions",
    icon: <Monitor className="h-5 w-5" />,
    color: "gray",
  },
  {
    id: "closure",
    label: "Organization Closure",
    description: "Request and manage organization closure.",
    path: "/settings/closure",
    icon: <AlertTriangle className="h-5 w-5" />,
    color: "red",
  },
];

const COLOR_STYLES: Record<string, { bg: string; text: string; border: string }> = {
  blue: { bg: "bg-primary-bg", text: "text-primary", border: "border-primary/20" },
  green: { bg: "bg-green-bg", text: "text-green", border: "border-green/20" },
  amber: { bg: "bg-amber-bg", text: "text-amber", border: "border-amber/20" },
  red: { bg: "bg-red-bg", text: "text-red", border: "border-red/20" },
  purple: { bg: "bg-purple-bg", text: "text-purple", border: "border-purple/20" },
  gray: { bg: "bg-surface-2", text: "text-ink-500", border: "border-line" },
};

export default function SettingsDashboardPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setLoading(false);
    };
    loadData();
  }, []);

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading settings...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Settings</h1>
        <p className="mt-1.5">Manage organization settings, staff, security, and integrations.</p>
      </div>

      {/* ── Settings Grid ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {SETTING_SECTIONS.map((section) => {
          const colorStyle = COLOR_STYLES[section.color] || COLOR_STYLES.gray;

          return (
            <div
              key={section.id}
              onClick={() => router.push(section.path)}
              className="group rounded-lg border border-line bg-white p-4 transition-colors hover:bg-surface-2 cursor-pointer"
            >
              <div className="flex items-start gap-3">
                <div className={`rounded-full p-2.5 flex-shrink-0 ${colorStyle.bg} ${colorStyle.text}`}>
                  {section.icon}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-semibold text-ink-950">{section.label}</div>
                  <div className="mt-0.5 text-xs text-ink-500">{section.description}</div>
                </div>
                <ArrowRight className="h-4 w-4 text-ink-400 transition-colors group-hover:text-primary flex-shrink-0" />
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}