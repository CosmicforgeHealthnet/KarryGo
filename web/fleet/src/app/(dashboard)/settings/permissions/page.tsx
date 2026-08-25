// src/app/(dashboard)/settings/permissions/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  ArrowLeft,
  Search,
  AlertTriangle,
  ChevronDown,
  SlidersHorizontal,
  X,
  Clock,
} from "lucide-react";

// ==================== TYPES ====================

type PermissionGroup = {
  id: string;
  name: string;
  description: string;
  permissions: {
    id: string;
    name: string;
    description: string;
    riskLevel: "Low" | "Medium" | "High" | "Critical";
  }[];
};

// ==================== MOCK DATA ====================

const MOCK_PERMISSION_GROUPS: PermissionGroup[] = [
  {
    id: "G-001",
    name: "Staff Management",
    description: "Permissions related to managing staff members.",
    permissions: [
      { id: "P-001", name: "View Staff", description: "View staff directory and profiles.", riskLevel: "Low" },
      { id: "P-002", name: "Invite Staff", description: "Send invitations to new staff members.", riskLevel: "Medium" },
      { id: "P-003", name: "Edit Staff", description: "Update staff profiles and roles.", riskLevel: "High" },
      { id: "P-004", name: "Suspend Staff", description: "Suspend or reactivate staff members.", riskLevel: "Critical" },
    ],
  },
  {
    id: "G-002",
    name: "Role Management",
    description: "Permissions related to managing roles.",
    permissions: [
      { id: "P-005", name: "View Roles", description: "View roles and their permissions.", riskLevel: "Low" },
      { id: "P-006", name: "Create Roles", description: "Create new custom roles.", riskLevel: "High" },
      { id: "P-007", name: "Edit Roles", description: "Update existing roles.", riskLevel: "High" },
      { id: "P-008", name: "Delete Roles", description: "Delete custom roles.", riskLevel: "Critical" },
    ],
  },
  {
    id: "G-003",
    name: "Organization Settings",
    description: "Permissions related to organization settings.",
    permissions: [
      { id: "P-009", name: "View Settings", description: "View organization settings.", riskLevel: "Low" },
      { id: "P-010", name: "Edit Settings", description: "Update organization settings.", riskLevel: "High" },
      { id: "P-011", name: "Manage Integrations", description: "Manage API clients and webhooks.", riskLevel: "High" },
    ],
  },
  {
    id: "G-004",
    name: "Operations",
    description: "Permissions related to operations and dispatch.",
    permissions: [
      { id: "P-012", name: "View Operations", description: "View dispatch board and operations.", riskLevel: "Low" },
      { id: "P-013", name: "Manage Dispatch", description: "Create and manage dispatch assignments.", riskLevel: "Medium" },
      { id: "P-014", name: "Manage Routes", description: "Create and manage routes.", riskLevel: "Medium" },
      { id: "P-015", name: "Manage Rescue", description: "Create and manage rescue cases.", riskLevel: "High" },
    ],
  },
  {
    id: "G-005",
    name: "Providers",
    description: "Permissions related to provider management.",
    permissions: [
      { id: "P-016", name: "View Providers", description: "View provider directory.", riskLevel: "Low" },
      { id: "P-017", name: "Manage Providers", description: "Add, edit, and manage providers.", riskLevel: "Medium" },
      { id: "P-018", name: "Suspend Providers", description: "Suspend or end provider memberships.", riskLevel: "High" },
    ],
  },
  {
    id: "G-006",
    name: "Fleet",
    description: "Permissions related to fleet management.",
    permissions: [
      { id: "P-019", name: "View Fleet", description: "View fleet assets and status.", riskLevel: "Low" },
      { id: "P-020", name: "Manage Assets", description: "Add, edit, and manage fleet assets.", riskLevel: "Medium" },
      { id: "P-021", name: "Manage Maintenance", description: "Create and manage maintenance work orders.", riskLevel: "Medium" },
      { id: "P-022", name: "Return to Service", description: "Authorize return-to-service decisions.", riskLevel: "High" },
    ],
  },
  {
    id: "G-007",
    name: "Finance",
    description: "Permissions related to finance and wallet.",
    permissions: [
      { id: "P-023", name: "View Finance", description: "View financial data and reports.", riskLevel: "Low" },
      { id: "P-024", name: "Manage Allocations", description: "Create and approve allocations.", riskLevel: "High" },
      { id: "P-025", name: "Manage Withdrawals", description: "Approve and process withdrawals.", riskLevel: "Critical" },
    ],
  },
  {
    id: "G-008",
    name: "Compliance",
    description: "Permissions related to compliance and safety.",
    permissions: [
      { id: "P-026", name: "View Compliance", description: "View compliance data and reports.", riskLevel: "Low" },
      { id: "P-027", name: "Manage Requirements", description: "Create and manage compliance requirements.", riskLevel: "Medium" },
      { id: "P-028", name: "Manage Restrictions", description: "Create and lift restrictions.", riskLevel: "High" },
    ],
  },
];

