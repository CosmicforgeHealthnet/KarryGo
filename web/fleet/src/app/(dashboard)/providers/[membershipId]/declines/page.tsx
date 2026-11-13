// src/app/(dashboard)/providers/[membershipId]/declines/page.tsx

"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import {
  Search,
  Filter,
  X,
  ChevronDown,
  Shield,
  FileText,
  CheckCircle2,
  Calendar,
  Store,
} from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { providerService } from "@/lib/services/providerService";
import { formatDateTime } from "@/lib/utils/providerLabels";
import type { ProviderDecline } from "@/lib/types/provider";

const ORIGIN_STYLES: Record<string, { label: string; bg: string; text: string; icon: React.ReactNode }> = {
  MARKETPLACE: {
    label: "Marketplace",
    bg: "bg-primary-bg",
    text: "text-primary",
    icon: <Store className="h-3 w-3" />,
  },
  PRIVATE: {
    label: "Company work",
    bg: "bg-purple-bg",
    text: "text-purple",
    icon: <FileText className="h-3 w-3" />,
  },
};

const FILTERS = ["All", "Safety", "Marketplace", "Company work"];

const reasonLabel = (code: string) =>
  code ? code.replace(/[_.]+/g, " ").toLowerCase().replace(/^\w/, (m) => m.toUpperCase()) : "No reason given";

export default function DeclinesTab() {
  const params = useParams();
  const membershipId = params.membershipId as string;

  const [declines, setDeclines] = useState<ProviderDecline[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [search, setSearch] = useState("");
  const [selectedFilter, setSelectedFilter] = useState("All");

  useEffect(() => {
    let cancelled = false;
    providerService
      .declines(membershipId)
      .then((data) => {
        if (!cancelled) setDeclines(data);
      })
      .catch((err) => {
        if (!cancelled) setError(describeApiError(err, "Declines could not be loaded."));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [membershipId]);

  const filteredDeclines = declines.filter((d) => {
    const q = search.trim().toLowerCase();
    const matchesSearch =
      q === "" || d.assignmentId.toLowerCase().includes(q) || d.reasonCode.toLowerCase().includes(q);
    const matchesFilter =
      selectedFilter === "All" ||
      (selectedFilter === "Safety" && d.safetyRelated) ||
      (selectedFilter === "Marketplace" && d.workOrigin === "MARKETPLACE") ||
      (selectedFilter === "Company work" && d.workOrigin === "PRIVATE");
    return matchesSearch && matchesFilter;
  });

  const hasFilters = selectedFilter !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedFilter("All");
  };

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-12 text-center text-sm text-ink-500">
        Loading declines…
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <ApiErrorNotice error={error} />

      <p className="text-xs text-ink-500">
        Providers can always decline work. Declines for safety reasons, such as an unsafe vehicle, are flagged and never
        count against the provider.
      </p>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by assignment or reason..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => {
            const currentIndex = FILTERS.indexOf(selectedFilter);
            const nextIndex = (currentIndex + 1) % FILTERS.length;
            setSelectedFilter(FILTERS[nextIndex]);
          }}
          className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2 whitespace-nowrap"
        >
          <Filter className="h-3.5 w-3.5 text-ink-400" />
          {selectedFilter}
          <ChevronDown className="h-3.5 w-3.5 text-ink-400" />
        </button>

        {hasFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filteredDeclines.length} records
        </span>
      </div>

      {/* ── Declines List ── */}
      {filteredDeclines.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-12">
          <div className="rounded-full bg-surface-2 p-3">
            <CheckCircle2 className="h-8 w-8 text-green" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No declines found</h3>
          <p className="mt-1 text-sm text-ink-500">
            {declines.length === 0
              ? "This provider has not declined any work."
              : "No decline records match your current filters."}
          </p>
          {hasFilters && (
            <button
              onClick={clearFilters}
              className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
            >
              Clear filters
            </button>
          )}
        </div>
      ) : (
        <div className="space-y-2.5">
          {filteredDeclines.map((record) => {
            const origin = ORIGIN_STYLES[record.workOrigin] || ORIGIN_STYLES.PRIVATE;

            return (
              <div
                key={`${record.workOrigin}-${record.assignmentId}`}
                className={`rounded-lg border p-3.5 transition-colors ${
                  record.safetyRelated
                    ? "border-red/20 bg-red-bg/10"
                    : "border-line bg-white hover:bg-surface-2"
                }`}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm font-semibold text-ink-950">{record.assignmentId}</span>
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${origin.bg} ${origin.text}`}
                    >
                      {origin.icon}
                      {origin.label}
                    </span>
                    {record.safetyRelated && (
                      <span className="flex items-center gap-1 rounded-full bg-red-bg px-2 py-0.5 text-[10px] font-semibold text-red">
                        <Shield className="h-3 w-3" />
                        Safety flag
                      </span>
                    )}
                  </div>
                  <p className="mt-1 text-sm text-ink-700">{reasonLabel(record.reasonCode)}</p>
                  <div className="mt-1.5 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                    <span className="flex items-center gap-1">
                      <Calendar className="h-3.5 w-3.5" />
                      {formatDateTime(record.declinedAt)}
                    </span>
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
