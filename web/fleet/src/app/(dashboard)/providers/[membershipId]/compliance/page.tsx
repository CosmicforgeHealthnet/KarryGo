// src/app/(dashboard)/providers/[providerId]/compliance/page.tsx

"use client";

import { useState } from "react";
import { COMPLIANCE_REQUIREMENTS } from "@/lib/mock-provider-data";
import {
  Shield,
  CheckCircle2,
  AlertCircle,
  XCircle,
  Clock,
  ExternalLink,
  RefreshCw,
  FileCheck,
  FileWarning,
  FileX,
  Search,
  Filter,
  X,
  ChevronDown,
} from "lucide-react";

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Verified: {
    bg: "bg-green-bg",
    text: "text-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  "Expiring soon": {
    bg: "bg-amber-bg",
    text: "text-amber",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Missing: {
    bg: "bg-red-bg",
    text: "text-red",
    icon: <XCircle className="h-3.5 w-3.5" />,
  },
};

const SOURCE_STYLES: Record<string, { bg: string; text: string }> = {
  Company: { bg: "bg-primary-bg", text: "text-primary" },
  "External authority": { bg: "bg-surface-2", text: "text-ink-600" },
};

const STATUS_FILTERS = ["All", "Verified", "Expiring soon", "Missing"];

export default function ComplianceTab() {
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [search, setSearch] = useState("");

  const filteredRequirements = COMPLIANCE_REQUIREMENTS.filter((req) => {
    const matchesSearch =
      search.trim() === "" ||
      req.name.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || req.status === selectedStatus;

    return matchesSearch && matchesStatus;
  });

  const verifiedCount = COMPLIANCE_REQUIREMENTS.filter((r) => r.status === "Verified").length;
  const expiringCount = COMPLIANCE_REQUIREMENTS.filter((r) => r.status === "Expiring soon").length;
  const missingCount = COMPLIANCE_REQUIREMENTS.filter((r) => r.status === "Missing").length;

  const hasFilters = selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  return (
    <div className="space-y-5">
      {/* ── Stats ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-3">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Verified</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{verifiedCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Expiring Soon</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{expiringCount}</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <XCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Missing</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-ink-950">{missingCount}</div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-white px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search requirements..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => {
            const currentIndex = STATUS_FILTERS.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % STATUS_FILTERS.length;
            setSelectedStatus(STATUS_FILTERS[nextIndex]);
          }}
          className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2 whitespace-nowrap"
        >
          <Filter className="h-3.5 w-3.5 text-ink-400" />
          {selectedStatus}
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
          {filteredRequirements.length} requirements
        </span>
      </div>

      {/* ── Requirements List ── */}
      {filteredRequirements.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-white py-12">
          <div className="rounded-full bg-surface-2 p-3">
            <FileCheck className="h-8 w-8 text-green" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">All requirements met</h3>
          <p className="mt-1 text-sm text-ink-500">
            No compliance requirements match your current filters.
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
          {filteredRequirements.map((req) => {
            const statusStyle = STATUS_STYLES[req.status] || STATUS_STYLES.Verified;
            const sourceStyle = SOURCE_STYLES[req.source] || SOURCE_STYLES.Company;

            return (
              <div
                key={req.id}
                className="flex flex-col gap-2 rounded-lg border border-line bg-white p-3.5 transition-colors hover:bg-surface-2 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-medium text-ink-950">{req.name}</span>
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                    >
                      {statusStyle.icon}
                      {req.status}
                    </span>
                    <span
                      className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${sourceStyle.bg} ${sourceStyle.text}`}
                    >
                      {req.source}
                    </span>
                  </div>
                  {req.expiry && req.expiry !== "—" && (
                    <div className="mt-0.5 flex items-center gap-1 text-[10px] text-ink-500">
                      <Clock className="h-3.5 w-3.5" />
                      Expires: {req.expiry}
                    </div>
                  )}
                </div>
                <button className="flex items-center gap-1 text-xs font-medium text-primary transition-colors hover:text-primary-hover whitespace-nowrap">
                  View details
                  <ExternalLink className="h-3.5 w-3.5" />
                </button>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}