// src/app/(dashboard)/operations/dispatch/page.tsx

"use client";

import { useState, useEffect } from "react";
import {
  Search,
  ChevronDown,
  RefreshCw,
  LifeBuoy,
  ArrowRight,
  MapPin,
  SlidersHorizontal,
} from "lucide-react";

// ✅ FIX: Add type for TONE
type ToneStyles = {
  [key: string]: { dot: string; text: string; bg: string };
};

const TONE: ToneStyles = {
  neutral: { dot: "bg-ink-400", text: "text-ink-600", bg: "bg-surface-2" },
  warning: { dot: "bg-amber", text: "text-amber", bg: "bg-amber-bg" },
  info: { dot: "bg-accent", text: "text-accent", bg: "bg-accent-bg" },
  success: { dot: "bg-green", text: "text-green", bg: "bg-green-bg" },
  critical: { dot: "bg-red", text: "text-red", bg: "bg-red-bg" },
  done: { dot: "bg-ink-400", text: "text-ink-500", bg: "bg-surface-2" },
};

// ✅ FIX: Add type for Pipeline items
type PipelineItem = {
  key: string;
  label: string;
  count: number;
  tone: string;
};

const PIPELINE: PipelineItem[] = [
  { key: "unassigned", label: "Unassigned", count: 3, tone: "neutral" },
  { key: "awaiting", label: "Awaiting", count: 2, tone: "warning" },
  { key: "ready", label: "Ready", count: 2, tone: "info" },
  { key: "active", label: "Active", count: 11, tone: "success" },
  { key: "exception", label: "Exception", count: 2, tone: "critical" },
  { key: "completed", label: "Completed", count: 24, tone: "done" },
];


type Job = {
  id: string;
  title: string;
  route: string;
  provider: string | null;
  meta: string;
  time: string;
  critical?: boolean;
};

type Column = {
  key: string;
  title: string;
  tone: string;
  action: string;
  live?: boolean;
  jobs: Job[];
};

const COLUMNS: Column[] = [
  {
    key: "unassigned",
    title: "Unassigned",
    tone: "neutral",
    action: "Assign",
    jobs: [
      { id: "KG-8532", title: "Priority delivery", route: "Ikeja → Lekki", provider: null, meta: "Fragile · 2 parcels", time: "14:10" },
      { id: "KG-8528", title: "Retail replenishment", route: "Yaba → Surulere", provider: null, meta: "Bike · 7.2 kg", time: "14:25" },
      { id: "KG-8521", title: "Private shipment", route: "Ikeja → VI", provider: null, meta: "Cold · 3 parcels", time: "14:40" },
    ],
  },
  {
    key: "awaiting",
    title: "Awaiting provider",
    tone: "warning",
    action: "Nudge",
    jobs: [
      { id: "KG-8518", title: "Parcel mission", route: "Lekki → Ikoyi", provider: "P. Okafor", meta: "Expires 02:18", time: "14:15" },
      { id: "KG-8512", title: "Hauling job", route: "Apapa → Ikeja", provider: "M. Bello", meta: "Van", time: "14:30" },
    ],
  },
  {
    key: "ready",
    title: "Ready",
    tone: "info",
    action: "Dispatch",
    jobs: [
      { id: "KG-8507", title: "Dispatch mission", route: "Yaba → Ikeja", provider: "Bike BR-88", meta: "Provider ready", time: "14:05" },
      { id: "KG-8504", title: "Bulk move", route: "Lekki → Marina", provider: "Truck TR-12", meta: "Manifest locked", time: "14:50" },
    ],
  },
  {
    key: "active",
    title: "Active",
    tone: "success",
    action: "Track",
    live: true,
    jobs: [
      { id: "KG-8491", title: "Express dispatch", route: "Ikeja → Lekki", provider: "P. Okafor", meta: "66% complete", time: "13:42" },
      { id: "KG-8487", title: "Hauling mission", route: "Apapa → Ikoyi", provider: "M. Bello", meta: "At pickup", time: "13:51" },
    ],
  },
  {
    key: "exception",
    title: "Exception",
    tone: "critical",
    action: "Rescue",
    jobs: [
      { id: "KG-8470", title: "Vehicle blocked", route: "Ikeja", provider: null, meta: "Safety block", time: "13:38", critical: true },
      { id: "KG-8466", title: "Receiver unavailable", route: "Lekki", provider: null, meta: "Attempt 1", time: "13:29" },
    ],
  },
  {
    key: "completed",
    title: "Completed",
    tone: "done",
    action: "View",
    jobs: [
      { id: "KG-8443", title: "Same-day delivery", route: "VI → Ikoyi", provider: "T. Adeyemi", meta: "Proof captured", time: "12:58" },
      { id: "KG-8431", title: "Retail restock", route: "Yaba → Ebute Metta", provider: "B. Chukwu", meta: "Proof captured", time: "12:20" },
    ],
  },
];

