// src/app/(dashboard)/operations/marketplace/page.tsx

"use client";

import { useState } from "react";
import Link from "next/link";
import {
  Package,
  Truck,
  Search,
  ListChecks,
  Clock3,
  CheckCircle2,
  XCircle,
  SlidersHorizontal,
  ArrowRight,
  X,
  MapPin,
  ChevronUp,
  ChevronDown,
} from "lucide-react";

const BTN_PRIMARY =
  "flex items-center justify-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-hover";
const BTN_SECONDARY =
  "flex items-center justify-center gap-1.5 rounded-lg border border-line bg-white px-3.5 py-2 text-sm font-semibold text-ink-800 transition-colors hover:bg-surface-2";

type Tone = "blue" | "green" | "amber" | "red";

const TONE_BG: Record<Tone, string> = {
  blue: "bg-primary-bg text-primary",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
};

const METRICS = [
  { label: "Open offers", value: "6", note: "Across 4 branches", tone: "blue" as Tone, trend: "+2", icon: <ListChecks className="h-[18px] w-[18px]" /> },
  { label: "Expiring soon", value: "2", note: "Under 20 min remaining", tone: "amber" as Tone, trend: "33%", icon: <Clock3 className="h-[18px] w-[18px]" /> },
  { label: "Accepted today", value: "18", note: "Company assignments", tone: "green" as Tone, trend: "+5", icon: <CheckCircle2 className="h-[18px] w-[18px]" /> },
  { label: "Declined today", value: "3", note: "2 flagged as safety", tone: "red" as Tone, trend: "17%", icon: <XCircle className="h-[18px] w-[18px]" /> },
];

type Offer = {
  id: string;
  service: "Dispatch" | "Hauling";
  pickup: string;
  dropoff: string;
  deadline: string;
  deadlineMinutes: number;
  vehicle: string;
  distance: string;
  load: string;
};

const OFFERS: Offer[] = [
  { id: "MKT-24081", service: "Dispatch", pickup: "Ikeja", dropoff: "Lekki", deadline: "27 min", deadlineMinutes: 27, vehicle: "Bike", distance: "6.8 km", load: "18 kg" },
  { id: "MKT-24079", service: "Hauling", pickup: "Apapa", dropoff: "Ikeja", deadline: "41 min", deadlineMinutes: 41, vehicle: "Van", distance: "15.4 km", load: "220 kg" },
  { id: "MKT-24073", service: "Dispatch", pickup: "Yaba", dropoff: "Victoria Island", deadline: "16 min", deadlineMinutes: 16, vehicle: "Bike", distance: "4.2 km", load: "8 kg" },
  { id: "MKT-24069", service: "Hauling", pickup: "Lekki", dropoff: "Marina", deadline: "52 min", deadlineMinutes: 52, vehicle: "Truck", distance: "21 km", load: "1.2 t" },
  { id: "MKT-24062", service: "Dispatch", pickup: "Surulere", dropoff: "Yaba", deadline: "13 min", deadlineMinutes: 13, vehicle: "Bike", distance: "3.4 km", load: "5 kg" },
  { id: "MKT-24058", service: "Hauling", pickup: "Ikeja", dropoff: "Ikorodu", deadline: "35 min", deadlineMinutes: 35, vehicle: "Van", distance: "17.8 km", load: "180 kg" },
];

const BRANCHES = ["All", "Ikeja", "Lekki", "Yaba", "Apapa"];
const SERVICES = ["All", "Dispatch", "Hauling"];

type SortField = "deadline" | "distance" | "load" | "id";
type SortDirection = "asc" | "desc";

function fuseTier(mins: number): { color: string; bg: string } {
  if (mins <= 15) return { color: "text-red", bg: "bg-red-bg" };
  if (mins <= 30) return { color: "text-amber", bg: "bg-amber-bg" };
  return { color: "text-primary", bg: "bg-primary-bg" };
}

