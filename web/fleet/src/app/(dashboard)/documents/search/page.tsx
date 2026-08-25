// src/app/(dashboard)/documents/search/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  SlidersHorizontal,
  X,
  ChevronDown,
  Users,
  Building2,
  Tag,
  Info,
  Download,
  Printer,
  Edit3,
  MoreHorizontal,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Paperclip,
  Link2,
  ExternalLink,
  FileText,
  File,
  Shield,
  Lock,
  EyeOff,
  AlertOctagon,
  FileCheck,
  Folder,
  HardDrive,
  Database,
  Cloud,
  History,
  GitBranch,
  Package,
  HelpCircle,
  Sparkles,
} from "lucide-react";

// ==================== TYPES ====================

type SearchResult = {
  id: string;
  type: "Customer" | "Provider" | "Staff" | "Asset" | "Trip" | "Order" | "Shipment" | "Package" | "Manifest" | "Route" | "Incident" | "Compliance" | "Document" | "Invoice" | "Warehouse" | "Fulfilment" | "Support";
  title: string;
  subtitle: string;
  reference: string;
  description: string;
  branch: string;
  status: string;
  updatedAt: string;
  relevance: number;
  tags: string[];
  url: string;
};

// ==================== MOCK DATA ====================

const MOCK_SEARCH_RESULTS: SearchResult[] = [
  {
    id: "SR-001",
    type: "Trip",
    title: "KG-8491 · Ikeja to Lekki",
    subtitle: "Dispatch trip · Express delivery",
    reference: "KG-8491",
    description: "Priority delivery from Ikeja depot to Lekki Phase 1. Currently in transit.",
    branch: "Ikeja",
    status: "In Transit",
    updatedAt: "2026-08-22T14:30:00Z",
    relevance: 95,
    tags: ["Express", "Delivery", "Priority"],
    url: "/operations/trips/KG-8491",
  },
  {
    id: "SR-002",
    type: "Provider",
    title: "Peter Okafor · P-044",
    subtitle: "Dispatch Rider · Ikeja branch",
    reference: "P-044",
    description: "Provider with 3 years experience. Currently assigned to BR-204.",
    branch: "Ikeja",
    status: "Online",
    updatedAt: "2026-08-22T14:15:00Z",
    relevance: 92,
    tags: ["Dispatch", "Active", "BR-204"],
    url: "/providers/P-044",
  },
  {
    id: "SR-003",
    type: "Asset",
    title: "Honda ACE 110 · BR-204",
    subtitle: "Motorcycle · Fleet asset",
    reference: "BR-204",
    description: "Motorcycle assigned to Peter Okafor. Safety block active.",
    branch: "Ikeja",
    status: "Safety Blocked",
    updatedAt: "2026-08-22T14:00:00Z",
    relevance: 88,
    tags: ["Motorcycle", "Safety", "BR-204"],
    url: "/fleet/assets/BR-204",
  },
  {
    id: "SR-004",
    type: "Incident",
    title: "INC-771 · Road incident",
    subtitle: "Vehicle collision · Opebi Road",
    reference: "INC-771",
    description: "Provider P-044 involved in collision. Investigation in progress.",
    branch: "Ikeja",
    status: "Investigating",
    updatedAt: "2026-08-22T09:00:00Z",
    relevance: 85,
    tags: ["Incident", "Collision", "Vehicle"],
    url: "/compliance/incidents/INC-771",
  },
  {
    id: "SR-005",
    type: "Document",
    title: "Vehicle Registration · DOC-2026-001",
    subtitle: "Fleet document · BR-204",
    reference: "DOC-2026-001",
    description: "Vehicle registration certificate for BR-204. Verified.",
    branch: "Ikeja",
    status: "Verified",
    updatedAt: "2026-08-15T10:00:00Z",
    relevance: 82,
    tags: ["Registration", "Vehicle", "Fleet"],
    url: "/documents/DOC-001",
  },
  {
    id: "SR-006",
    type: "Customer",
    title: "Charitos Hospital · C-003",
    subtitle: "Corporate customer · Ikeja",
    reference: "C-003",
    description: "Healthcare provider. Active service agreement. SLA breached.",
    branch: "Ikeja",
    status: "Active",
    updatedAt: "2026-08-22T11:00:00Z",
    relevance: 78,
    tags: ["Healthcare", "Corporate", "SLA"],
    url: "/crm/customers/C-003",
  },
  {
    id: "SR-007",
    type: "Order",
    title: "ORD-2026-884 · Private order",
    subtitle: "Shipment · Yaba to Lekki",
    reference: "ORD-2026-884",
    description: "Private order for Oakfield Stores. 3 packages.",
    branch: "Lekki",
    status: "Dispatched",
    updatedAt: "2026-08-22T13:00:00Z",
    relevance: 75,
    tags: ["Order", "Shipment", "Private"],
    url: "/private-ops/orders/ORD-2026-884",
  },
  {
    id: "SR-008",
    type: "Shipment",
    title: "SH-8821 · Medical supplies",
    subtitle: "Shipment · Charitos Hospital",
    reference: "SH-8821",
    description: "Medical supplies shipment for Charitos Hospital. Temperature-controlled.",
    branch: "Ikeja",
    status: "In Transit",
    updatedAt: "2026-08-22T12:30:00Z",
    relevance: 72,
    tags: ["Medical", "Temperature", "Shipment"],
    url: "/private-ops/shipments/SH-8821",
  },
  {
    id: "SR-009",
    type: "Route",
    title: "RP-204 · Ikeja dispatch route",
    subtitle: "Route plan · 12 stops",
    reference: "RP-204",
    description: "Dispatch route for KG-8491. 12 stops across Lekki.",
    branch: "Ikeja",
    status: "Active",
    updatedAt: "2026-08-22T14:00:00Z",
    relevance: 68,
    tags: ["Route", "Dispatch", "Lekki"],
    url: "/operations/routes/RP-204",
  },
  {
    id: "SR-010",
    type: "Invoice",
    title: "INV-7921 · Charitos Hospital",
    subtitle: "Invoice · ₦4,200,000",
    reference: "INV-7921",
    description: "Invoice for services delivered to Charitos Hospital. Due in 14 days.",
    branch: "Ikeja",
    status: "Outstanding",
    updatedAt: "2026-08-20T16:00:00Z",
    relevance: 65,
    tags: ["Invoice", "Customer", "Finance"],
    url: "/finance/invoices/INV-7921",
  },
  {
    id: "SR-011",
    type: "Package",
    title: "PKG-0802 · Large parcel",
    subtitle: "Package · 38 kg",
    reference: "PKG-0802",
    description: "Large parcel for Oakfield Stores. Fragile items.",
    branch: "Lekki",
    status: "In Transit",
    updatedAt: "2026-08-22T13:30:00Z",
    relevance: 62,
    tags: ["Package", "Fragile", "Large"],
    url: "/private-ops/packages/PKG-0802",
  },
  {
    id: "SR-012",
    type: "Manifest",
    title: "MF-121 · Van manifest",
    subtitle: "Manifest · 14 packages",
    reference: "MF-121",
    description: "Manifest for VN-041. 14 packages for Lekki delivery.",
    branch: "Lekki",
    status: "Closed",
    updatedAt: "2026-08-22T12:00:00Z",
    relevance: 58,
    tags: ["Manifest", "Van", "Delivery"],
    url: "/private-ops/manifests/MF-121",
  },
  {
    id: "SR-013",
    type: "Compliance",
    title: "Restriction RST-204",
    subtitle: "Asset restriction · BR-204",
    reference: "RST-204",
    description: "Vehicle BR-204 safety block. Requires re-inspection.",
    branch: "Ikeja",
    status: "Active",
    updatedAt: "2026-08-20T10:00:00Z",
    relevance: 55,
    tags: ["Restriction", "Safety", "Vehicle"],
    url: "/compliance/restrictions/RST-204",
  },
  {
    id: "SR-014",
    type: "Warehouse",
    title: "Ikeja Central Warehouse",
    subtitle: "Warehouse · 42,000 sq ft",
    reference: "WH-001",
    description: "Main warehouse for Ikeja branch. 14,200 SKUs on hand.",
    branch: "Ikeja",
    status: "Active",
    updatedAt: "2026-08-22T08:00:00Z",
    relevance: 50,
    tags: ["Warehouse", "Stock", "Ikeja"],
    url: "/warehouse/WH-001",
  },
  {
    id: "SR-015",
    type: "Fulfilment",
    title: "Fulfilment Order · F-2026-884",
    subtitle: "Picking in progress",
    reference: "F-2026-884",
    description: "Fulfilment order for Oakfield Stores. 12 items being picked.",
    branch: "Lekki",
    status: "Picking",
    updatedAt: "2026-08-22T11:00:00Z",
    relevance: 45,
    tags: ["Fulfilment", "Picking", "Order"],
    url: "/warehouse/WH-001/outbound/F-2026-884",
  },
  {
    id: "SR-016",
    type: "Support",
    title: "SUP-2026-001 · Support case",
    subtitle: "Technical issue",
    reference: "SUP-2026-001",
    description: "Dispatch plan creation failing. Engineering investigating.",
    branch: "Ikeja",
    status: "InProgress",
    updatedAt: "2026-08-22T10:15:00Z",
    relevance: 40,
    tags: ["Support", "Technical", "Bug"],
    url: "/comms/support/SUP-001",
  },
];

