// src/app/(setup)/layout.tsx

"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { usePathname, useRouter } from "next/navigation";
import Link from "next/link";
import Image from "next/image";
import {
  CheckCircle2,
  Circle,
  Home,
  Building2,
  User,
  Briefcase,
  FileText,
  FileCheck,
  ChevronRight,
  X,
  Menu,
  Loader2,
  AlertTriangle,
  RefreshCw,
} from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";

const ONBOARDING_STEPS = [
  { id: "create-company", label: "Create Company", path: "/setup/create-company", icon: <Building2 className="h-4 w-4" /> },
  { id: "profile", label: "Company Profile", path: "/setup/profile", icon: <User className="h-4 w-4" /> },
  { id: "capabilities", label: "Capabilities", path: "/setup/capabilities", icon: <Briefcase className="h-4 w-4" /> },
  { id: "documents", label: "Business Documents", path: "/setup/business-docs", icon: <FileText className="h-4 w-4" /> },
  { id: "submit", label: "Submit for Review", path: "/setup/submit", icon: <FileCheck className="h-4 w-4" /> },
];

const REVISITABLE_STEPS = ["profile", "capabilities", "documents"];
const ONE_TIME_STEPS = ["create-company", "submit"];

const STAGE_TO_STEP: Record<string, string> = {
  OWNER_CONTACT: "create-company",
  BUSINESS_PROFILE: "profile",
  OPERATING_CAPABILITIES: "capabilities",
  BUSINESS_DOCUMENTS: "documents",
  ACTIVATION: "submit",
};

const HEADER_HEIGHT_PX = 70;

// TODO: replace with the real support contact channel (mailbox, help-desk
// URL, or in-app chat trigger) once one exists — this is a placeholder so
// the button is at least honest about what clicking it does.
const SUPPORT_EMAIL = "support@karrygo.com";

type LoadState = "loading" | "ready" | "error";
type StepIconStatus = "completed" | "current" | "pending";

function clearSessionRoutingCookies() {
  document.cookie = "karrygo_has_session=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
  document.cookie = "karrygo_org_status=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
}

interface StepRowProps {
  step: (typeof ONBOARDING_STEPS)[number];
  index: number;
  iconStatus: StepIconStatus;
  isActivePage: boolean;
  isClickable: boolean;
  onSelect: () => void;
  variant: "mobile" | "desktop";
}

function StepRow({ step, index, iconStatus, isActivePage, isClickable, onSelect, variant }: StepRowProps) {
  const isDisabled = !isClickable;

  const rowClasses = `flex w-full items-center gap-3 rounded-xl px-3.5 py-2.5 text-left text-sm font-medium transition ${
    isActivePage
      ? "bg-primary-bg text-primary shadow-sm"
      : iconStatus === "completed"
      ? "text-green hover:bg-surface-2"
      : "text-ink-500 hover:bg-surface-2"
  } ${isDisabled ? "cursor-not-allowed opacity-50" : "cursor-pointer"}`;

  return (
    <button
      type="button"
      onClick={onSelect}
      disabled={isDisabled}
      aria-current={isActivePage ? "step" : undefined}
      aria-disabled={isDisabled}
      className={rowClasses}
    >
      {variant === "desktop" ? (
        <span
          className={`flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full text-[11px] font-semibold ${
            isActivePage
              ? "bg-primary text-white"
              : iconStatus === "completed"
              ? "bg-green-bg text-green"
              : "bg-surface-2 text-ink-400"
          }`}
        >
          {iconStatus === "completed" ? <CheckCircle2 className="h-4 w-4" /> : index + 1}
        </span>
      ) : iconStatus === "completed" ? (
        <CheckCircle2 className="h-4 w-4 flex-shrink-0 text-green" />
      ) : isActivePage ? (
        <div className="h-4 w-4 flex-shrink-0 rounded-full bg-primary" />
      ) : (
        <Circle className="h-4 w-4 flex-shrink-0 text-ink-300" />
      )}
      <span className="flex-1">{step.label}</span>
      {variant === "mobile" && isClickable && <ChevronRight className="h-3.5 w-3.5 flex-shrink-0 text-ink-400" />}
    </button>
  );
}