function initials(name: string | null) {
  if (!name) return null;
  return name.split(" ").map((p) => p[0]).join("").slice(0, 2).toUpperCase();
}

export default function DispatchBoardPage() {
  const [search, setSearch] = useState("");

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Dispatch board</h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Branch-scoped board across unassigned, pending provider response, ready, active, exception and completed work.
        </p>
      </div>

      {/* ── Pipeline flow strip ── */}
      <div className="grid grid-cols-3 gap-2 rounded-lg border border-line bg-white p-3 sm:grid-cols-6">
        {PIPELINE.map((stage) => {
    
          const tone = TONE[stage.tone] || TONE.neutral;
          const flagged = stage.key === "exception";
          return (
            <div key={stage.key} className="flex items-center">
              <div className={`flex-1 rounded-md px-3 py-2 ${flagged ? `${tone.bg}` : ""}`}>
                <div className="flex items-center gap-1.5">
                  <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                  <span className="text-[10px] font-semibold uppercase tracking-[0.08em] text-ink-500">
                    {stage.label}
                  </span>
                </div>
                <div className={`mt-1 font-display text-2xl font-bold ${flagged ? tone.text : "text-ink-950"}`}>
                  {stage.count}
                </div>
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
            placeholder="Search work, provider, route..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>
        <button className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3 text-sm font-medium text-ink-700 hover:bg-surface-2 transition-colors">
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
        </button>
        <span className="flex h-10 items-center rounded-lg bg-primary-bg px-3 text-sm font-medium text-primary">
          Urgent only
        </span>
      </div>

      {/* ── Board ── */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-6">
        {COLUMNS.map((col) => {
        
          const tone = TONE[col.tone] || TONE.neutral;
          const jobs = col.jobs.filter(
            (j) =>
              search.trim() === "" ||
              j.id.toLowerCase().includes(search.toLowerCase()) ||
              j.title.toLowerCase().includes(search.toLowerCase()) ||
              j.route.toLowerCase().includes(search.toLowerCase())
          );

          return (
            <div key={col.key} className="rounded-lg border border-line bg-white p-3">
              <div className="mb-2.5 flex items-center justify-between px-1">
                <div className="flex items-center gap-2">
                  <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                  <span className="text-sm font-semibold text-ink-950">{col.title}</span>
                </div>
                <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-bold text-ink-600">
                  {jobs.length}
                </span>
              </div>

              <div className="flex flex-col gap-2.5">
                {jobs.map((job) => (
                  <div
                    key={job.id}
                    className={`rounded-lg border p-3.5 ${
                      job.critical
                        ? "border-red/30 bg-red-bg"
                        : "border-line bg-white hover:bg-surface-2"
                    }`}
                  >
                    <div className="flex items-center justify-between">
                      <span className="font-mono text-[10px] text-ink-500">{job.id}</span>
                      <span className="text-[10px] text-ink-500">{job.time}</span>
                    </div>

                    <div className="mt-1.5 flex items-start gap-2">
                      {job.provider && (
                        <div className="mt-0.5 flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-primary-bg text-[9px] font-bold text-primary">
                          {initials(job.provider)}
                        </div>
                      )}
                      <div className="min-w-0">
                        <div className="truncate text-sm font-semibold text-ink-950">{job.title}</div>
                        <div className="mt-0.5 truncate text-xs text-ink-600">{job.route}</div>
                      </div>
                    </div>

                    <div className={`mt-2 text-[10px] font-medium ${job.critical ? "text-red" : "text-ink-500"}`}>
                      {job.meta}
                    </div>

                    <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                      {col.live ? (
                        <span className="flex items-center gap-1.5 text-[10px] font-semibold text-green">
                          <span className="relative flex h-1.5 w-1.5">
                            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green opacity-75" />
                            <span className="relative inline-flex h-1.5 w-1.5 rounded-full bg-green" />
                          </span>
                          Live
                        </span>
                      ) : (
                        <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}>
                          {col.title}
                        </span>
                      )}

                      {job.critical ? (
                        <button className="flex items-center gap-1 rounded-full bg-red px-2.5 py-1 text-[10px] font-semibold text-white">
                          <LifeBuoy className="h-3 w-3" />
                          Rescue
                        </button>
                      ) : (
                        <button className="flex items-center gap-1 text-xs font-semibold text-primary">
                          {col.action}
                          <ArrowRight className="h-3 w-3" />
                        </button>
                      )}
                    </div>
                  </div>
                ))}

                {jobs.length === 0 && (
                  <div className="flex h-24 items-center justify-center rounded-lg border border-dashed border-line text-center text-xs text-ink-500">
                    No matching work
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}