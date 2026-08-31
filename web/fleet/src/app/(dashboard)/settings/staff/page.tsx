// src/app/(dashboard)/settings/staff/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
  UserPlus,
  UserCheck,
  Clock,
  UserX,
  Search,
  SlidersHorizontal,
  X,
  Building2,
  Eye,
} from "lucide-react";

type StaffMember = {
  id: string;
  reference: string;
  name: string;
  email: string;
  phone: string;
  role: string;
  branch: string;
  department: string;
  status: "Active" | "Pending" | "Suspended" | "Invited" | "Inactive";
  primaryScope: string;
  invitationState: "Sent" | "Accepted" | "Expired" | "Pending" | "None";
  lastActive: string | null;
  createdAt: string;
  tags: string[];
};

const MOCK_STAFF: StaffMember[] = [
  {
    id: "S-001",
    reference: "STAFF-2026-001",
    name: "Favour Daniel",
    email: "favour@apexlogistics.com",
    phone: "+234 800 000 0000",
    role: "Organization Admin",
    branch: "Ikeja",
    department: "Executive",
    status: "Active",
    primaryScope: "Company-wide",
    invitationState: "Accepted",
    lastActive: "2026-08-23T10:30:00Z",
    createdAt: "2026-06-01T09:00:00Z",
    tags: ["Admin", "Executive"],
  },
  {
    id: "S-002",
    reference: "STAFF-2026-002",
    name: "Oluwaseun Adebayo",
    email: "seun@apexlogistics.com",
    phone: "+234 800 000 0001",
    role: "Operations Manager",
    branch: "Ikeja",
    department: "Operations",
    status: "Active",
    primaryScope: "Ikeja",
    invitationState: "Accepted",
    lastActive: "2026-08-23T09:15:00Z",
    createdAt: "2026-06-15T10:00:00Z",
    tags: ["Operations", "Manager"],
  },
  {
    id: "S-003",
    reference: "STAFF-2026-003",
    name: "Musa Bello",
    email: "musa@apexlogistics.com",
    phone: "+234 800 000 0002",
    role: "Fleet Manager",
    branch: "Lekki",
    department: "Fleet",
    status: "Active",
    primaryScope: "Lekki",
    invitationState: "Accepted",
    lastActive: "2026-08-22T16:45:00Z",
    createdAt: "2026-06-20T14:00:00Z",
    tags: ["Fleet", "Manager"],
  },
  {
    id: "S-004",
    reference: "STAFF-2026-004",
    name: "Amina Yusuf",
    email: "amina@apexlogistics.com",
    phone: "+234 800 000 0003",
    role: "Dispatch Supervisor",
    branch: "Yaba",
    department: "Operations",
    status: "Active",
    primaryScope: "Yaba",
    invitationState: "Accepted",
    lastActive: "2026-08-22T14:30:00Z",
    createdAt: "2026-07-01T11:00:00Z",
    tags: ["Dispatch", "Supervisor"],
  },
  {
    id: "S-005",
    reference: "STAFF-2026-005",
    name: "Chinedu Uche",
    email: "chinedu@apexlogistics.com",
    phone: "+234 800 000 0004",
    role: "Warehouse Manager",
    branch: "Ikeja",
    department: "Warehouse",
    status: "Pending",
    primaryScope: "Ikeja",
    invitationState: "Sent",
    lastActive: null,
    createdAt: "2026-08-22T08:00:00Z",
    tags: ["Warehouse", "Manager"],
  },
  {
    id: "S-006",
    reference: "STAFF-2026-006",
    name: "Esther Obi",
    email: "esther@apexlogistics.com",
    phone: "+234 800 000 0005",
    role: "Compliance Officer",
    branch: "All branches",
    department: "Compliance",
    status: "Active",
    primaryScope: "Company-wide",
    invitationState: "Accepted",
    lastActive: "2026-08-21T11:00:00Z",
    createdAt: "2026-07-15T09:00:00Z",
    tags: ["Compliance", "Officer"],
  },
  {
    id: "S-007",
    reference: "STAFF-2026-007",
    name: "Kabiru Ahmed",
    email: "kabiru@apexlogistics.com",
    phone: "+234 800 000 0006",
    role: "Hauling Supervisor",
    branch: "Apapa",
    department: "Operations",
    status: "Suspended",
    primaryScope: "Apapa",
    invitationState: "Accepted",
    lastActive: "2026-08-20T08:00:00Z",
    createdAt: "2026-07-20T13:00:00Z",
    tags: ["Hauling", "Supervisor"],
  },
];

