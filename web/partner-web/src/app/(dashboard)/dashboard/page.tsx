"use client";

import { useEffect, useState } from "react";
import { StatCardRow } from "@/components/dashboard/stat-card";
import { ActivityFeed } from "@/components/dashboard/activity-feed";
import { dashboardService } from "@/services/dashboard-service";
import { useAuthStore } from "@/store/auth-store";
import { ApiError } from "@/lib/api-client";
import type { ActivityItem, DashboardSummary } from "@/types";

export default function DashboardPage() {
  const token = useAuthStore((s) => s.token);
  const account = useAuthStore((s) => s.account);

  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [activity, setActivity] = useState<ActivityItem[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!token) {
      setIsLoading(false);
      return;
    }

    let cancelled = false;

    async function load(currentToken: string) {
      setIsLoading(true);
      setError(null);
      try {
        const [summaryRes, activityRes] = await Promise.all([
          dashboardService.getSummary(currentToken),
          dashboardService.getRecentActivity(currentToken),
        ]);
        if (cancelled) return;
        setSummary(summaryRes);
        setActivity(activityRes);
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof ApiError
            ? err.message
            : "Couldn't load your dashboard. Please try again."
        );
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    }

    load(token);
    return () => {
      cancelled = true;
    };
  }, [token]);

  const partnerName = account?.business_name || "Partner";

  return (
    <div>
      <h1 className="text-2xl font-bold text-ink-900">
        Welcome back, {partnerName}
      </h1>
      <p className="mt-1 text-sm text-ink-500">
        Here&apos;s what&apos;s on your counter today.
      </p>

      <div className="mt-6">
        {isLoading ? (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            {[0, 1, 2].map((i) => (
              <div
                key={i}
                className="h-[124px] animate-pulse rounded-2xl border border-ink-100 bg-ink-100/60"
              />
            ))}
          </div>
        ) : error ? (
          <div className="rounded-2xl border border-red-100 bg-red-50 p-5 text-sm text-red-600">
            {error}
          </div>
        ) : summary ? (
          <StatCardRow summary={summary} />
        ) : null}
      </div>

      <div className="mt-8 flex items-center justify-between">
        <h2 className="text-lg font-bold text-ink-900">Recent Activity</h2>
        <a href="#" className="text-sm font-semibold text-brand-500">
          View All
        </a>
      </div>

      <div className="mt-4">
        {!isLoading && !error && <ActivityFeed items={activity} />}
      </div>
    </div>
  );
}