// src/app/(dashboard)/compliance/proof-policies/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  FileText,
  FileCheck,
  FileSignature,
  FolderOpen,
  MapPin,
  Briefcase,
} from "lucide-react";

// ==================== TYPES ====================

type ProofPolicy = {
  id: string;
  reference: string;
  name: string;
  description: string;
  serviceType: string[];
  customerTypes: string[];
  proofTypes: ("OTP" | "Signature" | "Photo" | "Scan" | "Location" | "Video" | "Audio" | "Document")[];
  required: boolean;
  verificationMethod: "Manual" | "Automated" | "Hybrid";
  retentionPeriod: string;
  version: number;
  status: "Draft" | "Published" | "Archived" | "Superseded";
  publishedAt: string | null;
  publishedBy: string | null;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  branch: string;
  notes: string | null;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_POLICIES: ProofPolicy[] = [
  {
    id: "PP-001",
    reference: "PP-2026-001",
    name: "Delivery Proof - OTP Verification",
    description: "Requires OTP verification for all dispatch deliveries to confirm successful delivery.",
    serviceType: ["Dispatch"],
    customerTypes: ["All"],
    proofTypes: ["OTP"],
    required: true,
    verificationMethod: "Automated",
    retentionPeriod: "90 days",
    version: 3,
    status: "Published",
    publishedAt: "2026-07-01T10:00:00Z",
    publishedBy: "F. Daniel",
    createdAt: "2026-06-15T14:30:00Z",
    updatedAt: "2026-07-01T10:00:00Z",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: "OTP generated at dispatch and verified by recipient at delivery.",
    tags: ["Delivery", "OTP", "Automated"],
  },
  {
    id: "PP-002",
    reference: "PP-2026-002",
    name: "Signature Proof - High Value Items",
    description: "Requires recipient signature for high-value items above ₦500,000.",
    serviceType: ["Dispatch", "Hauling"],
    customerTypes: ["Corporate", "Enterprise"],
    proofTypes: ["Signature", "Photo"],
    required: true,
    verificationMethod: "Hybrid",
    retentionPeriod: "365 days",
    version: 2,
    status: "Published",
    publishedAt: "2026-06-20T09:00:00Z",
    publishedBy: "F. Daniel",
    createdAt: "2026-06-10T11:00:00Z",
    updatedAt: "2026-06-20T09:00:00Z",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: "Signature must be collected on delivery device. Photo of recipient with package required.",
    tags: ["HighValue", "Signature", "Photo"],
  },
  {
    id: "PP-003",
    reference: "PP-2026-003",
    name: "Photo Proof - Package Condition",
    description: "Requires photo of package at delivery to confirm condition and proper handling.",
    serviceType: ["Dispatch", "Hauling", "Courier"],
    customerTypes: ["All"],
    proofTypes: ["Photo"],
    required: true,
    verificationMethod: "Manual",
    retentionPeriod: "180 days",
    version: 1,
    status: "Published",
    publishedAt: "2026-05-15T14:00:00Z",
    publishedBy: "F. Daniel",
    createdAt: "2026-05-01T09:00:00Z",
    updatedAt: "2026-05-15T14:00:00Z",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: "Photo must clearly show package condition and delivery location.",
    tags: ["Photo", "Condition", "Delivery"],
  },
  {
    id: "PP-004",
    reference: "PP-2026-004",
    name: "Location Proof - Service Area Validation",
    description: "Requires location validation to confirm service was performed at the correct location.",
    serviceType: ["Dispatch", "Hauling"],
    customerTypes: ["All"],
    proofTypes: ["Location"],
    required: true,
    verificationMethod: "Automated",
    retentionPeriod: "90 days",
    version: 2,
    status: "Published",
    publishedAt: "2026-06-01T08:00:00Z",
    publishedBy: "F. Daniel",
    createdAt: "2026-05-20T13:00:00Z",
    updatedAt: "2026-06-01T08:00:00Z",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: "GPS location captured at service completion. Must match service location within 100m.",
    tags: ["Location", "GPS", "Automated"],
  },
  {
    id: "PP-005",
    reference: "PP-2026-005",
    name: "Scan Proof - Documentation for Hauling",
    description: "Requires scan of relevant documentation for hauling services, including delivery notes and customs documents.",
    serviceType: ["Hauling"],
    customerTypes: ["All"],
    proofTypes: ["Scan", "Document"],
    required: true,
    verificationMethod: "Manual",
    retentionPeriod: "365 days",
    version: 1,
    status: "Draft",
    publishedAt: null,
    publishedBy: null,
    createdAt: "2026-08-01T10:00:00Z",
    updatedAt: "2026-08-15T14:00:00Z",
    createdBy: "O. Adebayo",
    branch: "All branches",
    notes: "Hauling-specific documentation requirements being finalized.",
    tags: ["Hauling", "Documentation", "Scan"],
  },
  {
    id: "PP-006",
    reference: "PP-2026-006",
    name: "Video Proof - Incident Documentation",
    description: "Requires video documentation for incident reports and safety concerns.",
    serviceType: ["All"],
    customerTypes: ["All"],
    proofTypes: ["Video"],
    required: false,
    verificationMethod: "Manual",
    retentionPeriod: "365 days",
    version: 1,
    status: "Archived",
    publishedAt: "2026-04-10T11:00:00Z",
    publishedBy: "F. Daniel",
    createdAt: "2026-04-01T09:00:00Z",
    updatedAt: "2026-07-15T16:00:00Z",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: "Archived - replaced by updated incident reporting policy.",
    tags: ["Video", "Incident", "Archived"],
  },
  {
    id: "PP-007",
    reference: "PP-2026-007",
    name: "Audio Proof - Customer Verification",
    description: "Requires audio recording for customer verification on high-risk deliveries.",
    serviceType: ["Dispatch"],
    customerTypes: ["HighRisk"],
    proofTypes: ["Audio"],
    required: false,
    verificationMethod: "Manual",
    retentionPeriod: "90 days",
    version: 1,
    status: "Draft",
    publishedAt: null,
    publishedBy: null,
    createdAt: "2026-08-20T09:00:00Z",
    updatedAt: "2026-08-22T11:00:00Z",
    createdBy: "A. Yusuf",
    branch: "Ikeja",
    notes: "Pilot program for high-risk deliveries. Limited to Ikeja branch.",
    tags: ["Audio", "Verification", "Pilot"],
  },
  {
    id: "PP-008",
    reference: "PP-2026-008",
    name: "Multi-Proof - Complex Deliveries",
    description: "Requires multiple proof types for complex multi-stop deliveries.",
    serviceType: ["Dispatch", "Hauling"],
    customerTypes: ["Corporate", "Enterprise"],
    proofTypes: ["Signature", "Photo", "Location", "Document"],
    required: true,
    verificationMethod: "Hybrid",
    retentionPeriod: "180 days",
    version: 1,
    status: "Superseded",
    publishedAt: "2026-05-01T09:00:00Z",
    publishedBy: "F. Daniel",
    createdAt: "2026-04-15T14:00:00Z",
    updatedAt: "2026-07-15T16:00:00Z",
    createdBy: "F. Daniel",
    branch: "All branches",
    notes: "Superseded by PP-001 and PP-002 for specific service types.",
    tags: ["MultiProof", "Complex", "Superseded"],
  },
];

const PROOF_TYPE_ICONS: Record<string, React.ReactNode> = {
  OTP: <FileCheck className="h-3.5 w-3.5" />,
  Signature: <FileSignature className="h-3.5 w-3.5" />,
  Photo: <FileText className="h-3.5 w-3.5" />,
  Scan: <FileText className="h-3.5 w-3.5" />,
  Location: <MapPin className="h-3.5 w-3.5" />,
  Video: <FileText className="h-3.5 w-3.5" />,
  Audio: <FileText className="h-3.5 w-3.5" />,
  Document: <FolderOpen className="h-3.5 w-3.5" />,
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Draft: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Published: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Archived: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Superseded: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <RefreshCw className="h-3.5 w-3.5" />,
  },
};

