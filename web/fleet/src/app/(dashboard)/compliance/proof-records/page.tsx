// src/app/(dashboard)/compliance/proof-records/page.tsx

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
  FileText,
  FileCheck,
  MapPin,
  Briefcase,
} from "lucide-react";

// ==================== TYPES ====================

type ProofRecord = {
  id: string;
  reference: string;
  proofType: "OTP" | "Signature" | "Photo" | "Scan" | "Location" | "Video" | "Audio" | "Document";
  workReference: string;
  workType: "Trip" | "Order" | "Shipment" | "Delivery" | "Hauling";
  timestamp: string;
  capturedBy: string;
  capturedAt: string;
  location: string;
  branch: string;
  verificationStatus: "Pending" | "Verified" | "Rejected" | "Partial";
  verifiedBy: string | null;
  verifiedAt: string | null;
  rejectionReason: string | null;
  mediaReference: string | null;
  mediaType: string | null;
  customerName: string | null;
  customerSignature: string | null;
  otpValue: string | null;
  otpVerified: boolean;
  notes: string | null;
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_PROOF_RECORDS: ProofRecord[] = [
  {
    id: "PF-001",
    reference: "PF-2026-001",
    proofType: "OTP",
    workReference: "KG-8491",
    workType: "Trip",
    timestamp: "2026-08-22T14:30:00Z",
    capturedBy: "P. Okafor",
    capturedAt: "2026-08-22T14:30:00Z",
    location: "Lekki Phase 1 - Gate A",
    branch: "Ikeja",
    verificationStatus: "Verified",
    verifiedBy: "System",
    verifiedAt: "2026-08-22T14:30:00Z",
    rejectionReason: null,
    mediaReference: null,
    mediaType: null,
    customerName: "Adeyemi Stores",
    customerSignature: null,
    otpValue: "482719",
    otpVerified: true,
    notes: "OTP verified by recipient. Delivery confirmed.",
    tags: ["OTP", "Delivery", "Verified"],
    sourceFreshness: "2026-08-22T14:30:00Z",
  },
  {
    id: "PF-002",
    reference: "PF-2026-002",
    proofType: "Signature",
    workReference: "KG-8487",
    workType: "Trip",
    timestamp: "2026-08-22T13:15:00Z",
    capturedBy: "M. Bello",
    capturedAt: "2026-08-22T13:15:00Z",
    location: "Marina - Office 3B",
    branch: "Lekki",
    verificationStatus: "Verified",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-22T14:00:00Z",
    rejectionReason: null,
    mediaReference: "DOC-7721",
    mediaType: "Signature",
    customerName: "Greenline Foods",
    customerSignature: "O. Adeyemi",
    otpValue: null,
    otpVerified: false,
    notes: "Signature verified against customer records.",
    tags: ["Signature", "HighValue", "Verified"],
    sourceFreshness: "2026-08-22T14:00:00Z",
  },
  {
    id: "PF-003",
    reference: "PF-2026-003",
    proofType: "Photo",
    workReference: "KG-8507",
    workType: "Trip",
    timestamp: "2026-08-22T12:45:00Z",
    capturedBy: "A. Yusuf",
    capturedAt: "2026-08-22T12:45:00Z",
    location: "Yaba - Warehouse Zone C",
    branch: "Yaba",
    verificationStatus: "Verified",
    verifiedBy: "System",
    verifiedAt: "2026-08-22T13:00:00Z",
    rejectionReason: null,
    mediaReference: "IMG-2026-0844",
    mediaType: "Image",
    customerName: "Yaba Medical Supplies",
    customerSignature: null,
    otpValue: null,
    otpVerified: false,
    notes: "Package condition confirmed. No damage visible.",
    tags: ["Photo", "Condition", "Verified"],
    sourceFreshness: "2026-08-22T13:00:00Z",
  },
  {
    id: "PF-004",
    reference: "PF-2026-004",
    proofType: "Location",
    workReference: "KG-8512",
    workType: "Hauling",
    timestamp: "2026-08-22T11:30:00Z",
    capturedBy: "K. Ahmed",
    capturedAt: "2026-08-22T11:30:00Z",
    location: "Apapa Depot - Gate 2",
    branch: "Apapa",
    verificationStatus: "Verified",
    verifiedBy: "System",
    verifiedAt: "2026-08-22T11:30:00Z",
    rejectionReason: null,
    mediaReference: null,
    mediaType: null,
    customerName: "Apex Logistics",
    customerSignature: null,
    otpValue: null,
    otpVerified: false,
    notes: "GPS location verified. Within service area.",
    tags: ["Location", "GPS", "Verified"],
    sourceFreshness: "2026-08-22T11:30:00Z",
  },
  {
    id: "PF-005",
    reference: "PF-2026-005",
    proofType: "Scan",
    workReference: "KG-8512",
    workType: "Hauling",
    timestamp: "2026-08-22T10:45:00Z",
    capturedBy: "K. Ahmed",
    capturedAt: "2026-08-22T10:45:00Z",
    location: "Apapa Depot - Documentation",
    branch: "Apapa",
    verificationStatus: "Pending",
    verifiedBy: null,
    verifiedAt: null,
    rejectionReason: null,
    mediaReference: "SCAN-2026-0122",
    mediaType: "PDF",
    customerName: "Apex Logistics",
    customerSignature: null,
    otpValue: null,
    otpVerified: false,
    notes: "Delivery notes and customs documentation pending review.",
    tags: ["Scan", "Documentation", "Pending"],
    sourceFreshness: "2026-08-22T10:45:00Z",
  },
  {
    id: "PF-006",
    reference: "PF-2026-006",
    proofType: "Video",
    workReference: "INC-771",
    workType: "Trip",
    timestamp: "2026-08-22T06:50:00Z",
    capturedBy: "P. Okafor",
    capturedAt: "2026-08-22T06:50:00Z",
    location: "Opebi Road, Ikeja",
    branch: "Ikeja",
    verificationStatus: "Pending",
    verifiedBy: null,
    verifiedAt: null,
    rejectionReason: null,
    mediaReference: "VID-2026-0123",
    mediaType: "MP4",
    customerName: null,
    customerSignature: null,
    otpValue: null,
    otpVerified: false,
    notes: "Dashcam footage from incident. Awaiting review.",
    tags: ["Video", "Incident", "Pending"],
    sourceFreshness: "2026-08-22T06:50:00Z",
  },
  {
    id: "PF-007",
    reference: "PF-2026-007",
    proofType: "Signature",
    workReference: "KG-8479",
    workType: "Trip",
    timestamp: "2026-08-21T16:20:00Z",
    capturedBy: "P. Okafor",
    capturedAt: "2026-08-21T16:20:00Z",
    location: "Lekki Phase 1 - Gate B",
    branch: "Ikeja",
    verificationStatus: "Rejected",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-21T17:00:00Z",
    rejectionReason: "Signature does not match customer records. Recipient verification failed.",
    mediaReference: "DOC-8842",
    mediaType: "Signature",
    customerName: "Adeyemi Stores",
    customerSignature: "A. Adeyemi",
    otpValue: null,
    otpVerified: false,
    notes: "Customer disputed delivery. Signature mismatch.",
    tags: ["Signature", "Disputed", "Rejected"],
    sourceFreshness: "2026-08-21T17:00:00Z",
  },
  {
    id: "PF-008",
    reference: "PF-2026-008",
    proofType: "Photo",
    workReference: "KG-8504",
    workType: "Trip",
    timestamp: "2026-08-21T14:00:00Z",
    capturedBy: "M. Bello",
    capturedAt: "2026-08-21T14:00:00Z",
    location: "Lekki - Marina",
    branch: "Lekki",
    verificationStatus: "Partial",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-21T15:00:00Z",
    rejectionReason: null,
    mediaReference: "IMG-2026-0845",
    mediaType: "Image",
    customerName: "Oakfield Stores",
    customerSignature: null,
    otpValue: null,
    otpVerified: false,
    notes: "Photo quality marginal. Package visible but recipient not visible.",
    tags: ["Photo", "Partial", "Review"],
    sourceFreshness: "2026-08-21T15:00:00Z",
  },
  {
    id: "PF-009",
    reference: "PF-2026-009",
    proofType: "OTP",
    workReference: "KG-8504",
    workType: "Trip",
    timestamp: "2026-08-21T14:05:00Z",
    capturedBy: "M. Bello",
    capturedAt: "2026-08-21T14:05:00Z",
    location: "Lekki - Marina",
    branch: "Lekki",
    verificationStatus: "Verified",
    verifiedBy: "System",
    verifiedAt: "2026-08-21T14:05:00Z",
    rejectionReason: null,
    mediaReference: null,
    mediaType: null,
    customerName: "Oakfield Stores",
    customerSignature: null,
    otpValue: "391722",
    otpVerified: true,
    notes: "OTP verified. Delivery confirmed.",
    tags: ["OTP", "Delivery", "Verified"],
    sourceFreshness: "2026-08-21T14:05:00Z",
  },
  {
    id: "PF-010",
    reference: "PF-2026-010",
    proofType: "Audio",
    workReference: "WH-2026-884",
    workType: "Order",
    timestamp: "2026-08-21T11:30:00Z",
    capturedBy: "J. Adekunle",
    capturedAt: "2026-08-21T11:30:00Z",
    location: "Yaba Warehouse - Office",
    branch: "Yaba",
    verificationStatus: "Verified",
    verifiedBy: "System",
    verifiedAt: "2026-08-21T12:00:00Z",
    rejectionReason: null,
    mediaReference: "AUD-2026-0022",
    mediaType: "Audio",
    customerName: "Yaba Medical Supplies",
    customerSignature: null,
    otpValue: null,
    otpVerified: false,
    notes: "Customer verification confirmed via audio.",
    tags: ["Audio", "Verification", "Verified"],
    sourceFreshness: "2026-08-21T12:00:00Z",
  },
];

const PROOF_TYPE_ICONS: Record<string, React.ReactNode> = {
  OTP: <FileCheck className="h-3.5 w-3.5" />,
  Signature: <FileText className="h-3.5 w-3.5" />,
  Photo: <FileText className="h-3.5 w-3.5" />,
  Scan: <FileText className="h-3.5 w-3.5" />,
  Location: <MapPin className="h-3.5 w-3.5" />,
  Video: <FileText className="h-3.5 w-3.5" />,
  Audio: <FileText className="h-3.5 w-3.5" />,
  Document: <FileText className="h-3.5 w-3.5" />,
};

const PROOF_TYPE_COLORS: Record<string, { bg: string; text: string }> = {
  OTP: { bg: "bg-blue-bg", text: "text-blue" },
  Signature: { bg: "bg-purple-bg", text: "text-purple" },
  Photo: { bg: "bg-green-bg", text: "text-green" },
  Scan: { bg: "bg-amber-bg", text: "text-amber" },
  Location: { bg: "bg-surface-2", text: "text-ink-500" },
  Video: { bg: "bg-red-bg", text: "text-red" },
  Audio: { bg: "bg-blue-bg", text: "text-blue" },
  Document: { bg: "bg-surface-2", text: "text-ink-500" },
};

const VERIFICATION_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Pending: {
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
  Partial: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
  },
};

