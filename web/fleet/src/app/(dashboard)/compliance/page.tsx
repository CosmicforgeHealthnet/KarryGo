// src/app/(dashboard)/compliance/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Shield,
  Clock,
  AlertCircle,
  Eye,
  ArrowRight,
  Users,
  Download,
  FileText,
  Plus,
  Building2,
  AlertTriangle,
  Info,
  FileCheck,
  ListChecks,
  FileSearch,
  ClipboardList,
  FileSignature,
  BookOpen,
} from "lucide-react";

type ComplianceStats = {
  totalRestrictions: number;
  activeRestrictions: number;
  openIncidents: number;
  criticalIncidents: number;
  expiringDocuments: number;
  clearedThisWeek: number;
  complianceRate: number;
  subjects: {
    providers: number;
    assets: number;
    documents: number;
  };
};

type RecentAlert = {
  id: string;
  type: "Restriction" | "Incident" | "Document" | "Safety";
  title: string;
  severity: "Critical" | "High" | "Medium" | "Low";
  timestamp: string;
  subject: string;
};

type ComplianceRequirement = {
  id: string;
  name: string;
  subject: "Provider" | "Asset" | "Document";
  status: "Compliant" | "Warning" | "NonCompliant";
  dueDate: string;
  count: number;
};

const MOCK_STATS: ComplianceStats = {
  totalRestrictions: 11,
  activeRestrictions: 8,
  openIncidents: 6,
  criticalIncidents: 2,
  expiringDocuments: 19,
  clearedThisWeek: 24,
  complianceRate: 87,
  subjects: {
    providers: 4,
    assets: 3,
    documents: 19,
  },
};

const MOCK_ALERTS: RecentAlert[] = [
  { id: "ALT-001", type: "Restriction", title: "Vehicle BR-204 safety block", severity: "Critical", timestamp: "1 hour ago", subject: "BR-204" },
  { id: "ALT-002", type: "Incident", title: "Road incident - Provider P-044", severity: "High", timestamp: "3 hours ago", subject: "P-044" },
  { id: "ALT-003", type: "Document", title: "Licence expiry - Provider P-052", severity: "Medium", timestamp: "2 days ago", subject: "P-052" },
  { id: "ALT-004", type: "Safety", title: "Safety report filed - Route KG-8491", severity: "Low", timestamp: "4 hours ago", subject: "KG-8491" },
];

const MOCK_REQUIREMENTS: ComplianceRequirement[] = [
  { id: "REQ-001", name: "Vehicle Safety Inspection", subject: "Asset", status: "Compliant", dueDate: "2026-09-15", count: 42 },
  { id: "REQ-002", name: "Provider Certification", subject: "Provider", status: "Warning", dueDate: "2026-09-30", count: 8 },
  { id: "REQ-003", name: "Document Renewal", subject: "Document", status: "NonCompliant", dueDate: "2026-08-25", count: 19 },
  { id: "REQ-004", name: "Safety Training", subject: "Provider", status: "Warning", dueDate: "2026-10-15", count: 12 },
  { id: "REQ-005", name: "Asset Registration", subject: "Asset", status: "Compliant", dueDate: "2026-12-31", count: 45 },
];

const SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Critical: { bg: "bg-red-bg", text: "text-red", dot: "bg-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary", icon: <Info className="h-3.5 w-3.5" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400", icon: <Info className="h-3.5 w-3.5" /> },
};

const SUBJECT_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Provider: { bg: "bg-primary-bg", text: "text-primary", icon: <Users className="h-3.5 w-3.5" /> },
  Asset: { bg: "bg-green-bg", text: "text-green", icon: <Building2 className="h-3.5 w-3.5" /> },
  Document: { bg: "bg-amber-bg", text: "text-amber", icon: <FileText className="h-3.5 w-3.5" /> },
};

