// src/app/(dashboard)/fleet/inspections/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import {
  Search,
  Plus,
  ClipboardCheck,
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
  MoreVertical,
  Filter,
  ArrowRight,
  Calendar,
  Wrench,
  Shield,
  User,
  Truck,
  FileText,
  Award,
  TrendingUp,
  TrendingDown,
  Sparkles,
} from "lucide-react";

type Inspection = {
  id: string;
  assetId: string;
  assetName: string;
  template: string;
  inspector: string;
  branch: string;
  date: string;
  result: "Passed" | "Failed" | "Pending" | "Scheduled";
  criticalIssues: number;
  nextDue: string;
};

const INSPECTIONS: Inspection[] = [
  {
    id: "INSP-001",
    assetId: "BR-204",
    assetName: "Honda ACE 110",
    template: "Motorcycle Safety Check",
    inspector: "T. Adeyemi",
    branch: "Ikeja",
    date: "Today",
    result: "Failed",
    criticalIssues: 3,
    nextDue: "Immediate",
  },
  {
    id: "INSP-002",
    assetId: "VN-041",
    assetName: "Mercedes Sprinter",
    template: "Van Comprehensive",
    inspector: "F. Daniel",
    branch: "Lekki",
    date: "2 days ago",
    result: "Passed",
    criticalIssues: 0,
    nextDue: "In 90 days",
  },
  {
    id: "INSP-003",
    assetId: "TR-112",
    assetName: "Scania R500",
    template: "Truck Safety Inspection",
    inspector: "M. Bello",
    branch: "Apapa",
    date: "1 week ago",
    result: "Passed",
    criticalIssues: 0,
    nextDue: "In 60 days",
  },
  {
    id: "INSP-004",
    assetId: "BR-188",
    assetName: "Yamaha FZ",
    template: "Motorcycle Safety Check",
    inspector: "A. Yusuf",
    branch: "Yaba",
    date: "3 days ago",
    result: "Passed",
    criticalIssues: 0,
    nextDue: "In 90 days",
  },
  {
    id: "INSP-005",
    assetId: "VN-029",
    assetName: "Ford Transit",
    template: "Van Comprehensive",
    inspector: "K. Ahmed",
    branch: "Ikeja",
    date: "5 days ago",
    result: "Failed",
    criticalIssues: 2,
    nextDue: "Immediate",
  },
  {
    id: "INSP-006",
    assetId: "BR-171",
    assetName: "Honda CB150X",
    template: "Motorcycle Safety Check",
    inspector: "C. Uche",
    branch: "Lekki",
    date: "1 day ago",
    result: "Passed",
    criticalIssues: 0,
    nextDue: "In 90 days",
  },
];