const VERIFICATION_STYLES: Record<string, { bg: string; text: string }> = {
  Manual: { bg: "bg-amber-bg", text: "text-amber" },
  Automated: { bg: "bg-green-bg", text: "text-green" },
  Hybrid: { bg: "bg-purple-bg", text: "text-purple" },
};

const STATUS_FILTERS = ["All", "Draft", "Published", "Archived", "Superseded"];
const SERVICE_FILTERS = ["All", "Dispatch", "Hauling", "Courier"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function ProofPoliciesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ProofPolicy[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedService, setSelectedService] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_POLICIES);
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
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      (item.publishedBy && item.publishedBy.toLowerCase().includes(search.toLowerCase()));

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesService = selectedService === "All" || item.serviceType.includes(selectedService);
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesService && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading proof policies...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedService !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedService("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedService !== "All" ? 1 : 0) +
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
  const totalPublished = data.filter((d) => d.status === "Published").length;
  const totalDraft = data.filter((d) => d.status === "Draft").length;
  const totalRequired = data.filter((d) => d.required).length;
  const totalAutomated = data.filter((d) => d.verificationMethod === "Automated" || d.verificationMethod === "Hybrid").length;

  const goToPolicyDetail = (id: string) => {
    router.push(`/compliance/proof-policies/${id}`);
  };

  const goToCreatePolicy = () => {
    router.push("/compliance/proof-policies/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Proof Policies
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Service and customer proof requirements including OTP, signature, photo, scan, location, and more.
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
            onClick={goToCreatePolicy}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Policy
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Published</div>
          <div className="text-lg font-bold text-green">{totalPublished}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Draft</div>
          <div className="text-lg font-bold text-amber">{totalDraft}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Required</div>
          <div className="text-lg font-bold text-red">{totalRequired}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, reference, description..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "Draft", "Published", "Archived", "Superseded"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <FileCheck className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const services = ["All", "Dispatch", "Hauling", "Courier"];
            const currentIndex = services.indexOf(selectedService);
            const nextIndex = (currentIndex + 1) % services.length;
            setSelectedService(services[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Briefcase className="h-4 w-4 text-ink-400" />
          {selectedService}
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

      {/* ===== POLICIES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileSignature className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No proof policies found</h3>
          <p className="mt-1 text-sm text-ink-500">No policies match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Policy</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Proof Types</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Verification</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Version</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Draft;
                  const verificationStyle = VERIFICATION_STYLES[item.verificationMethod] || VERIFICATION_STYLES.Manual;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToPolicyDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.name}</div>
                        <div className="flex items-center gap-2 mt-0.5">
                          <span className="font-mono text-[10px] text-ink-500">{item.reference}</span>
                          {item.required && (
                            <span className="text-[9px] text-red font-medium">Required</span>
                          )}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-wrap gap-1">
                          {item.proofTypes.map((type) => (
                            <span
                              key={type}
                              className="flex items-center gap-0.5 rounded-full bg-blue-bg px-2 py-0.5 text-[9px] font-medium text-blue"
                            >
                              {PROOF_TYPE_ICONS[type] || <FileText className="h-3 w-3" />}
                              {type}
                            </span>
                          ))}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${verificationStyle.bg} ${verificationStyle.text}`}>
                          {item.verificationMethod}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs font-mono">v{item.version}</td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
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
          CMP-11 · Proof Policies
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} policies shown
        </span>
      </div>
    </div>
  );
}