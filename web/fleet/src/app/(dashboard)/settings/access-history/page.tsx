// src/app/(dashboard)/settings/access-history/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  History,
  RefreshCw,
  Clock,
  AlertTriangle,
  Eye,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Download,
  FileText,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Shield,
  Lock,
  EyeOff,
  Settings,
  AlertCircle,
  CheckCircle2,
  Home,
  Users,
  Tag,
  Info,
  ListChecks,
  Printer,
  Edit3,
  Trash2,
  MoreHorizontal,
  ExternalLink,
  FolderOpen,
  HardDrive,
  Database,
  Cloud,
  GitBranch,
  BookOpen,
  Book,
  ArrowRight,
} from "lucide-react";

// ==================== TYPES ====================

type AccessEvent = {
  id: string;
  reference: string;
  action: string;
  resource: string;
  actor: {
    id: string;
    name: string;
    email: string;
  };
  role: string;
  branch: string;
  timestamp: string;
  riskFlag: "Low" | "Medium" | "High" | "Critical";
  requestReference: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_ACCESS_EVENTS: AccessEvent[] = [
  {
    id: "AE-001",
    reference: "AUD-2026-001",
    action: "Login",
    resource: "Authentication",
    actor: { id: "S-001", name: "Favour Daniel", email: "favour@apexlogistics.com" },
    role: "Organization Admin",
    branch: "Ikeja",
    timestamp: "2026-08-23T10:30:00Z",
    riskFlag: "Low",
    requestReference: "REQ-001",
  },
  {
    id: "AE-002",
    reference: "AUD-2026-002",
    action: "Role Update",
    resource: "Staff: S-005",
    actor: { id: "S-001", name: "Favour Daniel", email: "favour@apexlogistics.com" },
    role: "Organization Admin",
    branch: "Ikeja",
    timestamp: "2026-08-22T16:00:00Z",
    riskFlag: "High",
    requestReference: "APR-2026-001",
  },
  {
    id: "AE-003",
    reference: "AUD-2026-003",
    action: "Approval",
    resource: "Request: AR-004",
    actor: { id: "S-006", name: "Esther Obi", email: "esther@apexlogistics.com" },
    role: "Compliance Officer",
    branch: "All branches",
    timestamp: "2026-08-22T17:00:00Z",
    riskFlag: "Medium",
    requestReference: "AR-004",
  },
  {
    id: "AE-004",
    reference: "AUD-2026-004",
    action: "Staff Suspension",
    resource: "Staff: S-007",
    actor: { id: "S-001", name: "Favour Daniel", email: "favour@apexlogistics.com" },
    role: "Organization Admin",
    branch: "Apapa",
    timestamp: "2026-08-22T16:30:00Z",
    riskFlag: "Critical",
    requestReference: "AR-004",
  },
  {
    id: "AE-005",
    reference: "AUD-2026-005",
    action: "Wallet Withdrawal",
    resource: "₦1,200,000 · UBA ••3901",
    actor: { id: "S-002", name: "Oluwaseun Adebayo", email: "seun@apexlogistics.com" },
    role: "Operations Manager",
    branch: "Ikeja",
    timestamp: "2026-08-23T08:30:00Z",
    riskFlag: "High",
    requestReference: "APR-2026-002",
  },
  {
    id: "AE-006",
    reference: "AUD-2026-006",
    action: "API Access",
    resource: "Production API Key",
    actor: { id: "S-006", name: "Esther Obi", email: "esther@apexlogistics.com" },
    role: "Compliance Officer",
    branch: "All branches",
    timestamp: "2026-08-22T10:00:00Z",
    riskFlag: "Medium",
    requestReference: null,
  },
  {
    id: "AE-007",
    reference: "AUD-2026-007",
    action: "Allocation",
    resource: "P-044 · ₦120,000",
    actor: { id: "S-003", name: "Musa Bello", email: "musa@apexlogistics.com" },
    role: "Fleet Manager",
    branch: "Lekki",
    timestamp: "2026-08-23T08:00:00Z",
    riskFlag: "Low",
    requestReference: "APR-2026-003",
  },
];

const RISK_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Low: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  High: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const RISK_FILTERS = ["All", "Low", "Medium", "High", "Critical"];

export default function AccessHistoryPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<AccessEvent[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedRisk, setSelectedRisk] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_ACCESS_EVENTS);
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
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.action.toLowerCase().includes(search.toLowerCase()) ||
      item.resource.toLowerCase().includes(search.toLowerCase()) ||
      item.actor.name.toLowerCase().includes(search.toLowerCase());

    const matchesRisk = selectedRisk === "All" || item.riskFlag === selectedRisk;

    return matchesSearch && matchesRisk;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading access history...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedRisk !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedRisk("All");
  };

  const filterCount = (selectedRisk !== "All" ? 1 : 0);

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

  const goToAuditDetail = (id: string) => {
    router.push(`/settings/audit/${id}`);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Access History
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Immutable role/access and approval decision history.
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
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by action, resource, actor..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const risks = ["All", "Low", "Medium", "High", "Critical"];
            const currentIndex = risks.indexOf(selectedRisk);
            const nextIndex = (currentIndex + 1) % risks.length;
            setSelectedRisk(risks[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedRisk}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
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
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== ACCESS EVENTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <History className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No access events found</h3>
          <p className="mt-1 text-sm text-ink-500">No events match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[1000px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Event</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Action</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Actor</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Risk</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Time</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const riskStyle = RISK_STYLES[item.riskFlag] || RISK_STYLES.Low;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToAuditDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-mono text-[11px] font-medium text-ink-950">{item.reference}</div>
                        <div className="text-[10px] text-ink-500">{item.resource}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.action}</td>
                      <td className="px-4 py-3">
                        <div className="text-xs font-medium text-ink-950">{item.actor.name}</div>
                        <div className="text-[10px] text-ink-500">{item.role}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${riskStyle.bg} ${riskStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${riskStyle.dot}`} />
                          {item.riskFlag}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.timestamp)}</td>
                      <td className="px-4 py-3 text-right">
                        <button className="text-ink-400 group-hover:text-primary transition-colors">
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

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <History className="h-4 w-4" />
          IAM-12 · Access History
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} events shown
        </span>
      </div>
    </div>
  );
}