const RESULT_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Passed: {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Scheduled: {
    bg: "bg-blue-bg",
    text: "text-blue",
    icon: <Calendar className="h-3.5 w-3.5" />,
  },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const RESULT_FILTERS = ["All", "Passed", "Failed", "Pending", "Scheduled"];

export default function InspectionsPage() {
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedResult, setSelectedResult] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Inspection[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(INSPECTIONS);
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
      item.inspector.toLowerCase().includes(search.toLowerCase());

    const matchesBranch =
      selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesResult = selectedResult === "All" || item.result === selectedResult;

    return matchesSearch && matchesBranch && matchesResult;
  });

  const failedCount = data.filter((d) => d.result === "Failed").length;
  const passedCount = data.filter((d) => d.result === "Passed").length;
  const pendingCount = data.filter((d) => d.result === "Pending" || d.result === "Scheduled").length;

  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading inspections...</span>
        </div>
      </div>
    );
  }

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedResult !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedResult("All");
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
                Inspections
              </h1>
            </div>
            <span className="rounded-full bg-primary-bg px-2.5 py-0.5 text-[11px] font-semibold text-primary">
              {data.length}
            </span>
            <div className="flex items-center gap-1 rounded-full bg-red-bg px-2.5 py-0.5 text-[10px] font-semibold text-red">
              <AlertCircle className="h-3 w-3" />
              {failedCount} failed
            </div>
          </div>
          <p className="mt-1 text-[13.5px] text-ink-600">
            Asset inspections, results, defects, and generated work orders.
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
          <Link
            href="/fleet/inspections/templates"
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <FileText className="h-3.5 w-3.5" />
            Templates
          </Link>
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Plus className="h-3.5 w-3.5" />
            New Inspection
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-green-bg p-1.5 group-hover:scale-110 transition-transform">
              <CheckCircle2 className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Passed</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{passedCount}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+2 this week</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-red-bg p-1.5 group-hover:scale-110 transition-transform">
              <XCircle className="h-4 w-4 text-red" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Failed</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{failedCount}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-red">
            <AlertCircle className="h-3 w-3" />
            <span>Requires action</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-amber-bg p-1.5 group-hover:scale-110 transition-transform">
              <Clock className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Pending</div>
              <div className="font-display text-[18px] font-bold text-ink-950">{pendingCount}</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-amber">
            <Calendar className="h-3 w-3" />
            <span>Scheduled</span>
          </div>
        </div>
        <div className="group rounded-xl border border-line bg-white p-3.5 shadow-sm transition hover:shadow-md hover:-translate-y-0.5 cursor-default">
          <div className="flex items-center gap-2.5">
            <div className="rounded-full bg-primary-bg p-1.5 group-hover:scale-110 transition-transform">
              <Award className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Compliance Rate</div>
              <div className="font-display text-[18px] font-bold text-ink-950">88%</div>
            </div>
          </div>
          <div className="mt-1 flex items-center gap-1 text-[10px] text-green">
            <TrendingUp className="h-3 w-3" />
            <span>+5% vs last month</span>
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
            placeholder="Search by ID, asset, or inspector..."
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
                (selectedResult !== "All" ? 1 : 0)}
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Result</div>
              <div className="flex gap-1.5">
                {RESULT_FILTERS.map((result) => (
                  <button
                    key={result}
                    onClick={() => setSelectedResult(result)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedResult === result
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {result}
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

      {/* ===== INSPECTION GRID ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <ClipboardCheck className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No inspections found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            No inspections match your current filters. Create a new inspection to get started.
          </p>
          <button className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            New Inspection
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredData.map((item) => {
            const resultStyle = RESULT_STYLES[item.result] || RESULT_STYLES.Pending;
            const isFailed = item.result === "Failed";
            const isPassed = item.result === "Passed";

            return (
              <Link
                key={item.id}
                href={`/fleet/inspections/${item.id}`}
                className={`group relative rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  isFailed
                    ? "border-red/30 bg-red-bg/10 hover:border-red/50"
                    : isPassed
                    ? "border-green/20 bg-white hover:border-green/40"
                    : "border-amber/20 bg-white/70 hover:border-amber/40"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  isFailed ? "bg-red-500/5" : isPassed ? "bg-green-500/5" : "bg-amber-500/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-2.5">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-xl ${
                        isFailed
                          ? "bg-red-bg text-red"
                          : isPassed
                          ? "bg-green-bg text-green"
                          : "bg-amber-bg text-amber"
                      }`}
                    >
                      {resultStyle.icon}
                    </div>
                    <div>
                      <div className="flex items-center gap-1.5">
                        <span className="text-[14px] font-semibold text-ink-950">
                          {item.id}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {item.assetId}
                        </span>
                      </div>
                      <div className="text-[12px] text-ink-500">{item.assetName}</div>
                    </div>
                  </div>
                  <div className="flex flex-col items-end gap-0.5">
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${resultStyle.bg} ${resultStyle.text}`}
                    >
                      {resultStyle.icon}
                      {item.result}
                    </span>
                    {item.criticalIssues > 0 && (
                      <span className="rounded-full bg-red-bg px-2 py-0.5 text-[9px] font-semibold text-red">
                        {item.criticalIssues} critical
                      </span>
                    )}
                  </div>
                </div>

                <div className="relative z-10 mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-500">
                    <User className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{item.inspector}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-500 col-span-2">
                    <Calendar className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.date}</span>
                    <span className="text-ink-300">·</span>
                    <span className="text-ink-400">Next: {item.nextDue}</span>
                  </div>
                </div>

                <div className="relative z-10 mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="text-[10px] text-ink-400 truncate max-w-[180px]">
                    {item.template}
                  </div>
                  <div className="flex items-center gap-1 text-[11px] font-medium text-ink-400 group-hover:text-primary transition-colors">
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