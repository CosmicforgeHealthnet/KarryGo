// src/app/(dashboard)/providers/[providerId]/layout.tsx

"use client";

import Link from "next/link";
import { usePathname, useParams } from "next/navigation";
import {
  User,
  MapPin,
  Truck,
  Package,
  Clock,
  CheckCircle2,
  AlertCircle,
  Edit3,
  GitBranch,
  Rocket,
  TrendingUp,
  Layers,
  UserCheck,
  UserX,
  Shield,
  Calendar,
  Phone,
  Mail,
  Star,
  MoreVertical,
  Settings,
  Activity,
  FileText,
  Briefcase,
  Users,
} from "lucide-react";
import { PROVIDER_DETAIL } from "@/lib/mock-provider-data";

const TABS = [
  { label: "Overview", segment: "", icon: Layers },
  { label: "Work Setup", segment: "work-setup", icon: Briefcase },
  { label: "Availability", segment: "availability", icon: Activity },
  { label: "Declines", segment: "declines", icon: FileText },
  { label: "Compliance", segment: "compliance", icon: Shield },
  { label: "Membership", segment: "membership", icon: Users },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Online: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Busy: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Offline: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Leave: { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
};

const ELIGIBILITY_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Eligible: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Warning: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Blocked: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function ProviderDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const providerId = params.providerId as string;
  const provider = PROVIDER_DETAIL;

  const basePath = `/providers/${providerId}`;
  const statusStyle = STATUS_STYLES[provider.status] || STATUS_STYLES.Offline;
  const eligibilityStyle = ELIGIBILITY_STYLES[provider.eligibility] || ELIGIBILITY_STYLES.Eligible;

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div>
        <div className="flex items-center gap-2.5 flex-wrap">
          <h1>{provider.name}</h1>
          <span className="font-mono text-sm text-ink-400">{providerId}</span>
          <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
            <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
            {provider.status}
          </span>
          <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${eligibilityStyle.bg} ${eligibilityStyle.text}`}>
            <span className={`h-1.5 w-1.5 rounded-full ${eligibilityStyle.dot}`} />
            {provider.eligibility}
          </span>
        </div>
        <p className="mt-0.5 text-sm text-ink-500">
          {provider.service} · {provider.branch} branch
        </p>
      </div>

      {/* ── Tabs ── */}
      <div className="flex gap-1 border-b border-line overflow-x-auto scrollbar-hide">
        {TABS.map((tab) => {
          const href = tab.segment ? `${basePath}/${tab.segment}` : basePath;
          const active = pathname === href;
          const Icon = tab.icon;

          return (
            <Link
              key={tab.label}
              href={href}
              className={`flex items-center gap-1.5 px-3.5 py-2.5 text-sm font-medium whitespace-nowrap transition-colors ${
                active
                  ? "text-primary border-b-2 border-primary"
                  : "text-ink-600 hover:text-ink-950"
              }`}
            >
              <Icon className="h-4 w-4" />
              {tab.label}
            </Link>
          );
        })}
      </div>

      {/* ── Children ── */}
      <div>{children}</div>
    </div>
  );
}