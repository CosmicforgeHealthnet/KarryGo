// src/app/(dashboard)/compliance/incidents/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  Clock,
  AlertCircle,
  AlertTriangle,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  AlertOctagon,
} from "lucide-react";

// ==================== TYPES ====================

type Incident = {
  id: string;
  reference: string;
  type: "Accident" | "NearMiss" | "PropertyDamage" | "SafetyIncident" | "SecurityIncident" | "VehicleIncident" | "Other";
  severity: "Critical" | "High" | "Medium" | "Low";
  title: string;
  description: string;
  location: string;
  branch: string;
  affectedPeople: string[];
  affectedAssets: string[];
  affectedTrips: string[];
  status: "Open" | "Investigating" | "UnderReview" | "Resolved" | "Closed";
  owner: string | null;
  priority: "Urgent" | "High" | "Normal" | "Low";
  createdAt: string;
  updatedAt: string;
  resolvedAt: string | null;
  resolvedBy: string | null;
  evidenceCount: number;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_INCIDENTS: Incident[] = [
  {
    id: "INC-771",
    reference: "INC-2026-771",
    type: "VehicleIncident",
    severity: "High",
    title: "Road incident - Provider P-044",
    description: "Provider P-044 involved in a collision with another vehicle on Opebi Road. No injuries reported but vehicle BR-204 sustained damage.",
    location: "Opebi Road, Ikeja",
    branch: "Ikeja",
    affectedPeople: ["P-044 - Peter Okafor"],
    affectedAssets: ["BR-204 - Honda ACE 110"],
    affectedTrips: ["KG-8479"],
    status: "Investigating",
    owner: "F. Daniel",
    priority: "High",
    createdAt: "2026-08-22T06:30:00Z",
    updatedAt: "2026-08-22T09:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    evidenceCount: 4,
    tags: ["Collision", "Vehicle", "Provider"],
  },
  {
    id: "INC-772",
    reference: "INC-2026-772",
    type: "SafetyIncident",
    severity: "Critical",
    title: "Slippery floor hazard - Injury reported",
    description: "Staff member slipped on oil spill at Ikeja depot. Minor injury reported. Area now cordoned off.",
    location: "Ikeja Depot - Fuel Bay",
    branch: "Ikeja",
    affectedPeople: ["S. Ogunlade - Staff"],
    affectedAssets: [],
    affectedTrips: [],
    status: "UnderReview",
    owner: "F. Daniel",
    priority: "Urgent",
    createdAt: "2026-08-21T14:30:00Z",
    updatedAt: "2026-08-22T08:30:00Z",
    resolvedAt: null,
    resolvedBy: null,
    evidenceCount: 2,
    tags: ["SlipHazard", "Injury", "Staff"],
  },
  {
    id: "INC-773",
    reference: "INC-2026-773",
    type: "PropertyDamage",
    severity: "Medium",
    title: "Warehouse shelving collapse",
    description: "Section of shelving in Yaba warehouse collapsed. No injuries. Inventory damaged.",
    location: "Yaba Warehouse - Zone B",
    branch: "Yaba",
    affectedPeople: [],
    affectedAssets: [],
    affectedTrips: [],
    status: "Resolved",
    owner: "O. Adebayo",
    priority: "Normal",
    createdAt: "2026-08-20T11:15:00Z",
    updatedAt: "2026-08-21T16:00:00Z",
    resolvedAt: "2026-08-21T16:00:00Z",
    resolvedBy: "O. Adebayo",
    evidenceCount: 3,
    tags: ["Warehouse", "Shelving", "Inventory"],
  },
  {
    id: "INC-774",
    reference: "INC-2026-774",
    type: "SecurityIncident",
    severity: "High",
    title: "Unauthorized access - Apapa depot",
    description: "Security breach at Apapa depot south gate. Unauthorized person gained entry. CCTV footage being reviewed.",
    location: "Apapa Depot - South Gate",
    branch: "Apapa",
    affectedPeople: [],
    affectedAssets: [],
    affectedTrips: [],
    status: "Investigating",
    owner: "F. Daniel",
    priority: "High",
    createdAt: "2026-08-22T00:30:00Z",
    updatedAt: "2026-08-22T08:00:00Z",
    resolvedAt: null,
    resolvedBy: null,
    evidenceCount: 5,
    tags: ["Security", "Breach", "Depot"],
  },
  {
    id: "INC-775",
    reference: "INC-2026-775",
    type: "NearMiss",
    severity: "Medium",
    title: "Near miss - Warehouse forklift",
    description: "Forklift nearly collided with pedestrian in Yaba warehouse aisle 4. No injury but safety concern raised.",
    location: "Yaba Warehouse - Aisle 4",
    branch: "Yaba",
    affectedPeople: ["J. Adekunle - Staff"],
    affectedAssets: ["FL-03 - Forklift"],
    affectedTrips: ["WH-2026-884"],
    status: "Closed",
    owner: "O. Adebayo",
    priority: "Normal",
    createdAt: "2026-08-17T11:15:00Z",
    updatedAt: "2026-08-19T13:00:00Z",
    resolvedAt: "2026-08-19T13:00:00Z",
    resolvedBy: "O. Adebayo",
    evidenceCount: 2,
    tags: ["Forklift", "Warehouse", "NearMiss"],
  },
  {
    id: "INC-776",
    reference: "INC-2026-776",
    type: "VehicleIncident",
    severity: "Low",
    title: "Minor vehicle scrape - Van VN-029",
    description: "Van VN-029 scraped against loading bay wall. Minor cosmetic damage. No operational impact.",
    location: "Lekki Depot - Loading Bay",
    branch: "Lekki",
    affectedPeople: ["M. Bello"],
    affectedAssets: ["VN-029 - Toyota Hiace"],
    affectedTrips: ["KG-8504"],
    status: "Closed",
    owner: "M. Bello",
    priority: "Low",
    createdAt: "2026-08-19T09:30:00Z",
    updatedAt: "2026-08-20T14:00:00Z",
    resolvedAt: "2026-08-20T14:00:00Z",
    resolvedBy: "M. Bello",
    evidenceCount: 1,
    tags: ["Vehicle", "Damage", "Minor"],
  },
  {
    id: "INC-777",
    reference: "INC-2026-777",
    type: "SafetyIncident",
    severity: "Critical",
    title: "Unsafe vehicle - TR-112",
    description: "Pre-trip inspection revealed brake issues on TR-112. Vehicle placed out of service.",
    location: "Apapa Depot",
    branch: "Apapa",
    affectedPeople: ["K. Ahmed"],
    affectedAssets: ["TR-112 - Mercedes Actros"],
    affectedTrips: ["KG-8512"],
    status: "Closed",
    owner: "M. Bello",
    priority: "Urgent",
    createdAt: "2026-08-19T16:20:00Z",
    updatedAt: "2026-08-21T11:00:00Z",
    resolvedAt: "2026-08-21T11:00:00Z",
    resolvedBy: "M. Bello",
    evidenceCount: 3,
    tags: ["Vehicle", "Brakes", "Maintenance"],
  },
];

const TYPE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Accident: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  NearMiss: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  PropertyDamage: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  SafetyIncident: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  SecurityIncident: { bg: "bg-purple-bg", text: "text-purple", icon: <Shield className="h-3.5 w-3.5" /> },
  VehicleIncident: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Other: { bg: "bg-surface-2", text: "text-ink-500", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Critical: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertOctagon className="h-3.5 w-3.5" />,
  },
  High: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
  },
  Medium: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Low: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Open: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Investigating: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  UnderReview: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Resolved: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string }> = {
  Urgent: { bg: "bg-red-bg", text: "text-red" },
  High: { bg: "bg-amber-bg", text: "text-amber" },
  Normal: { bg: "bg-blue-bg", text: "text-blue" },
  Low: { bg: "bg-surface-2", text: "text-ink-500" },
};

