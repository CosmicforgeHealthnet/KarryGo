// src/app/(dashboard)/providers/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  Search,
  Plus,
  Users,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Truck,
  Package,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Eye,
  MoreVertical,
  Phone,
  Mail,
  Star,
  Filter,
  ArrowRight,
  Activity,
  UserCheck,
  UserX,
  Shield,
  Calendar,
} from "lucide-react";

type Provider = {
  id: string;
  name: string;
  initials: string;
  service: "Dispatch" | "Hauling";
  branch: string;
  status: "Online" | "Busy" | "Offline" | "Leave";
  eligibility: "Eligible" | "Warning" | "Blocked";
  asset: string;
  rating: number;
  phone: string;
  email: string;
  lastActive: string;
  avatarColor?: string;
};

// Mock data
const PROVIDERS: Provider[] = [
  {
    id: "P-044",
    name: "Peter Okafor",
    initials: "PO",
    service: "Dispatch",
    branch: "Ikeja",
    status: "Online",
    eligibility: "Eligible",
    asset: "Bike BR-88",
    rating: 4.8,
    phone: "+234 802 345 6789",
    email: "peter.okafor@example.com",
    lastActive: "12 sec ago",
    avatarColor: "bg-blue-500",
  },
  {
    id: "P-038",
    name: "Musa Bello",
    initials: "MB",
    service: "Hauling",
    branch: "Apapa",
    status: "Busy",
    eligibility: "Eligible",
    asset: "Van VN-04",
    rating: 4.2,
    phone: "+234 803 456 7890",
    email: "musa.bello@example.com",
    lastActive: "45 sec ago",
    avatarColor: "bg-green-500",
  },
  {
    id: "P-052",
    name: "Amina Yusuf",
    initials: "AY",
    service: "Dispatch",
    branch: "Yaba",
    status: "Online",
    eligibility: "Warning",
    asset: "Bike BR-71",
    rating: 4.6,
    phone: "+234 804 567 8901",
    email: "amina.yusuf@example.com",
    lastActive: "2 min ago",
    avatarColor: "bg-amber-500",
  },
  {
    id: "P-061",
    name: "Chinedu Uche",
    initials: "CU",
    service: "Dispatch",
    branch: "Lekki",
    status: "Offline",
    eligibility: "Eligible",
    asset: "Bike BR-20",
    rating: 4.9,
    phone: "+234 805 678 9012",
    email: "chinedu.uche@example.com",
    lastActive: "4 min ago",
    avatarColor: "bg-purple-500",
  },
  {
    id: "P-029",
    name: "Kabiru Ahmed",
    initials: "KA",
    service: "Hauling",
    branch: "Lekki",
    status: "Leave",
    eligibility: "Blocked",
    asset: "Truck TR-12",
    rating: 4.4,
    phone: "+234 806 789 0123",
    email: "kabiru.ahmed@example.com",
    lastActive: "1 hr ago",
    avatarColor: "bg-red-500",
  },
  {
    id: "P-073",
    name: "Esther Obi",
    initials: "EO",
    service: "Dispatch",
    branch: "Ikeja",
    status: "Online",
    eligibility: "Eligible",
    asset: "Bike BR-12",
    rating: 4.7,
    phone: "+234 807 890 1234",
    email: "esther.obi@example.com",
    lastActive: "3 min ago",
    avatarColor: "bg-pink-500",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Online: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Busy: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Offline: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Leave: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
};

const ELIGIBILITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Eligible: {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <CheckCircle2 className="h-3 w-3" />,
  },
  Warning: {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <AlertCircle className="h-3 w-3" />,
  },
  Blocked: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <XCircle className="h-3 w-3" />,
  },
};

