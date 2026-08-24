"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";

type MembershipStatus = "active" | "suspended" | "pending";

type Organization = {
  id: string;
  name: string;
  role: string;
  status: MembershipStatus;
};

// MOCK: replace with the real membership list once fleet-partner-service
// auth endpoints are wired in. Includes one of each status so every UI
// state (active / suspended / pending) stays visible during development.
const MOCK_ORGS: Organization[] = [
  { id: "org_1", name: "Lagos Freight Partners", role: "Operations Manager", status: "active" },
  { id: "org_2", name: "Abuja Cold Chain Ltd", role: "Fleet Manager", status: "suspended" },
  { id: "org_3", name: "Kano Logistics Group", role: "Owner", status: "pending" },
];

const STATUS_LABEL: Record<MembershipStatus, string> = {
  active: "Active",
  suspended: "Suspended",
  pending: "Pending verification",
};

const STATUS_STYLE: Record<MembershipStatus, string> = {
  active: "bg-green-bg text-green",
  suspended: "bg-red-bg text-red",
  pending: "bg-amber-bg text-amber",
};

export default function ChooseOrganizationPage() {
  const router = useRouter();
  const [selecting, setSelecting] = useState<string | null>(null);

  async function handleSelect(org: Organization) {
    if (org.status !== "active" || selecting) return;
    setSelecting(org.id);

    // MOCK: simulates entering the org context.
    await new Promise((resolve) => setTimeout(resolve, 500));
    router.push("/");
  }

  return (
    <div>
      <h1 className="font-display text-[36px] font-light leading-tight tracking-[-0.02em] text-ink-950">
        Choose organization
      </h1>
      <p className="mt-2 text-[14px] text-ink-600">
        Select the company you want to work in.
      </p>

      <div className="mt-8 divide-y divide-line border-y border-line">
        {MOCK_ORGS.map((org) => {
          const disabled = org.status !== "active";
          const isSelecting = selecting === org.id;
          return (
            <button
              key={org.id}
              type="button"
              onClick={() => handleSelect(org)}
              disabled={disabled || selecting !== null}
              className={`flex w-full items-center justify-between gap-4 py-4 text-left transition ${
                disabled
                  ? "cursor-not-allowed opacity-60"
                  : "cursor-pointer hover:bg-surface-2"
              }`}
            >
              <div className="min-w-0">
                <div className="truncate text-[15px] font-semibold text-ink-950">
                  {org.name}
                </div>
                <div className="mt-0.5 text-[13px] text-ink-600">
                  {org.role}
                </div>
              </div>

              <div className="flex flex-shrink-0 items-center gap-3">
                <span
                  className={`rounded-full px-2.5 py-1 text-[11px] font-semibold ${STATUS_STYLE[org.status]}`}
                >
                  {isSelecting ? "Entering…" : STATUS_LABEL[org.status]}
                </span>
              </div>
            </button>
          );
        })}
      </div>

      <p className="mt-6 text-[12.5px] leading-relaxed text-ink-500">
        Suspended or pending memberships can&apos;t be entered. Contact your
        organization administrator for access.
      </p>
    </div>
  );
}