const REQUIREMENT_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Compliant: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Warning: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  NonCompliant: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function ComplianceOverviewPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setLoading(false);
    };
    loadData();
  }, []);

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading compliance data...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Compliance Overview</h1>
          <p className="mt-1.5 text-sm text-ink-600">
            Organization, provider and asset compliance status, upcoming expiries, restrictions and unresolved issues.
          </p>
        </div>
        <div className="flex items-center gap-2.5">
          <button className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-white px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
        </div>
      </div>

      {/* ── Stats ── */}
      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-red-bg text-red">
              <AlertCircle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Active Restrictions</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-red">
            {MOCK_STATS.activeRestrictions}
          </div>
          <div className="mt-1.5 text-[10px] text-ink-500">{MOCK_STATS.totalRestrictions} total</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <AlertTriangle className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Open Incidents</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-amber">
            {MOCK_STATS.openIncidents}
          </div>
          <div className="mt-1.5 text-[10px] text-red">{MOCK_STATS.criticalIncidents} critical</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Clock className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Expiring Documents</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-amber">
            {MOCK_STATS.expiringDocuments}
          </div>
          <div className="mt-1.5 text-[10px] text-ink-500">Within 30 days</div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-8 w-8 items-center justify-center rounded-md bg-green-bg text-green">
              <Shield className="h-[18px] w-[18px]" />
            </div>
            <span className="text-xs font-semibold text-ink-600">Compliance Rate</span>
          </div>
          <div className="mt-3 font-display text-[26px] font-bold tracking-[-0.02em] text-green">
            {MOCK_STATS.complianceRate}%
          </div>
          <div className="mt-1.5 text-[10px] text-green">+{MOCK_STATS.clearedThisWeek} cleared this week</div>
        </div>
      </div>

      {/* ── Quick Navigation ── */}
      <div className="rounded-lg border border-line bg-white p-5">
        <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500 mb-3">Navigate</div>
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
          {[
            { label: "Requirements", icon: ListChecks, path: "/compliance/requirements" },
            { label: "Provider Compliance", icon: Users, path: "/compliance/providers" },
            { label: "Asset Compliance", icon: Building2, path: "/compliance/assets" },
            { label: "Restrictions", icon: AlertCircle, path: "/compliance/restrictions" },
            { label: "Safety Reports", icon: FileSearch, path: "/compliance/safety-reports" },
            { label: "Incidents", icon: AlertTriangle, path: "/compliance/incidents" },
            { label: "Corrective Actions", icon: ClipboardList, path: "/compliance/corrective-actions" },
            { label: "Proof Policies", icon: FileSignature, path: "/compliance/proof-policies" },
            { label: "Proof Records", icon: FileCheck, path: "/compliance/proof-records" },
            { label: "Safety Reference", icon: BookOpen, path: "/compliance/safety-references" },
          ].map((item) => (
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

      {/* ── Main Grid ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ── Left Column ── */}
        <div className="space-y-5">
          {/* Recent Alerts */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center justify-between mb-3">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Recent</div>
                <div className="text-sm font-semibold text-ink-950">Alerts</div>
              </div>
              <button className="text-xs font-medium text-primary transition-colors hover:text-primary-hover flex items-center gap-1">
                View all <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="space-y-2">
              {MOCK_ALERTS.map((alert) => {
                const severityStyle = SEVERITY_STYLES[alert.severity] || SEVERITY_STYLES.Medium;
                return (
                  <div key={alert.id} className="flex items-center justify-between rounded-lg border border-line p-3 transition-colors hover:bg-surface-2">
                    <div className="flex items-center gap-3">
                      <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                        {severityStyle.icon}
                        {alert.severity}
                      </span>
                      <div>
                        <div className="text-sm font-medium text-ink-950">{alert.title}</div>
                        <div className="text-xs text-ink-500">{alert.subject} · {alert.timestamp}</div>
                      </div>
                    </div>
                    <button className="text-ink-400 hover:text-primary transition-colors">
                      <Eye className="h-4 w-4" />
                    </button>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Requirements Status */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center justify-between mb-3">
              <div>
                <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
                <div className="text-sm font-semibold text-ink-950">Requirements</div>
              </div>
              <button
                onClick={() => router.push("/compliance/requirements")}
                className="text-xs font-medium text-primary transition-colors hover:text-primary-hover flex items-center gap-1"
              >
                View all <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
            <div className="space-y-2">
              {MOCK_REQUIREMENTS.map((req) => {
                const subjectStyle = SUBJECT_STYLES[req.subject] || SUBJECT_STYLES.Document;
                const statusStyle = REQUIREMENT_STATUS_STYLES[req.status] || REQUIREMENT_STATUS_STYLES.Compliant;
                return (
                  <div key={req.id} className="flex items-center justify-between rounded-lg border border-line p-2.5 transition-colors hover:bg-surface-2">
                    <div className="flex items-center gap-2">
                      <span className={subjectStyle.text}>{subjectStyle.icon}</span>
                      <div>
                        <div className="text-sm font-medium text-ink-950">{req.name}</div>
                        <div className="text-xs text-ink-500">{req.subject} · {req.count} items</div>
                      </div>
                    </div>
                    <div className="text-right">
                      <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        <span className={`h-1 w-1 rounded-full ${statusStyle.dot}`} />
                        {req.status}
                      </span>
                      <div className="text-[10px] text-ink-400">Due: {req.dueDate}</div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        </div>

        {/* ── Right Column ── */}
        <div className="space-y-5">
          {/* Subject Summary */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500 mb-3">Summary</div>
            <div className="space-y-2 text-sm">
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Providers</span>
                <span className="font-semibold text-ink-950">{MOCK_STATS.subjects.providers}</span>
              </div>
              <div className="flex items-center justify-between border-b border-line pb-2">
                <span className="text-ink-500">Assets</span>
                <span className="font-semibold text-ink-950">{MOCK_STATS.subjects.assets}</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-ink-500">Documents</span>
                <span className="font-semibold text-ink-950">{MOCK_STATS.subjects.documents}</span>
              </div>
            </div>
          </div>

          {/* Quick Actions */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500 mb-3">Actions</div>
            <div className="space-y-2">
              <button
                onClick={() => router.push("/compliance/requirements/create")}
                className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <Plus className="h-4 w-4" />
                Create Requirement
              </button>
              <button
                onClick={() => router.push("/compliance/safety-reports/create")}
                className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <FileText className="h-4 w-4" />
                File Safety Report
              </button>
              <button
                onClick={() => router.push("/compliance/incidents/report")}
                className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <AlertTriangle className="h-4 w-4" />
                Report Incident
              </button>
              <button
                onClick={() => router.push("/compliance/proof-records/create")}
                className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
              >
                <FileCheck className="h-4 w-4" />
                Record Proof
              </button>
            </div>
          </div>

          {/* Safety First */}
          <div className="rounded-lg border border-primary/20 bg-primary-bg/20 p-4">
            <div className="flex items-start gap-2.5">
              <Shield className="h-5 w-5 flex-shrink-0 text-primary" />
              <div>
                <h4 className="text-sm font-semibold text-primary">Safety First</h4>
                <p className="text-xs text-ink-600 mt-1">
                  Safety concerns are handled through proper compliance workflow. No automatic disciplinary action or deduction is applied.
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}