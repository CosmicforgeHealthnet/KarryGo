// src/app/(dashboard)/workforce/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Search,
  Plus,
  Users,
  SlidersHorizontal,
  X,
  ChevronDown,
  MapPin,
  Clock,
  CheckCircle2,
  XCircle,
  AlertCircle,
  ArrowRight,
  User,
  Briefcase,
  Shield,
  GraduationCap,
  Activity,
  CalendarDays,
} from "lucide-react";

type WorkforcePerson = {
  id: string;
  initials: string;
  name: string;
  relationship: "Employee" | "Provider" | "Contractor";
  role: string;
  branch: string;
  status: "Active" | "On Leave" | "Suspended" | "Inactive";
  currentShift: string | null;
  eligibility: "Eligible" | "Warning" | "Blocked";
  qualifications: number;
  training: number;
  joinedAt: string;
  lastActive: string;
};

const PEOPLE: WorkforcePerson[] = [
  {
    id: "P-044",
    initials: "PO",
    name: "Peter Okafor",
    relationship: "Provider",
    role: "Dispatch Rider",
    branch: "Ikeja",
    status: "Active",
    currentShift: "06:00 - 10:00",
    eligibility: "Eligible",
    qualifications: 3,
    training: 2,
    joinedAt: "Jan 2024",
    lastActive: "12 sec ago",
  },
  {
    id: "P-038",
    initials: "MB",
    name: "Musa Bello",
    relationship: "Provider",
    role: "Hauling Driver",
    branch: "Apapa",
    status: "Active",
    currentShift: "12:00 - 18:00",
    eligibility: "Eligible",
    qualifications: 4,
    training: 3,
    joinedAt: "Mar 2024",
    lastActive: "45 sec ago",
  },
  {
    id: "P-052",
    initials: "AY",
    name: "Amina Yusuf",
    relationship: "Employee",
    role: "Dispatch Supervisor",
    branch: "Yaba",
    status: "On Leave",
    currentShift: null,
    eligibility: "Warning",
    qualifications: 5,
    training: 4,
    joinedAt: "Feb 2024",
    lastActive: "2 min ago",
  },
  {
    id: "P-061",
    initials: "CU",
    name: "Chinedu Uche",
    relationship: "Provider",
    role: "Dispatch Rider",
    branch: "Lekki",
    status: "Active",
    currentShift: "16:00 - 20:00",
    eligibility: "Eligible",
    qualifications: 2,
    training: 1,
    joinedAt: "Jan 2024",
    lastActive: "4 min ago",
  },
  {
    id: "P-029",
    initials: "KA",
    name: "Kabiru Ahmed",
    relationship: "Provider",
    role: "Hauling Driver",
    branch: "Lekki",
    status: "Suspended",
    currentShift: null,
    eligibility: "Blocked",
    qualifications: 3,
    training: 2,
    joinedAt: "Jan 2024",
    lastActive: "1 hr ago",
  },
  {
    id: "P-073",
    initials: "EO",
    name: "Esther Obi",
    relationship: "Employee",
    role: "Fleet Manager",
    branch: "Ikeja",
    status: "Active",
    currentShift: "08:00 - 17:00",
    eligibility: "Eligible",
    qualifications: 6,
    training: 5,
    joinedAt: "Jan 2024",
    lastActive: "3 min ago",
  },
  {
    id: "P-088",
    initials: "TF",
    name: "Tunde Fatoki",
    relationship: "Contractor",
    role: "Maintenance Tech",
    branch: "Ikeja",
    status: "Active",
    currentShift: "10:00 - 18:00",
    eligibility: "Eligible",
    qualifications: 4,
    training: 2,
    joinedAt: "Jun 2024",
    lastActive: "8 min ago",
  },
  {
    id: "P-091",
    initials: "NA",
    name: "Ngozi Akpan",
    relationship: "Employee",
    role: "Warehouse Supervisor",
    branch: "Apapa",
    status: "Inactive",
    currentShift: null,
    eligibility: "Warning",
    qualifications: 3,
    training: 3,
    joinedAt: "Feb 2024",
    lastActive: "2 days ago",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  "On Leave": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Suspended: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <XCircle className="h-3.5 w-3.5" /> },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const ELIGIBILITY_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Eligible: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Warning: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Blocked: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const RELATIONSHIP_STYLES: Record<string, { bg: string; text: string }> = {
  Employee: { bg: "bg-primary-bg", text: "text-primary" },
  Provider: { bg: "bg-green-bg", text: "text-green" },
  Contractor: { bg: "bg-amber-bg", text: "text-amber" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATUS_FILTERS = ["All", "Active", "On Leave", "Suspended", "Inactive"];
const RELATIONSHIP_FILTERS = ["All", "Employee", "Provider", "Contractor"];

export default function WorkforceDirectoryPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedRelationship, setSelectedRelationship] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<WorkforcePerson[]>([]);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PEOPLE);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.id.toLowerCase().includes(search.toLowerCase()) ||
      item.role.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesRelationship = selectedRelationship === "All" || item.relationship === selectedRelationship;

    return matchesSearch && matchesBranch && matchesStatus && matchesRelationship;
  });

  const stats = {
    total: data.length,
    active: data.filter((d) => d.status === "Active").length,
    onLeave: data.filter((d) => d.status === "On Leave").length,
    suspended: data.filter((d) => d.status === "Suspended").length,
    eligible: data.filter((d) => d.eligibility === "Eligible").length,
  };

  const hasActiveFilters =
    selectedBranch !== "All branches" ||
    selectedStatus !== "All" ||
    selectedRelationship !== "All" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedStatus("All");
    setSelectedRelationship("All");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading workforce...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Workforce Directory</h1>
          <p className="mt-1.5">
            Staff, providers, and contractors with roles, availability, and eligibility status.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover">
            <Plus className="h-3.5 w-3.5" />
            Add Person
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Users className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Total</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{stats.total}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{stats.active}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">On Leave</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{stats.onLeave}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Shield className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Eligible</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{stats.eligible}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, ID, or role..."
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
          <span className="text-ink-400">Relationship:</span>
          {selectedRelationship}
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
                (selectedRelationship !== "All" ? 1 : 0)}
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
          {filteredData.length} people
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
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Relationship</span>
              <div className="flex flex-wrap gap-1.5">
                {RELATIONSHIP_FILTERS.map((rel) => (
                  <button
                    key={rel}
                    onClick={() => setSelectedRelationship(rel)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedRelationship === rel
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {rel}
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

      {/* ── Person Cards ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No people found</h3>
          <p className="mt-1 max-w-sm text-center text-sm text-ink-500">
            No people match your current filters. Add a new person to get started.
          </p>
          <button className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
            <Plus className="inline h-3.5 w-3.5 mr-1" />
            Add Person
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
            const eligibilityStyle = ELIGIBILITY_STYLES[item.eligibility] || ELIGIBILITY_STYLES.Eligible;
            const relationshipStyle = RELATIONSHIP_STYLES[item.relationship] || RELATIONSHIP_STYLES.Employee;

            return (
              <div
                key={item.id}
                onClick={() => router.push(`/workforce/${item.id}`)}
                className={`group rounded-lg border p-4 transition-colors cursor-pointer ${
                  item.status === "Active"
                    ? "border-green/20 bg-white hover:border-green/40 hover:bg-surface-2"
                    : item.status === "On Leave"
                    ? "border-amber/20 bg-amber-bg/10 hover:border-amber/40 hover:bg-amber-bg/20"
                    : item.status === "Suspended"
                    ? "border-red/20 bg-red-bg/10 hover:border-red/40 hover:bg-red-bg/20"
                    : "border-line bg-white hover:border-ink-300 hover:bg-surface-2"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full text-base font-bold text-white ${
                        item.status === "Active" ? "bg-primary" :
                        item.status === "On Leave" ? "bg-amber" :
                        item.status === "Suspended" ? "bg-red" :
                        "bg-ink-400"
                      }`}
                    >
                      {item.initials}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">{item.name}</span>
                        <span className="font-mono text-[10px] text-ink-400">{item.id}</span>
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                        >
                          {statusStyle.icon}
                          {item.status}
                        </span>
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${eligibilityStyle.bg} ${eligibilityStyle.text}`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${eligibilityStyle.dot}`} />
                          {item.eligibility}
                        </span>
                        <span
                          className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${relationshipStyle.bg} ${relationshipStyle.text}`}
                        >
                          {item.relationship}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <Briefcase className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{item.role}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-xs text-ink-500">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.branch}</span>
                  </div>
                  {item.currentShift ? (
                    <div className="flex items-center gap-1.5 text-xs text-ink-500 col-span-2">
                      <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                      <span>Shift: {item.currentShift}</span>
                    </div>
                  ) : (
                    <div className="flex items-center gap-1.5 text-xs text-ink-400 col-span-2">
                      <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                      <span>No shift assigned</span>
                    </div>
                  )}
                </div>

                <div className="mt-3 flex items-center gap-3 border-t border-line pt-2.5">
                  <div className="flex items-center gap-1 text-[10px] text-ink-500">
                    <GraduationCap className="h-3.5 w-3.5" />
                    <span>{item.qualifications} quals</span>
                  </div>
                  <div className="flex items-center gap-1 text-[10px] text-ink-500">
                    <Activity className="h-3.5 w-3.5" />
                    <span>{item.training} trainings</span>
                  </div>
                  <div className="flex items-center gap-1 text-[10px] text-ink-400 ml-auto">
                    <CalendarDays className="h-3.5 w-3.5" />
                    <span>{item.joinedAt}</span>
                  </div>
                </div>

                <div className="mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    Last active: {item.lastActive}
                  </div>
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