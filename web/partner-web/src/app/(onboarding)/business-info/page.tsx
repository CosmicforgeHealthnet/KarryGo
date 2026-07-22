"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ChevronDown } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { useOnboardingStore } from "@/store/onboarding-store";
import { useAuthStore } from "@/store/auth-store";
import { profileService } from "@/services/profile-service";
import { NIGERIAN_STATES } from "@/lib/nigerian-states";
import { ApiError } from "@/lib/api-client";

export default function BusinessInfoPage() {
  const router = useRouter();
  const stored = useOnboardingStore();
  const setBusinessInfo = useOnboardingStore((s) => s.setBusinessInfo);
  const token = useAuthStore((s) => s.token);
  const account = useAuthStore((s) => s.account);
  const setSession = useAuthStore((s) => s.setSession);
  const expiresAt = useAuthStore((s) => s.expiresAt);

  const businessEmail = account?.business_email ?? "";

  const [businessName, setBusinessName] = useState(stored.businessName);
  const [businessPhone, setBusinessPhone] = useState(stored.businessPhone);
  const [state, setState] = useState(stored.state);
  const [city, setCity] = useState(stored.city);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const canContinue =
    businessName.trim() && businessPhone.trim() && state && city.trim();

  const handleContinue = async () => {
    if (!canContinue || loading) return;
    if (!token) {
      setError("Your session has expired. Please sign up again.");
      return;
    }
    setError(null);
    setLoading(true);
    try {
      const { account: updatedAccount } = await profileService.updateProfile(
        {
          business_name: businessName,
          business_phone: businessPhone,
          state,
          city,
        },
        token
      );
      setSession({ token, expires_at: expiresAt ?? "", account: updatedAccount });
      setBusinessInfo({ businessName, businessPhone, businessEmail, state, city });
      router.push("/business-logo");
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Something went wrong. Please try again.");
      }
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <h1 className="text-2xl font-bold text-ink-900">Business Information</h1>
      <p className="mt-1 text-sm text-ink-500">Tell us about your business.</p>

      <div className="mt-6 space-y-5">
        <Input
          id="businessName"
          label="Business Name"
          placeholder="Demai UI"
          value={businessName}
          onChange={(e) => setBusinessName(e.target.value)}
        />
        <Input
          id="businessPhone"
          label="Business Phone Number"
          type="tel"
          placeholder="13456781234"
          value={businessPhone}
          onChange={(e) => setBusinessPhone(e.target.value)}
        />
        <Input
          id="businessEmail"
          label="Business Email"
          type="email"
          value={businessEmail}
          disabled
        />

        <div>
          <label className="mb-2 block text-sm font-medium text-ink-900">
            Location
          </label>
          <div className="space-y-3">
            <div className="relative">
              <select
                value={state}
                onChange={(e) => setState(e.target.value)}
                className="w-full appearance-none rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 focus:border-brand-500 focus:outline-none"
              >
                <option value="">State</option>
                {NIGERIAN_STATES.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </select>
              <ChevronDown className="pointer-events-none absolute right-4 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-500" />
            </div>
            <Input
              id="city"
              placeholder="City"
              value={city}
              onChange={(e) => setCity(e.target.value)}
            />
          </div>
        </div>

        {error && <p className="text-xs text-red-500">{error}</p>}
      </div>

      <div className="mt-8">
        <Button onClick={handleContinue} disabled={!canContinue || loading}>
          {loading ? "Saving..." : "Continue"}
        </Button>
      </div>
    </div>
  );
}