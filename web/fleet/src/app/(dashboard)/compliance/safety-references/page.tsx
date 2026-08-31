// src/app/(dashboard)/compliance/safety-references/page.tsx

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
  BookOpen,
} from "lucide-react";

// ==================== TYPES ====================

type SafetyReference = {
  id: string;
  reference: string;
  title: string;
  description: string;
  type: "Incident" | "Accident" | "NearMiss" | "Hazard" | "SafetyConcern" | "Regulatory" | "Authority";
  externalReference: string | null;
  externalService: string | null;
  authority: string | null;
  verificationState: "Unverified" | "PendingVerification" | "Verified" | "Rejected" | "Expired";
  verificationDate: string | null;
  verifiedBy: string | null;
  sourceDate: string | null;
  location: string;
  branch: string;
  relatedWorkReference: string | null;
  relatedIncidentReference: string | null;
  relatedSafetyReportReference: string | null;
  createdAt: string;
  createdBy: string;
  updatedAt: string;
  updatedBy: string;
  notes: string | null;
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_REFERENCES: SafetyReference[] = [
  {
    id: "SREF-001",
    reference: "SREF-2026-001",
    title: "Police Report - Collision on Opebi Road",
    description: "Official police report for the collision incident involving provider P-044 on Opebi Road, Ikeja.",
    type: "Incident",
    externalReference: "PR-2026-8842",
    externalService: "Police",
    authority: "Nigeria Police Force - Ikeja Division",
    verificationState: "Verified",
    verificationDate: "2026-08-22T09:30:00Z",
    verifiedBy: "F. Daniel",
    sourceDate: "2026-08-22T08:00:00Z",
    location: "Opebi Road, Ikeja",
    branch: "Ikeja",
    relatedWorkReference: "KG-8479",
    relatedIncidentReference: "INC-771",
    relatedSafetyReportReference: null,
    createdAt: "2026-08-22T09:30:00Z",
    createdBy: "F. Daniel",
    updatedAt: "2026-08-22T09:30:00Z",
    updatedBy: "F. Daniel",
    notes: "Police report confirms other vehicle at fault. Official document uploaded.",
    tags: ["Police", "Incident", "Verified"],
    sourceFreshness: "2026-08-22T09:30:00Z",
  },
  {
    id: "SREF-002",
    reference: "SREF-2026-002",
    title: "Safety Authority - Fuel Handling Protocol",
    description: "Regulatory authority guidance on fuel handling and spill prevention at depot facilities.",
    type: "Regulatory",
    externalReference: "SA-2026-112",
    externalService: "Regulatory",
    authority: "National Safety Authority - Petroleum Division",
    verificationState: "Verified",
    verificationDate: "2026-08-15T14:00:00Z",
    verifiedBy: "F. Daniel",
    sourceDate: "2026-08-01T00:00:00Z",
    location: "National",
    branch: "All branches",
    relatedWorkReference: null,
    relatedIncidentReference: null,
    relatedSafetyReportReference: "SR-001",
    createdAt: "2026-08-15T14:00:00Z",
    createdBy: "F. Daniel",
    updatedAt: "2026-08-15T14:00:00Z",
    updatedBy: "F. Daniel",
    notes: "Regulatory guidance applicable to all depot operations.",
    tags: ["Regulatory", "Fuel", "Safety"],
    sourceFreshness: "2026-08-15T14:00:00Z",
  },
  {
    id: "SREF-003",
    reference: "SREF-2026-003",
    title: "External Safety Audit - Apapa Depot",
    description: "Third-party safety audit report for Apapa Depot conducted by SafeOps International.",
    type: "Authority",
    externalReference: "AUD-2026-442",
    externalService: "SafeOps International",
    authority: "SafeOps International",
    verificationState: "PendingVerification",
    verificationDate: null,
    verifiedBy: null,
    sourceDate: "2026-08-20T00:00:00Z",
    location: "Apapa Depot",
    branch: "Apapa",
    relatedWorkReference: null,
    relatedIncidentReference: null,
    relatedSafetyReportReference: null,
    createdAt: "2026-08-21T10:00:00Z",
    createdBy: "M. Bello",
    updatedAt: "2026-08-21T10:00:00Z",
    updatedBy: "M. Bello",
    notes: "Audit report received. Pending internal review and verification.",
    tags: ["Audit", "External", "Pending"],
    sourceFreshness: "2026-08-21T10:00:00Z",
  },
  {
    id: "SREF-004",
    reference: "SREF-2026-004",
    title: "Unconfirmed Vendor Safety Report",
    description: "Safety report from third-party vendor regarding equipment safety concerns. Unverified source.",
    type: "SafetyConcern",
    externalReference: "VS-2026-009",
    externalService: "Vendor",
    authority: null,
    verificationState: "Unverified",
    verificationDate: null,
    verifiedBy: null,
    sourceDate: "2026-08-18T00:00:00Z",
    location: "Ikeja Depot",
    branch: "Ikeja",
    relatedWorkReference: null,
    relatedIncidentReference: null,
    relatedSafetyReportReference: null,
    createdAt: "2026-08-19T09:00:00Z",
    createdBy: "O. Adebayo",
    updatedAt: "2026-08-19T09:00:00Z",
    updatedBy: "O. Adebayo",
    notes: "Vendor report requires verification before any operational consequences can be authorized.",
    tags: ["Vendor", "Unverified", "Equipment"],
    sourceFreshness: "2026-08-19T09:00:00Z",
  },
  {
    id: "SREF-005",
    reference: "SREF-2026-005",
    title: "LASEMA Incident Report - Lekki Flooding",
    description: "LASEMA report on localized flooding in Lekki area affecting multiple logistics operations.",
    type: "Incident",
    externalReference: "LASEMA-2026-771",
    externalService: "LASEMA",
    authority: "Lagos State Emergency Management Agency",
    verificationState: "Verified",
    verificationDate: "2026-08-14T11:00:00Z",
    verifiedBy: "F. Daniel",
    sourceDate: "2026-08-14T00:00:00Z",
    location: "Lekki",
    branch: "Lekki",
    relatedWorkReference: "KG-8504",
    relatedIncidentReference: null,
    relatedSafetyReportReference: null,
    createdAt: "2026-08-14T11:00:00Z",
    createdBy: "F. Daniel",
    updatedAt: "2026-08-14T11:00:00Z",
    updatedBy: "F. Daniel",
    notes: "Official LASEMA report confirms flooding impact. Operational adjustments made.",
    tags: ["LASEMA", "Flooding", "Verified"],
    sourceFreshness: "2026-08-14T11:00:00Z",
  },
  {
    id: "SREF-006",
    reference: "SREF-2026-006",
    title: "NEMA Safety Guidelines - Hazardous Materials",
    description: "NEMA safety guidelines for transportation and handling of hazardous materials.",
    type: "Regulatory",
    externalReference: "NEMA-2026-221",
    externalService: "NEMA",
    authority: "National Emergency Management Agency",
    verificationState: "PendingVerification",
    verificationDate: null,
    verifiedBy: null,
    sourceDate: "2026-08-10T00:00:00Z",
    location: "National",
    branch: "All branches",
    relatedWorkReference: null,
    relatedIncidentReference: null,
    relatedSafetyReportReference: null,
    createdAt: "2026-08-11T13:00:00Z",
    createdBy: "F. Daniel",
    updatedAt: "2026-08-11T13:00:00Z",
    updatedBy: "F. Daniel",
    notes: "New guidelines received. Review in progress. No operational changes until verification complete.",
    tags: ["NEMA", "Regulatory", "Hazardous"],
    sourceFreshness: "2026-08-11T13:00:00Z",
  },
  {
    id: "SREF-007",
    reference: "SREF-2026-007",
    title: "Expired Vehicle Inspection Standard",
    description: "Previous vehicle inspection standard that has been superseded.",
    type: "Authority",
    externalReference: "VIS-2025-881",
    externalService: "Government",
    authority: "Federal Road Safety Commission",
    verificationState: "Expired",
    verificationDate: "2025-12-31T00:00:00Z",
    verifiedBy: "F. Daniel",
    sourceDate: "2025-12-31T00:00:00Z",
    location: "National",
    branch: "All branches",
    relatedWorkReference: null,
    relatedIncidentReference: null,
    relatedSafetyReportReference: null,
    createdAt: "2025-12-31T10:00:00Z",
    createdBy: "F. Daniel",
    updatedAt: "2026-08-01T00:00:00Z",
    updatedBy: "System",
    notes: "Standard expired. Superseded by current 2026 standard.",
    tags: ["Expired", "Vehicle", "Standard"],
    sourceFreshness: "2026-08-01T00:00:00Z",
  },
  {
    id: "SREF-008",
    reference: "SREF-2026-008",
    title: "FRSC Incident Report - TR-112",
    description: "FRSC incident report related to TR-112 safety concern and subsequent maintenance.",
    type: "Incident",
    externalReference: "FRSC-2026-112",
    externalService: "FRSC",
    authority: "Federal Road Safety Commission",
    verificationState: "Verified",
    verificationDate: "2026-08-21T11:30:00Z",
    verifiedBy: "M. Bello",
    sourceDate: "2026-08-21T00:00:00Z",
    location: "Apapa",
    branch: "Apapa",
    relatedWorkReference: "KG-8512",
    relatedIncidentReference: "INC-777",
    relatedSafetyReportReference: null,
    createdAt: "2026-08-21T11:30:00Z",
    createdBy: "M. Bello",
    updatedAt: "2026-08-21T11:30:00Z",
    updatedBy: "M. Bello",
    notes: "FRSC confirms vehicle safety issue resolved.",
    tags: ["FRSC", "Incident", "Verified"],
    sourceFreshness: "2026-08-21T11:30:00Z",
  },
  {
    id: "SREF-009",
    reference: "SREF-2026-009",
    title: "Insurance Claim Reference - BR-204",
    description: "Insurance claim reference for BR-204 damage from collision incident.",
    type: "Incident",
    externalReference: "IC-2026-8842",
    externalService: "Insurance",
    authority: "Allianz Insurance",
    verificationState: "PendingVerification",
    verificationDate: null,
    verifiedBy: null,
    sourceDate: "2026-08-22T00:00:00Z",
    location: "Ikeja",
    branch: "Ikeja",
    relatedWorkReference: "KG-8479",
    relatedIncidentReference: "INC-771",
    relatedSafetyReportReference: null,
    createdAt: "2026-08-22T10:00:00Z",
    createdBy: "F. Daniel",
    updatedAt: "2026-08-22T10:00:00Z",
    updatedBy: "F. Daniel",
    notes: "Insurance claim submitted. Awaiting assessment confirmation.",
    tags: ["Insurance", "Claim", "Pending"],
    sourceFreshness: "2026-08-22T10:00:00Z",
  },
];

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Incident: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Accident: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  NearMiss: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Hazard: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  SafetyConcern: { bg: "bg-purple-bg", text: "text-purple", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Regulatory: { bg: "bg-green-bg", text: "text-green", icon: <BookOpen className="h-3.5 w-3.5" /> },
  Authority: { bg: "bg-blue-bg", text: "text-blue", icon: <Shield className="h-3.5 w-3.5" /> },
};

const VERIFICATION_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Unverified: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  PendingVerification: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Verified: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Rejected: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Expired: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
};

