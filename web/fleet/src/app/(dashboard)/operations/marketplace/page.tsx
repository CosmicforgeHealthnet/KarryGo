// src/app/(dashboard)/operations/marketplace/page.tsx

"use client";

import React, { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Package,
  Truck,
  Search,
  ListChecks,
  Clock3,
  CheckCircle2,
  XCircle,
  SlidersHorizontal,
  ArrowRight,
  X,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { marketplaceService } from "@/lib/services/marketplaceService";
import type {
  CorporateOfferMetrics,
  CorporateOfferSummary,
} from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Presentation helpers
// ---------------------------------------------------------------------------

type Tone = "blue" | "green" | "amber" | "red";

const TONE_BG: Record<Tone, string> = {
  blue: "bg-primary-bg text-primary",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
};

const ZERO_METRICS: CorporateOfferMetrics = {
  openOffers: 0,
  expiringSoon: 0,
  acceptedToday: 0,
  declinedToday: 0,
};

function formatCountdown(expiresAtIso: string): {
  text: string;
  minutes: number;
} {
  const expires = new Date(expiresAtIso).getTime();
  if (!Number.isFinite(expires)) {
    return { text: "—", minutes: Number.MAX_SAFE_INTEGER };
  }
  const minutes = Math.max(0, Math.floor((expires - Date.now()) / 60_000));
  if (minutes <= 0) return { text: "expired", minutes: 0 };
  if (minutes < 60) return { text: `${minutes} min`, minutes };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { text: `${hours} h`, minutes };
  const days = Math.floor(hours / 24);
  return { text: `${days} d`, minutes };
}

function fuseTier(mins: number): { color: string; bg: string } {
  if (mins <= 15) return { color: "text-red", bg: "bg-red-bg" };
  if (mins <= 30) return { color: "text-amber", bg: "bg-amber-bg" };
  return { color: "text-primary", bg: "bg-primary-bg" };
}

function serviceLabel(sourceService: string): string {
  switch (sourceService) {
    case "DISPATCH":
      return "Dispatch";
    case "TAXI":
      return "Taxi";
    case "HAULING":
      return "Hauling";
    default:
      return sourceService || "—";
  }
}

function statusLabel(status: string): string {
  switch (status) {
    case "OFFERED":
      return "Open";
    case "RESERVATION_PENDING":
      return "Reserving";
    case "ORGANIZATION_ACCEPTED":
      return "Accepted";
    case "ASSIGNMENT_PENDING":
      return "Assigning";
    case "PROVIDER_ACCEPTED":
      return "Provider accepted";
    case "EXECUTION_REQUESTED":
    case "EXECUTION_CREATED":
      return "Starting";
    case "IN_PROGRESS":
      return "In progress";
    case "COMPLETED":
      return "Completed";
    case "ORGANIZATION_DECLINED":
      return "Declined";
    case "EXPIRED":
      return "Expired";
    case "CANCELLED":
      return "Cancelled";
    default:
      return status || "—";
  }
}

// Best-effort human-readable pickup/dropoff line from the projection summary.
// The backend stores these as opaque maps; when none of the known keys are
// present, we render a dash rather than inventing a value.
function summaryLabel(summary: Record<string, unknown> | undefined): string {
  if (!summary) return "—";
  const candidateKeys = ["label", "name", "reference", "address"];
  for (const key of candidateKeys) {
    const value = summary[key];
    if (typeof value === "string" && value.trim() !== "") {
      return value.trim();
    }
  }
  return "—";
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function MarketplaceOffersPage() {
  const [offers, setOffers] = useState<CorporateOfferSummary[]>([]);
  const [metrics, setMetrics] = useState<CorporateOfferMetrics>(ZERO_METRICS);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [selectedService, setSelectedService] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    Promise.all([
      marketplaceService.listOffers({ status: "OFFERED" }),
      marketplaceService.getMetrics(),
    ])
      .then(([offersData, metricsData]) => {
        if (cancelled) return;
        setOffers(offersData);
        setMetrics(metricsData);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          "Failed to load marketplace offers.";
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const services = useMemo(() => {
    const set = new Set<string>();
    offers.forEach((o) => {
      if (o.sourceService) set.add(o.sourceService);
    });
    return ["All", ...Array.from(set)];
  }, [offers]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return offers.filter((o) => {
      if (selectedService !== "All" && o.sourceService !== selectedService) {
        return false;
      }
      if (q === "") return true;
      const haystack = [
        o.offerId,
        o.requestProjectionId,
        summaryLabel(o.pickupSummary),
        summaryLabel(o.dropoffSummary),
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [offers, search, selectedService]);

  const hasActiveFilters = selectedService !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedService("All");
  };

 const metricCards: {
  label: string;
  value: number;
  tone: Tone;
  icon: React.ReactNode;
}[] = [
    {
      label: "Open offers",
      value: metrics.openOffers,
      tone: "blue",
      icon: <ListChecks className="h-[18px] w-[18px]" />,
    },
    {
      label: "Expiring soon",
      value: metrics.expiringSoon,
      tone: "amber",
      icon: <Clock3 className="h-[18px] w-[18px]" />,
    },
    {
      label: "Accepted today",
      value: metrics.acceptedToday,
      tone: "green",
      icon: <CheckCircle2 className="h-[18px] w-[18px]" />,
    },
    {
      label: "Declined today",
      value: metrics.declinedToday,
      tone: "red",
      icon: <XCircle className="h-[18px] w-[18px]" />,
    },
  ];

  return (
    <div className="mx-auto max-w-[1600px] space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Marketplace offers</h1>
        <p className="max-w-2xl">
          Current eligible KarryGo opportunities. Sort by deadline to see urgent
          offers first.
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-red-300/40 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {/* ── Metric grid ── */}
      <div className="grid grid-cols-2 gap-3.5 lg:grid-cols-4">
        {metricCards.map((m) => (
          <div
            key={m.label}
            className="rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400"
          >
            <div className="flex items-center gap-2.5">
              <div
                className={`flex h-8 w-8 items-center justify-center rounded-md ${TONE_BG[m.tone]}`}
              >
                {m.icon}
              </div>
              <span className="text-xs font-semibold text-ink-600">
                {m.label}
              </span>
            </div>
            <div className="mt-3 flex items-end justify-between">
              <span className="font-display text-3xl font-bold tracking-[-0.02em] text-ink-950">
                {m.value}
              </span>
            </div>
          </div>
        ))}
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search offers..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <div className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-700 whitespace-nowrap">
          <Package className="h-3.5 w-3.5 text-ink-400" />
          {selectedService === "All"
            ? "All services"
            : serviceLabel(selectedService)}
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filtered.length} offers
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Service
              </span>
              <div className="flex flex-wrap gap-1.5">
                {services.map((service) => (
                  <button
                    key={service}
                    onClick={() => setSelectedService(service)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedService === service
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {service === "All" ? "All" : serviceLabel(service)}
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

      {/* ── Table ── */}
      <div className="overflow-x-auto rounded-lg border border-line bg-surface">
        {loading ? (
          <div className="px-4 py-16 text-center text-sm text-ink-500">
            Loading offers…
          </div>
        ) : offers.length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
            title="No open offers"
            description="When the platform sends work your way, it will appear here."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
            title="No offers match your filters"
            description="Try adjusting your search or clearing the filters."
            action={
              <button
                onClick={clearFilters}
                className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
              >
                Clear filters
              </button>
            }
          />
        ) : (
          <table className="w-full min-w-[900px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-2">
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Offer
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Route
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Service
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Status
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Deadline
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">
                  Action
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((o) => {
                const { text: countdown, minutes } = formatCountdown(o.expiresAt);
                const tier = fuseTier(minutes);
                const isUrgent = minutes <= 15;
                const pickup = summaryLabel(o.pickupSummary);
                const dropoff = summaryLabel(o.dropoffSummary);

                return (
                  <tr
                    key={o.offerId}
                    className="border-b border-line transition-colors hover:bg-surface-2"
                  >
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-2.5">
                        <div
                          className={`flex h-8 w-8 items-center justify-center rounded-md ${tier.bg} ${tier.color}`}
                        >
                          {o.sourceService === "HAULING" ? (
                            <Truck className="h-[15px] w-[15px]" />
                          ) : (
                            <Package className="h-[15px] w-[15px]" />
                          )}
                        </div>
                        <div>
                          <span className="font-semibold text-ink-950">
                            {serviceLabel(o.sourceService)}
                          </span>
                          <div className="font-mono text-[10px] text-ink-400">
                            {o.offerId}
                          </div>
                        </div>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-1.5">
                        <span>{pickup}</span>
                        <ArrowRight className="h-3 w-3 text-ink-400 flex-shrink-0" />
                        <span>{dropoff}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 font-medium text-ink-700 whitespace-nowrap">
                      {o.serviceType || "—"}
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <span className="rounded-full bg-surface-2 px-2.5 py-1 text-[11px] font-semibold text-ink-700">
                        {statusLabel(o.status)}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <div className="flex items-center gap-2">
                        <div
                          className={`h-1.5 w-1.5 rounded-full ${tier.color.replace("text", "bg")} flex-shrink-0`}
                        />
                        <span className={`font-mono font-bold ${tier.color}`}>
                          {isUrgent ? "🔴 " : ""}
                          {countdown}
                        </span>
                      </div>
                    </td>
                    <td className="px-4 py-3.5 text-right whitespace-nowrap">
                      <div className="flex items-center justify-end gap-2">
                        <Link
                          href={`/operations/marketplace/${o.offerId}`}
                          className="rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
                        >
                          Details
                        </Link>
                        <Link
                          href={`/operations/marketplace/${o.offerId}/candidates`}
                          className="rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-hover transition-colors whitespace-nowrap"
                        >
                          Compare
                        </Link>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}