type RiskStyle = {
  bg: string;
  text: string;
  dot: string;
};

const RISK_STYLES: Record<string, RiskStyle> = {
  Low: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Medium: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  High: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

const RISK_FILTERS = ["All", "Low", "Medium", "High", "Critical"];

export default function PermissionCataloguePage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<PermissionGroup[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [search, setSearch] = useState<string>("");
  const [selectedRisk, setSelectedRisk] = useState<string>("All");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_PERMISSION_GROUPS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.map((group) => ({
    ...group,
    permissions: group.permissions.filter((p) => {
      const matchesSearch =
        search.trim() === "" ||
        p.name.toLowerCase().includes(search.toLowerCase()) ||
        p.description.toLowerCase().includes(search.toLowerCase());
      const matchesRisk = selectedRisk === "All" || p.riskLevel === selectedRisk;
      return matchesSearch && matchesRisk;
    }),
  })).filter((group) => group.permissions.length > 0);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading permission catalogue...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedRisk !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedRisk("All");
  };

  const filterCount = (selectedRisk !== "All" ? 1 : 0);

  const goBack = () => {
    router.push("/settings/roles");
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/settings/roles")}
              className="hover:text-primary transition-colors"
            >
              Roles
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">Permissions</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Permission Catalogue
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Permission groups in business language. Use as reference while editing roles.
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
            onClick={goBack}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search permissions..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const risks = ["All", "Low", "Medium", "High", "Critical"];
            const currentIndex = risks.indexOf(selectedRisk);
            const nextIndex = (currentIndex + 1) % risks.length;
            setSelectedRisk(risks[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedRisk}
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

      {/* ===== PERMISSION GROUPS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Shield className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No permissions found</h3>
          <p className="mt-1 text-sm text-ink-500">No permissions match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="space-y-4">
          {filteredData.map((group) => (
            <div key={group.id} className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
              <div className="p-4 border-b border-line bg-surface-2/50">
                <div className="flex items-center gap-2">
                  <Shield className="h-4 w-4 text-ink-400" />
                  <span className="text-sm font-semibold text-ink-950">{group.name}</span>
                  <span className="text-xs text-ink-400">{group.permissions.length} permissions</span>
                </div>
                <div className="mt-0.5 text-xs text-ink-500">{group.description}</div>
              </div>
              <div className="divide-y divide-line">
                {group.permissions.map((perm) => {
                  const riskStyle = RISK_STYLES[perm.riskLevel] || RISK_STYLES.Low;

                  return (
                    <div key={perm.id} className="flex items-center justify-between p-4 hover:bg-surface-2 transition-colors">
                      <div>
                        <div className="font-medium text-ink-950 text-sm">{perm.name}</div>
                        <div className="text-xs text-ink-500">{perm.description}</div>
                      </div>
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${riskStyle.bg} ${riskStyle.text}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${riskStyle.dot}`} />
                        {perm.riskLevel}
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          IAM-06 · Permission Catalogue
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.reduce((acc, g) => acc + g.permissions.length, 0)} permissions shown
        </span>
      </div>
    </div>
  );
}