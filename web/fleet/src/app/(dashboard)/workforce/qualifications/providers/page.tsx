// src/app/(dashboard)/workforce/qualifications/providers/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Wrench,
  Search,
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
  Activity,
  GraduationCap,
  UserCheck,
  UserX,
  Send,
} from "lucide-react";

type ProviderQualification = {
  id: string;
  providerId: string;
  providerName: string;
  initials: string;
  qualificationId: string;
  qualificationName: string;
  category: "Safety" | "Technical" | "Compliance" | "Training" | "Medical";
  status: "Verified" | "Expiring" | "Missing" | "Pending";
  issueDate: string | null;
  expiryDate: string | null;
  evidenceUrl: string | null;
  branch: string;
  service: "Dispatch" | "Hauling" | "Warehouse" | "All";
  verifiedBy: string | null;
  lastUpdated: string;
};

// Mock data
const PROVIDER_QUALIFICATIONS: ProviderQualification[] = [
  {
    id: "PQ-001",
    providerId: "P-044",
    providerName: "Peter Okafor",
    initials: "PO",
    qualificationId: "Q-001",
    qualificationName: "Defensive Riding Training",
    category: "Safety",
    status: "Verified",
    issueDate: "2026-01-15",
    expiryDate: "2028-01-15",
    evidenceUrl: "/documents/po_defensive.pdf",
    branch: "Ikeja",
    service: "Dispatch",
    verifiedBy: "T. Adeyemi",
    lastUpdated: "2 days ago",
  },
  {
    id: "PQ-002",
    providerId: "P-044",
    providerName: "Peter Okafor",
    initials: "PO",
    qualificationId: "Q-002",
    qualificationName: "Vehicle Roadworthiness",
    category: "Compliance",
    status: "Expiring",
    issueDate: "2025-08-20",
    expiryDate: "2026-08-20",
    evidenceUrl: "/documents/po_roadworthy.pdf",
    branch: "Ikeja",
    service: "Dispatch",
    verifiedBy: "F. Daniel",
    lastUpdated: "5 days ago",
  },
  {
    id: "PQ-003",
    providerId: "P-044",
    providerName: "Peter Okafor",
    initials: "PO",
    qualificationId: "Q-003",
    qualificationName: "Background Check",
    category: "Compliance",
    status: "Verified",
    issueDate: "2026-01-05",
    expiryDate: "2029-01-05",
    evidenceUrl: "/documents/po_background.pdf",
    branch: "Ikeja",
    service: "Dispatch",
    verifiedBy: "M. Bello",
    lastUpdated: "3 days ago",
  },
  {
    id: "PQ-004",
    providerId: "P-038",
    providerName: "Musa Bello",
    initials: "MB",
    qualificationId: "Q-001",
    qualificationName: "Defensive Riding Training",
    category: "Safety",
    status: "Missing",
    issueDate: null,
    expiryDate: null,
    evidenceUrl: null,
    branch: "Apapa",
    service: "Hauling",
    verifiedBy: null,
    lastUpdated: "1 week ago",
  },
  {
    id: "PQ-005",
    providerId: "P-038",
    providerName: "Musa Bello",
    initials: "MB",
    qualificationId: "Q-002",
    qualificationName: "Vehicle Roadworthiness",
    category: "Compliance",
    status: "Verified",
    issueDate: "2025-09-10",
    expiryDate: "2026-09-10",
    evidenceUrl: "/documents/mb_roadworthy.pdf",
    branch: "Apapa",
    service: "Hauling",
    verifiedBy: "F. Daniel",
    lastUpdated: "1 day ago",
  },
  {
    id: "PQ-006",
    providerId: "P-052",
    providerName: "Amina Yusuf",
    initials: "AY",
    qualificationId: "Q-004",
    qualificationName: "Medical Fitness Certificate",
    category: "Medical",
    status: "Pending",
    issueDate: null,
    expiryDate: null,
    evidenceUrl: null,
    branch: "Yaba",
    service: "Dispatch",
    verifiedBy: null,
    lastUpdated: "2 hours ago",
  },
  {
    id: "PQ-007",
    providerId: "P-061",
    providerName: "Chinedu Uche",
    initials: "CU",
    qualificationId: "Q-001",
    qualificationName: "Defensive Riding Training",
    category: "Safety",
    status: "Verified",
    issueDate: "2026-02-01",
    expiryDate: "2028-02-01",
    evidenceUrl: "/documents/cu_defensive.pdf",
    branch: "Lekki",
    service: "Dispatch",
    verifiedBy: "T. Adeyemi",
    lastUpdated: "4 days ago",
  },
  {
    id: "PQ-008",
    providerId: "P-061",
    providerName: "Chinedu Uche",
    initials: "CU",
    qualificationId: "Q-006",
    qualificationName: "First Aid Certification",
    category: "Training",
    status: "Expiring",
    issueDate: "2024-08-15",
    expiryDate: "2026-08-15",
    evidenceUrl: "/documents/cu_firstaid.pdf",
    branch: "Lekki",
    service: "Dispatch",
    verifiedBy: "A. Yusuf",
    lastUpdated: "6 days ago",
  },
  {
    id: "PQ-009",
    providerId: "P-029",
    providerName: "Kabiru Ahmed",
    initials: "KA",
    qualificationId: "Q-001",
    qualificationName: "Defensive Riding Training",
    category: "Safety",
    status: "Verified",
    issueDate: "2025-11-01",
    expiryDate: "2027-11-01",
    evidenceUrl: "/documents/ka_defensive.pdf",
    branch: "Apapa",
    service: "Hauling",
    verifiedBy: "M. Bello",
    lastUpdated: "2 weeks ago",
  },
  {
    id: "PQ-010",
    providerId: "P-073",
    providerName: "Esther Obi",
    initials: "EO",
    qualificationId: "Q-006",
    qualificationName: "First Aid Certification",
    category: "Training",
    status: "Verified",
    issueDate: "2025-12-10",
    expiryDate: "2028-12-10",
    evidenceUrl: "/documents/eo_firstaid.pdf",
    branch: "Ikeja",
    service: "All",
    verifiedBy: "T. Adeyemi",
    lastUpdated: "1 day ago",
  },
];

