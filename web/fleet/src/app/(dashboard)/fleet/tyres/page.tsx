// src/app/(dashboard)/fleet/tyres/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  Circle,
  RefreshCw,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Eye,
  Filter,
  ArrowRight,
  Calendar,
  User,
  Truck,
  FileText,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Award,
  Zap,
  Gauge,
  AlertTriangle,
  MoreHorizontal,
  Edit3,
  Trash2,
  Car,
  Bike,
  List,
} from "lucide-react";

type Tyre = {
  id: string;
  assetId: string;
  assetName: string;
  assetType: string;
  position: "FL" | "FR" | "RL" | "RR" | "Spare";
  brand: string;
  size: string;
  condition: "New" | "Good" | "Worn" | "Critical" | "Replaced";
  installedDate: string;
  lastRotation: string;
  mileage: string;
  branch: string;
  status: "Active" | "Inactive";
};

const TYRES: Tyre[] = [
  {
    id: "TYR-001",
    assetId: "BR-204",
    assetName: "Honda ACE 110",
    assetType: "Motorcycle",
    position: "FL",
    brand: "Michelin",
    size: "120/80 R17",
    condition: "Worn",
    installedDate: "2026-03-10",
    lastRotation: "2026-07-15",
    mileage: "8,440 km",
    branch: "Ikeja",
    status: "Active",
  },
  {
    id: "TYR-002",
    assetId: "BR-204",
    assetName: "Honda ACE 110",
    assetType: "Motorcycle",
    position: "FR",
    brand: "Michelin",
    size: "120/80 R17",
    condition: "Good",
    installedDate: "2026-03-10",
    lastRotation: "2026-07-15",
    mileage: "8,440 km",
    branch: "Ikeja",
    status: "Active",
  },
  {
    id: "TYR-003",
    assetId: "VN-041",
    assetName: "Mercedes Sprinter",
    assetType: "Van",
    position: "FL",
    brand: "Bridgestone",
    size: "225/75 R16",
    condition: "Good",
    installedDate: "2026-01-20",
    lastRotation: "2026-06-10",
    mileage: "28,450 km",
    branch: "Lekki",
    status: "Active",
  },
  {
    id: "TYR-004",
    assetId: "VN-041",
    assetName: "Mercedes Sprinter",
    assetType: "Van",
    position: "FR",
    brand: "Bridgestone",
    size: "225/75 R16",
    condition: "Good",
    installedDate: "2026-01-20",
    lastRotation: "2026-06-10",
    mileage: "28,450 km",
    branch: "Lekki",
    status: "Active",
  },
  {
    id: "TYR-005",
    assetId: "VN-041",
    assetName: "Mercedes Sprinter",
    assetType: "Van",
    position: "RL",
    brand: "Bridgestone",
    size: "225/75 R16",
    condition: "Worn",
    installedDate: "2026-01-20",
    lastRotation: "2026-06-10",
    mileage: "28,450 km",
    branch: "Lekki",
    status: "Active",
  },
  {
    id: "TYR-006",
    assetId: "VN-041",
    assetName: "Mercedes Sprinter",
    assetType: "Van",
    position: "RR",
    brand: "Bridgestone",
    size: "225/75 R16",
    condition: "Critical",
    installedDate: "2026-01-20",
    lastRotation: "2026-06-10",
    mileage: "28,450 km",
    branch: "Lekki",
    status: "Active",
  },
  {
    id: "TYR-007",
    assetId: "TR-112",
    assetName: "Scania R500",
    assetType: "Truck",
    position: "FL",
    brand: "Goodyear",
    size: "295/80 R22.5",
    condition: "Good",
    installedDate: "2025-11-05",
    lastRotation: "2026-05-20",
    mileage: "65,220 km",
    branch: "Apapa",
    status: "Active",
  },
  {
    id: "TYR-008",
    assetId: "BR-188",
    assetName: "Yamaha FZ",
    assetType: "Motorcycle",
    position: "FL",
    brand: "Pirelli",
    size: "110/70 R17",
    condition: "New",
    installedDate: "2026-08-01",
    lastRotation: "2026-08-01",
    mileage: "850 km",
    branch: "Yaba",
    status: "Active",
  },
  {
    id: "TYR-009",
    assetId: "BR-188",
    assetName: "Yamaha FZ",
    assetType: "Motorcycle",
    position: "FR",
    brand: "Pirelli",
    size: "140/70 R17",
    condition: "New",
    installedDate: "2026-08-01",
    lastRotation: "2026-08-01",
    mileage: "850 km",
    branch: "Yaba",
    status: "Active",
  },
];

const POSITION_LABELS: Record<string, string> = {
  FL: "Front Left",
  FR: "Front Right",
  RL: "Rear Left",
  RR: "Rear Right",
  Spare: "Spare",
};

const POSITION_COLORS: Record<string, string> = {
  FL: "bg-blue-500",
  FR: "bg-blue-400",
  RL: "bg-amber-500",
  RR: "bg-amber-400",
  Spare: "bg-ink-400",
};

const CONDITION_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  New: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Good: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Worn: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Replaced: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const CONDITION_FILTERS = ["All", "New", "Good", "Worn", "Critical", "Replaced"];