const TYPE_ICONS: Record<string, React.ReactNode> = {
  Customer: <Users className="h-3.5 w-3.5" />,
  Provider: <User className="h-3.5 w-3.5" />,
  Staff: <User className="h-3.5 w-3.5" />,
  Asset: <File className="h-3.5 w-3.5" />,
  Trip: <MapPin className="h-3.5 w-3.5" />,
  Order: <Briefcase className="h-3.5 w-3.5" />,
  Shipment: <Package className="h-3.5 w-3.5" />,
  Package: <Package className="h-3.5 w-3.5" />,
  Manifest: <FileText className="h-3.5 w-3.5" />,
  Route: <GitBranch className="h-3.5 w-3.5" />,
  Incident: <AlertTriangle className="h-3.5 w-3.5" />,
  Compliance: <Shield className="h-3.5 w-3.5" />,
  Document: <FileText className="h-3.5 w-3.5" />,
  Invoice: <FileText className="h-3.5 w-3.5" />,
  Warehouse: <Building2 className="h-3.5 w-3.5" />,
  Fulfilment: <CheckCircle2 className="h-3.5 w-3.5" />,
  Support: <HelpCircle className="h-3.5 w-3.5" />,
};

const TYPE_STYLES: Record<string, { bg: string; text: string }> = {
  Customer: { bg: "bg-blue-bg", text: "text-blue" },
  Provider: { bg: "bg-purple-bg", text: "text-purple" },
  Staff: { bg: "bg-green-bg", text: "text-green" },
  Asset: { bg: "bg-amber-bg", text: "text-amber" },
  Trip: { bg: "bg-blue-bg", text: "text-blue" },
  Order: { bg: "bg-green-bg", text: "text-green" },
  Shipment: { bg: "bg-purple-bg", text: "text-purple" },
  Package: { bg: "bg-amber-bg", text: "text-amber" },
  Manifest: { bg: "bg-surface-2", text: "text-ink-500" },
  Route: { bg: "bg-blue-bg", text: "text-blue" },
  Incident: { bg: "bg-red-bg", text: "text-red" },
  Compliance: { bg: "bg-purple-bg", text: "text-purple" },
  Document: { bg: "bg-blue-bg", text: "text-blue" },
  Invoice: { bg: "bg-green-bg", text: "text-green" },
  Warehouse: { bg: "bg-amber-bg", text: "text-amber" },
  Fulfilment: { bg: "bg-green-bg", text: "text-green" },
  Support: { bg: "bg-red-bg", text: "text-red" },
};

