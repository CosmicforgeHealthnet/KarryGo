// src/app/(dashboard)/providers/[providerId]/declines/page.tsx

"use client";

import { useState } from "react";
import { DECLINES } from "@/lib/mock-provider-data";
import {
  Search,
  Filter,
  X,
  ChevronDown,
  AlertCircle,
  Shield,
  Clock,
  User,
  FileText,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Calendar,
  ArrowRight,
} from "lucide-react";

const CATEGORY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Safety: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <Shield className="h-3 w-3" />,
  },
  Availability: {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <Clock className="h-3 w-3" />,
  },
  Operational: {
    bg: "bg-primary-bg",
    text: "text-primary",
    icon: <FileText className="h-3 w-3" />,
  },
  Personal: {
    bg: "bg-purple-bg",
    text: "text-purple",
    icon: <User className="h-3 w-3" />,
  },
  Technical: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    icon: <AlertCircle className="h-3 w-3" />,
  },
};

const CATEGORIES = ["All", "Safety", "Availability", "Operational", "Personal", "Technical"];

export default function DeclinesTab() {
  const [search, setSearch] = useState("");
  const [selectedCategory, setSelectedCategory] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  const filteredDeclines = DECLINES.filter((d) => {
    const matchesSearch =
      search.trim() === "" ||
      d.reference.toLowerCase().includes(search.toLowerCase()) ||
      d.note.toLowerCase().includes(search.toLowerCase());

    const matchesCategory = selectedCategory === "All" || d.category === selectedCategory;

    return matchesSearch && matchesCategory;
  });

  const hasFilters = selectedCategory !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedCategory("All");
  };

  return (
    <div className="space-y-5">
      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference or note..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => {
            const currentIndex = CATEGORIES.indexOf(selectedCategory);
            const nextIndex = (currentIndex + 1) % CATEGORIES.length;
            setSelectedCategory(CATEGORIES[nextIndex]);
          }}
          className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2 whitespace-nowrap"
        >
          <Filter className="h-3.5 w-3.5 text-ink-400" />
          {selectedCategory}
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
            No decline records match your current filters.
          </p>
          <button
            onClick={clearFilters}
            className="mt-3.5 rounded-md bg-primary px-4 py-1.5 text-xs font-semibold text-white hover:bg-primary/90 transition-colors"
          >
            Clear filters
          </button>
        </div>
      ) : (
        <div className="space-y-2.5">
          {filteredDeclines.map((record) => {
            const categoryStyle = CATEGORY_STYLES[record.category] || CATEGORY_STYLES.Operational;

            return (
              <div
                key={record.id}
                className={`rounded-lg border p-3.5 transition-colors ${
                  record.safetyFlag
                    ? "border-red/20 bg-red-bg/10"
                    : "border-line bg-white hover:bg-surface-2"
                }`}
              >
                <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-mono text-sm font-semibold text-ink-950">
                        {record.reference}
                      </span>
                      <span
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}
                      >
                        {categoryStyle.icon}
                        {record.category}
                      </span>
                      {record.safetyFlag && (
                        <span className="flex items-center gap-1 rounded-full bg-red-bg px-2 py-0.5 text-[10px] font-semibold text-red">
                          <Shield className="h-3 w-3" />
                          Safety flag
                        </span>
                      )}
                    </div>
                    <p className="mt-1 text-sm text-ink-700">{record.note}</p>
                    <div className="mt-1.5 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                      <span className="flex items-center gap-1">
                        <Calendar className="h-3.5 w-3.5" />
                        {record.time}
                      </span>
                      <span className="text-ink-300">·</span>
                      <span className="flex items-center gap-1">
                        <ArrowRight className="h-3.5 w-3.5" />
                        Outcome: {record.outcome}
                      </span>
                    </div>
                  </div>
                  <button className="text-xs font-medium text-primary transition-colors hover:text-primary-hover whitespace-nowrap">
                    View →
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}