export default function MarketplaceOffersPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All");
  const [selectedService, setSelectedService] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [sortField, setSortField] = useState<SortField>("deadline");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");

  const filtered = OFFERS.filter((o) => {
    const matchesSearch =
      search.trim() === "" ||
      o.id.toLowerCase().includes(search.toLowerCase()) ||
      o.pickup.toLowerCase().includes(search.toLowerCase()) ||
      o.dropoff.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All" || o.pickup === selectedBranch || o.dropoff === selectedBranch;
    const matchesService = selectedService === "All" || o.service === selectedService;

    return matchesSearch && matchesBranch && matchesService;
  });

  const sorted = [...filtered].sort((a, b) => {
    let aVal: string | number = a[sortField];
    let bVal: string | number = b[sortField];

    if (sortField === "deadline") {
      aVal = a.deadlineMinutes;
      bVal = b.deadlineMinutes;
    } else if (sortField === "distance") {
      aVal = parseFloat(a.distance);
      bVal = parseFloat(b.distance);
    } else if (sortField === "load") {
      aVal = parseFloat(a.load);
      bVal = parseFloat(b.load);
    }

    if (aVal < bVal) return sortDirection === "asc" ? -1 : 1;
    if (aVal > bVal) return sortDirection === "asc" ? 1 : -1;
    return 0;
  });

  const hasActiveFilters = selectedBranch !== "All" || selectedService !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All");
    setSelectedService("All");
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

  return (
    <div className="mx-auto max-w-[1600px] space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Marketplace offers</h1>
        <p className="max-w-2xl">Current eligible KarryGo opportunities. Sort by deadline to see urgent offers first.</p>
      </div>

      {/* ── Metric grid ── */}
      <div className="grid grid-cols-2 gap-3.5 lg:grid-cols-4">
        {METRICS.map((m) => (
          <div key={m.label} className="rounded-lg border border-line bg-white p-4 transition-colors hover:border-ink-300">
            <div className="flex items-center gap-2.5">
              <div className={`flex h-8 w-8 items-center justify-center rounded-md ${TONE_BG[m.tone]}`}>
                {m.icon}
              </div>
              <span className="text-xs font-semibold text-ink-600">{m.label}</span>
            </div>
            <div className="mt-3 flex items-end justify-between">
              <span className="font-display text-3xl font-bold tracking-[-0.02em] text-ink-950">{m.value}</span>
              <span className={`rounded-full px-2 py-0.5 text-[10px] font-bold ${TONE_BG[m.tone]}`}>
                {m.trend}
              </span>
            </div>
            <p className="mt-1.5 text-xs text-ink-500">{m.note}</p>
          </div>
        ))}
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search offers..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <Package className="h-3.5 w-3.5 text-ink-400" />
          {selectedService}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters ? "border-primary bg-primary-bg text-primary" : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
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

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">{sorted.length} offers</span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
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
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Service</span>
              <div className="flex flex-wrap gap-1.5">
                {SERVICES.map((service) => (
                  <button
                    key={service}
                    onClick={() => setSelectedService(service)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedService === service
                        ? "bg-primary text-white"
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
                <button onClick={() => handleSort("id")} className="flex items-center gap-1 hover:text-ink-700">
                  Offer <SortIcon field="id" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Route</th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Vehicle</th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("distance")} className="flex items-center gap-1 hover:text-ink-700">
                  Distance <SortIcon field="distance" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("load")} className="flex items-center gap-1 hover:text-ink-700">
                  Load <SortIcon field="load" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("deadline")} className="flex items-center gap-1 hover:text-ink-700">
                  Deadline <SortIcon field="deadline" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">Action</th>
            </tr>
          </thead>
          <tbody>
            {sorted.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-4 py-12 text-center text-ink-500">
                  <div className="flex flex-col items-center gap-2">
                    <Package className="h-8 w-8 text-ink-300" />
                    <p className="font-medium">No offers match your filters</p>
                    <button onClick={clearFilters} className="text-sm text-primary hover:underline">
                      Clear filters
                    </button>
                  </div>
                </td>
              </tr>
            ) : (
              sorted.map((o) => {
                const tier = fuseTier(o.deadlineMinutes);
                const isUrgent = o.deadlineMinutes <= 15;
                return (
                  <tr key={o.id} className="border-b border-line transition-colors hover:bg-surface-1/50">
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-2.5">
                        <div className={`flex h-8 w-8 items-center justify-center rounded-md ${tier.bg} ${tier.color}`}>
                          {o.service === "Hauling" ? <Truck className="h-[15px] w-[15px]" /> : <Package className="h-[15px] w-[15px]" />}
                        </div>
                        <div>
                          <span className="font-semibold text-ink-950">{o.service}</span>
                          <div className="font-mono text-[10px] text-ink-400">{o.id}</div>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-1.5">
                        <span>{o.pickup}</span>
                        <ArrowRight className="h-3 w-3 text-ink-300 flex-shrink-0" />
                        <span>{o.dropoff}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 font-medium text-ink-700 whitespace-nowrap">{o.vehicle}</td>
                    <td className="px-4 py-3.5 font-mono font-medium text-ink-700 whitespace-nowrap">{o.distance}</td>
                    <td className="px-4 py-3.5 font-mono font-medium text-ink-700 whitespace-nowrap">{o.load}</td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-2">
                        <div className={`h-1.5 w-1.5 rounded-full ${tier.color.replace("text", "bg")} flex-shrink-0`} />
                        <span className={`font-mono font-bold ${tier.color}`}>
                          {isUrgent ? "🔴 " : ""}{o.deadline}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 text-right whitespace-nowrap">
                      <div className="flex items-center justify-end gap-2">
                        <Link href={`/operations/marketplace/${o.id}`} className="rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap">
                          Details
                        </Link>
                        <Link href={`/operations/marketplace/${o.id}/candidates`} className="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-hover transition-colors whitespace-nowrap">
                          Compare
                        </Link>
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