"use client";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuthStore } from "@/lib/stores/authStore";
import type { Organization } from "@/lib/types/auth";
import { organizationDestination } from "@/lib/auth/organizationDestination";
import { organizationService } from "@/lib/services/organizationService";
import { ErrorNotice, humanize } from "../../(setup)/setup/onboarding-ui";
export default function ChooseOrganization() {
  const router = useRouter();
  const organizations = useAuthStore((s) => s.organizations);
  const hydrated = useAuthStore((s) => s._hasHydrated);
  const [selecting, setSelecting] = useState<string | null>(null);
  const [error, setError] = useState<unknown>(null);
  useEffect(() => {
    if (hydrated && organizations.length === 0)
      router.replace(organizationDestination(null));
  }, [hydrated, organizations.length, router]);
  async function select(org: Organization) {
    if (selecting) return;
    setSelecting(org.membershipId);
    setError(null);
    try {
      await useAuthStore.getState().selectOrganization(org.membershipId);
      const current = await organizationService.getCurrentOrganization();
      router.push(organizationDestination(current.status));
    } catch (err) {
      setError(err);
      setSelecting(null);
    }
  }
  return (
    <div className="mx-auto w-full max-w-xl space-y-5 p-5">
      <h1 className="text-2xl font-bold text-ink-950">Choose your company</h1>
      <p className="text-sm text-ink-600">
        Continue company setup, review your submission or open an active
        company.
      </p>
      {error != null && <ErrorNotice error={error} />}
      <ul className="space-y-3">
        {organizations.map((org) => {
          const eligible = !["SUSPENDED", "ARCHIVED", "CLOSED"].includes(
            (org.organizationStatus ?? org.status).toUpperCase(),
          );
          return (
            <li key={org.membershipId}>
              <button
                className="w-full rounded-xl border border-line bg-surface p-4 text-left text-ink-950 focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-60"
                disabled={!!selecting || !eligible}
                onClick={() => select(org)}
              >
                <span className="block font-semibold">
                  {org.organizationName}
                </span>
                <span className="mt-1 block text-sm text-ink-500">
                  {selecting === org.membershipId
                    ? "Opening company…"
                    : org.organizationStatus
                      ? humanize(org.organizationStatus)
                      : `Membership ${humanize(org.status)}`}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
