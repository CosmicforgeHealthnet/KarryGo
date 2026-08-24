// src/app/(dashboard)/operations/assignments/[assignmentId]/layout.tsx

"use client";

import Link from "next/link";
import { usePathname, useParams } from "next/navigation";
import { ASSIGNMENT_DETAIL } from "@/lib/mock-marketplace-data";

const TABS = [
  { label: "Overview", segment: "" },
  { label: "Reassign", segment: "reassign" },
  { label: "History", segment: "history" },
];

const STATUS_STYLE: Record<string, string> = {
  "Pending response": "bg-amber-bg text-amber",
  Accepted: "bg-green-bg text-green",
  Declined: "bg-red-bg text-red",
  Expired: "bg-surface-2 text-ink-600",
  "In Progress": "bg-primary-bg text-primary",
  Completed: "bg-green-bg text-green",
};

export default function AssignmentDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const assignmentId = params.assignmentId as string;
  const assignment = ASSIGNMENT_DETAIL;
  const basePath = `/operations/assignments/${assignmentId}`;

  return (
    <div className="space-y-5">
      {/* ── Header ── */}
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2.5">
            <h1>{assignmentId}</h1>
            <span
              className={`rounded-full px-2.5 py-1 text-xs font-semibold ${STATUS_STYLE[assignment.status]}`}
            >
              {assignment.status}
            </span>
          </div>
          <p className="mt-0.5 text-sm text-ink-500">
            Offer {assignment.offerId} · {assignment.provider} · {assignment.branch}
          </p>
        </div>
        <Link
          href="/operations/assignments"
          className="flex-shrink-0 text-sm font-medium text-ink-600 hover:text-primary transition-colors"
        >
          ← Back to assignments
        </Link>
      </div>

      {/* ── Tabs ── */}
      <div className="flex gap-1 border-b border-line">
        {TABS.map((tab) => {
          const href = tab.segment ? `${basePath}/${tab.segment}` : basePath;
          const active = pathname === href;
          return (
            <Link
              key={tab.label}
              href={href}
              className={`relative px-3.5 py-2.5 text-sm font-semibold transition-colors ${
                active ? "text-primary" : "text-ink-600 hover:text-ink-950"
              }`}
            >
              {tab.label}
              {active && (
                <span className="absolute inset-x-0 -bottom-px h-[2px] rounded-full bg-primary" />
              )}
            </Link>
          );
        })}
      </div>

      {/* ── Content ── */}
      <div>{children}</div>
    </div>
  );
}