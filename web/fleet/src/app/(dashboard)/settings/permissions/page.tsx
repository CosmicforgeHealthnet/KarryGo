// src/app/(dashboard)/settings/permissions/page.tsx

"use client";

import { useState, useEffect, useCallback } from "react";
import { staffService, staffErrorMessage } from "@/lib/services/staffService";
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

// Risk is derived from what a permission lets someone do: reading is low,
// changing records medium, approving or managing people high, and moving money
// critical.
const riskOf = (code: string): PermissionGroup["permissions"][number]["riskLevel"] => {
  const c = code.toLowerCase();
  if (/(wallet|payout|withdraw|payroll|posting|settlement|refund|transfer)/.test(c)) return "Critical";
  if (/(approve|manage|suspend|delete|remove|invite|close|revoke|grant)/.test(c)) return "High";
  if (/(read|view|list|export)$/.test(c) || c.endsWith(".read")) return "Low";
  return "Medium";
};

const titleCase = (value: string) =>
  value.replace(/[_.]+/g, " ").replace(/\b\w/g, (m) => m.toUpperCase()).trim();

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

  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const permissions = await staffService.permissions();
      setError(null);
      const groups = new Map<string, PermissionGroup>();
      for (const p of permissions.filter((x) => x.isActive)) {
        const category = p.category || p.code.split(".")[0] || "general";
        const group = groups.get(category) ?? {
          id: category,
          name: titleCase(category),
          description: `Permissions for ${titleCase(category).toLowerCase()}.`,
          permissions: [],
        };
        group.permissions.push({ id: p.code, name: p.description || titleCase(p.code), description: p.code, riskLevel: riskOf(p.code) });
        groups.set(category, group);
      }
      setData([...groups.values()].sort((x, y) => x.name.localeCompare(y.name)));
      setSecondsAgo(0);
    } catch (e) {
      setError(staffErrorMessage(e, "Permissions could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    // Started from a promise callback so no state is set synchronously
    // inside the effect.
    Promise.resolve().then(load);
  }, [load]);

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
            Every permission a role can grant, grouped by area. Risk shows how sensitive each one is.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => load()}
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
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">{error ? "Permissions could not be loaded" : "No permissions found"}</h3>
          <p className="mt-1 text-sm text-ink-500">{error ?? "No permissions match your current filters."}</p>
          <button onClick={error ? () => { setLoading(true); load(); } : clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            {error ? "Try again" : "Clear filters"}
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