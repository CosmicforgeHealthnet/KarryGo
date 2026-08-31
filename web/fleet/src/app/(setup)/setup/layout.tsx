// src/app/(onboarding)/layout.tsx

"use client";

import { useState, useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import Link from "next/link";
import Image from "next/image";
import {
  ArrowLeft,
  CheckCircle2,
  Circle,
  Home,
  Building2,
  User,
  Briefcase,
  FileText,
  ShieldCheck,
  Wallet,
  FileCheck,
  ChevronRight,
  X,
  Menu,
} from "lucide-react";

// ===== ONBOARDING STEPS =====
const ONBOARDING_STEPS = [
  { id: "create-company", label: "Create Company", path: "/setup/create-company", icon: <Building2 className="h-4 w-4" /> },
  { id: "profile", label: "Company Profile", path: "/setup/profile", icon: <User className="h-4 w-4" /> },
  { id: "capabilities", label: "Capabilities", path: "/setup/capabilities", icon: <Briefcase className="h-4 w-4" /> },
  { id: "documents", label: "Documents", path: "/setup/business-docs", icon: <FileText className="h-4 w-4" /> },
  { id: "verification", label: "Verification", path: "/setup/verification", icon: <ShieldCheck className="h-4 w-4" /> },
  { id: "wallet-provisioning", label: "Wallet", path: "/setup/wallet-provisioning", icon: <Wallet className="h-4 w-4" /> },
  { id: "submit", label: "Submit", path: "/setup/submit", icon: <FileCheck className="h-4 w-4" /> },
];

function getCurrentStepIndex(pathname: string) {
  return ONBOARDING_STEPS.findIndex((step) => pathname.startsWith(step.path));
}

function getStepStatus(index: number, currentIndex: number) {
  if (index < currentIndex) return "completed";
  if (index === currentIndex) return "current";
  return "pending";
}

export default function OnboardingLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  const currentIndex = getCurrentStepIndex(pathname);
  const isOnboardingHome = pathname === "/onboarding";
  const isSubmitted = pathname === "/onboarding/submit" || pathname === "/onboarding/verification";

  // Close mobile menu on route change
  useEffect(() => {
    setIsMobileMenuOpen(false);
  }, [pathname]);

  // If on the home page, show full-width without steps sidebar
  if (isOnboardingHome) {
    return (
      <div className="min-h-screen bg-surface-2">
        {/* Minimal header for home page */}
        <header className="sticky top-0 z-20 flex h-[70px] items-center justify-between px-4 sm:px-6 bg-white/80 backdrop-blur-xl border-b border-line">
          <div className="flex items-center gap-3">
            <Link href="/" className="flex items-center gap-2">
              <Image
                src="/images/fleet-logo.png"
                alt="KarryGo Fleet OS"
                width={36}
                height={36}
                className="rounded-xl object-contain"
              />
              <span className="font-display text-xl font-bold tracking-tight text-ink-950">
                KarryGo
              </span>
            </Link>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => router.push("/")}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
            >
              <Home className="h-3.5 w-3.5" />
              Dashboard
            </button>
          </div>
        </header>
        <main className="max-w-5xl mx-auto px-4 py-8 sm:px-6 lg:px-8">
          {children}
        </main>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-surface-2">
      {/* ===== HEADER ===== */}
      <header className="sticky top-0 z-20 flex h-[70px] items-center justify-between px-4 sm:px-6 bg-white/80 backdrop-blur-xl border-b border-line">
        <div className="flex items-center gap-3">
          <Link href="/" className="flex items-center gap-2">
            <Image
              src="/images/fleet-logo.png"
              alt="KarryGo Fleet OS"
              width={36}
              height={36}
              className="rounded-xl object-contain"
            />
            <span className="font-display text-xl font-bold tracking-tight text-ink-950">
              KarryGo
            </span>
          </Link>
          <span className="hidden sm:inline text-xs font-medium text-ink-400 bg-surface-2 px-2.5 py-1 rounded-full">
            Onboarding
          </span>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={() => router.push("/onboarding")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Home className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">Home</span>
          </button>
          <button
            onClick={() => setIsMobileMenuOpen(!isMobileMenuOpen)}
            className="lg:hidden flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-white/70 shadow-sm hover:bg-white transition-colors"
          >
            <Menu className="h-4 w-4 text-ink-600" />
          </button>
        </div>
      </header>

      {/* ===== MOBILE MENU OVERLAY ===== */}
      {isMobileMenuOpen && (
        <div
          className="fixed inset-0 z-30 bg-black/40 backdrop-blur-sm lg:hidden"
          onClick={() => setIsMobileMenuOpen(false)}
        />
      )}

      {/* ===== MOBILE MENU ===== */}
      <div
        className={`fixed top-0 right-0 z-40 h-full w-72 bg-white shadow-xl transform transition-transform duration-300 ease-in-out lg:hidden ${
          isMobileMenuOpen ? "translate-x-0" : "translate-x-full"
        }`}
      >
        <div className="flex h-[70px] items-center justify-between px-4 border-b border-line">
          <span className="font-semibold text-ink-950">Onboarding Steps</span>
          <button
            onClick={() => setIsMobileMenuOpen(false)}
            className="flex h-8 w-8 items-center justify-center rounded-lg hover:bg-surface-2 transition-colors"
          >
            <X className="h-4 w-4 text-ink-600" />
          </button>
        </div>
        <div className="p-4 space-y-1">
          {ONBOARDING_STEPS.map((step, index) => {
            const status = getStepStatus(index, currentIndex);
            return (
              <Link
                key={step.id}
                href={step.path}
                onClick={() => setIsMobileMenuOpen(false)}
                className={`flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-sm font-medium transition ${
                  status === "current"
                    ? "bg-primary-bg text-primary"
                    : status === "completed"
                    ? "text-green"
                    : "text-ink-500 hover:bg-surface-2"
                }`}
              >
                {status === "completed" ? (
                  <CheckCircle2 className="h-4 w-4 text-green" />
                ) : status === "current" ? (
                  <div className="h-4 w-4 rounded-full bg-primary" />
                ) : (
                  <Circle className="h-4 w-4 text-ink-300" />
                )}
                <span className="flex-1">{step.label}</span>
                <ChevronRight className="h-3.5 w-3.5 text-ink-400" />
              </Link>
            );
          })}
        </div>
      </div>

      {/* ===== MAIN CONTENT ===== */}
      <div className="flex">
        {/* ===== SIDEBAR ===== */}
        <aside className="hidden lg:block w-72 flex-shrink-0 border-r border-line bg-white/50 min-h-[calc(100vh-70px)] p-5">
          <div className="mb-4">
            <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
              Onboarding Progress
            </div>
            <div className="mt-2 h-1.5 w-full rounded-full bg-surface-2 overflow-hidden">
              <div
                className="h-full rounded-full bg-primary transition-all duration-500"
                style={{
                  width: `${Math.min(
                    ((currentIndex + 1) / ONBOARDING_STEPS.length) * 100,
                    100
                  )}%`,
                }}
              />
            </div>
            <div className="mt-1.5 text-xs text-ink-500">
              Step {Math.min(currentIndex + 1, ONBOARDING_STEPS.length)} of {ONBOARDING_STEPS.length}
            </div>
          </div>

          <div className="space-y-1">
            {ONBOARDING_STEPS.map((step, index) => {
              const status = getStepStatus(index, currentIndex);
              const isActive = status === "current";

              return (
                <Link
                  key={step.id}
                  href={step.path}
                  className={`flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-sm font-medium transition ${
                    isActive
                      ? "bg-primary-bg text-primary shadow-sm"
                      : status === "completed"
                      ? "text-green hover:bg-surface-2"
                      : "text-ink-500 hover:bg-surface-2"
                  }`}
                >
                  <span
                    className={`flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full text-[11px] font-semibold ${
                      isActive
                        ? "bg-primary text-white"
                        : status === "completed"
                        ? "bg-green-bg text-green"
                        : "bg-surface-2 text-ink-400"
                    }`}
                  >
                    {status === "completed" ? (
                      <CheckCircle2 className="h-4 w-4" />
                    ) : (
                      index + 1
                    )}
                  </span>
                  <span className="flex-1">{step.label}</span>
                  {isActive && <div className="h-2 w-2 rounded-full bg-primary" />}
                </Link>
              );
            })}
          </div>

          <div className="mt-6 rounded-xl border border-line bg-white p-4">
            <div className="text-xs font-medium text-ink-500">Need help?</div>
            <p className="mt-1 text-xs text-ink-400">
              Contact support if you need assistance with the onboarding process.
            </p>
            <button className="mt-2 text-xs font-medium text-primary hover:underline">
              Contact Support →
            </button>
          </div>
        </aside>

        {/* ===== CONTENT ===== */}
        <main className="flex-1 min-w-0 px-4 py-6 sm:px-6 lg:px-8">
          {children}
        </main>
      </div>
    </div>
  );
}