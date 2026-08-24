// src/app/(dashboard)/providers/[providerId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import {
  MapPin,
  Truck,
  Package,
  Clock,
  Phone,
  Mail,
  Star,
  Calendar,
  Activity,
  Briefcase,
  Target,
  Award,
  Zap,
  FileText,
  User,
} from "lucide-react";
import { PROVIDER_DETAIL } from "@/lib/mock-provider-data";

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

export default function ProviderOverviewTab() {
  const params = useParams();
  const providerId = params.providerId as string;
  const [loading, setLoading] = useState(true);
  const [provider, setProvider] = useState(PROVIDER_DETAIL);

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading...</span>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ── Stats Row ── */}
      <div className="grid grid-cols-3 gap-4">
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 items-center justify-center rounded-md bg-amber-bg text-amber">
              <Star className="h-[18px] w-[18px] fill-amber" />
            </div>
            <div>
              <span className="text-xs font-medium text-ink-500">Rating</span>
              <div className="text-xl font-bold text-ink-950">{provider.rating}</div>
            </div>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 items-center justify-center rounded-md bg-primary-bg text-primary">
              <Briefcase className="h-[18px] w-[18px]" />
            </div>
            <div>
              <span className="text-xs font-medium text-ink-500">Jobs Completed</span>
              <div className="text-xl font-bold text-ink-950">{provider.completedJobs}</div>
            </div>
          </div>
        </div>
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex items-center gap-2.5">
            <div className="flex h-9 w-9 items-center justify-center rounded-md bg-green-bg text-green">
              <Target className="h-[18px] w-[18px]" />
            </div>
            <div>
              <span className="text-xs font-medium text-ink-500">Acceptance Rate</span>
              <div className="text-xl font-bold text-ink-950">{provider.acceptanceRate}%</div>
            </div>
          </div>
        </div>
      </div>

      {/* ── Main Content ── */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-3">
        {/* ── Left: Contact + Performance ── */}
        <div className="lg:col-span-2 space-y-5">
          {/* Contact */}
          <div className="rounded-lg border border-line bg-white p-5">
            <h2 className="text-sm font-semibold text-ink-950 mb-3">Contact</h2>
            <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
              <div className="flex items-center gap-2 text-sm text-ink-700">
                <Phone className="h-4 w-4 text-ink-400" />
                {provider.phone}
              </div>
              <div className="flex items-center gap-2 text-sm text-ink-700">
                <Mail className="h-4 w-4 text-ink-400" />
                {provider.email}
              </div>
              <div className="flex items-center gap-2 text-sm text-ink-700">
                <MapPin className="h-4 w-4 text-ink-400" />
                {provider.branch}
              </div>
              <div className="flex items-center gap-2 text-sm text-ink-700">
                <Calendar className="h-4 w-4 text-ink-400" />
                Joined {provider.memberSince}
              </div>
              <div className="flex items-center gap-2 text-sm text-ink-700">
                <Clock className="h-4 w-4 text-ink-400" />
                Last active {provider.lastActive}
              </div>
            </div>
          </div>

          {/* Performance */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center gap-2.5 mb-4">
              <Award className="h-5 w-5 text-primary" />
              <h2 className="text-sm font-semibold text-ink-950">Performance</h2>
            </div>
            <div className="space-y-3">
              <div>
                <div className="flex justify-between text-xs">
                  <span className="text-ink-500">Acceptance Rate</span>
                  <span className="font-semibold text-ink-950">{provider.acceptanceRate}%</span>
                </div>
                <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                  <div className="h-full rounded-full bg-green" style={{ width: `${provider.acceptanceRate}%` }} />
                </div>
              </div>
              <div>
                <div className="flex justify-between text-xs">
                  <span className="text-ink-500">On-Time Delivery</span>
                  <span className="font-semibold text-ink-950">{provider.onTimeRate}%</span>
                </div>
                <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                  <div className="h-full rounded-full bg-primary" style={{ width: `${provider.onTimeRate}%` }} />
                </div>
              </div>
              <div>
                <div className="flex justify-between text-xs">
                  <span className="text-ink-500">Utilization</span>
                  <span className="font-semibold text-ink-950">{provider.utilization}%</span>
                </div>
                <div className="mt-1 h-1.5 w-full rounded-full bg-surface-3 overflow-hidden">
                  <div className="h-full rounded-full bg-amber" style={{ width: `${provider.utilization}%` }} />
                </div>
              </div>
            </div>
          </div>
        </div>

        {/* ── Right: Activity + Actions ── */}
        <div className="space-y-5">
          {/* Activity */}
          <div className="rounded-lg border border-line bg-white p-5">
            <div className="flex items-center gap-2.5 mb-3">
              <Zap className="h-5 w-5 text-amber" />
              <h2 className="text-sm font-semibold text-ink-950">Activity</h2>
            </div>
            <div className="grid grid-cols-2 gap-2">
              <div className="rounded-md bg-surface-2 px-3 py-2.5 text-center">
                <div className="text-[10px] font-medium text-ink-500">Today</div>
                <div className="text-base font-bold text-ink-950">{provider.todayJobs}</div>
              </div>
              <div className="rounded-md bg-surface-2 px-3 py-2.5 text-center">
                <div className="text-[10px] font-medium text-ink-500">Week</div>
                <div className="text-base font-bold text-ink-950">{provider.weekJobs}</div>
              </div>
              <div className="rounded-md bg-surface-2 px-3 py-2.5 text-center">
                <div className="text-[10px] font-medium text-ink-500">Month</div>
                <div className="text-base font-bold text-ink-950">{provider.monthJobs}</div>
              </div>
              <div className="rounded-md bg-surface-2 px-3 py-2.5 text-center">
                <div className="text-[10px] font-medium text-ink-500">Earnings</div>
                <div className="text-base font-bold text-ink-950">₦{provider.earnings}</div>
              </div>
            </div>
          </div>

          {/* Actions */}
          <div className="rounded-lg border border-line bg-white p-5">
            <h2 className="text-sm font-semibold text-ink-950 mb-3">Actions</h2>
            <div className="space-y-2">
              <button className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
                <Truck className="h-4 w-4" />
                Assign to Trip
              </button>
              <button className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
                <Calendar className="h-4 w-4" />
                View Schedule
              </button>
              <button className="flex w-full items-center gap-2 rounded-md border border-line bg-white px-4 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2">
                <FileText className="h-4 w-4" />
                View History
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}