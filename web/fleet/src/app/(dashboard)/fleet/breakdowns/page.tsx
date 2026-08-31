// src/app/(dashboard)/fleet/breakdowns/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  Wrench,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Calendar,
  User,
  Truck,
  FileText,
  TrendingUp,
  Circle,
  AlertTriangle,
  Flame,
  Zap,
} from "lucide-react";

type Breakdown = {
  id: string;
  assetId: string;
  assetName: string;
  assetType: string;
  severity: "Critical" | "High" | "Medium" | "Low";
  status: "Open" | "In Progress" | "Resolved" | "Closed";
  type: "Mechanical" | "Electrical" | "Accident" | "Tyre" | "Other";
  description: string;
  location: string;
  branch: string;
  reportedBy: string;
  assignedTo: string | null;
  reportedAt: string;
  resolvedAt: string | null;
  downtime: string;
  towingRequired: boolean;
};

const BREAKDOWNS: Breakdown[] = [
  {
    id: "BRK-001",
    assetId: "BR-204",
    assetName: "Honda ACE 110",
    assetType: "Motorcycle",
    severity: "Critical",
    status: "In Progress",
    type: "Mechanical",
    description: "Front brake failure - complete brake system failure",
    location: "Ikeja-Lekki Expressway",
    branch: "Ikeja",
    reportedBy: "Peter Okafor",
    assignedTo: "T. Adeyemi",
    reportedAt: "2026-08-17T14:10:00Z",
    resolvedAt: null,
    downtime: "2 days",
    towingRequired: true,
  },
  {
    id: "BRK-002",
    assetId: "TR-112",
    assetName: "Scania R500",
    assetType: "Truck",
    severity: "High",
    status: "Open",
    type: "Tyre",
    description: "Rear tyre blowout - emergency",
    location: "Apapa Port",
    branch: "Apapa",
    reportedBy: "Kabiru Ahmed",
    assignedTo: null,
    reportedAt: "2026-08-17T11:30:00Z",
    resolvedAt: null,
    downtime: "1 day",
    towingRequired: false,
  },
  {
    id: "BRK-003",
    assetId: "VN-041",
    assetName: "Mercedes Sprinter",
    assetType: "Van",
    severity: "Medium",
    status: "In Progress",
    type: "Electrical",
    description: "Alternator failure - battery not charging",
    location: "Lekki Phase 1",
    branch: "Lekki",
    reportedBy: "Musa Bello",
    assignedTo: "M. Bello",
    reportedAt: "2026-08-16T16:45:00Z",
    resolvedAt: null,
    downtime: "2 days",
    towingRequired: true,
  },
  {
    id: "BRK-004",
    assetId: "BR-188",
    assetName: "Yamaha FZ",
    assetType: "Motorcycle",
    severity: "Low",
    status: "Resolved",
    type: "Mechanical",
    description: "Chain snapped - replacement completed",
    location: "Yaba",
    branch: "Yaba",
    reportedBy: "Amina Yusuf",
    assignedTo: "A. Yusuf",
    reportedAt: "2026-08-15T09:20:00Z",
    resolvedAt: "2026-08-15T13:45:00Z",
    downtime: "4 hrs",
    towingRequired: false,
  },
  {
    id: "BRK-005",
    assetId: "VN-029",
    assetName: "Ford Transit",
    assetType: "Van",
    severity: "Critical",
    status: "Open",
    type: "Accident",
    description: "Minor collision - front bumper damage",
    location: "Ikeja",
    branch: "Ikeja",
    reportedBy: "Tunde Adeyemi",
    assignedTo: null,
    reportedAt: "2026-08-14T07:30:00Z",
    resolvedAt: null,
    downtime: "3 days",
    towingRequired: true,
  },
  {
    id: "BRK-006",
    assetId: "TR-098",
    assetName: "MAN TGS",
    assetType: "Truck",
    severity: "High",
    status: "Closed",
    type: "Tyre",
    description: "Tyre replacement - 3 tyres replaced",
    location: "Apapa",
    branch: "Apapa",
    reportedBy: "Kabiru Ahmed",
    assignedTo: "K. Ahmed",
    reportedAt: "2026-08-12T13:15:00Z",
    resolvedAt: "2026-08-13T09:00:00Z",
    downtime: "1 day",
    towingRequired: false,
  },
];

const SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: <Clock className="h-3.5 w-3.5" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const TYPE_ICONS: Record<string, React.ReactNode> = {
  Mechanical: <Wrench className="h-3.5 w-3.5" />,
  Electrical: <Zap className="h-3.5 w-3.5" />,
  Accident: <AlertTriangle className="h-3.5 w-3.5" />,
  Tyre: <Circle className="h-3.5 w-3.5" />,
  Other: <FileText className="h-3.5 w-3.5" />,
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const SEVERITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const STATUS_FILTERS = ["All", "Open", "In Progress", "Resolved", "Closed"];

export default function BreakdownsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Breakdown[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(BREAKDOWNS);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.assetId.toLowerCase().includes(search.toLowerCase()) ||
      item.assetName.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.location.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;

    return matchesSearch && matchesBranch && matchesSeverity && matchesStatus;
  });

  const openCount = data.filter((d) => d.status === "Open" || d.status === "In Progress").length;
  const criticalCount = data.filter((d) => d.severity === "Critical").length;
  const resolvedCount = data.filter((d) => d.status === "Resolved" || d.status === "Closed").length;

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedSeverity !== "All" ||
    selectedStatus !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedSeverity("All");
    setSelectedStatus("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading breakdowns...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Breakdowns</h1>
          <p className="mt-1.5">
            Breakdowns and recovery cases with severity, status, and towing requirements.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover">
            <Plus className="h-3.5 w-3.5" />
            Report Breakdown
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{openCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg/70 text-red">
              <Flame className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Critical</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{criticalCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Resolved</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{resolvedCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, asset, or location..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Severity:</span>
          {selectedSeverity}
          <span className="text-ink-300">·</span>
          <span className="text-ink-400">Status:</span>
          {selectedStatus}
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
                (selectedSeverity !== "All" ? 1 : 0) +
                (selectedStatus !== "All" ? 1 : 0)}
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
          {filteredData.length} breakdowns
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Severity</span>
              <div className="flex flex-wrap gap-1.5">
                {SEVERITY_FILTERS.map((severity) => (
                  <button
                    key={severity}
                    onClick={() => setSelectedSeverity(severity)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedSeverity === severity
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {severity}
                  </button>
                ))}
              </div>
            </div>
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
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Breakdown Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Wrench className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No breakdowns found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No breakdowns match your current filters. Report a new breakdown to get started.
          </p>
          <button className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Report Breakdown
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filteredData.map((item) => {
            const severityStyle = SEVERITY_STYLES[item.severity] || SEVERITY_STYLES.Low;
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Open;
            const TypeIcon = TYPE_ICONS[item.type] || <FileText className="h-3.5 w-3.5" />;
            const isOpen = item.status === "Open" || item.status === "In Progress";
            const isCritical = item.severity === "Critical";

            return (
              <div
                key={item.id}
                onClick={() => router.push(`/fleet/breakdowns/${item.id}`)}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  isCritical
                    ? "border-red/30 bg-red-bg/10 hover:border-red/50 hover:bg-red-bg/20"
                    : isOpen
                    ? "border-amber/30 bg-amber-bg/10 hover:border-amber/50 hover:bg-amber-bg/20"
                    : "border-green/20 bg-white hover:border-green/40 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-10 w-10 items-center justify-center rounded-md ${
                        isCritical
                          ? "bg-red-bg text-red"
                          : isOpen
                          ? "bg-amber-bg text-amber"
                          : "bg-green-bg text-green"
                      }`}
                    >
                      <Wrench className="h-4 w-4" />
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">{item.id}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.assetId}</span>
                      </div>
                      <div className="text-xs text-ink-500">{item.assetName}</div>
                    </div>
                  </div>
                  <div className="flex flex-col items-end gap-0.5">
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}
                    >
                      {severityStyle.icon}
                      {item.severity}
                    </span>
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                    >
                      <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                      {item.status}
                    </span>
                  </div>
                </div>

                <div className="mt-2">
                  <div className="flex items-center gap-2 text-xs text-ink-500">
                    <span className="flex items-center gap-1">{TypeIcon}{item.type}</span>
                    {item.towingRequired && (
                      <span className="flex items-center gap-1 rounded-full bg-red-bg/50 px-2 py-0.5 text-[10px] font-medium text-red">
                        <AlertCircle className="h-3 w-3" />
                        Towing required
                      </span>
                    )}
                  </div>
                  <p className="mt-1 text-sm text-ink-700 line-clamp-2">{item.description}</p>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{item.location}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <User className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{item.reportedBy}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Calendar className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{new Date(item.reportedAt).toLocaleDateString()}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>Downtime: {item.downtime}</span>
                  </div>
                </div>

                {item.assignedTo && (
                  <div className="mt-2 flex items-center gap-1.5 text-xs text-ink-500 border-t border-line pt-2">
                    <User className="h-3.5 w-3.5" />
                    <span>Assigned to: <span className="font-medium text-ink-950">{item.assignedTo}</span></span>
                  </div>
                )}

                <div className="mt-3 flex items-center justify-end border-t border-line pt-2.5">
                  <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
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