const TYPE_FILTERS = ["All", "Incident", "Accident", "NearMiss", "Hazard", "SafetyConcern", "Regulatory", "Authority"];
const VERIFICATION_FILTERS = ["All", "Unverified", "PendingVerification", "Verified", "Rejected", "Expired"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function SafetyReferencePage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SafetyReference[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedVerification, setSelectedVerification] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_REFERENCES);
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
      item.title.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      (item.authority && item.authority.toLowerCase().includes(search.toLowerCase())) ||
      (item.externalReference && item.externalReference.toLowerCase().includes(search.toLowerCase()));

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesVerification = selectedVerification === "All" || item.verificationState === selectedVerification;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesType && matchesVerification && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading safety references...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedType !== "All" ||
    selectedVerification !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
    setSelectedVerification("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedType !== "All" ? 1 : 0) +
    (selectedVerification !== "All" ? 1 : 0) +
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
  const totalVerified = data.filter((d) => d.verificationState === "Verified").length;
  const totalUnverified = data.filter((d) => d.verificationState === "Unverified").length;
  const totalPending = data.filter((d) => d.verificationState === "PendingVerification").length;
  const totalIncident = data.filter((d) => d.type === "Incident").length;

  const goToReferenceDetail = (id: string) => {
    router.push(`/compliance/safety-references/${id}`);
  };

  const goToCreateReference = () => {
    router.push("/compliance/safety-references/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Safety Reference
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            External incident/safety references, authoritative services, verification states and unconfirmed warnings.
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
            onClick={goToCreateReference}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Record Reference
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Verified</div>
          <div className="text-lg font-bold text-green">{totalVerified}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Unverified</div>
          <div className="text-lg font-bold text-ink-500">{totalUnverified}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Incident References</div>
          <div className="text-lg font-bold text-red">{totalIncident}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, title, authority..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Incident", "Accident", "NearMiss", "Hazard", "SafetyConcern", "Regulatory", "Authority"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <BookOpen className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Unverified", "PendingVerification", "Verified", "Rejected", "Expired"];
            const currentIndex = statuses.indexOf(selectedVerification);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedVerification(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Shield className="h-4 w-4 text-ink-400" />
          {selectedVerification}
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

      {/* ===== REFERENCES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <BookOpen className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No safety references found</h3>
          <p className="mt-1 text-sm text-ink-500">No references match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Authority</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Verification</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Related</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.SafetyConcern;
                  const verificationStyle = VERIFICATION_STYLES[item.verificationState] || VERIFICATION_STYLES.Unverified;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToReferenceDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.title}</div>
                        <div className="font-mono text-[10px] text-ink-500 mt-0.5">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.icon}
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs text-ink-950 truncate max-w-[120px]">
                          {item.authority || "—"}
                        </div>
                        {item.externalReference && (
                          <div className="text-[9px] text-ink-400 font-mono truncate max-w-[120px]">
                            Ref: {item.externalReference}
                          </div>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${verificationStyle.bg} ${verificationStyle.text}`}>
                          {verificationStyle.icon}
                          {item.verificationState}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3">
                        <div className="flex flex-col gap-0.5">
                          {item.relatedIncidentReference && (
                            <span className="text-[9px] text-blue font-mono">{item.relatedIncidentReference}</span>
                          )}
                          {item.relatedSafetyReportReference && (
                            <span className="text-[9px] text-amber font-mono">{item.relatedSafetyReportReference}</span>
                          )}
                          {!item.relatedIncidentReference && !item.relatedSafetyReportReference && (
                            <span className="text-[9px] text-ink-400">—</span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ===== SAFETY WARNING ===== */}
      <div className="rounded-2xl border border-amber/20 bg-amber-bg/30 p-5 shadow-sm">
        <div className="flex items-start gap-3">
          <div className="rounded-full bg-amber-bg p-2 flex-shrink-0">
            <AlertTriangle className="h-5 w-5 text-amber" />
          </div>
          <div>
            <h4 className="text-sm font-semibold text-amber">Unverified References Cannot Authorize Consequences</h4>
            <p className="mt-1 text-sm text-amber/80">
              Unverified references must not authorize financial or operational consequences. 
              Always verify external references before taking action based on them.
            </p>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          CMP-13 · Safety Reference
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} references shown
        </span>
      </div>
    </div>
  );
}