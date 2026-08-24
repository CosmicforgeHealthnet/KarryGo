// src/app/(dashboard)/compliance/requirements/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  ListChecks,
  Search,
  RefreshCw,
  Filter,
  SlidersHorizontal,
  Archive,
  X,
  ChevronDown,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Users,
  MoreHorizontal,
  CalendarDays,
  Download,
  FileText,
  Plus,
  DollarSign,
  Building2,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Tag,
  Shield,
  CreditCard,
  Receipt,
  Star,
  UserPlus,
  Send,
  Edit3,
  Trash2,
  Zap,
  Target,
  PieChart,
  BarChart3,
  FileCheck,
  HandCoins,
  Calendar,
  Check,
  XCircle,
  Activity,
  LineChart,
  BarChart,
  Gauge,
  ShieldAlert,
  ShieldCheck,
  ShieldQuestion,
  FileSearch,
  ClipboardList,
  BookOpen,
  FileSignature,
  FolderOpen,
} from "lucide-react";

type ComplianceRequirement = {
  id: string;
  reference: string;
  name: string;
  description: string;
  subject: "Provider" | "Asset" | "Document";
  type: "Document" | "Qualification" | "Inspection" | "Training" | "Certification";
  status: "Active" | "Inactive" | "Archived";
  validityPeriod: string;
  requiredFor: string[];
  documentTypes: string[];
  verificationMethod: string;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  branch: string;
  notes: string | null;
};

// Mock data
const REQUIREMENTS: ComplianceRequirement[] = [
  {
    id: "REQ-001",
    reference: "REQ-2026-001",
    name: "Vehicle Safety Inspection",
    description: "Annual safety inspection for all fleet vehicles.",
    subject: "Asset",
    type: "Inspection",
    status: "Active",
    validityPeriod: "1 year",
    requiredFor: ["All Vehicles", "Motorcycles", "Trucks", "Vans"],
    documentTypes: ["Inspection Report", "Safety Certificate"],
    verificationMethod: "External Inspection Authority",
    createdAt: "2026-01-15",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: "Inspection must be conducted by certified authority.",
  },
  {
    id: "REQ-002",
    reference: "REQ-2026-002",
    name: "Provider Certification",
    description: "Mandatory certification for all active providers.",
    subject: "Provider",
    type: "Certification",
    status: "Active",
    validityPeriod: "2 years",
    requiredFor: ["Dispatch Rider", "Hauling Driver", "Dispatch Supervisor"],
    documentTypes: ["Certificate", "Training Record"],
    verificationMethod: "Internal Review",
    createdAt: "2026-02-01",
    updatedAt: "2026-08-20",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: null,
  },
  {
    id: "REQ-003",
    reference: "REQ-2026-003",
    name: "Document Renewal",
    description: "Regular renewal of all operational documents.",
    subject: "Document",
    type: "Document",
    status: "Active",
    validityPeriod: "1 year",
    requiredFor: ["All Staff", "All Providers"],
    documentTypes: ["ID Card", "License", "Insurance"],
    verificationMethod: "Document Verification Service",
    createdAt: "2026-03-01",
    updatedAt: "2026-08-20",
    createdBy: "O. Adebayo",
    branch: "All branches",
    notes: "Document renewal reminders will be sent 30 days before expiry.",
  },
  {
    id: "REQ-004",
    reference: "REQ-2026-004",
    name: "Safety Training",
    description: "Annual safety training for all staff and providers.",
    subject: "Provider",
    type: "Training",
    status: "Active",
    validityPeriod: "1 year",
    requiredFor: ["All Staff", "All Providers"],
    documentTypes: ["Training Certificate", "Attendance Record"],
    verificationMethod: "Internal Training Team",
    createdAt: "2026-04-01",
    updatedAt: "2026-08-20",
    createdBy: "A. Yusuf",
    branch: "All branches",
    notes: "Training must be completed before the end of Q4.",
  },
  {
    id: "REQ-005",
    reference: "REQ-2026-005",
    name: "Asset Registration",
    description: "Registration of all fleet assets.",
    subject: "Asset",
    type: "Document",
    status: "Inactive",
    validityPeriod: "3 years",
    requiredFor: ["All Vehicles"],
    documentTypes: ["Registration Certificate", "Insurance"],
    verificationMethod: "Government Agency",
    createdAt: "2026-05-01",
    updatedAt: "2026-07-15",
    createdBy: "F. Daniel",
    branch: "Apapa",
    notes: "Currently under review for updates.",
  },
  {
    id: "REQ-006",
    reference: "REQ-2026-006",
    name: "Warehouse Safety Standards",
    description: "Safety standards for warehouse operations.",
    subject: "Asset",
    type: "Inspection",
    status: "Active",
    validityPeriod: "6 months",
    requiredFor: ["Warehouse", "Storage Area"],
    documentTypes: ["Safety Inspection Report"],
    verificationMethod: "Internal Safety Team",
    createdAt: "2026-06-15",
    updatedAt: "2026-08-20",
    createdBy: "O. Adebayo",
    branch: "Ikeja",
    notes: null,
  },
  {
    id: "REQ-007",
    reference: "REQ-2026-007",
    name: "Provider Drug Test",
    description: "Mandatory drug test for all providers.",
    subject: "Provider",
    type: "Certification",
    status: "Archived",
    validityPeriod: "1 year",
    requiredFor: ["Dispatch Rider", "Hauling Driver"],
    documentTypes: ["Test Result"],
    verificationMethod: "External Testing Lab",
    createdAt: "2025-10-01",
    updatedAt: "2026-01-15",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: "Archived - replaced by new policy.",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Inactive: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Archived: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Archive className="h-3.5 w-3.5" />,
  },
};

