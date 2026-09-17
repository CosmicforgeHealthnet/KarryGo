// src/app/(auth)/choose-organization/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Building2 } from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";

export default function ChooseOrganizationPage() {
  const router = useRouter();
  const { organizations, selectedOrganization, selectOrganization, isLoading, setLoading, setError } = useAuthStore();
  const [selecting, setSelecting] = useState<string | null>(null);

  const hasActiveOrg = organizations.some((org) => org.status === "active");

  useEffect(() => {
    if (organizations.length === 0) {
      router.push("/setup");
    }
  }, [organizations, router]);

  async function handleSelect(org: any) {
    if (org.status !== "active" || selecting) return;
    setSelecting(org.membershipId);
    setLoading(true);
    setError(null);

    try {
      await selectOrganization(org.membershipId);
      router.push("/");
    } catch {
      setError("Failed to select organization. Please try again.");
      setSelecting(null);
    } finally {
      setLoading(false);
    }
  }

  if (!hasActiveOrg) {
    const pendingOrg = organizations.find((org) => org.status === "pending");

    return (
      <div>
        <h1 className="font-display text-[40px] font-light leading-tight tracking-[-0.02em] text-ink-950">
          Choose organization
        </h1>
        <p className="mt-3 text-[16px] text-ink-600">
          Select the company you want to work in.
        </p>

        <div className="mt-8 rounded-xl border border-line bg-surface-2 px-5 py-6">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-surface p-2 shadow-sm">
              <Building2 className="h-5 w-5 text-primary" />
            </div>
            <div className="min-w-0">
              <p className="text-[15px] font-semibold text-ink-950">
                {pendingOrg
                  ? `${pendingOrg.organizationName} is still being set up`
                  : "Your organization is still being set up"}
              </p>
              <p className="mt-1 text-[14px] leading-relaxed text-ink-600">
                Finish onboarding to activate it. You&apos;ll be able to enter
                it here once it&apos;s approved.
              </p>
              <button
                type="button"
                onClick={() => router.push("/setup")}
                className="mt-4 flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-[13.5px] font-semibold text-white transition hover:bg-primary-hover"
              >
                Continue onboarding
                <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>
        </div>

        {organizations.some((org) => org.status === "suspended") && (
          <p className="mt-6 text-[13.5px] leading-relaxed text-ink-500">
            Suspended memberships can&apos;t be entered. Contact your
            organization administrator for access.
          </p>
        )}
      </div>
    );
  }

  return (
    <div>
      <h1 className="font-display text-[40px] font-light leading-tight tracking-[-0.02em] text-ink-950">
        Choose organization
      </h1>
      <p className="mt-3 text-[16px] text-ink-600">
        Select the company you want to work in.
      </p>

      <div className="mt-8 divide-y divide-line border-y border-line">
        {organizations.map((org) => {
          const disabled = org.status !== "active";
          const isSelecting = selecting === org.membershipId;
          const statusLabel: Record<string, string> = {
            active: "Active",
            suspended: "Suspended",
            pending: "Pending verification",
          };
          const statusStyle: Record<string, string> = {
            active: "bg-green-bg text-green",
            suspended: "bg-red-bg text-red",
            pending: "bg-amber-bg text-amber",
          };

          return (
            <button
              key={org.membershipId}
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
                <div className="truncate text-[17px] font-semibold text-ink-950">
                  {org.organizationName}
                </div>
                <div className="mt-0.5 text-[14px] text-ink-600">
                  {org.isOwner ? "Owner" : "Member"}
                </div>
              </div>

              <div className="flex flex-shrink-0 items-center gap-3">
                <span
                  className={`rounded-full px-2.5 py-1 text-[12px] font-semibold ${statusStyle[org.status] || "bg-surface-2 text-ink-500"}`}
                >
                  {isSelecting ? "Entering…" : statusLabel[org.status] || org.status}
                </span>
              </div>
            </button>
          );
        })}
      </div>

      <p className="mt-6 text-[13.5px] leading-relaxed text-ink-500">
        Suspended or pending memberships can&apos;t be entered. Contact your
        organization administrator for access.
      </p>
    </div>
  );
}