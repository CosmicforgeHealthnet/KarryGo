// src/app/(dashboard)/operations/dispatch/page.tsx

"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Search,
  SlidersHorizontal,
  X,
  ArrowRight,
  Clock,
  Users,
  Truck,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { controlTowerService } from "@/lib/services/controlTowerService";
import type {
  Board,
  OperationalAlert,
  PendingAssignmentRow,
  RoutePlan,
  TrackingProjection,
  UnassignedWork,
} from "@/lib/types/controlTower";

// ---------------------------------------------------------------------------
// Card model — one shape per column, but each card carries only the fields
// that its source array actually provides.
// ---------------------------------------------------------------------------

type CardKind = "Unassigned" | "Awaiting" | "Active" | "Exception";

type Card = {
  id: string;
  kind: CardKind;
  subtitle: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  status: string;
  updatedAt?: string;
  href?: string;
  critical?: boolean;
};

function unassignedToCard(u: UnassignedWork): Card {
  return {
    id: u.id,
    kind: "Unassigned",
    subtitle: u.actionReason || u.sourceType,
    status: u.status,
  };
}

function pendingToCard(a: PendingAssignmentRow): Card {
  return {
    id: a.id,
    kind: "Awaiting",
    subtitle: a.executionService,
    providerId: a.providerId || undefined,
    vehicleOrTruckReference: a.vehicleOrTruckReference || undefined,
    status: a.status,
    updatedAt: a.responseDeadline,
    href: `/operations/assignments/${encodeURIComponent(a.id)}`,
  };
}

function acceptedToCard(a: PendingAssignmentRow): Card {
  return {
    id: a.id,
    kind: "Active",
    subtitle: a.executionService,
    providerId: a.providerId || undefined,
    vehicleOrTruckReference: a.vehicleOrTruckReference || undefined,
    status: a.status,
    updatedAt: a.responseDeadline,
    href: `/operations/assignments/${encodeURIComponent(a.id)}`,
  };
}

function tripToCard(t: TrackingProjection): Card {
  return {
    id: t.canonicalTripReference,
    kind: "Active",
    subtitle: t.sourceService,
    providerId: t.providerId || undefined,
    vehicleOrTruckReference: t.vehicleOrTruckReference || undefined,
    status: t.operationalStatus || "UNKNOWN",
    updatedAt: t.receivedAt,
    href: `/operations/trips/${encodeURIComponent(t.canonicalTripReference)}`,
  };
}

function routeToCard(r: RoutePlan): Card {
  return {
    id: r.routeReference || r.id,
    kind: "Active",
    subtitle: r.executionService,
    providerId: r.providerId || undefined,
    vehicleOrTruckReference: r.vehicleOrTruckReference || undefined,
    status: r.status,
    updatedAt: r.updatedAt,
    href: `/operations/routes/${encodeURIComponent(r.id)}`,
  };
}

function alertToCard(a: OperationalAlert): Card {
  return {
    id: a.id,
    kind: "Exception",
    subtitle: a.alertType,
    status: a.severity,
    updatedAt: a.createdAt,
    href: `/operations/alerts/${encodeURIComponent(a.id)}`,
    critical: (a.severity || "").toUpperCase() === "CRITICAL",
  };
}

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

type Tone = "blue" | "green" | "amber" | "red" | "gray";

const TONE_DOT: Record<Tone, string> = {
  blue: "bg-primary",
  green: "bg-green",
  amber: "bg-amber",
  red: "bg-red",
  gray: "bg-ink-400",
};

const TONE_PILL: Record<Tone, string> = {
  blue: "bg-primary-bg text-primary",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
  gray: "bg-surface-2 text-ink-500",
};

function statusTone(status: string): Tone {
  const s = (status || "").toUpperCase();
  if (
    s === "ACCEPTED" ||
    s === "READY_FOR_EXECUTION" ||
    s === "IN_PROGRESS" ||
    s === "COMPLETED" ||
    s === "PUBLISHED" ||
    s === "FRESH"
  )
    return "green";
  if (
    s === "PENDING" ||
    s === "PENDING_PROVIDER_RESPONSE" ||
    s === "AGING" ||
    s === "MEDIUM" ||
    s === "HIGH"
  )
    return "amber";
  if (
    s === "DECLINED" ||
    s === "EXPIRED" ||
    s === "CANCELLED" ||
    s === "FAILED" ||
    s === "STALE" ||
    s === "CRITICAL"
  )
    return "red";
  if (s === "EN_ROUTE" || s === "ASSIGNED") return "blue";
  return "gray";
}