const SUBJECT_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Provider: { bg: "bg-blue-bg", text: "text-blue", icon: <Users className="h-3.5 w-3.5" /> },
  Asset: { bg: "bg-green-bg", text: "text-green", icon: <Building2 className="h-3.5 w-3.5" /> },
  Document: { bg: "bg-amber-bg", text: "text-amber", icon: <FileText className="h-3.5 w-3.5" /> },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Document: { bg: "bg-blue-bg", text: "text-blue" },
  Qualification: { bg: "bg-purple-bg", text: "text-purple" },
  Inspection: { bg: "bg-amber-bg", text: "text-amber" },
  Training: { bg: "bg-green-bg", text: "text-green" },
  Certification: { bg: "bg-red-bg", text: "text-red" },
};

const STATUS_FILTERS = ["All", "Active", "Inactive", "Archived"];
const SUBJECT_FILTERS = ["All", "Provider", "Asset", "Document"];
const TYPE_FILTERS = ["All", "Document", "Qualification", "Inspection", "Training", "Certification"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function ComplianceRequirementsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedSubject, setSelectedSubject] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ComplianceRequirement[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(REQUIREMENTS);
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
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.requiredFor.some(r => r.toLowerCase().includes(search.toLowerCase()));

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesSubject = selectedSubject === "All" || item.subject === selectedSubject;
    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesSubject && matchesType && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading compliance requirements...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedSubject !== "All" || selectedType !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedSubject("All");
    setSelectedType("All");
    setSelectedBranch("All branches");
  };

  const goToRequirementDetail = (requirementId: string) => {
    router.push(`/compliance/requirements/${requirementId}`);
  };

  const goToCreateRequirement = () => {
    router.push("/compliance/requirements/create");
  };

  // Stats
  const totalRequirements = data.length;
  const activeCount = data.filter((d) => d.status === "Active").length;
  const inactiveCount = data.filter((d) => d.status === "Inactive").length;
  const providerCount = data.filter((d) => d.subject === "Provider").length;
  const assetCount = data.filter((d) => d.subject === "Asset").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Compliance Requirements
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Requirement subject, service/location applicability, required document/qualification, validity and status.
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
          <button
            onClick={goToCreateRequirement}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Requirement
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{totalRequirements}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-green">{activeCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Provider</div>
          <div className="text-lg font-bold text-blue">{providerCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Asset</div>
          <div className="text-lg font-bold text-green">{assetCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, reference, or applicable role..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Active", "Inactive", "Archived"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const subjects = ["All", "Provider", "Asset", "Document"];
            const currentIndex = subjects.indexOf(selectedSubject);
            const nextIndex = (currentIndex + 1) % subjects.length;
            setSelectedSubject(subjects[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Shield className="h-4 w-4 text-ink-400" />
          {selectedSubject}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const types = ["All", "Document", "Qualification", "Inspection", "Training", "Certification"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Tag className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Building2 className="h-4 w-4 text-ink-400" />
          {selectedBranch}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedSubject !== "All" ? 1 : 0) + (selectedType !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Subject</div>
              <div className="flex gap-1.5">
                {SUBJECT_FILTERS.map((subject) => (
                  <button
                    key={subject}
                    onClick={() => setSelectedSubject(subject)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedSubject === subject
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {subject}
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
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== REQUIREMENTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <ListChecks className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No requirements found</h3>
          <p className="mt-1 text-sm text-ink-500">No compliance requirements match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[950px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Requirement</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Subject</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Validity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
                  const subjectStyle = SUBJECT_STYLES[item.subject] || SUBJECT_STYLES.Document;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Document;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToRequirementDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950">{item.name}</div>
                        <div className="text-[11px] text-ink-500 font-mono">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${subjectStyle.bg} ${subjectStyle.text}`}>
                          {subjectStyle.icon}
                          {item.subject}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600">{item.validityPeriod}</td>
                      <td className="px-4 py-3 text-ink-600">{item.branch}</td>
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
          <ListChecks className="h-4 w-4" />
          CMP-02 · Compliance Requirements
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} requirements shown
        </span>
      </div>
    </div>
  );
}