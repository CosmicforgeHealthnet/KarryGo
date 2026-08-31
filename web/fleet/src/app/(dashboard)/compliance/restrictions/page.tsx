// src/app/(dashboard)/compliance/restrictions/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  AlertOctagon,
} from "lucide-react";

// ==================== TYPES ====================

type Restriction = {
  id: string;
  reference: string;
  subjectType: "Provider" | "Asset" | "Document" | "Location";
  subjectId: string;
  subjectName: string;
  type: "Safety" | "Document" | "Training" | "Qualification" | "Compliance" | "Inspection" | "Maintenance";
  severity: "Critical" | "High" | "Medium" | "Low";
  reason: string;
  source: string;
  effectiveDate: string;
  expiryDate: string | null;
  clearanceState: "Active" | "PendingClearance" | "Cleared" | "Expired";
  clearanceAction: string | null;
  clearedBy: string | null;
  clearedAt: string | null;
  notes: string | null;
  branch: string;
  createdAt: string;
  createdBy: string;
};

// ==================== MOCK DATA ====================

const MOCK_RESTRICTIONS: Restriction[] = [
  {
    id: "RST-204",
    reference: "RST-2026-204",
    subjectType: "Asset",
    subjectId: "BR-204",
    subjectName: "Vehicle BR-204",
    type: "Safety",
    severity: "Critical",
    reason: "Failed inspection - Front brake",
    source: "Vehicle Inspection",
    effectiveDate: "2026-08-20T00:00:00Z",
    expiryDate: null,
    clearanceState: "Active",
    clearanceAction: "Complete repairs and pass re-inspection",
    clearedBy: null,
    clearedAt: null,
    notes: "Front brake pads worn beyond safe limit. Vehicle cannot operate until fixed.",
    branch: "Ikeja",
    createdAt: "2026-08-20T10:30:00Z",
    createdBy: "F. Daniel",
  },
  {
    id: "RST-771",
    reference: "RST-2026-771",
    subjectType: "Provider",
    subjectId: "P-044",
    subjectName: "Peter Okafor",
    type: "Document",
    severity: "High",
    reason: "Expired ID card",
    source: "Compliance Requirements",
    effectiveDate: "2026-08-20T00:00:00Z",
    expiryDate: null,
    clearanceState: "Active",
    clearanceAction: "Upload valid ID card for verification",
    clearedBy: null,
    clearedAt: null,
    notes: "Provider ID card expired on 2026-08-20. New document required.",
    branch: "Ikeja",
    createdAt: "2026-08-20T09:15:00Z",
    createdBy: "O. Adebayo",
  },
  {
    id: "RST-052",
    reference: "RST-2026-052",
    subjectType: "Provider",
    subjectId: "P-052",
    subjectName: "Amina Yusuf",
    type: "Training",
    severity: "Critical",
    reason: "Safety training overdue",
    source: "Training Compliance",
    effectiveDate: "2026-07-01T00:00:00Z",
    expiryDate: null,
    clearanceState: "Active",
    clearanceAction: "Complete safety training and upload certificate",
    clearedBy: null,
    clearedAt: null,
    notes: "Annual safety training not completed. Provider cannot be assigned.",
    branch: "Yaba",
    createdAt: "2026-07-01T08:00:00Z",
    createdBy: "A. Yusuf",
  },
  {
    id: "RST-119",
    reference: "RST-2026-119",
    subjectType: "Document",
    subjectId: "DOC-119",
    subjectName: "Licence - P-052",
    type: "Document",
    severity: "Medium",
    reason: "Licence expired",
    source: "Document Management",
    effectiveDate: "2026-08-10T00:00:00Z",
    expiryDate: null,
    clearanceState: "PendingClearance",
    clearanceAction: "New licence uploaded, awaiting verification",
    clearedBy: null,
    clearedAt: null,
    notes: "Provider licence expired. New document submitted for verification.",
    branch: "Yaba",
    createdAt: "2026-08-10T14:20:00Z",
    createdBy: "F. Daniel",
  },
  {
    id: "RST-188",
    reference: "RST-2026-188",
    subjectType: "Asset",
    subjectId: "TR-09",
    subjectName: "Vehicle TR-09",
    type: "Maintenance",
    severity: "Medium",
    reason: "Maintenance evidence required",
    source: "Maintenance Schedule",
    effectiveDate: "2026-08-15T00:00:00Z",
    expiryDate: null,
    clearanceState: "PendingClearance",
    clearanceAction: "Upload maintenance completion report",
    clearedBy: null,
    clearedAt: null,
    notes: "Scheduled maintenance completed but evidence not yet uploaded.",
    branch: "Apapa",
    createdAt: "2026-08-15T11:00:00Z",
    createdBy: "M. Bello",
  },
  {
    id: "RST-992",
    reference: "RST-2026-992",
    subjectType: "Provider",
    subjectId: "P-071",
    subjectName: "Chinedu Uche",
    type: "Qualification",
    severity: "Low",
    reason: "Drug test renewal due",
    source: "Provider Qualification",
    effectiveDate: "2026-08-25T00:00:00Z",
    expiryDate: null,
    clearanceState: "Active",
    clearanceAction: "Complete new drug test and upload results",
    clearedBy: null,
    clearedAt: null,
    notes: "Drug test expires in 3 days. Reminder sent to provider.",
    branch: "Ikeja",
    createdAt: "2026-08-22T07:30:00Z",
    createdBy: "O. Adebayo",
  },
  {
    id: "RST-445",
    reference: "RST-2026-445",
    subjectType: "Asset",
    subjectId: "VN-029",
    subjectName: "Vehicle VN-029",
    type: "Inspection",
    severity: "High",
    reason: "Overdue inspection",
    source: "Inspection Schedule",
    effectiveDate: "2026-08-01T00:00:00Z",
    expiryDate: "2026-08-31T00:00:00Z",
    clearanceState: "Expired",
    clearanceAction: "Schedule and complete inspection",
    clearedBy: null,
    clearedAt: null,
    notes: "Vehicle inspection overdue by 21 days. Immediate action required.",
    branch: "Lekki",
    createdAt: "2026-08-01T09:00:00Z",
    createdBy: "F. Daniel",
  },
  {
    id: "RST-664",
    reference: "RST-2026-664",
    subjectType: "Provider",
    subjectId: "P-031",
    subjectName: "Esther Obi",
    type: "Compliance",
    severity: "Medium",
    reason: "Compliance review pending",
    source: "Compliance Review",
    effectiveDate: "2026-08-18T00:00:00Z",
    expiryDate: null,
    clearanceState: "Cleared",
    clearanceAction: "Review completed - all clear",
    clearedBy: "F. Daniel",
    clearedAt: "2026-08-22T12:00:00Z",
    notes: "Compliance review completed. All requirements satisfied.",
    branch: "Lekki",
    createdAt: "2026-08-18T10:00:00Z",
    createdBy: "A. Yusuf",
  },
];

const SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Critical: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertOctagon className="h-3.5 w-3.5" />,
  },
  High: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
  },
  Medium: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Low: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
};

const CLEARANCE_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  PendingClearance: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Cleared: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Safety: { bg: "bg-red-bg", text: "text-red" },
  Document: { bg: "bg-blue-bg", text: "text-blue" },
  Training: { bg: "bg-green-bg", text: "text-green" },
  Qualification: { bg: "bg-purple-bg", text: "text-purple" },
  Compliance: { bg: "bg-amber-bg", text: "text-amber" },
  Inspection: { bg: "bg-surface-2", text: "text-ink-500" },
  Maintenance: { bg: "bg-blue-bg", text: "text-blue" },
};

const SEVERITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const CLEARANCE_FILTERS = ["All", "Active", "PendingClearance", "Cleared", "Expired"];
const SUBJECT_FILTERS = ["All", "Provider", "Asset", "Document", "Location"];
const TYPE_FILTERS = ["All", "Safety", "Document", "Training", "Qualification", "Compliance", "Inspection", "Maintenance"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function RestrictionsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Restriction[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedClearance, setSelectedClearance] = useState("All");
  const [selectedSubject, setSelectedSubject] = useState("All");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_RESTRICTIONS);
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
      item.subjectName.toLowerCase().includes(search.toLowerCase()) ||
      item.reason.toLowerCase().includes(search.toLowerCase()) ||
      item.subjectId.toLowerCase().includes(search.toLowerCase());

    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesClearance = selectedClearance === "All" || item.clearanceState === selectedClearance;
    const matchesSubject = selectedSubject === "All" || item.subjectType === selectedSubject;
    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesSeverity && matchesClearance && matchesSubject && matchesType && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading restrictions...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedSeverity !== "All" ||
    selectedClearance !== "All" ||
    selectedSubject !== "All" ||
    selectedType !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedSeverity("All");
    setSelectedClearance("All");
    setSelectedSubject("All");
    setSelectedType("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedSeverity !== "All" ? 1 : 0) +
    (selectedClearance !== "All" ? 1 : 0) +
    (selectedSubject !== "All" ? 1 : 0) +
    (selectedType !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

  const formatDate = (date: string | null) => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

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

  // Stats
  const totalActive = data.filter((d) => d.clearanceState === "Active").length;
  const totalPending = data.filter((d) => d.clearanceState === "PendingClearance").length;
  const totalCritical = data.filter((d) => d.severity === "Critical").length;
  const totalHigh = data.filter((d) => d.severity === "High").length;
  const totalCleared = data.filter((d) => d.clearanceState === "Cleared").length;

  const goToRestrictionDetail = (id: string) => {
    router.push(`/compliance/restrictions/${id}`);
  };

  const goToCreateRestriction = () => {
    router.push("/compliance/restrictions/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Restrictions
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Restricted subjects, reason/type, source, propagation state, effective time and clearance state.
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
          <button
            onClick={goToCreateRestriction}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Restriction
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-5">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-red">{totalActive}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending Clearance</div>
          <div className="text-lg font-bold text-amber">{totalPending}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Critical</div>
          <div className="text-lg font-bold text-red">{totalCritical}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Cleared</div>
          <div className="text-lg font-bold text-green">{totalCleared}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, subject, reason..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const severities = ["All", "Critical", "High", "Medium", "Low"];
            const currentIndex = severities.indexOf(selectedSeverity);
            const nextIndex = (currentIndex + 1) % severities.length;
            setSelectedSeverity(severities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedSeverity}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const clearances = ["All", "Active", "PendingClearance", "Cleared", "Expired"];
            const currentIndex = clearances.indexOf(selectedClearance);
            const nextIndex = (currentIndex + 1) % clearances.length;
            setSelectedClearance(clearances[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Shield className="h-4 w-4 text-ink-400" />
          {selectedClearance}
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

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Subject</div>
              <div className="flex gap-1.5 flex-wrap">
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
              <div className="flex gap-1.5 flex-wrap">
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
              <div className="flex gap-1.5 flex-wrap">
                {BRANCHES.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedBranch === branch
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {branch}
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

      {/* ===== RESTRICTIONS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Shield className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No restrictions found</h3>
          <p className="mt-1 text-sm text-ink-500">No restrictions match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[900px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Subject</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Severity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">State</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const severityStyle = SEVERITY_STYLES[item.severity] || SEVERITY_STYLES.Medium;
                  const clearanceStyle = CLEARANCE_STYLES[item.clearanceState] || CLEARANCE_STYLES.Active;
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Compliance;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToRestrictionDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                    >
                      <td className="px-4 py-3">
                        <div className="font-mono text-[11px] font-medium text-ink-950">{item.reference}</div>
                        <div className="text-[10px] text-ink-500 truncate max-w-[140px]">{item.reason}</div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex items-center gap-1.5">
                          <div>
                            <div className="text-xs font-medium text-ink-950">{item.subjectName}</div>
                            <div className="text-[9px] text-ink-500 font-mono">{item.subjectId}</div>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                          {severityStyle.icon}
                          {item.severity}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${clearanceStyle.bg} ${clearanceStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${clearanceStyle.dot}`} />
                          {item.clearanceState}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
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
          <Shield className="h-4 w-4" />
          CMP-05 · Restrictions
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} restrictions shown
        </span>
      </div>
    </div>
  );
}