type StatusStyles = {
  [key: string]: { bg: string; text: string; dot: string; icon: React.ReactNode };
};

const STATUS_STYLES: StatusStyles = {
  Verified: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Expiring: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Missing: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
  Pending: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
};

type CategoryStyles = {
  [key: string]: { bg: string; text: string; icon: React.ReactNode };
};

const CATEGORY_STYLES: CategoryStyles = {
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
  Training: {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <GraduationCap className="h-3.5 w-3.5" />,
  },
  Medical: {
    bg: "bg-purple-bg",
    text: "text-purple",
    icon: <Activity className="h-3.5 w-3.5" />,
  },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Verified", "Expiring", "Missing", "Pending"];
const CATEGORY_FILTERS = ["All", "Safety", "Technical", "Compliance", "Training", "Medical"];

export default function ProviderQualificationsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ProviderQualification[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PROVIDER_QUALIFICATIONS);
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
      item.providerName.toLowerCase().includes(search.toLowerCase()) ||
      item.qualificationName.toLowerCase().includes(search.toLowerCase()) ||
      item.providerId.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesCategory = selectedCategory === "All" || item.category === selectedCategory;

    return matchesSearch && matchesBranch && matchesStatus && matchesCategory;
  });

  const stats = {
    total: data.length,
    verified: data.filter((d) => d.status === "Verified").length,
    expiring: data.filter((d) => d.status === "Expiring").length,
    missing: data.filter((d) => d.status === "Missing").length,
    pending: data.filter((d) => d.status === "Pending").length,
  };

  // Navigation handlers
  const goBack = () => {
    router.push("/workforce/qualifications");
  };

  const goToPerson = (personId: string) => {
    router.push(`/workforce/${personId}`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading provider qualifications...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedStatus !== "All" ||
    selectedCategory !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
    setSelectedCategory("All");
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
                Provider Qualifications
              </h1>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                Qualification status, expiry, and verification for all providers.
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
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Verified</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.verified}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+3 this month</span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5">
              <AlertCircle className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Expiring</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.expiring}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-amber">
            <Clock className="h-3 w-3" />
            <span>Needs renewal</span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-red-bg p-1.5">
              <XCircle className="h-4 w-4 text-red" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Missing</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.missing}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-red">
            <AlertCircle className="h-3 w-3" />
            <span>Action required</span>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Clock className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Pending</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{stats.pending}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-blue">
            <Clock className="h-3 w-3" />
            <span>In review</span>
          </div>
        </div>
      </div>

      // ... rest of the component (the table rendering code stays the same as before) ...

      {/* ===== FOOTER - Add this at the bottom ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <GraduationCap className="h-4 w-4" />
          WRK-11 · Provider Qualifications
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} qualifications shown
        </span>
      </div>
    </div>
  );
}