const TYPE_FILTERS = ["All", "Customer", "Provider", "Staff", "Asset", "Trip", "Order", "Shipment", "Package", "Manifest", "Route", "Incident", "Compliance", "Document", "Invoice", "Warehouse", "Fulfilment", "Support"];
const BRANCH_FILTERS = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

// ===== SIMPLE TOAST FUNCTION =====
const toast = (message: string) => {
  if (typeof window !== "undefined") {
    const toastEl = document.createElement("div");
    toastEl.className = "fixed bottom-4 right-4 bg-ink-950 text-white px-4 py-2 rounded-xl text-sm shadow-lg z-50 animate-in slide-in-from-bottom-5";
    toastEl.textContent = message;
    document.body.appendChild(toastEl);
    setTimeout(() => {
      toastEl.classList.add("opacity-0", "transition-opacity", "duration-300");
      setTimeout(() => toastEl.remove(), 300);
    }, 3000);
  }
};

export default function EnterpriseSearchPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<SearchResult[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [hasSearched, setHasSearched] = useState(false);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const performSearch = () => {
    if (!searchQuery.trim()) {
      setData([]);
      setHasSearched(false);
      return;
    }

    setLoading(true);
    setHasSearched(true);

    setTimeout(() => {
      const results = MOCK_SEARCH_RESULTS.filter((item) => {
        const matchesQuery =
          item.title.toLowerCase().includes(searchQuery.toLowerCase()) ||
          item.subtitle.toLowerCase().includes(searchQuery.toLowerCase()) ||
          item.reference.toLowerCase().includes(searchQuery.toLowerCase()) ||
          item.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
          item.tags.some((t) => t.toLowerCase().includes(searchQuery.toLowerCase()));

        const matchesType = selectedType === "All" || item.type === selectedType;
        const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

        return matchesQuery && matchesType && matchesBranch;
      });

      results.sort((a, b) => b.relevance - a.relevance);

      setData(results);
      setLoading(false);
    }, 600);
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      performSearch();
    }
  };

  const clearSearch = () => {
    setSearchQuery("");
    setData([]);
    setHasSearched(false);
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

  const hasActiveFilters = selectedType !== "All" || selectedBranch !== "All branches";

  const clearFilters = () => {
    setSelectedType("All");
    setSelectedBranch("All branches");
  };

  const filterCount = (selectedType !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0);

  const goToResult = (result: SearchResult) => {
    router.push(result.url);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Enterprise Search
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Search across customers, workforce, shipments, packages, manifests, routes, assets, incidents, compliance, invoices, warehouse stock, fulfilment and support.
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
        </div>
      </div>

      {/* ===== SEARCH BAR ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm p-4">
        <div className="flex flex-col gap-3 sm:flex-row">
          <div className="flex-1 relative">
            <div className="absolute inset-y-0 left-0 flex items-center pl-4 pointer-events-none">
              <Search className="h-4 w-4 text-ink-400" />
            </div>
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Search for anything... (e.g., trip ID, provider name, invoice number)"
              className="w-full rounded-xl border border-line bg-white/70 pl-10 pr-4 py-3 text-sm text-ink-950 outline-none focus:border-primary placeholder:text-ink-400"
            />
            {searchQuery && (
              <button
                onClick={clearSearch}
                className="absolute inset-y-0 right-0 flex items-center pr-4"
              >
                <X className="h-4 w-4 text-ink-400 hover:text-ink-600 transition-colors" />
              </button>
            )}
          </div>
          <div className="flex gap-2 flex-shrink-0">
            <button
              onClick={() => setShowFilters(!showFilters)}
              className={`flex h-[46px] items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
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
            <button
              onClick={performSearch}
              disabled={!searchQuery.trim() || loading}
              className={`flex h-[46px] items-center gap-2 rounded-xl px-6 text-sm font-semibold text-white shadow-sm transition ${
                !searchQuery.trim() || loading
                  ? "bg-ink-400 cursor-not-allowed"
                  : "bg-primary shadow-primary/20 hover:bg-primary/90"
              }`}
            >
              {loading ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  Searching...
                </>
              ) : (
                <>
                  <Search className="h-4 w-4" />
                  Search
                </>
              )}
            </button>
          </div>
        </div>

        {/* ===== FILTER DRAWER ===== */}
        {showFilters && (
          <div className="mt-3 rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
            <div className="flex flex-wrap items-center gap-6">
              <div>
                <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Type</div>
                <div className="flex gap-1.5 flex-wrap max-h-[80px] overflow-y-auto">
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
                  {BRANCH_FILTERS.map((branch) => (
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
      </div>

      {/* ===== RESULTS ===== */}
      {hasSearched && (
        <div>
          {loading ? (
            <div className="py-12 text-center">
              <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
              <p className="mt-3 text-sm text-ink-600">Searching...</p>
            </div>
          ) : data.length === 0 ? (
            <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
              <div className="rounded-full bg-surface-2 p-4">
                <Search className="h-8 w-8 text-ink-400" />
              </div>
              <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No results found</h3>
              <p className="mt-1 text-sm text-ink-500">
                No results match your search query "{searchQuery}".
              </p>
              <button
                onClick={clearSearch}
                className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
              >
                Clear search
              </button>
            </div>
          ) : (
            <>
              <div className="flex items-center justify-between mb-3">
                <span className="text-sm text-ink-500">
                  Found {data.length} result{data.length !== 1 ? "s" : ""}
                </span>
                <span className="text-xs text-ink-400">
                  Permission-filtered results
                </span>
              </div>
              <div className="space-y-3">
                {data.map((result) => {
                  const typeIcon = TYPE_ICONS[result.type] || <FileText className="h-3.5 w-3.5" />;
                  const typeStyle = TYPE_STYLES[result.type] || TYPE_STYLES.Document;

                  return (
                    <div
                      key={result.id}
                      onClick={() => goToResult(result)}
                      className="rounded-2xl border border-line bg-white p-4 shadow-sm hover:shadow-md transition-all cursor-pointer group"
                    >
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div className="flex-1 min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                              {typeIcon}
                              {result.type}
                            </span>
                            <span className="font-medium text-ink-950">{result.title}</span>
                            <span className="text-xs text-ink-400 font-mono">{result.reference}</span>
                          </div>
                          <div className="mt-1 text-sm text-ink-600">{result.subtitle}</div>
                          <div className="mt-0.5 text-xs text-ink-500">{result.description}</div>
                          <div className="mt-1.5 flex flex-wrap items-center gap-2 text-xs text-ink-500">
                            <span className="flex items-center gap-1">
                              <MapPin className="h-3 w-3" />
                              {result.branch}
                            </span>
                            <span className="text-ink-300">|</span>
                            <span className="flex items-center gap-1">
                              <Clock className="h-3 w-3" />
                              Updated {formatTime(result.updatedAt)}
                            </span>
                            <span className="text-ink-300">|</span>
                            <span className="flex items-center gap-1 text-primary">
                              <Sparkles className="h-3 w-3" />
                              {result.relevance}% relevance
                            </span>
                          </div>
                          {result.tags.length > 0 && (
                            <div className="mt-1 flex flex-wrap gap-1">
                              {result.tags.map((tag) => (
                                <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                                  {tag}
                                </span>
                              ))}
                            </div>
                          )}
                        </div>
                        <div className="flex-shrink-0">
                          <button className="text-ink-400 group-hover:text-primary transition-colors">
                            <ArrowRight className="h-4 w-4" />
                          </button>
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>
            </>
          )}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Search className="h-4 w-4" />
          DAT-07 · Enterprise Search
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          {hasSearched ? `${data.length} results found` : "Ready to search"}
        </span>
      </div>
    </div>
  );
}