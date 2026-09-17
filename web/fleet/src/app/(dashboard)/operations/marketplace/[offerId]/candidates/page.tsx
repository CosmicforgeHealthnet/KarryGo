// src/app/(dashboard)/operations/marketplace/[offerId]/candidates/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  ArrowLeft,
  Search,
  CheckCircle2,
  XCircle,
  AlertCircle,
  SlidersHorizontal,
  UserCheck,
  User,
  X,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { marketplaceService } from "@/lib/services/marketplaceService";
import type {
  CorporateCandidate,
  CorporateOfferDetail,
} from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Presentation helpers
// ---------------------------------------------------------------------------

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

// Reason codes come from the backend eligibility check. Turn the raw code into
// a short, human-readable phrase. Unknown codes are shown as-is rather than
// hidden — an operator can then search the code.
function reasonLabel(reason: string | undefined): string {
  if (!reason) return "";
  switch (reason) {
    case "eligibility_unavailable":
      return "Eligibility check unavailable";
    case "PROVIDER_NOT_ELIGIBLE":
      return "Provider not eligible";
    case "PROVIDER_MEMBERSHIP_NOT_ACTIVE":
      return "Membership not active";
    case "PROVIDER_ASSOCIATION_PENDING":
      return "Association pending";
    case "PROVIDER_PROVISIONING_INCOMPLETE":
      return "Provisioning incomplete";
    case "PROVIDER_SHIFT_INVALID":
      return "No valid shift";
    case "SOURCE_SERVICE_UNAVAILABLE":
      return "Source service unavailable";
    case "VEHICLE_NOT_ELIGIBLE":
      return "Vehicle not eligible";
    case "PROVIDER_HAS_CONFLICTING_WORK":
      return "Already on other work";
    default:
      return reason;
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function CandidatesPage() {
  const params = useParams();
  const offerId = typeof params.offerId === "string" ? params.offerId : "";

  const [offer, setOffer] = useState<CorporateOfferDetail | null>(null);
  const [candidates, setCandidates] = useState<CorporateCandidate[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [selectedEligibility, setSelectedEligibility] = useState<
    "all" | "eligible" | "ineligible"
  >("all");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    if (!offerId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    marketplaceService
      .getOffer(offerId)
      .then((detail) => {
        if (cancelled) return null;
        setOffer(detail);
        return marketplaceService.listCandidates(detail.offer.requestProjectionId);
      })
      .then((list) => {
        if (cancelled || !list) return;
        setCandidates(list);
      })
      .catch((err) => {
        if (cancelled) return;
        const status = err?.response?.status;
        if (status === 404) {
          setError("Offer not found.");
        } else {
          const message =
            err?.response?.data?.error?.message ??
            err?.message ??
            "Failed to load candidates.";
          setError(message);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [offerId]);

  const eligibleCount = useMemo(
    () => candidates.filter((c) => c.eligible).length,
    [candidates],
  );
  const ineligibleCount = useMemo(
    () => candidates.filter((c) => !c.eligible).length,
    [candidates],
  );

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return candidates.filter((c) => {
      if (selectedEligibility === "eligible" && !c.eligible) return false;
      if (selectedEligibility === "ineligible" && c.eligible) return false;
      if (q === "") return true;
      const haystack = [c.membershipId, c.providerId, c.serviceCategory]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [candidates, search, selectedEligibility]);

  const hasActiveFilters = selectedEligibility !== "all" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedEligibility("all");
  };

  // ---- Loading skeleton ----
  if (loading) {
    return (
      <div className="mx-auto max-w-[1400px] space-y-5">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="flex items-center gap-3">
            <div className="h-9 w-9 rounded-lg border border-line bg-surface" />
            <div>
              <div className="h-5 w-52 rounded bg-surface-2" />
              <div className="mt-1.5 h-3 w-32 rounded bg-surface-2" />
            </div>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
          {Array.from({ length: 2 }).map((_, i) => (
            <div
              key={i}
              className="rounded-lg border border-line bg-surface p-4"
            >
              <div className="h-3 w-20 rounded bg-surface-2" />
              <div className="mt-3 h-8 w-12 rounded bg-surface-2" />
            </div>
          ))}
        </div>
        <div className="rounded-lg border border-line bg-surface p-16">
          <div className="mx-auto h-4 w-40 rounded bg-surface-2" />
        </div>
      </div>
    );
  }

  // ---- Error / not-found ----
  if (error || !offer) {
    return (
      <div className="mx-auto max-w-[1400px] space-y-5">
        <div className="flex items-center gap-3">
          <Link
            href="/operations/marketplace"
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 transition-colors hover:bg-surface-2 hover:text-ink-900"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>Eligible candidates</h1>
            <p className="font-mono text-sm text-ink-500">{offerId}</p>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/trips-empty.png"
            title={error ?? "Offer not found"}
            description="This offer may have expired, been cancelled, or been accepted by another organization."
            actionHref="/operations/marketplace"
            actionLabel="Back to marketplace"
          />
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-[1400px] space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href={`/operations/marketplace/${offerId}`}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-600 transition-colors hover:bg-surface-2 hover:text-ink-900"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>Eligible candidates</h1>
            <p className="mt-1.5 text-sm text-ink-500">
              Providers eligible for{" "}
              <span className="font-mono text-ink-800">{offer.offer.offerId}</span>
              {" · "}
              {serviceLabel(offer.offer.sourceService)}
            </p>
          </div>
        </div>
      </div>

      {/* ── Stats cards ── */}
      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2">
        <div className="rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <CheckCircle2 className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">
              Eligible
            </span>
          </div>
          <div className="mt-3 font-display text-3xl font-bold tracking-[-0.02em] text-ink-950">
            {eligibleCount}
          </div>
        </div>

        <div className="rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <XCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">
              Ineligible
            </span>
          </div>
          <div className="mt-3 font-display text-3xl font-bold tracking-[-0.02em] text-ink-950">
            {ineligibleCount}
          </div>
        </div>
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search candidates..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || selectedEligibility !== "all"
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {selectedEligibility !== "all" && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              1
            </span>
          )}
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
          {filtered.length} candidates
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Eligibility
              </span>
              <div className="flex flex-wrap gap-1.5">
                {[
                  { value: "all", label: "All" },
                  { value: "eligible", label: "Eligible" },
                  { value: "ineligible", label: "Ineligible" },
                ].map((option) => (
                  <button
                    key={option.value}
                    onClick={() =>
                      setSelectedEligibility(
                        option.value as "all" | "eligible" | "ineligible",
                      )
                    }
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedEligibility === option.value
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {option.label}
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
        {candidates.length === 0 ? (
          <EmptyState
            image="/images/vehicles-empty.png"
            title="No eligible candidates"
            description="There are no provider memberships available for this request right now."
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            image="/images/vehicles-empty.png"
            title="No candidates match your filters"
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
                  Candidate
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Provider ID
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Service
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Eligibility
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">
                  Action
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((c) => (
                <tr
                  key={c.membershipId}
                  className="border-b border-line transition-colors hover:bg-surface-2"
                >
                  <td className="px-4 py-3.5 whitespace-nowrap">
                    <div className="flex items-center gap-2.5">
                      <div
                        className={`flex h-9 w-9 items-center justify-center rounded-md flex-shrink-0 ${
                          c.eligible
                            ? "bg-green-bg text-green"
                            : "bg-red-bg text-red"
                        }`}
                      >
                        <User className="h-[18px] w-[18px]" />
                      </div>
                      <div>
                        <span className="font-mono text-sm font-semibold text-ink-950">
                          {c.membershipId}
                        </span>
                        <div className="text-[10px] text-ink-400">
                          Membership
                        </div>
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-3.5 font-mono text-sm text-ink-700 whitespace-nowrap">
                    {c.providerId || "—"}
                  </td>
                  <td className="px-4 py-3.5 whitespace-nowrap">
                    {serviceLabel(c.serviceCategory)}
                  </td>
                  <td className="px-4 py-3.5 whitespace-nowrap">
                    {c.eligible ? (
                      <div className="flex items-center gap-1.5">
                        <CheckCircle2 className="h-3.5 w-3.5 flex-shrink-0 text-green" />
                        <span className="font-semibold text-green">
                          Eligible
                        </span>
                      </div>
                    ) : (
                      <div className="flex items-start gap-1.5">
                        <AlertCircle className="mt-0.5 h-3.5 w-3.5 flex-shrink-0 text-red" />
                        <div className="flex flex-col">
                          <span className="font-semibold text-red">
                            Ineligible
                          </span>
                          {c.reason && (
                            <span className="text-xs text-ink-500">
                              {reasonLabel(c.reason)}
                            </span>
                          )}
                        </div>
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-3.5 text-right whitespace-nowrap">
                    {c.eligible ? (
                      <Link
                        href={`/operations/marketplace/${offerId}/assign?membershipId=${encodeURIComponent(c.membershipId)}`}
                        className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover whitespace-nowrap"
                      >
                        <UserCheck className="h-3.5 w-3.5" />
                        Select
                      </Link>
                    ) : (
                      <span className="rounded-md bg-surface-2 px-3 py-1.5 text-xs font-medium text-ink-400 cursor-not-allowed whitespace-nowrap">
                        Unavailable
                      </span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}