export default function TyresPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedCondition, setSelectedCondition] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Tyre[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(TYRES);
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
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.assetId.toLowerCase().includes(search.toLowerCase()) ||
      item.assetName.toLowerCase().includes(search.toLowerCase()) ||
      item.brand.toLowerCase().includes(search.toLowerCase()) ||
      item.size.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesCondition =
      selectedCondition === "All" || item.condition === selectedCondition;

    return matchesSearch && matchesBranch && matchesCondition;
  });

  const criticalCount = data.filter((d) => d.condition === "Critical").length;
  const wornCount = data.filter((d) => d.condition === "Worn").length;
  const goodCount = data.filter((d) => d.condition === "Good" || d.condition === "New").length;

  // ===== NAVIGATION HANDLER =====
  const goToTyreDetail = (tyreId: string) => {
    router.push(`/fleet/tyres/${tyreId}`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading tyres...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedCondition !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedCondition("All");
  };

  return (
    <div className="mx-auto max-w-[1400px] space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <div className="relative">
              <div className="absolute -top-1 -right-1">
                <Sparkles className="h-4 w-4 text-amber animate-pulse" />
              </div>
              <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                Tyres
              </h1>
            </div>
            <span className="rounded-full bg-primary-bg px-2.5 py-0.5 text-[11px] font-semibold text-primary">
              {data.length}
            </span>
            <div className="flex items-center gap-1 rounded-full bg-red-bg px-2.5 py-0.5 text-[10px] font-semibold text-red">
              <AlertCircle className="h-3 w-3" />
              {criticalCount} critical
            </div>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-600">
            Tyre identity, position, condition, and lifecycle history.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Plus className="h-3.5 w-3.5" />
            Add Tyre
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-3">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5 group-hover:scale-110 transition-transform">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Good / New</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{goodCount}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+2 this month</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5 group-hover:scale-110 transition-transform">
              <AlertTriangle className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Worn</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{wornCount}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-amber">
            <Clock className="h-3 w-3" />
            <span>Replace soon</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-red-bg p-1.5 group-hover:scale-110 transition-transform">
              <XCircle className="h-4 w-4 text-red" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Critical</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{criticalCount}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-red">
            <AlertCircle className="h-3 w-3" />
            <span>Immediate action</span>
          </div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[240px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, asset, or brand..."
            className="w-full bg-transparent text-[13px] text-ink-950 outline-none placeholder:text-ink-400 py-2"
          />
        </div>

        <button
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[12px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <MapPin className="h-4 w-4 text-ink-400" />
          {selectedBranch}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-[12px] font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedBranch !== "All branches" ? 1 : 0) +
                (selectedCondition !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-[12px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Condition</div>
              <div className="flex gap-1.5">
                {CONDITION_FILTERS.map((condition) => (
                  <button
                    key={condition}
                    onClick={() => setSelectedCondition(condition)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedCondition === condition
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {condition}
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

      {/* ===== TYRES CARDS WITH NAVIGATION ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Circle className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No tyres found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            No tyres match your current filters. Add a new tyre to get started.
          </p>
          <button className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Add Tyre
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredData.map((item) => {
            const conditionStyle = CONDITION_STYLES[item.condition] || CONDITION_STYLES.Good;
            const isCritical = item.condition === "Critical";
            const isWorn = item.condition === "Worn";

            return (
              <div
                key={item.id}
                onClick={() => goToTyreDetail(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  isCritical
                    ? "border-red/30 bg-red-bg/10 hover:border-red/50"
                    : isWorn
                    ? "border-amber/30 bg-amber-bg/10 hover:border-amber/50"
                    : "border-line bg-white hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  isCritical ? "bg-red-500/5" : isWorn ? "bg-amber-500/5" : "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-xl ${
                        isCritical
                          ? "bg-red-bg text-red"
                          : isWorn
                          ? "bg-amber-bg text-amber"
                          : "bg-green-bg text-green"
                      }`}
                    >
                      <Circle className="h-4 w-4" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.id}
                        </span>
                        <span className="font-mono text-[11px] text-ink-400">
                          {item.assetId}
                        </span>
                      </div>
                      <div className="text-[12px] text-ink-500">{item.assetName}</div>
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${conditionStyle.bg} ${conditionStyle.text}`}
                  >
                    <span className={`h-1.5 w-1.5 rounded-full ${conditionStyle.dot}`} />
                    {item.condition}
                  </span>
                </div>

                <div className="relative z-10 mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <span className={`h-2.5 w-2.5 rounded-full ${POSITION_COLORS[item.position]}`} />
                    <span className="font-mono">{item.position}</span>
                    <span className="text-ink-400">({POSITION_LABELS[item.position]})</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <Gauge className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.mileage}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <span className="font-semibold">{item.brand}</span>
                    <span className="text-ink-400">·</span>
                    <span>{item.size}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                </div>

                <div className="relative z-10 mt-2 flex flex-wrap items-center gap-2 text-[11px] text-ink-500">
                  <span className="flex items-center gap-1">
                    <Calendar className="h-3.5 w-3.5" />
                    Installed: {item.installedDate}
                  </span>
                  <span className="text-ink-300">·</span>
                  <span className="flex items-center gap-1">
                    <RefreshCw className="h-3.5 w-3.5" />
                    Rotated: {item.lastRotation}
                  </span>
                </div>

                <div className="relative z-10 mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span className={`text-[11px] font-medium ${
                    isCritical ? "text-red" : isWorn ? "text-amber" : "text-ink-500"
                  }`}>
                    {isCritical ? "⚠ Replace immediately" : isWorn ? "⚠ Replace soon" : "✓ Good condition"}
                  </span>
                  <div className="flex items-center gap-1 text-[11px] font-medium text-ink-400 group-hover:text-primary transition-colors">
                    View <ArrowRight className="h-3.5 w-3.5 group-hover:translate-x-0.5 transition-transform" />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}