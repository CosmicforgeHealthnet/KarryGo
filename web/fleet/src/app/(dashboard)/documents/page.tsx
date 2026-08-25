// src/app/(dashboard)/documents/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  FileText,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
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
  Minus,
  Send,
  Paperclip,
  Link2,
  ExternalLink,
  File,
  FolderOpen,
  Shield,
  Lock,
  EyeOff,
  AlertOctagon,
  FileCheck,
  FileSignature,
  Folder,
  HardDrive,
  Database,
  Cloud,
} from "lucide-react";

type Document = {
  id: string;
  reference: string;
  name: string;
  type: "PDF" | "Image" | "Spreadsheet" | "Document" | "Archive" | "Other";
  category: "Business" | "Compliance" | "Provider" | "Fleet" | "Financial" | "Operational" | "Legal" | "Other";
  ownerType: "Provider" | "Asset" | "Customer" | "Staff" | "Company" | "Trip" | "Other";
  ownerId: string;
  ownerName: string;
  status: "Uploaded" | "Scanning" | "Verified" | "Rejected" | "Expired" | "Archived";
  visibility: "Public" | "Internal" | "Restricted" | "Confidential";
  verificationStatus: "Pending" | "Verified" | "Rejected" | "NotRequired";
  verifiedBy: string | null;
  verifiedAt: string | null;
  retentionPeriod: string;
  expiryDate: string | null;
  size: string;
  version: number;
  uploadedBy: string;
  uploadedAt: string;
  updatedAt: string;
  tags: string[];
  sourceFreshness: string;
};

