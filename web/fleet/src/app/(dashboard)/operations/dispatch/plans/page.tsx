// src/app/(dashboard)/operations/dispatch/plans/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Search,
  MapPin,
  Plus,
  SlidersHorizontal,
  X,
  Calendar,
  Users,
  Clock,
  ChevronUp,
  ChevronDown,
} from "lucide-react";
import { DISPATCH_PLANS } from "@/lib/mock-dispatch-data";

const STATUS_STYLE: Record<string, { bg: string; text: string; dot: string }> = {
  Draft: { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" },
  Locked: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Published: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

type SortField = "name" | "status" | "branch" | "jobCount" | "window";
type SortDirection = "asc" | "desc";

const STATUS_ORDER = { Draft: 0, Locked: 1, Published: 2 };

export default function DispatchPlansPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [plans, setPlans] = useState(DISPATCH_PLANS);
  const [sortField, setSortField] = useState<SortField>("status");
  const [sortDirection, setSortDirection] = useState<SortDirection>("asc");

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 500);
    return () => clearTimeout(timer);
  }, []);

  const filteredPlans = plans.filter((plan) => {
    const matchesSearch =
      search.trim() === "" ||
      plan.name.toLowerCase().includes(search.toLowerCase()) ||
      plan.id.toLowerCase().includes(search.toLowerCase()) ||
      plan.owner.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || plan.branch === selectedBranch;

    return matchesSearch && matchesBranch;
  });

  const sorted = [...filteredPlans].sort((a, b) => {
    let aVal: string | number = a[sortField as keyof typeof a] as string | number;
    let bVal: string | number = b[sortField as keyof typeof b] as string | number;

    if (sortField === "status") {
      aVal = STATUS_ORDER[a.status as keyof typeof STATUS_ORDER];
      bVal = STATUS_ORDER[b.status as keyof typeof STATUS_ORDER];
    }

    if (aVal < bVal) return sortDirection === "asc" ? -1 : 1;
    if (aVal > bVal) return sortDirection === "asc" ? 1 : -1;
    return 0;
  });

  const getStatusCounts = () => {
    const counts = { Draft: 0, Locked: 0, Published: 0 };
    plans.forEach((p) => counts[p.status]++);
    return counts;
  };

  const statusCounts = getStatusCounts();

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === "asc" ? "desc" : "asc");
    } else {
      setSortField(field);
      setSortDirection("asc");
    }
  };

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) return <ChevronUp className="h-3 w-3 text-ink-300" />;
    return sortDirection === "asc" ? (
      <ChevronUp className="h-3 w-3 text-primary" />
    ) : (
      <ChevronDown className="h-3 w-3 text-primary" />
    );
  };

  const handleCreatePlan = () => {
    router.push("/operations/dispatch/plans/create");
  };

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading plans...</span>
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
            <h1>Dispatch plans</h1>
            <span className="rounded-full bg-primary-bg px-2.5 py-0.5 text-xs font-semibold text-primary">
              {plans.length}
            </span>
          </div>
          <p className="mt-1.5">Branch-scoped planning windows and their publication status.</p>
        </div>
        <button
          onClick={handleCreatePlan}
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Create
        </button>
      </div>

      {/* ── Status stats ── */}
      <div className="grid grid-cols-3 gap-3.5">
        {Object.entries(statusCounts).map(([status, count]) => {
          const style = STATUS_STYLE[status];
          return (
            <div key={status} className="rounded-lg border border-line bg-white p-4">
              <div className="flex items-center gap-2">
                <span className={`h-2 w-2 rounded-full ${style.dot}`} />
                <span className="text-xs font-medium text-ink-600">{status}</span>
                <span className="ml-auto font-display text-[26px] font-bold text-ink-950">
                  {count}
                </span>
              </div>
            </div>
          );
        })}
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search plans..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <MapPin className="h-3.5 w-3.5 text-ink-400" />
          {selectedBranch}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || selectedBranch !== "All branches"
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
        </button>

        {(selectedBranch !== "All branches" || search) && (
          <button
            onClick={() => {
              setSelectedBranch("All branches");
              setSearch("");
            }}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {sorted.length} plans
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-6">
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

      {/* ── Table ── */}
      <div className="overflow-x-auto rounded-lg border border-line bg-white">
        <table className="w-full min-w-[700px] border-collapse text-left">
          <thead>
            <tr className="border-b border-line bg-surface-1">
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("name")} className="flex items-center gap-1 hover:text-ink-700">
                  Plan <SortIcon field="name" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("status")} className="flex items-center gap-1 hover:text-ink-700">
                  Status <SortIcon field="status" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("branch")} className="flex items-center gap-1 hover:text-ink-700">
                  Branch <SortIcon field="branch" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("window")} className="flex items-center gap-1 hover:text-ink-700">
                  Window <SortIcon field="window" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                <button onClick={() => handleSort("jobCount")} className="flex items-center gap-1 hover:text-ink-700">
                  Jobs <SortIcon field="jobCount" />
                </button>
              </th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">Owner</th>
              <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">Action</th>
            </tr>
          </thead>
          <tbody>
            {sorted.length === 0 ? (
              <tr>
                <td colSpan={7} className="px-4 py-12 text-center text-ink-500">
                  <div className="flex flex-col items-center gap-2">
                    <Calendar className="h-8 w-8 text-ink-300" />
                    <p className="text-sm font-medium">No plans match your filters</p>
                    <button
                      onClick={() => {
                        setSelectedBranch("All branches");
                        setSearch("");
                      }}
                      className="text-sm text-primary hover:underline"
                    >
                      Clear filters
                    </button>
                  </div>
                </td>
              </tr>
            ) : (
              sorted.map((plan) => {
                const style = STATUS_STYLE[plan.status];
                return (
                  <tr key={plan.id} className="border-b border-line transition-colors hover:bg-surface-1/50">
                    <td className="px-4 py-3.5">
                      <div>
                        <span className="font-semibold text-ink-950">{plan.name}</span>
                        <div className="text-[10px] text-ink-400">{plan.id}</div>
                      </div>
                    </td>
                    <td className="px-4 py-3.5">
                      <span className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-semibold ${style.bg} ${style.text}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${style.dot}`} />
                        {plan.status}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 text-sm text-ink-700">{plan.branch}</td>
                    <td className="px-4 py-3.5 text-sm text-ink-700">{plan.window}</td>
                    <td className="px-4 py-3.5 text-sm font-medium text-ink-950">{plan.jobCount}</td>
                    <td className="px-4 py-3.5 text-sm text-ink-600">{plan.owner}</td>
                    <td className="px-4 py-3.5 text-right">
                      <Link
                        href={`/operations/dispatch/plans/${plan.id}`}
                        className="inline-flex items-center gap-1 rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                      >
                        View
                      </Link>
                    </td>
                  </tr>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}