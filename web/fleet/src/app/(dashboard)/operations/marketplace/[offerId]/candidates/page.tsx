// src/app/(dashboard)/operations/marketplace/[offerId]/candidates/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  Search,
  MapPin,
  Truck,
  Users,
  CheckCircle2,
  XCircle,
  AlertCircle,
  SlidersHorizontal,
  UserCheck,
  Shield,
  Star,
  X,
  ChevronUp,
  ChevronDown,
} from "lucide-react";

type Candidate = {
  id: string;
  name: string;
  type: "Provider" | "Asset";
  service: "Dispatch" | "Hauling";
  branch: string;
  status: "Eligible" | "Warning" | "Blocked";
  assignedAsset?: string;
  reason: string;
  rating: number;
  completedJobs: number;
  availability: "Available" | "Busy" | "Offline";
  distance: string;
  eta: string;
};

const CANDIDATES: Candidate[] = [
  {
    id: "P-044",
    name: "Peter Okafor",
    type: "Provider",
    service: "Dispatch",
    branch: "Ikeja",
    status: "Eligible",
    assignedAsset: "Bike BR-88",
    reason: "Eligible - all requirements met",
    rating: 4.8,
    completedJobs: 342,
    availability: "Available",
    distance: "0.8 km",
    eta: "3 min",
  },
  {
    id: "P-052",
    name: "Amina Yusuf",
    type: "Provider",
    service: "Dispatch",
    branch: "Ikeja",
    status: "Warning",
    assignedAsset: "Bike BR-71",
    reason: "Training due in 3 days",
    rating: 4.6,
    completedJobs: 178,
    availability: "Available",
    distance: "1.2 km",
    eta: "5 min",
  },
  {
    id: "P-038",
    name: "Musa Bello",
    type: "Provider",
    service: "Hauling",
    branch: "Ikeja",
    status: "Blocked",
    assignedAsset: "Van VN-04",
    reason: "Safety block - inspection required",
    rating: 4.2,
    completedJobs: 89,
    availability: "Busy",
    distance: "2.5 km",
    eta: "N/A",
  },
  {
    id: "P-061",
    name: "Chinedu Uche",
    type: "Provider",
    service: "Dispatch",
    branch: "Lekki",
    status: "Eligible",
    assignedAsset: "Bike BR-20",
    reason: "Eligible - all requirements met",
    rating: 4.9,
    completedJobs: 512,
    availability: "Available",
    distance: "1.8 km",
    eta: "4 min",
  },
  {
    id: "P-029",
    name: "Kabiru Ahmed",
    type: "Provider",
    service: "Hauling",
    branch: "Apapa",
    status: "Warning",
    assignedAsset: "Truck TR-12",
    reason: "Document expiry in 2 days",
    rating: 4.4,
    completedJobs: 234,
    availability: "Offline",
    distance: "3.4 km",
    eta: "N/A",
  },
  {
    id: "BR-204",
    name: "Honda ACE 110",
    type: "Asset",
    service: "Dispatch",
    branch: "Ikeja",
    status: "Blocked",
    reason: "Safety block - failed inspection",
    rating: 0,
    completedJobs: 0,
    availability: "Busy",
    distance: "0.5 km",
    eta: "N/A",
  },
];

const STATUS_STYLES = {
  Eligible: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: CheckCircle2 },
  Warning: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: AlertCircle },
  Blocked: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: XCircle },
};

