// src/app/(dashboard)/operations/dispatch/unassigned/page.tsx

"use client";

import { useState, useEffect } from "react";
import {
  Search,
  MapPin,
  AlertCircle,
  Clock,
  Package,
  Truck,
  ShoppingBag,
  UserPlus,
  Eye,
  SlidersHorizontal,
  X,
} from "lucide-react";
import { UNASSIGNED_WORK, type UnassignedWork } from "@/lib/mock-dispatch-data";

const URGENCY_STYLES: Record<string, { dot: string; text: string; bg: string }> = {
  High: { dot: "bg-red", text: "text-red", bg: "bg-red-bg" },
  Medium: { dot: "bg-amber", text: "text-amber", bg: "bg-amber-bg" },
  Low: { dot: "bg-accent", text: "text-accent", bg: "bg-accent-bg" },
};

const SERVICE_ICONS: Record<string, React.ReactNode> = {
  Dispatch: <Package className="h-3.5 w-3.5" />,
  Hauling: <Truck className="h-3.5 w-3.5" />,
  Private: <ShoppingBag className="h-3.5 w-3.5" />,
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function UnassignedWorkPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [filterUrgency, setFilterUrgency] = useState<string | null>(null);
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [unassignedWork, setUnassignedWork] = useState<UnassignedWork[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setUnassignedWork(UNASSIGNED_WORK);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredWork = unassignedWork.filter((work) => {
    const matchesSearch =
      search.trim() === "" ||
      work.id.toLowerCase().includes(search.toLowerCase()) ||
      work.title.toLowerCase().includes(search.toLowerCase()) ||
      work.route.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || work.branch === selectedBranch;
    const matchesUrgency = filterUrgency === null || work.urgency === filterUrgency;

    return matchesSearch && matchesBranch && matchesUrgency;
  });

  const urgentCount = unassignedWork.filter((w) => w.urgency === "High").length;
  const blockedCount = unassignedWork.filter((w) => w.blocker !== null).length;
  const readyCount = unassignedWork.filter((w) => w.blocker === null).length;

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <p className="ml-3">Loading unassigned work...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Unassigned work</h1>
        <p className="mt-1.5">
          Work requiring provider and asset assignment. Review readiness blockers and urgency before dispatching.
        </p>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Urgent</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{urgentCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Blocked</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{blockedCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Package className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Ready to assign</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{readyCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, title, or route..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Urgency:</span>
          {filterUrgency || "All"}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || filterUrgency !== null
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {filterUrgency !== null && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">1</span>
          )}
        </button>

        {(filterUrgency !== null || selectedBranch !== "All branches" || search) && (
          <button
            onClick={() => {
              setFilterUrgency(null);
              setSelectedBranch("All branches");
              setSearch("");
            }}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Urgency</span>
              <div className="flex flex-wrap gap-1.5">
                {["High", "Medium", "Low"].map((urgency) => (
                  <button
                    key={urgency}
                    onClick={() => setFilterUrgency(filterUrgency === urgency ? null : urgency)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      filterUrgency === urgency
                        ? `bg-${urgency === "High" ? "red" : urgency === "Medium" ? "amber" : "accent"}-bg text-${urgency === "High" ? "red" : urgency === "Medium" ? "amber" : "accent"}`
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {urgency}
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
              onClick={() => {
                setFilterUrgency(null);
                setSelectedBranch("All branches");
                setSearch("");
              }}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Work list ── */}
      {filteredWork.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <Package className="h-8 w-8 text-ink-300" />
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No unassigned work</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            All work has been assigned or there are no items matching your filters.
          </p>
          <button
            onClick={() => {
              setSearch("");
              setFilterUrgency(null);
              setSelectedBranch("All branches");
            }}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-3.5 lg:grid-cols-2 xl:grid-cols-3">
          {filteredWork.map((work) => {
            const urgencyStyle = URGENCY_STYLES[work.urgency];
            const ServiceIcon = SERVICE_ICONS[work.serviceType];

            return (
              <div key={work.id} className="rounded-lg border border-line bg-white p-4">
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-1.5">
                    <span className="font-mono text-[10px] text-ink-500">{work.id}</span>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${urgencyStyle.bg} ${urgencyStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${urgencyStyle.dot}`} />
                      {work.urgency}
                    </span>
                  </div>
                  <div className="flex items-center gap-1 text-[10px] text-ink-500">
                    <Clock className="h-3 w-3" />
                    {work.pickupTime}
                  </div>
                </div>

                <div className="mt-2">
                  <h3 className="text-sm font-semibold text-ink-950">{work.title}</h3>
                  <div className="mt-0.5 flex items-center gap-1.5 text-xs text-ink-600">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{work.route}</span>
                  </div>
                </div>

                <div className="mt-2.5 flex flex-wrap items-center gap-1.5 border-t border-line pt-2.5">
                  <span className="flex items-center gap-1 rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-medium text-ink-600">
                    {ServiceIcon}
                    {work.serviceType}
                  </span>
                  <span className="flex items-center gap-1 rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-medium text-ink-600">
                    <Package className="h-3 w-3" />
                    {work.parcelCount}
                  </span>
                  <span className="flex items-center gap-1 rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-medium text-ink-600">
                    {work.distance}
                  </span>
                </div>

                <div className="mt-1.5 flex flex-wrap gap-1">
                  {work.fragile && (
                    <span className="rounded-full bg-amber-bg px-2 py-0.5 text-[9px] font-semibold text-amber">
                      Fragile
                    </span>
                  )}
                  {work.coldChain && (
                    <span className="rounded-full bg-accent-bg px-2 py-0.5 text-[9px] font-semibold text-accent">
                      Cold chain
                    </span>
                  )}
                  {work.blocker && (
                    <span className="flex items-center gap-1 rounded-full bg-red-bg px-2 py-0.5 text-[9px] font-semibold text-red">
                      <AlertCircle className="h-3 w-3" />
                      {work.blocker}
                    </span>
                  )}
                </div>

                <div className="mt-3 flex items-center gap-2 border-t border-line pt-2.5">
                  <button className="flex-1 rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-hover transition-colors">
                    Assign
                  </button>
                  <button className="flex items-center gap-1 rounded-md border border-line bg-white px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors">
                    <Eye className="h-3.5 w-3.5" />
                    Details
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}