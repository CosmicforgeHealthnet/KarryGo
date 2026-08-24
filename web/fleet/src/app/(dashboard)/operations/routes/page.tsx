// src/app/(dashboard)/operations/routes/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  Search,
  MapPin,
  Truck,
  Package,
  Clock,
  RefreshCw,
  SlidersHorizontal,
  X,
  Plus,
  Layers,
  Rocket,
  Edit3,
  ArrowRight,
  Users,
} from "lucide-react";
import { ROUTES } from "@/lib/mock-routes-data";

type Route = {
  id: string;
  name: string;
  service: "Dispatch" | "Hauling";
  branch: string;
  status: "Draft" | "Locked" | "Published" | "Archived";
  stopCount: number;
  provider: { id: string; name: string; rating: number };
  asset: { id: string; name: string; type: string };
  distance: string;
  duration: string;
  eta: string;
  lastUpdated: string;
  progress: number;
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Draft: { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" },
  Locked: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Published: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Archived: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const SERVICE_ICONS = {
  Dispatch: <Package className="h-4 w-4" />,
  Hauling: <Truck className="h-4 w-4" />,
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Draft", "Locked", "Published", "Archived"];

export default function RoutesPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [statusFilter, setStatusFilter] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [routes, setRoutes] = useState<Route[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setRoutes(ROUTES);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredRoutes = routes.filter((route) => {
    const matchesSearch =
      search.trim() === "" ||
      route.id.toLowerCase().includes(search.toLowerCase()) ||
      route.name.toLowerCase().includes(search.toLowerCase()) ||
      route.provider.name.toLowerCase().includes(search.toLowerCase()) ||
      route.asset.id.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || route.branch === selectedBranch;
    const matchesStatus = statusFilter === "All" || route.status === statusFilter;

    return matchesSearch && matchesBranch && matchesStatus;
  });

  const draftCount = routes.filter((r) => r.status === "Draft").length;
  const publishedCount = routes.filter((r) => r.status === "Published").length;
  const lockedCount = routes.filter((r) => r.status === "Locked").length;

  const hasActiveFilters = statusFilter !== "All" || selectedBranch !== "All branches" || search !== "";

  const clearFilters = () => {
    setStatusFilter("All");
    setSelectedBranch("All branches");
    setSearch("");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading routes...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1>Route plans</h1>
        
          </div>
          <p className="mt-1.5">Planned routes with assigned providers, assets, and stop sequences.</p>
        </div>
        <Link
          href="/operations/routes/create"
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Create route
        </Link>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-surface-2 text-ink-600">
              <Edit3 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Draft</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{draftCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Locked</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{lockedCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Rocket className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Published</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{publishedCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search routes..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {statusFilter}
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
          {(statusFilter !== "All" || selectedBranch !== "All branches") && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(statusFilter !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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
          {filteredRoutes.length} routes
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
                    onClick={() => setStatusFilter(status)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      statusFilter === status
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

      {/* ── Routes Cards ── */}
      {filteredRoutes.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Layers className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No routes found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No routes match your current filters. Create a new route to get started.
          </p>
          <Link
            href="/operations/routes/create"
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Create route
          </Link>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredRoutes.map((route) => {
            const statusStyle = STATUS_STYLES[route.status] || STATUS_STYLES.Draft;
            const ServiceIcon = SERVICE_ICONS[route.service];
            const isPublished = route.status === "Published";

            return (
              <Link
                key={route.id}
                href={`/operations/routes/${route.id}`}
                className="group flex flex-col rounded-lg border border-line bg-white p-4 transition-colors hover:bg-surface-2"
              >
                {/* Header */}
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-ink-950 truncate">
                        {route.name}
                      </span>
                      <span className="font-mono text-[10px] text-ink-400 flex-shrink-0">
                        {route.id}
                      </span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center gap-1.5">
                      <span
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                        {route.status}
                      </span>
                      {isPublished && (
                        <span className="flex items-center gap-1.5 rounded-full bg-green-bg px-2 py-0.5 text-[10px] font-semibold text-green">
                          <span className="relative flex h-1.5 w-1.5">
                            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                          </span>
                          Live
                        </span>
                      )}
                    </div>
                  </div>
                  <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-primary-bg text-primary">
                    {ServiceIcon}
                  </div>
                </div>

                {/* Details */}
                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{route.branch}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Layers className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{route.stopCount} stops</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Users className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{route.provider.name}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Truck className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-mono text-[10px]">{route.asset.id}</span>
                  </div>
                </div>

                {/* Divider */}
                <div className="mt-3 border-t border-line" />

                {/* Footer */}
                <div className="mt-3 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <div className="h-1.5 w-16 rounded-full bg-surface-3 overflow-hidden">
                      <div
                        className={`h-full rounded-full transition-all ${
                          route.progress === 100 ? "bg-green" : isPublished ? "bg-primary" : "bg-ink-400"
                        }`}
                        style={{ width: `${route.progress}%` }}
                      />
                    </div>
                    <span className="text-[10px] font-medium text-ink-500">
                      {route.progress}%
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Clock className="h-3.5 w-3.5 text-ink-400" />
                    <span className="font-medium">{route.eta}</span>
                  </div>
                  <ArrowRight className="h-4 w-4 text-ink-300 transition-colors group-hover:text-primary" />
                </div>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}