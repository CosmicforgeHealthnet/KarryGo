// src/app/(dashboard)/compliance/page.tsx

"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import {
  AlertCircle,
  AlertTriangle,
  BookOpen,
  Building2,
  ClipboardList,
  FileCheck,
  FileSearch,
  FileSignature,
  ListChecks,
  Shield,
  ShieldCheck,
  Users,
} from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { Loading, StatusPill, formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { complianceService, incidentService } from "@/lib/services/safetyService";
import type { ComplianceCheck, Incident, Restriction } from "@/lib/types/safety";

const NAV = [
  { label: "Requirements", icon: ListChecks, path: "/compliance/requirements" },
  { label: "Compliance Checks", icon: ShieldCheck, path: "/compliance/checks" },
  { label: "Provider Compliance", icon: Users, path: "/compliance/providers" },
  { label: "Asset Compliance", icon: Building2, path: "/compliance/assets" },
  { label: "Restrictions", icon: AlertCircle, path: "/compliance/restrictions" },
  { label: "Safety Reports", icon: FileSearch, path: "/compliance/safety-reports" },
  { label: "Incidents", icon: AlertTriangle, path: "/compliance/incidents" },
  { label: "Corrective Actions", icon: ClipboardList, path: "/compliance/corrective-actions" },
  { label: "Proof Policies", icon: FileSignature, path: "/compliance/proof-policies" },
  { label: "Proof Records", icon: FileCheck, path: "/compliance/proof-records" },
  { label: "Safety Reference", icon: BookOpen, path: "/compliance/safety-references" },
];

// Compliance overview from live data: restrictions in force, open incidents
// and the most recent compliance checks.
export default function ComplianceOverviewPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [restrictions, setRestrictions] = useState<Restriction[]>([]);
  const [incidents, setIncidents] = useState<Incident[]>([]);
  const [checks, setChecks] = useState<ComplianceCheck[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  useEffect(() => {
    Promise.allSettled([complianceService.listRestrictions(), incidentService.list(), complianceService.listChecks()]).then(([r, i, c]) => {
      if (r.status === "fulfilled") setRestrictions(r.value);
      if (i.status === "fulfilled") setIncidents(i.value);
      if (c.status === "fulfilled") setChecks(c.value);
      const failed = [r, i, c].find((x) => x.status === "rejected") as PromiseRejectedResult | undefined;
      if (failed) setError(describeApiError(failed.reason, "Some compliance data could not be loaded."));
      setLoading(false);
    });
  }, []);

  if (loading) return <Loading label="Loading compliance data..." />;

  const applied = restrictions.filter((r) => r.state !== "CLEARED");
  const openIncidents = incidents.filter((i) => i.status !== "CLOSED" && i.status !== "RESOLVED");
  const compliantChecks = checks.filter((c) => c.overallState === "COMPLIANT" || c.overallState === "NOT_APPLICABLE").length;
  const rate = checks.length ? Math.round((compliantChecks / checks.length) * 100) : null;

  const cards = [
    { label: "Active Restrictions", value: applied.length, sub: `${restrictions.length} recorded in total`, tone: "text-red", bg: "bg-red-bg", icon: AlertCircle },
    { label: "Open Incidents", value: openIncidents.length, sub: `${openIncidents.filter((i) => i.severity === "CRITICAL").length} critical`, tone: "text-amber", bg: "bg-amber-bg", icon: AlertTriangle },
    { label: "Checks Run", value: checks.length, sub: "Most recent 200", tone: "text-primary", bg: "bg-primary-bg", icon: ShieldCheck },
    { label: "Compliance Rate", value: rate == null ? "—" : `${rate}%`, sub: "Of recent checks", tone: "text-green", bg: "bg-green-bg", icon: Shield },
  ];

  const recent = [
    ...applied.map((r) => ({ id: r.id, kind: "Restriction", title: `${humanize(r.reasonCode)} · ${humanize(r.subjectType)} ${r.subjectReference}`, severity: r.severity, at: r.effectiveAt, path: "/compliance/restrictions" })),
    ...openIncidents.map((i) => ({ id: i.id, kind: "Incident", title: `${humanize(i.incidentType)} · ${i.description}`, severity: i.severity, at: i.occurredAt, path: `/compliance/incidents/${i.id}` })),
  ]
    .sort((a, b) => (a.at < b.at ? 1 : -1))
    .slice(0, 8);

  return (
    <div className="space-y-5">
      <div>
        <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Compliance Overview</h1>
        <p className="mt-1.5 text-sm text-ink-600">Provider and asset compliance, restrictions in force and unresolved incidents.</p>
      </div>

      <ApiErrorNotice error={error} />

      <div className="grid grid-cols-1 gap-3.5 sm:grid-cols-2 lg:grid-cols-4">
        {cards.map((c) => (
          <div key={c.label} className="rounded-lg border border-line bg-white p-4">
            <div className="flex items-center gap-2.5">
              <div className={`flex h-8 w-8 items-center justify-center rounded-md ${c.bg} ${c.tone}`}>
                <c.icon className="h-[18px] w-[18px]" />
              </div>
              <span className="text-xs font-semibold text-ink-600">{c.label}</span>
            </div>
            <div className={`mt-3 font-display text-[26px] font-bold tracking-[-0.02em] ${c.tone}`}>{c.value}</div>
            <div className="mt-1.5 text-[10px] text-ink-500">{c.sub}</div>
          </div>
        ))}
      </div>

      <div className="rounded-lg border border-line bg-white p-5">
        <div className="mb-3 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Navigate</div>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
          {NAV.map((item) => (
            <button
              key={item.label}
              onClick={() => router.push(item.path)}
              className="flex items-center gap-2 rounded-md border border-line bg-white px-3 py-2 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              <item.icon className="h-4 w-4 text-ink-400" />
              {item.label}
            </button>
          ))}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="mb-3 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Needs attention</div>
          {recent.length === 0 ? (
            <p className="text-sm text-ink-500">No active restrictions or open incidents.</p>
          ) : (
            <ul className="divide-y divide-line">
              {recent.map((item) => (
                <li key={`${item.kind}-${item.id}`}>
                  <button onClick={() => router.push(item.path)} className="flex w-full items-center justify-between gap-3 py-2.5 text-left hover:bg-surface-2">
                    <div className="min-w-0">
                      <div className="truncate text-sm font-medium text-ink-950">{item.title}</div>
                      <div className="text-[11px] text-ink-500">
                        {item.kind} · {formatWhen(item.at)}
                      </div>
                    </div>
                    <StatusPill status={item.severity} />
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="mb-3 flex items-center justify-between">
            <span className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Recent checks</span>
            <button onClick={() => router.push("/compliance/checks")} className="text-xs font-medium text-primary">
              Run a check
            </button>
          </div>
          {checks.length === 0 ? (
            <p className="text-sm text-ink-500">No checks run yet.</p>
          ) : (
            <ul className="space-y-2">
              {checks.slice(0, 6).map((c) => (
                <li key={c.id} className="flex items-center justify-between gap-2 text-sm">
                  <span className="truncate text-ink-700">
                    {humanize(c.subjectType)} <span className="font-mono text-[11px]">{c.subjectReference}</span>
                  </span>
                  <StatusPill status={c.overallState} />
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}