const WORK_TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Trip: { bg: "bg-blue-bg", text: "text-blue" },
  Order: { bg: "bg-green-bg", text: "text-green" },
  Shipment: { bg: "bg-amber-bg", text: "text-amber" },
  Delivery: { bg: "bg-purple-bg", text: "text-purple" },
  Hauling: { bg: "bg-red-bg", text: "text-red" },
};

const PROOF_FILTERS = ["All", "OTP", "Signature", "Photo", "Scan", "Location", "Video", "Audio", "Document"];
const VERIFICATION_FILTERS = ["All", "Pending", "Verified", "Rejected", "Partial"];
const WORK_TYPE_FILTERS = ["All", "Trip", "Order", "Shipment", "Delivery", "Hauling"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function ProofRecordsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ProofRecord[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedProofType, setSelectedProofType] = useState("All");
  const [selectedVerification, setSelectedVerification] = useState("All");
  const [selectedWorkType, setSelectedWorkType] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_PROOF_RECORDS);
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
      item.workReference.toLowerCase().includes(search.toLowerCase()) ||
      (item.customerName && item.customerName.toLowerCase().includes(search.toLowerCase())) ||
      (item.capturedBy && item.capturedBy.toLowerCase().includes(search.toLowerCase()));

    const matchesProofType = selectedProofType === "All" || item.proofType === selectedProofType;
    const matchesVerification = selectedVerification === "All" || item.verificationStatus === selectedVerification;
    const matchesWorkType = selectedWorkType === "All" || item.workType === selectedWorkType;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesProofType && matchesVerification && matchesWorkType && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading proof records...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedProofType !== "All" ||
    selectedVerification !== "All" ||
    selectedWorkType !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedProofType("All");
    setSelectedVerification("All");
    setSelectedWorkType("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedProofType !== "All" ? 1 : 0) +
    (selectedVerification !== "All" ? 1 : 0) +
    (selectedWorkType !== "All" ? 1 : 0) +
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
  const totalVerified = data.filter((d) => d.verificationStatus === "Verified").length;
  const totalPending = data.filter((d) => d.verificationStatus === "Pending").length;
  const totalRejected = data.filter((d) => d.verificationStatus === "Rejected").length;
  const totalOTP = data.filter((d) => d.proofType === "OTP").length;

  const goToRecordDetail = (id: string) => {
    router.push(`/compliance/proof-records/${id}`);
  };

  const goToCreateRecord = () => {
    router.push("/compliance/proof-records/create");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Proof Records
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Work reference, proof type, source time, actor, verification summary and media reference.
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
            onClick={goToCreateRecord}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Record Proof
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{totalPending}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">OTP Verified</div>
          <div className="text-lg font-bold text-blue">{totalOTP}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, work, customer..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "OTP", "Signature", "Photo", "Scan", "Location", "Video", "Audio", "Document"];
            const currentIndex = types.indexOf(selectedProofType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedProofType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <FileCheck className="h-4 w-4 text-ink-400" />
          {selectedProofType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Pending", "Verified", "Rejected", "Partial"];
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
          onClick={() => {
            const workTypes = ["All", "Trip", "Order", "Shipment", "Delivery", "Hauling"];
            const currentIndex = workTypes.indexOf(selectedWorkType);
            const nextIndex = (currentIndex + 1) % workTypes.length;
            setSelectedWorkType(workTypes[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Briefcase className="h-4 w-4 text-ink-400" />
          {selectedWorkType}
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

      {/* ===== RECORDS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No proof records found</h3>
          <p className="mt-1 text-sm text-ink-500">No records match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Proof Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Work</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Verification</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Captured By</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Time</th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const proofIcon = PROOF_TYPE_ICONS[item.proofType] || <FileText className="h-3.5 w-3.5" />;
                  const proofColor = PROOF_TYPE_COLORS[item.proofType] || PROOF_TYPE_COLORS.Document;
                  const verificationStyle = VERIFICATION_STYLES[item.verificationStatus] || VERIFICATION_STYLES.Pending;
                  const workTypeStyle = WORK_TYPE_STYLES[item.workType] || WORK_TYPE_STYLES.Trip;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToRecordDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                    >
                      <td className="px-4 py-3">
                        <div className="font-mono text-[11px] font-medium text-ink-950">{item.reference}</div>
                        {item.customerName && (
                          <div className="text-[10px] text-ink-500 truncate max-w-[120px]">{item.customerName}</div>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${proofColor.bg} ${proofColor.text}`}>
                          {proofIcon}
                          {item.proofType}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex flex-col">
                          <span className="font-mono text-[11px] text-ink-950">{item.workReference}</span>
                          <span className={`text-[9px] font-medium ${workTypeStyle.bg} ${workTypeStyle.text} rounded-full px-2 py-0.5 w-fit`}>
                            {item.workType}
                          </span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${verificationStyle.bg} ${verificationStyle.text}`}>
                          {verificationStyle.icon}
                          {item.verificationStatus}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">
                        {item.capturedBy}
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.timestamp)}</td>
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
          CMP-12 · Proof Records
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} records shown
        </span>
      </div>
    </div>
  );
}