const SERVICE_ICONS = {
  Dispatch: <Package className="h-3.5 w-3.5" />,
  Hauling: <Truck className="h-3.5 w-3.5" />,
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Online", "Busy", "Offline", "Leave"];
const ELIGIBILITY_FILTERS = ["All", "Eligible", "Warning", "Blocked"];

export default function ProvidersPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedEligibility, setSelectedEligibility] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Provider[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PROVIDERS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.asset.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesEligibility =
      selectedEligibility === "All" || item.eligibility === selectedEligibility;

    return matchesSearch && matchesBranch && matchesStatus && matchesEligibility;
  });

  const onlineCount = data.filter((d) => d.status === "Online").length;
  const eligibleCount = data.filter((d) => d.eligibility === "Eligible").length;
  const blockedCount = data.filter((d) => d.eligibility === "Blocked").length;

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedStatus !== "All" ||
    selectedEligibility !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
    setSelectedEligibility("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading providers...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Managed Providers</h1>
          <p className="mt-1.5">
            Company-linked Dispatch and Hauling providers with membership, platform state, availability, eligibility and restrictions.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <Link
            href="/providers/add"
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Add
          </Link>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Activity className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Online</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{onlineCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <UserCheck className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Eligible</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{eligibleCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <Shield className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Blocked</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{blockedCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, ID, or asset..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {selectedStatus}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Eligibility:</span>
          {selectedEligibility}
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
              {(selectedBranch !== "All branches" ? 1 : 0) +
                (selectedStatus !== "All" ? 1 : 0) +
                (selectedEligibility !== "All" ? 1 : 0)}
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
          {filteredData.length} providers
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === status
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Eligibility</span>
              <div className="flex flex-wrap gap-1.5">
                {ELIGIBILITY_FILTERS.map((eligibility) => (
                  <button
                    key={eligibility}
                    onClick={() => setSelectedEligibility(eligibility)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedEligibility === eligibility
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {eligibility}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</span>
              <div className="flex flex-wrap gap-1.5">
                {BRANCHES.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedBranch === branch
                        ? "bg-primary text-white"
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
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Provider Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No providers found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No providers match your current filters. Add a new provider to get started.
          </p>
          <Link
            href="/providers/add"
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Add provider
          </Link>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Offline;
            const eligibilityStyle = ELIGIBILITY_STYLES[item.eligibility] || ELIGIBILITY_STYLES.Eligible;
            const ServiceIcon = SERVICE_ICONS[item.service];
            const isEligible = item.eligibility === "Eligible";
            const isOnline = item.status === "Online";

            return (
              <Link
                key={item.id}
                href={`/providers/${item.id}`}
                className={`group rounded-lg border p-4 transition-colors ${
                  isEligible
                    ? isOnline
                      ? "border-green/20 bg-white hover:border-green/40 hover:bg-surface-2"
                      : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                    : item.eligibility === "Warning"
                    ? "border-amber/20 bg-white hover:border-amber/40 hover:bg-surface-2"
                    : "border-red/20 bg-red-bg/10 hover:border-red/40 hover:bg-red-bg/20"
                }`}
              >
                <div className="flex items-start gap-3">
                  <div
                    className={`flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full text-base font-bold text-white ${item.avatarColor || "bg-primary"}`}
                  >
                    {item.initials}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-semibold text-ink-950">
                        {item.name}
                      </span>
                      <span className="font-mono text-[10px] text-ink-400 flex-shrink-0">
                        {item.id}
                      </span>
                    </div>
                    <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                      <span
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                        {item.status}
                      </span>
                      <span
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${eligibilityStyle.bg} ${eligibilityStyle.text}`}
                      >
                        {eligibilityStyle.icon}
                        {item.eligibility}
                      </span>
                      <span className="flex items-center gap-1 rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                        {ServiceIcon}
                        {item.service}
                      </span>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Truck className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-mono text-[10px]">{item.asset}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Star className="h-3.5 w-3.5 text-amber fill-amber" />
                    <span>{item.rating}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Clock className="h-3.5 w-3.5" />
                    <span>{item.lastActive}</span>
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-2 text-[10px] text-ink-400">
                    <Phone className="h-3.5 w-3.5" />
                    <span className="truncate max-w-[100px]">{item.phone}</span>
                  </div>
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
                  </div>
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}