const MOCK_DOCUMENTS: Document[] = [
  {
    id: "DOC-001",
    reference: "DOC-2026-001",
    name: "Vehicle Registration - BR-204",
    type: "PDF",
    category: "Fleet",
    ownerType: "Asset",
    ownerId: "BR-204",
    ownerName: "Honda ACE 110",
    status: "Verified",
    visibility: "Internal",
    verificationStatus: "Verified",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-15T10:00:00Z",
    retentionPeriod: "7 years",
    expiryDate: "2026-12-31",
    size: "2.4 MB",
    version: 1,
    uploadedBy: "M. Bello",
    uploadedAt: "2026-08-10T14:30:00Z",
    updatedAt: "2026-08-15T10:00:00Z",
    tags: ["Registration", "Vehicle", "Fleet"],
    sourceFreshness: "2026-08-15T10:00:00Z",
  },
  {
    id: "DOC-002",
    reference: "DOC-2026-002",
    name: "Provider ID - P-044",
    type: "PDF",
    category: "Provider",
    ownerType: "Provider",
    ownerId: "P-044",
    ownerName: "Peter Okafor",
    status: "Expired",
    visibility: "Internal",
    verificationStatus: "Rejected",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-08-20T09:00:00Z",
    retentionPeriod: "3 years",
    expiryDate: "2026-08-20",
    size: "1.8 MB",
    version: 1,
    uploadedBy: "P. Okafor",
    uploadedAt: "2026-08-01T11:00:00Z",
    updatedAt: "2026-08-20T09:00:00Z",
    tags: ["Provider", "ID", "Expired"],
    sourceFreshness: "2026-08-20T09:00:00Z",
  },
  {
    id: "DOC-003",
    reference: "DOC-2026-003",
    name: "Insurance Certificate - VN-041",
    type: "PDF",
    category: "Fleet",
    ownerType: "Asset",
    ownerId: "VN-041",
    ownerName: "Toyota Hiace",
    status: "Verified",
    visibility: "Internal",
    verificationStatus: "Verified",
    verifiedBy: "System",
    verifiedAt: "2026-08-01T14:00:00Z",
    retentionPeriod: "5 years",
    expiryDate: "2026-09-15",
    size: "3.2 MB",
    version: 1,
    uploadedBy: "M. Bello",
    uploadedAt: "2026-07-25T09:00:00Z",
    updatedAt: "2026-08-01T14:00:00Z",
    tags: ["Insurance", "Vehicle", "Fleet"],
    sourceFreshness: "2026-08-01T14:00:00Z",
  },
  {
    id: "DOC-004",
    reference: "DOC-2026-004",
    name: "Service Contract - Charitos Hospital",
    type: "PDF",
    category: "Legal",
    ownerType: "Customer",
    ownerId: "C-003",
    ownerName: "Charitos Hospital",
    status: "Verified",
    visibility: "Restricted",
    verificationStatus: "Verified",
    verifiedBy: "F. Daniel",
    verifiedAt: "2026-07-15T11:00:00Z",
    retentionPeriod: "10 years",
    expiryDate: "2026-12-31",
    size: "5.6 MB",
    version: 2,
    uploadedBy: "O. Adebayo",
    uploadedAt: "2026-07-10T16:00:00Z",
    updatedAt: "2026-07-15T11:00:00Z",
    tags: ["Contract", "Customer", "Legal"],
    sourceFreshness: "2026-07-15T11:00:00Z",
  },
  {
    id: "DOC-005",
    reference: "DOC-2026-005",
    name: "Driver License - P-044",
    type: "Image",
    category: "Provider",
    ownerType: "Provider",
    ownerId: "P-044",
    ownerName: "Peter Okafor",
    status: "Uploaded",
    visibility: "Internal",
    verificationStatus: "Pending",
    verifiedBy: null,
    verifiedAt: null,
    retentionPeriod: "3 years",
    expiryDate: null,
    size: "1.2 MB",
    version: 1,
    uploadedBy: "P. Okafor",
    uploadedAt: "2026-08-22T08:00:00Z",
    updatedAt: "2026-08-22T08:00:00Z",
    tags: ["Provider", "License", "Pending"],
    sourceFreshness: "2026-08-22T08:00:00Z",
  },
  {
    id: "DOC-006",
    reference: "DOC-2026-006",
    name: "Maintenance Record - TR-112",
    type: "Document",
    category: "Fleet",
    ownerType: "Asset",
    ownerId: "TR-112",
    ownerName: "Mercedes Actros",
    status: "Verified",
    visibility: "Internal",
    verificationStatus: "Verified",
    verifiedBy: "M. Bello",
    verifiedAt: "2026-08-21T10:00:00Z",
    retentionPeriod: "5 years",
    expiryDate: null,
    size: "4.8 MB",
    version: 1,
    uploadedBy: "M. Bello",
    uploadedAt: "2026-08-20T15:00:00Z",
    updatedAt: "2026-08-21T10:00:00Z",
    tags: ["Maintenance", "Vehicle", "Fleet"],
    sourceFreshness: "2026-08-21T10:00:00Z",
  },
  {
    id: "DOC-007",
    reference: "DOC-2026-007",
    name: "Financial Report Q3 2026",
    type: "Spreadsheet",
    category: "Financial",
    ownerType: "Company",
    ownerId: "ORG-001",
    ownerName: "Apex Logistics",
    status: "Scanning",
    visibility: "Restricted",
    verificationStatus: "Pending",
    verifiedBy: null,
    verifiedAt: null,
    retentionPeriod: "7 years",
    expiryDate: null,
    size: "8.2 MB",
    version: 1,
    uploadedBy: "F. Daniel",
    uploadedAt: "2026-08-22T13:00:00Z",
    updatedAt: "2026-08-22T13:00:00Z",
    tags: ["Financial", "Report", "Scanning"],
    sourceFreshness: "2026-08-22T13:00:00Z",
  },
  {
    id: "DOC-008",
    reference: "DOC-2026-008",
    name: "Incident Report - INC-771",
    type: "Document",
    category: "Operational",
    ownerType: "Trip",
    ownerId: "KG-8479",
    ownerName: "KG-8479",
    status: "Archived",
    visibility: "Internal",
    verificationStatus: "NotRequired",
    verifiedBy: null,
    verifiedAt: null,
    retentionPeriod: "3 years",
    expiryDate: null,
    size: "2.1 MB",
    version: 1,
    uploadedBy: "F. Daniel",
    uploadedAt: "2026-08-22T07:00:00Z",
    updatedAt: "2026-08-22T14:00:00Z",
    tags: ["Incident", "Report", "Archived"],
    sourceFreshness: "2026-08-22T14:00:00Z",
  },
  {
    id: "DOC-009",
    reference: "DOC-2026-009",
    name: "W9 Form - Provider P-071",
    type: "PDF",
    category: "Provider",
    ownerType: "Provider",
    ownerId: "P-071",
    ownerName: "Kabiru Ahmed",
    status: "Uploaded",
    visibility: "Confidential",
    verificationStatus: "Pending",
    verifiedBy: null,
    verifiedAt: null,
    retentionPeriod: "3 years",
    expiryDate: null,
    size: "0.9 MB",
    version: 1,
    uploadedBy: "K. Ahmed",
    uploadedAt: "2026-08-22T15:00:00Z",
    updatedAt: "2026-08-22T15:00:00Z",
    tags: ["Provider", "Tax", "Confidential"],
    sourceFreshness: "2026-08-22T15:00:00Z",
  },
];

const TYPE_ICONS: Record<string, React.ReactNode> = {
  PDF: <File className="h-3.5 w-3.5" />,
  Image: <File className="h-3.5 w-3.5" />,
  Spreadsheet: <File className="h-3.5 w-3.5" />,
  Document: <FileText className="h-3.5 w-3.5" />,
  Archive: <File className="h-3.5 w-3.5" />,
  Other: <File className="h-3.5 w-3.5" />,
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Uploaded: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: <Cloud className="h-3.5 w-3.5" /> },
  Scanning: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <Clock className="h-3.5 w-3.5" /> },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <Folder className="h-3.5 w-3.5" /> },
};

const VERIFICATION_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  NotRequired: { bg: "bg-surface-2", text: "text-ink-500", icon: <Minus className="h-3.5 w-3.5" /> },
};

const VISIBILITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Public: { bg: "bg-green-bg", text: "text-green", icon: <Users className="h-3.5 w-3.5" /> },
  Internal: { bg: "bg-primary-bg", text: "text-primary", icon: <Building2 className="h-3.5 w-3.5" /> },
  Restricted: { bg: "bg-amber-bg", text: "text-amber", icon: <Lock className="h-3.5 w-3.5" /> },
  Confidential: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-3.5 w-3.5" /> },
};

const CATEGORY_FILTERS = ["All", "Business", "Compliance", "Provider", "Fleet", "Financial", "Operational", "Legal", "Other"];
const STATUS_FILTERS = ["All", "Uploaded", "Scanning", "Verified", "Rejected", "Expired", "Archived"];
const OWNER_FILTERS = ["All", "Provider", "Asset", "Customer", "Staff", "Company", "Trip", "Other"];

export default function DocumentLibraryPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Document[]>([]);
  const [search, setSearch] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedOwner, setSelectedOwner] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_DOCUMENTS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.ownerName.toLowerCase().includes(search.toLowerCase()) ||
      item.tags.some(t => t.toLowerCase().includes(search.toLowerCase()));

    const matchesCategory = selectedCategory === "All" || item.category === selectedCategory;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesOwner = selectedOwner === "All" || item.ownerType === selectedOwner;

    return matchesSearch && matchesCategory && matchesStatus && matchesOwner;
  });

  const totalVerified = data.filter((d) => d.status === "Verified").length;
  const totalPending = data.filter((d) => d.status === "Uploaded" || d.status === "Scanning").length;
  const totalExpired = data.filter((d) => d.status === "Expired").length;
  const totalRejected = data.filter((d) => d.status === "Rejected").length;

  const hasActiveFilters =
    selectedCategory !== "All" ||
    selectedStatus !== "All" ||
    selectedOwner !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedCategory("All");
    setSelectedStatus("All");
    setSelectedOwner("All");
  };

  const filterCount =
    (selectedCategory !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedOwner !== "All" ? 1 : 0);

  const formatDate = (date: string | null) => {
    if (!date) return "—";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading documents...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Document Library</h1>
          <p className="mt-1.5 text-sm text-ink-600">
            Document type, owner/subject, status, visibility, verification, retention, expiry and latest version.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
          <button
            onClick={() => router.push("/documents/upload")}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Upload Document
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <FileText className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Verified</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalVerified}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Pending</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalPending}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Expired</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalExpired}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search documents..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <Folder className="h-3.5 w-3.5 text-ink-400" />
          {selectedCategory}
          <span className="text-ink-300">·</span>
          <FileCheck className="h-3.5 w-3.5 text-ink-400" />
          {selectedStatus}
          <span className="text-ink-300">·</span>
          <User className="h-3.5 w-3.5 text-ink-400" />
          {selectedOwner}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
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
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filteredData.length} documents
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Category</span>
              <div className="flex flex-wrap gap-1.5">
                {CATEGORY_FILTERS.map((cat) => (
                  <button
                    key={cat}
                    onClick={() => setSelectedCategory(cat)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedCategory === cat
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {cat}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((st) => (
                  <button
                    key={st}
                    onClick={() => setSelectedStatus(st)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === st
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {st}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Owner Type</span>
              <div className="flex flex-wrap gap-1.5">
                {OWNER_FILTERS.map((owner) => (
                  <button
                    key={owner}
                    onClick={() => setSelectedOwner(owner)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedOwner === owner
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {owner}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Documents Table ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <FileText className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No documents found</h3>
          <p className="mt-1 text-sm text-ink-500">No documents match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-line bg-white">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[900px]">
              <thead>
                <tr className="bg-surface-1">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Document</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Verification</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Owner</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Expiry</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Version</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const typeIcon = TYPE_ICONS[item.type] || TYPE_ICONS.Other;
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Uploaded;
                  const verificationStyle = VERIFICATION_STYLES[item.verificationStatus] || VERIFICATION_STYLES.Pending;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => router.push(`/documents/${item.id}`)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3 whitespace-nowrap">
                        <div className="flex items-center gap-2">
                          <span className="text-ink-400">{typeIcon}</span>
                          <div>
                            <div className="font-medium text-ink-950 text-xs">{item.name}</div>
                            <div className="font-mono text-[10px] text-ink-500">{item.reference}</div>
                          </div>
                        </div>
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <span className="rounded-full px-2.5 py-0.5 text-[10px] font-semibold bg-surface-2 text-ink-600">
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${verificationStyle.bg} ${verificationStyle.text}`}>
                          {verificationStyle.icon}
                          {item.verificationStatus}
                        </span>
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <div className="text-xs text-ink-950">{item.ownerName}</div>
                        <div className="text-[9px] text-ink-400">{item.ownerType}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs whitespace-nowrap">
                        {item.expiryDate ? formatDate(item.expiryDate) : "—"}
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs font-mono whitespace-nowrap">
                        v{item.version}
                      </td>
                      <td className="px-4 py-3 text-right whitespace-nowrap">
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
    </div>
  );
}