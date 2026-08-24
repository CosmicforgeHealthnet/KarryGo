// src/app/(dashboard)/operations/dispatch/plans/[planId]/page.tsx

"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { DISPATCH_PLANS } from "@/lib/mock-dispatch-data";
import { useState, useEffect } from "react";
import type { DispatchPlan, DispatchJob } from "@/lib/mock-dispatch-data";

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Draft: { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" },
  Locked: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Published: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
};

const JOB_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  "In Progress": { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" },
  Pending: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
};

export default function DispatchPlanDetailPage() {
  const params = useParams();
  const router = useRouter();
  const planId = params.planId as string;

  const [plan, setPlan] = useState<DispatchPlan | null>(null);
  const [loading, setLoading] = useState(true);
  const [jobs, setJobs] = useState<DispatchJob[]>([]);

  useEffect(() => {
    const fetchPlan = async () => {
      setLoading(true);
      try {
        await new Promise((resolve) => setTimeout(resolve, 500));

        const foundPlan = DISPATCH_PLANS.find((p) => p.id === planId);
        if (foundPlan) {
          setPlan(foundPlan);
          setJobs(foundPlan.jobs || []);
        } else {
          router.push("/operations/dispatch/plans");
        }
      } catch (error) {
        console.error("Error fetching plan:", error);
        router.push("/operations/dispatch/plans");
      } finally {
        setLoading(false);
      }
    };

    fetchPlan();
  }, [planId, router]);

  if (loading) {
    return (
      <div className="py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <span className="ml-3 text-sm text-ink-600">Loading plan details...</span>
        </div>
      </div>
    );
  }

  if (!plan) {
    return null;
  }

  const statusStyle = STATUS_STYLES[plan.status] || STATUS_STYLES.Draft;

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>{plan.name}</h1>
          <p className="mt-1.5 text-sm text-ink-500">
            {plan.branch} · {plan.window} · Owner: {plan.owner} · {plan.id}
          </p>
        </div>
        <Link
          href="/operations/dispatch/plans"
          className="text-sm font-medium text-ink-600 hover:text-primary transition-colors"
        >
          ← Back to plans
        </Link>
      </div>

      {/* ── Status ── */}
      <div className="flex items-center gap-3">
        <span className={`flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
          <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
          {plan.status}
        </span>
        <span className="text-sm text-ink-500">{plan.jobCount} total jobs</span>
      </div>

      {/* ── Jobs Table ── */}
      <div className="overflow-x-auto rounded-lg border border-line bg-white">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <span className="text-sm font-semibold text-ink-950">
            Included work ({jobs.length})
          </span>
          <button className="text-sm font-medium text-primary hover:text-primary-hover transition-colors">
            + Add job
          </button>
        </div>

        {jobs.length === 0 ? (
          <div className="py-12 text-center text-ink-500">
            <p>No jobs assigned to this plan yet</p>
            <button className="mt-2 text-sm font-medium text-primary hover:text-primary-hover">
              Add your first job
            </button>
          </div>
        ) : (
          <table className="w-full border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-1">
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Job</th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Status</th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Location</th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">Provider</th>
                <th className="px-4 py-2.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right">Action</th>
              </tr>
            </thead>
            <tbody>
              {jobs.map((job) => {
                const jobStyle = JOB_STATUS_STYLES[job.status] || JOB_STATUS_STYLES.Pending;
                return (
                  <tr key={job.id} className="border-b border-line transition-colors hover:bg-surface-1/50">
                    <td className="px-4 py-3">
                      <div>
                        <span className="font-mono text-[10px] text-ink-400">{job.id}</span>
                        <div className="text-sm font-medium text-ink-950">{job.title}</div>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${jobStyle.bg} ${jobStyle.text}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${jobStyle.dot}`} />
                        {job.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm text-ink-600">{job.location}</td>
                    <td className="px-4 py-3 text-sm font-medium text-ink-700">{job.provider}</td>
                    <td className="px-4 py-3 text-right">
                      <button className="rounded-md border border-line px-3 py-1 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors">
                        View
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>

      {/* ── Actions ── */}
      <div className="flex flex-wrap gap-2.5">
        {plan.status === "Draft" && (
          <>
            <button className="rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white hover:bg-primary-hover transition-colors">
              Publish Plan
            </button>
            <button className="rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-semibold text-ink-700 hover:bg-surface-2 transition-colors">
              Edit Plan
            </button>
          </>
        )}
        {plan.status === "Locked" && (
          <button className="rounded-lg bg-primary px-4 py-2.5 text-sm font-semibold text-white hover:bg-primary-hover transition-colors">
            Unlock Plan
          </button>
        )}
        {plan.status === "Published" && (
          <button className="rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm font-semibold text-red hover:bg-red-bg/70 transition-colors">
            Archive Plan
          </button>
        )}
        <button className="rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-semibold text-ink-700 hover:bg-surface-2 transition-colors">
          Export Data
        </button>
      </div>
    </div>
  );
}