export default function OnboardingLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [stageStatusError, setStageStatusError] = useState(false);
  const mobileDrawerRef = useRef<HTMLDivElement>(null);
  const hasLoadedOnce = useRef(false);

  const { _hasHydrated, accessToken, logout } = useAuthStore();
  const { organization, setOrganization } = useOrganizationStore();
  const { stageStatuses, setStageStatuses } = useOnboardingStore();

  const loadStageStatuses = useCallback(
    async (organizationId: string) => {
      setStageStatusError(false);
      try {
        const status = await organizationService.getOnboardingStatus(organizationId);
        const statusMap: Record<string, string> = {};
        status.stages.forEach((stage) => {
          const stepId = STAGE_TO_STEP[stage.stage];
          if (stepId) {
            statusMap[stepId] = stage.status;
          }
        });
        setStageStatuses(statusMap);
      } catch (e) {
        console.error("Failed to fetch onboarding status:", e);
        setStageStatusError(true);
      }
    },
    [setStageStatuses]
  );

  const loadOrganization = useCallback(async () => {
    setLoadState("loading");
    try {
      const org = await organizationService.getCurrentOrganization();
      setOrganization(org);

      // A single, immediate decision made from the data we just fetched —
      // not a separate effect reacting to state that may be stale by the
      // time it re-runs.
      if (org.status !== "DRAFT") {
        router.replace("/");
        return;
      }

      setLoadState("ready");
      hasLoadedOnce.current = true;
      await loadStageStatuses(org.organizationId);
    } catch (error: any) {
      if (error?.response?.status === 401) {
        clearSessionRoutingCookies();
        router.replace("/sign-in");
        return;
      }
      console.error("Failed to fetch organization:", error);
      setLoadState("error");
    }
  }, [router, setOrganization, loadStageStatuses]);

  useEffect(() => {
    if (!_hasHydrated) return;
    if (!accessToken) {
      router.replace("/sign-in");
      return;
    }
    loadOrganization();
    // Intentionally run only when hydration/auth state changes — not on
    // every render of loadOrganization, which would refetch constantly.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [accessToken, _hasHydrated, router]);

  // Refresh step statuses on navigation between setup steps, so completing
  // a step and moving to the next one reflects immediately in the sidebar
  // without requiring a full page reload.
  useEffect(() => {
    if (!hasLoadedOnce.current || !organization?.organizationId) return;
    loadStageStatuses(organization.organizationId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pathname]);

  useEffect(() => {
    setIsMobileMenuOpen(false);
  }, [pathname]);

  useEffect(() => {
    if (!isMobileMenuOpen) return;

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setIsMobileMenuOpen(false);
    };
    document.addEventListener("keydown", handleEscape);

    // Move focus into the drawer when it opens, and don't fight the user
    // if they've already focused something inside it themselves.
    const frame = requestAnimationFrame(() => {
      if (mobileDrawerRef.current && !mobileDrawerRef.current.contains(document.activeElement)) {
        mobileDrawerRef.current.querySelector<HTMLElement>("button")?.focus();
      }
    });

    return () => {
      document.removeEventListener("keydown", handleEscape);
      cancelAnimationFrame(frame);
    };
  }, [isMobileMenuOpen]);

  if (!_hasHydrated || loadState === "loading") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-surface-2">
        <Loader2 className="h-8 w-8 animate-spin text-primary" />
      </div>
    );
  }

  if (!accessToken) {
    return null;
  }

  if (loadState === "error") {
    return (
      <div className="flex min-h-screen items-center justify-center bg-surface-2 px-4">
        <div className="w-full max-w-sm rounded-2xl border border-line bg-white p-6 text-center shadow-sm">
          <div className="mx-auto mb-3 flex h-11 w-11 items-center justify-center rounded-full bg-red-bg">
            <AlertTriangle className="h-5 w-5 text-red" />
          </div>
          <h2 className="text-[15px] font-semibold text-ink-950">Couldn't load your account</h2>
          <p className="mt-1 text-[13px] text-ink-500">
            Something went wrong reaching the server. Check your connection and try again.
          </p>
          <button
            type="button"
            onClick={loadOrganization}
            className="mt-4 flex w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 py-2.5 text-[13px] font-semibold text-white transition hover:bg-primary-hover"
          >
            <RefreshCw className="h-3.5 w-3.5" />
            Try again
          </button>
          <button
            type="button"
            onClick={async () => {
              await logout();
              router.replace("/sign-in");
            }}
            className="mt-2 w-full text-[12px] font-medium text-ink-500 transition hover:text-ink-700"
          >
            Sign out instead
          </button>
        </div>
      </div>
    );
  }

  const isOnboardingHome = pathname === "/setup";

  const getIconStatus = (stepId: string): StepIconStatus => {
    const apiStatus = stageStatuses[stepId];
    if (apiStatus === "COMPLETE") return "completed";
    if (apiStatus === "IN_PROGRESS") return "current";
    return "pending";
  };

  const isStepClickable = (stepId: string, isActivePage: boolean): boolean => {
    if (isActivePage) return true;

    const status = stageStatuses[stepId];
    const isCompleted = status === "COMPLETE";
    const isInProgress = status === "IN_PROGRESS";

    if (isCompleted && ONE_TIME_STEPS.includes(stepId)) return false;
    if (isCompleted && REVISITABLE_STEPS.includes(stepId)) return true;
    if (isInProgress) return true;

    return false;
  };

  const handleStepClick = (stepId: string, path: string, isActivePage: boolean) => {
    if (!isStepClickable(stepId, isActivePage)) return;
    router.push(path);
  };

  const completedCount = ONBOARDING_STEPS.filter((s) => stageStatuses[s.id] === "COMPLETE").length;
  const totalSteps = ONBOARDING_STEPS.length;

  if (isOnboardingHome) {
    return (
      <div className="min-h-screen bg-surface-2">
        <header
          className="sticky top-0 z-20 flex items-center justify-between px-4 sm:px-6 bg-white/80 backdrop-blur-xl border-b border-line"
          style={{ height: HEADER_HEIGHT_PX }}
        >
          <div className="flex items-center gap-3">
            <Link href="/" className="flex items-center gap-2">
              <Image
                src="/images/fleet-logo.png"
                alt="KarryGo Fleet OS"
                width={36}
                height={36}
                className="rounded-xl object-contain"
              />
              <span className="font-display text-xl font-bold tracking-tight text-ink-950">KarryGo</span>
            </Link>
          </div>
          <div className="flex items-center gap-2" />
        </header>
        <main className="max-w-5xl mx-auto px-4 py-8 sm:px-6 lg:px-8">{children}</main>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-surface-2">
      <header
        className="sticky top-0 z-20 flex items-center justify-between px-4 sm:px-6 bg-white/80 backdrop-blur-xl border-b border-line"
        style={{ height: HEADER_HEIGHT_PX }}
      >
        <div className="flex items-center gap-3">
          <Link href="/" className="flex items-center gap-2">
            <Image
              src="/images/fleet-logo.png"
              alt="KarryGo Fleet OS"
              width={36}
              height={36}
              className="rounded-xl object-contain"
            />
            <span className="font-display text-xl font-bold tracking-tight text-ink-950">KarryGo</span>
          </Link>
          <span className="hidden sm:inline text-xs font-medium text-ink-400 bg-surface-2 px-2.5 py-1 rounded-full">
            Onboarding
          </span>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => router.push("/setup")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Home className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Home</span>
          </button>
          <button
            onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
            aria-label="Open onboarding steps menu"
            aria-expanded={isMobileMenuOpen}
            className="lg:hidden flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-white/70 shadow-sm hover:bg-white transition-colors"
          >
            <Menu className="h-4 w-4 text-ink-600" />
          </button>
        </div>
      </header>

      {isMobileMenuOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/40 backdrop-blur-sm lg:hidden"
          onClick={() => setIsMobileMenuOpen(false)}
          aria-hidden="true"
        />
      )}

      <div
        ref={mobileDrawerRef}
        role="dialog"
        aria-modal="true"
        aria-label="Onboarding steps"
        className={`fixed top-0 right-0 z-40 h-full w-72 bg-white shadow-xl transform transition-transform duration-300 ease-in-out lg:hidden ${
          isMobileMenuOpen ? "translate-x-0" : "translate-x-full pointer-events-none"
        }`}
      >
        <div
          className="flex items-center justify-between px-4 border-b border-line"
          style={{ height: HEADER_HEIGHT_PX }}
        >
          <span className="font-semibold text-ink-950">Onboarding Steps</span>
          <button
            onClick={() => setIsMobileMenuOpen(false)}
            aria-label="Close onboarding steps menu"
            className="flex h-8 w-8 items-center justify-center rounded-lg hover:bg-surface-2 transition-colors"
          >
            <X className="h-4 w-4 text-ink-600" />
          </button>
        </div>
        <div className="p-4 space-y-1">
          {ONBOARDING_STEPS.map((step, index) => {
            const isActivePage = pathname === step.path;
            return (
              <StepRow
                key={step.id}
                step={step}
                index={index}
                iconStatus={getIconStatus(step.id)}
                isActivePage={isActivePage}
                isClickable={isStepClickable(step.id, isActivePage)}
                onSelect={() => handleStepClick(step.id, step.path, isActivePage)}
                variant="mobile"
              />
            );
          })}
        </div>
      </div>

      <div className="flex">
        <aside
          className="hidden lg:block w-72 flex-shrink-0 border-r border-line bg-white/50 sticky overflow-y-auto p-5"
          style={{
            top: HEADER_HEIGHT_PX,
            height: `calc(100vh - ${HEADER_HEIGHT_PX}px)`,
          }}
        >
          <div className="mb-4">
            <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
              Onboarding Progress
            </div>
            <div className="mt-2 h-1.5 w-full rounded-full bg-surface-2 overflow-hidden">
              <div
                className="h-full rounded-full bg-primary transition-all duration-500"
                style={{
                  width: `${Math.min((completedCount / totalSteps) * 100, 100)}%`,
                }}
              />
            </div>
            <div className="mt-1.5 flex items-center gap-1.5 text-xs text-ink-500">
              <span>{completedCount}/{totalSteps} complete</span>
              {stageStatusError && (
                <button
                  type="button"
                  onClick={() => organization?.organizationId && loadStageStatuses(organization.organizationId)}
                  className="flex items-center gap-1 text-red hover:underline"
                  title="Progress may be out of date — click to retry"
                >
                  <AlertTriangle className="h-3 w-3" />
                  Retry
                </button>
              )}
            </div>
          </div>

          <div className="space-y-1">
            {ONBOARDING_STEPS.map((step, index) => {
              const isActivePage = pathname === step.path;
              return (
                <StepRow
                  key={step.id}
                  step={step}
                  index={index}
                  iconStatus={getIconStatus(step.id)}
                  isActivePage={isActivePage}
                  isClickable={isStepClickable(step.id, isActivePage)}
                  onSelect={() => handleStepClick(step.id, step.path, isActivePage)}
                  variant="desktop"
                />
              );
            })}
          </div>

          <div className="mt-6 rounded-xl border border-line bg-white p-4">
            <div className="text-xs font-medium text-ink-500">Need help?</div>
            <p className="mt-1 text-xs text-ink-400">
              Contact support if you need assistance with the onboarding process.
            </p>
            <a
              href={`mailto:${SUPPORT_EMAIL}`}
              className="mt-2 inline-block text-xs font-medium text-primary hover:underline"
            >
              Contact Support →
            </a>
          </div>
        </aside>

        <main className="flex-1 min-w-0 px-4 py-6 sm:px-6 lg:px-8">{children}</main>
      </div>
    </div>
  );
}