function timeAgo(iso: string | undefined): string {
  if (!iso) return "";
  const t = new Date(iso).getTime();
  if (!Number.isFinite(t)) return "";
  const sec = Math.max(1, Math.floor((Date.now() - t) / 1000));
  if (sec < 60) return `${sec}s ago`;
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min}m ago`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.floor(hr / 24);
  return `${day}d ago`;
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

const COLUMN_META: Record<
  CardKind,
  { title: string; tone: Tone; action?: string }
> = {
  Unassigned: { title: "Unassigned", tone: "gray", action: "Assign" },
  Awaiting: { title: "Awaiting provider", tone: "amber", action: "Nudge" },
  Active: { title: "Active", tone: "green", action: "Track" },
  Exception: { title: "Exception", tone: "red", action: "Rescue" },
};

const COLUMN_ORDER: CardKind[] = ["Unassigned", "Awaiting", "Active", "Exception"];

export default function DispatchBoardPage() {
  const [board, setBoard] = useState<Board | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [showFilters, setShowFilters] = useState(false);
  const [onlyCritical, setOnlyCritical] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    controlTowerService
      .getBoard({ limit: 200 })
      .then((data) => {
        if (!cancelled) setBoard(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          "Failed to load dispatch board.";
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const columns = useMemo(() => {
    const empty: Record<CardKind, Card[]> = {
      Unassigned: [],
      Awaiting: [],
      Active: [],
      Exception: [],
    };
    if (!board) return empty;

    empty.Unassigned = board.unassigned.map(unassignedToCard);
    empty.Awaiting = board.pendingAssignments.map(pendingToCard);
    empty.Active = [
      ...board.acceptedAssignments.map(acceptedToCard),
      ...board.activeTrips.map(tripToCard),
      ...board.activeRoutes.map(routeToCard),
    ];
    empty.Exception = board.alerts
      .filter(
        (a) =>
          a.status === "OPEN" ||
          a.status === "ACKNOWLEDGED" ||
          a.status === "IN_PROGRESS",
      )
      .map(alertToCard);
    return empty;
  }, [board]);

  const filteredColumns = useMemo(() => {
    const q = search.trim().toLowerCase();
    const out: Record<CardKind, Card[]> = {
      Unassigned: [],
      Awaiting: [],
      Active: [],
      Exception: [],
    };
    for (const kind of COLUMN_ORDER) {
      out[kind] = columns[kind].filter((c) => {
        if (onlyCritical && !c.critical) return false;
        if (q === "") return true;
        const haystack = [
          c.id,
          c.subtitle,
          c.providerId ?? "",
          c.vehicleOrTruckReference ?? "",
          c.status,
        ]
          .join(" ")
          .toLowerCase();
        return haystack.includes(q);
      });
    }
    return out;
  }, [columns, search, onlyCritical]);

  const pipeline = useMemo(
    () =>
      COLUMN_ORDER.map((kind) => ({
        kind,
        title: COLUMN_META[kind].title,
        tone: COLUMN_META[kind].tone,
        count: columns[kind].length,
      })),
    [columns],
  );

  const totalShown = useMemo(
    () => COLUMN_ORDER.reduce((n, k) => n + filteredColumns[k].length, 0),
    [filteredColumns],
  );

  const totalOnBoard = useMemo(
    () => COLUMN_ORDER.reduce((n, k) => n + columns[k].length, 0),
    [columns],
  );

  return (
    <div className="space-y-5">
      <div>
        <h1>Dispatch board</h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Branch-scoped board across unassigned, pending provider response,
          active and exception work.
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-red/20 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-2 gap-2 rounded-lg border border-line bg-surface p-3 sm:grid-cols-4">
        {pipeline.map((stage) => {
          const dot = TONE_DOT[stage.tone];
          const isCritical = stage.kind === "Exception";
          return (
            <div
              key={stage.kind}
              className={`rounded-md px-3 py-2 ${
                isCritical && stage.count > 0 ? "bg-red-bg" : ""
              }`}
            >
              <div className="flex items-center gap-1.5">
                <span className={`h-1.5 w-1.5 rounded-full ${dot}`} />
                <span className="text-[10px] font-semibold uppercase tracking-[0.08em] text-ink-500">
                  {stage.title}
                </span>
              </div>
              <div
                className={`mt-1 font-display text-2xl font-bold ${
                  isCritical && stage.count > 0 ? "text-red" : "text-ink-950"
                }`}
              >
                {stage.count}
              </div>
            </div>
          );
        })}
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search work, provider, vehicle..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>
        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors ${
            showFilters || onlyCritical
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {onlyCritical && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              1
            </span>
          )}
        </button>
        {(search !== "" || onlyCritical) && (
          <button
            onClick={() => {
              setSearch("");
              setOnlyCritical(false);
            }}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}
        <span className="ml-auto text-sm text-ink-500">{totalShown} shown</span>
      </div>

      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <label className="flex cursor-pointer items-center gap-2 text-sm text-ink-700">
            <input
              type="checkbox"
              checked={onlyCritical}
              onChange={(e) => setOnlyCritical(e.target.checked)}
              className="h-4 w-4 rounded border-line"
            />
            Show only critical items
          </label>
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading dispatch board…
        </div>
      ) : !board ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/live-operations-empty.png"
            title="Dispatch board unavailable"
            description="The dispatch board is not currently responding."
          />
        </div>
      ) : totalOnBoard === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/live-operations-empty.png"
            title="No work on the board"
            description="Unassigned, awaiting, active and exception items will appear here."
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
          {COLUMN_ORDER.map((kind) => {
            const meta = COLUMN_META[kind];
            const dot = TONE_DOT[meta.tone];
            const cards = filteredColumns[kind];
            return (
              <div
                key={kind}
                className="rounded-lg border border-line bg-surface p-3"
              >
                <div className="mb-2.5 flex items-center justify-between px-1">
                  <div className="flex items-center gap-2">
                    <span className={`h-1.5 w-1.5 rounded-full ${dot}`} />
                    <span className="text-sm font-semibold text-ink-950">
                      {meta.title}
                    </span>
                  </div>
                  <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-bold text-ink-600">
                    {cards.length}
                  </span>
                </div>

                <div className="flex flex-col gap-2.5">
                  {cards.map((c) => {
                    const tone = statusTone(c.status);
                    return (
                      <div
                        key={`${c.kind}-${c.id}`}
                        className={`rounded-lg border p-3.5 ${
                          c.critical
                            ? "border-red/30 bg-red-bg"
                            : "border-line bg-surface hover:bg-surface-2"
                        }`}
                      >
                        <div className="flex items-center justify-between">
                          <span className="font-mono text-[10px] text-ink-500 truncate">
                            {c.id}
                          </span>
                          {c.updatedAt && (
                            <span className="text-[10px] text-ink-500 whitespace-nowrap ml-2">
                              {timeAgo(c.updatedAt)}
                            </span>
                          )}
                        </div>

                        <div className="mt-1.5 text-xs text-ink-600 truncate">
                          {c.subtitle}
                        </div>

                        {(c.providerId || c.vehicleOrTruckReference) && (
                          <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-ink-500">
                            {c.providerId && (
                              <span className="flex items-center gap-1">
                                <Users className="h-3 w-3" />
                                <span className="font-mono truncate">
                                  {c.providerId}
                                </span>
                              </span>
                            )}
                            {c.vehicleOrTruckReference && (
                              <span className="flex items-center gap-1">
                                <Truck className="h-3 w-3" />
                                <span className="font-mono truncate">
                                  {c.vehicleOrTruckReference}
                                </span>
                              </span>
                            )}
                          </div>
                        )}

                        <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                          <span
                            className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${TONE_PILL[tone]}`}
                          >
                            {c.status}
                          </span>
                          {c.href ? (
                            <Link
                              href={c.href}
                              className="flex items-center gap-1 text-xs font-semibold text-primary hover:text-primary-hover"
                            >
                              {c.critical ? "Rescue" : meta.action || "View"}
                              <ArrowRight className="h-3 w-3" />
                            </Link>
                          ) : (
                            <span className="flex items-center gap-1 text-xs font-medium text-ink-400">
                              <Clock className="h-3 w-3" />
                              No action
                            </span>
                          )}
                        </div>
                      </div>
                    );
                  })}

                  {cards.length === 0 && (
                    <div className="flex h-24 items-center justify-center rounded-lg border border-dashed border-line text-center text-xs text-ink-500">
                      No items
                    </div>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}