type StatusStyle = {
  bg: string;
  text: string;
  dot: string;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Suspended: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Invited: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Inactive: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

type InvitationStyle = {
  bg: string;
  text: string;
};

const INVITATION_STYLES: Record<string, InvitationStyle> = {
  Sent: { bg: "bg-primary-bg", text: "text-primary" },
  Accepted: { bg: "bg-green-bg", text: "text-green" },
  Expired: { bg: "bg-red-bg", text: "text-red" },
  Pending: { bg: "bg-amber-bg", text: "text-amber" },
  None: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATUS_FILTERS = ["All", "Active", "Pending", "Suspended", "Invited", "Inactive"];
const BRANCH_FILTERS = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function StaffDirectoryPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<StaffMember[]>([]);
  const [search, setSearch] = useState<string>("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [selectedBranch, setSelectedBranch] = useState<string>("All branches");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_STAFF);
      setLoading(false);
    };
    loadData();
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.email.toLowerCase().includes(search.toLowerCase()) ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.role.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesBranch;
  });

  const totalActive = data.filter((d) => d.status === "Active").length;
  const totalPending = data.filter((d) => d.status === "Pending" || d.status === "Invited").length;
  const totalSuspended = data.filter((d) => d.status === "Suspended").length;

  const hasActiveFilters =
    selectedStatus !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const filterCount = (selectedStatus !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0);

  const formatTime = (date: string | null): string => {
    if (!date) return "Never";
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

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading staff directory...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Staff Directory</h1>
          <p className="mt-1.5 text-sm text-ink-600">
            Staff name, status, primary scope, role summary, branch/department and invitation state.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button
            onClick={() => router.push("/settings/staff/invite")}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <UserPlus className="h-3.5 w-3.5" />
            Invite Staff
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
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <UserCheck className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalActive}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Pending</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalPending}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <UserX className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Suspended</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{totalSuspended}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, email, role..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <UserCheck className="h-3.5 w-3.5 text-ink-400" />
          {selectedStatus}
          <span className="text-ink-300">·</span>
          <Building2 className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
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
              {filterCount}
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
          {filteredData.length} staff
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
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</span>
              <div className="flex flex-wrap gap-1.5">
                {BRANCH_FILTERS.map((branch) => (
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

      {/* ── Staff Table ── */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No staff found</h3>
          <p className="mt-1 text-sm text-ink-500">No staff members match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-line bg-white">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[800px]">
              <thead>
                <tr className="bg-surface-1">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Staff</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Role</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Invitation</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap">Last Active</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500 whitespace-nowrap"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.Active;
                  const invitationStyle = INVITATION_STYLES[item.invitationState] || INVITATION_STYLES.None;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => router.push(`/settings/staff/${item.id}`)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3 whitespace-nowrap">
                        <div className="font-medium text-ink-950 text-xs">{item.name}</div>
                        <div className="text-[10px] text-ink-500 font-mono">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs whitespace-nowrap">{item.role}</td>
                      <td className="px-4 py-3 text-ink-600 text-xs whitespace-nowrap">{item.branch}</td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                          {item.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${invitationStyle.bg} ${invitationStyle.text}`}>
                          {item.invitationState}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs whitespace-nowrap">
                        {item.lastActive ? formatTime(item.lastActive) : "—"}
                      </td>
                      <td className="px-4 py-3 text-right whitespace-nowrap">
                        <button className="text-ink-400 group-hover:text-primary transition-colors">
                          <Eye className="h-4 w-4" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}