const TYPE_FILTERS = ["All", "Accident", "NearMiss", "PropertyDamage", "SafetyIncident", "SecurityIncident", "VehicleIncident", "Other"];
const SEVERITY_FILTERS = ["All", "Critical", "High", "Medium", "Low"];
const STATUS_FILTERS = ["All", "Open", "Investigating", "UnderReview", "Resolved", "Closed"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function IncidentsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Incident[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedType, setSelectedType] = useState("All");
  const [selectedSeverity, setSelectedSeverity] = useState("All");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_INCIDENTS);
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
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.title.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.location.toLowerCase().includes(search.toLowerCase()) ||
      item.owner?.toLowerCase().includes(search.toLowerCase());

    const matchesType = selectedType === "All" || item.type === selectedType;
    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesType && matchesSeverity && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading incidents...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedType !== "All" ||
    selectedSeverity !== "All" ||
    selectedStatus !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedType("All");
    setSelectedSeverity("All");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedType !== "All" ? 1 : 0) +
    (selectedSeverity !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

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

  // Stats
  const totalOpen = data.filter((d) => d.status === "Open" || d.status === "Investigating").length;
  const totalCritical = data.filter((d) => d.severity === "Critical").length;
  const totalResolved = data.filter((d) => d.status === "Resolved" || d.status === "Closed").length;
  const totalUrgent = data.filter((d) => d.priority === "Urgent").length;

  const goToIncidentDetail = (id: string) => {
    router.push(`/compliance/incidents/${id}`);
  };

  const goToCreateIncident = () => {
    router.push("/compliance/incidents/report");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Incidents
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Incident type, severity, affected people/assets/trips, state and owner.
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
          <button
            onClick={goToCreateIncident}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Report Incident
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Open</div>
          <div className="text-lg font-bold text-red">{totalOpen}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Critical</div>
          <div className="text-lg font-bold text-red">{totalCritical}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Resolved</div>
          <div className="text-lg font-bold text-green">{totalResolved}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, title, owner..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const types = ["All", "Accident", "NearMiss", "PropertyDamage", "SafetyIncident", "SecurityIncident", "VehicleIncident", "Other"];
            const currentIndex = types.indexOf(selectedType);
            const nextIndex = (currentIndex + 1) % types.length;
            setSelectedType(types[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertCircle className="h-4 w-4 text-ink-400" />
          {selectedType}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const severities = ["All", "Critical", "High", "Medium", "Low"];
            const currentIndex = severities.indexOf(selectedSeverity);
            const nextIndex = (currentIndex + 1) % severities.length;
            setSelectedSeverity(severities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedSeverity}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Open", "Investigating", "UnderReview", "Resolved", "Closed"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertCircle className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
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

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
              <div className="flex gap-1.5 flex-wrap">
                {BRANCHES.map((branch) => (
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

      {/* ===== INCIDENTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <AlertCircle className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No incidents found</h3>
          <p className="mt-1 text-sm text-ink-500">No incidents match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[900px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Incident</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Severity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const typeStyle = TYPE_STYLES[item.type] || TYPE_STYLES.Other;
                  const severityStyle = SEVERITY_STYLES[item.severity] || SEVERITY_STYLES.Medium;
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Open;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToIncidentDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.title}</div>
                        <div className="font-mono text-[10px] text-ink-500 mt-0.5">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                          {typeStyle.icon}
                          {item.type}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                          {severityStyle.icon}
                          {item.severity}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          CMP-08 · Incidents
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} incidents shown
        </span>
      </div>
    </div>
  );
}