const AVAILABILITY_STYLES = {
  Available: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Busy: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Offline: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

type SortField = "status" | "distance" | "rating" | "name";
type SortDirection = "asc" | "desc";

const STATUS_ORDER = { Eligible: 0, Warning: 1, Blocked: 2 };

export default function CandidatesPage() {
  const params = useParams();
  const offerId = params.offerId as string;

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState<string | null>(null);
  const [selectedService, setSelectedService] = useState<string | null>(null);
  const [showFilters, setShowFilters] = useState(false);
  const [sortField, setSortField] = useState<SortField>("status");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  const filtered = CANDIDATES.filter((c) => {
    const matchesSearch =
      search.trim() === "" ||
      c.name.toLowerCase().includes(search.toLowerCase()) ||
      c.id.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === null || c.status === selectedStatus;
    const matchesService = selectedService === null || c.service === selectedService;

    return matchesSearch && matchesStatus && matchesService;
  });

  const sorted = [...filtered].sort((a, b) => {
    let aVal: string | number = a[sortField];
    let bVal: string | number = b[sortField];

    if (sortField === "status") {
      aVal = STATUS_ORDER[a.status as keyof typeof STATUS_ORDER];
      bVal = STATUS_ORDER[b.status as keyof typeof STATUS_ORDER];
    } else if (sortField === "distance") {
      aVal = parseFloat(a.distance);
      bVal = parseFloat(b.distance);
    } else if (sortField === "rating") {
      aVal = a.rating;
      bVal = b.rating;
    }

    if (aVal < bVal) return sortDirection === "asc" ? -1 : 1;
    if (aVal > bVal) return sortDirection === "asc" ? 1 : -1;
    return 0;
  });

  const eligibleCount = CANDIDATES.filter((c) => c.status === "Eligible").length;
  const warningCount = CANDIDATES.filter((c) => c.status === "Warning").length;
  const blockedCount = CANDIDATES.filter((c) => c.status === "Blocked").length;

  const hasActiveFilters = selectedStatus !== null || selectedService !== null || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus(null);
    setSelectedService(null);
  };

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortField(field);
      setSortDirection("asc");
    }
  };

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return <ChevronUp className="h-3 w-3 text-ink-300" />;
    return sortDirection === "asc" ? (
      <ChevronUp className="h-3 w-3 text-primary" />
    ) : (
      <ChevronDown className="h-3 w-3 text-primary" />
    );
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <p className="ml-3">Loading candidates...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-[1400px] space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Eligible candidates</h1>
          <p className="mt-1.5">
            Eligible providers and assets for offer <span className="font-mono text-ink-800">{offerId}</span>
          </p>
        </div>
        <Link
          href={`/operations/marketplace/${offerId}/assign`}
          className="flex items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover whitespace-nowrap"
        >
          <UserCheck className="h-3.5 w-3.5" />
          Assign
        </Link>
      </div>

      {/* ── Stats cards ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4 transition-colors hover:border-ink-300">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Eligible</span>
          </div>
          <div className="mt-3 font-display text-3xl font-bold tracking-[-0.02em] text-ink-950">{eligibleCount}</div>
          <p className="mt-1.5 text-xs text-ink-500">Ready for assignment</p>
        </div>

        <div className="rounded-lg border border-line bg-white p-4 transition-colors hover:border-ink-300">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Warnings</span>
          </div>
          <div className="mt-3 font-display text-3xl font-bold tracking-[-0.02em] text-ink-950">{warningCount}</div>
          <p className="mt-1.5 text-xs text-ink-500">Requires review</p>
        </div>

        <div className="rounded-lg border border-line bg-white p-4 transition-colors hover:border-ink-300">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <XCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Blocked</span>
          </div>
          <div className="mt-3 font-display text-3xl font-bold tracking-[-0.02em] text-ink-950">{blockedCount}</div>
          <p className="mt-1.5 text-xs text-ink-500">Cannot be assigned</p>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search candidates..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedStatus || "All status"}
          <span className="text-ink-300">·</span>
          <Truck className="h-3.5 w-3.5 text-ink-400" />
          {selectedService || "All services"}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || selectedStatus !== null || selectedService !== null
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {(selectedStatus !== null || selectedService !== null) && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedStatus !== null ? 1 : 0) + (selectedService !== null ? 1 : 0)}
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

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">{sorted.length} candidates</span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
              <div className="flex flex-wrap gap-1.5">
                {["Eligible", "Warning", "Blocked"].map((status) => {
                  const style = STATUS_STYLES[status as keyof typeof STATUS_STYLES];
                  return (
                    <button
                      key={status}
                      onClick={() => setSelectedStatus(selectedStatus === status ? null : status)}
                      className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                        selectedStatus === status
                          ? `${style.bg} ${style.text}`
                          : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                      }`}
                    >
                      {status}
                    </button>
                  );
                })}
              </div>
            </div>
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Service</span>
              <div className="flex flex-wrap gap-1.5">
                {["Dispatch", "Hauling"].map((service) => (
                  <button
                    key={service}
                    onClick={() => setSelectedService(selectedService === service ? null : service)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedService === service
                        ? "bg-primary-bg text-primary"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {service}
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

      {/* ── Table ── */}
      <div className="overflow-x-auto rounded-lg border border-line bg-white">
        <table className="w-full min-w-[900px] border-collapse text-left">
          <thead>
            <tr className="border-b border-line bg-surface-1">
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("name")} className="flex items-center gap-1 hover:text-ink-700">
                  Candidate <SortIcon field="name" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Type</th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("status")} className="flex items-center gap-1 hover:text-ink-700">
                  Status <SortIcon field="status" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("distance")} className="flex items-center gap-1 hover:text-ink-700">
                  Distance <SortIcon field="distance" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("rating")} className="flex items-center gap-1 hover:text-ink-700">
                  Rating <SortIcon field="rating" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Availability</th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">Action</th>
            </tr>
          </thead>
          <tbody>
            {sorted.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-4 py-12 text-center text-ink-500">
                  <div className="flex flex-col items-center gap-2">
                    <Users className="h-8 w-8 text-ink-300" />
                    <p className="font-medium">No candidates match your filters</p>
                    <button onClick={clearFilters} className="text-sm text-primary hover:underline">
                      Clear filters
                    </button>
                  </div>
                </td>
              </tr>
            ) : (
              sorted.map((c) => {
                const statusStyle = STATUS_STYLES[c.status];
                const StatusIcon = statusStyle.icon;
                const availabilityStyle = AVAILABILITY_STYLES[c.availability];
                const isEligible = c.status === "Eligible";

                return (
                  <tr key={c.id} className="border-b border-line transition-colors hover:bg-surface-1/50">
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-2.5">
                        <div className={`flex h-9 w-9 items-center justify-center rounded-md ${statusStyle.bg} ${statusStyle.text} flex-shrink-0`}>
                          {c.type === "Provider" ? <Users className="h-[18px] w-[18px]" /> : <Truck className="h-[18px] w-[18px]" />}
                        </div>
                        <div>
                          <span className="font-semibold text-ink-950">{c.name}</span>
                          <div className="font-mono text-[10px] text-ink-400">{c.id}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">{c.type}</td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-1.5">
                        <StatusIcon className={`h-3.5 w-3.5 ${statusStyle.text} flex-shrink-0`} />
                        <span className={`font-semibold ${statusStyle.text}`}>{c.status}</span>
                      </div>
                      <div className="text-xs text-ink-500">{c.reason}</div>
                    </td>
                    <td className="px-4 py-3.5 font-mono font-medium text-ink-700 whitespace-nowrap">{c.distance}</td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      {c.type === "Provider" ? (
                        <div className="flex items-center gap-1 whitespace-nowrap">
                          <Star className="h-3.5 w-3.5 fill-amber text-amber flex-shrink-0" />
                          <span className="font-semibold text-ink-950">{c.rating}</span>
                          <span className="text-xs text-ink-500">({c.completedJobs} jobs)</span>
                        </div>
                      ) : (
                        <span className="text-ink-500">—</span>
                      )}
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-1.5 whitespace-nowrap">
                        <span className={`h-1.5 w-1.5 rounded-full ${availabilityStyle.dot} flex-shrink-0`} />
                        <span className={`font-medium ${availabilityStyle.text}`}>{c.availability}</span>
                        {c.eta !== "N/A" && <span className="text-xs text-ink-500">· {c.eta}</span>}
                      </div>
                    </td>
                    <td className="px-4 py-3.5 text-right whitespace-nowrap">
                      <div className="flex items-center justify-end gap-2">
                        <button className="rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors">
                          <Shield className="h-3.5 w-3.5" />
                        </button>
                        {isEligible ? (
                          <Link
                            href={`/operations/marketplace/${offerId}/assign?candidate=${c.id}`}
                            className="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-hover transition-colors whitespace-nowrap"
                          >
                            Select
                          </Link>
                        ) : (
                          <span className="rounded-md bg-surface-2 px-3 py-1.5 text-xs font-medium text-ink-400 cursor-not-allowed whitespace-nowrap">
                            Blocked
                          </span>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}