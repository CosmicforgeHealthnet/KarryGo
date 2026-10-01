// src/app/(dashboard)/layout.tsx

"use client";

import { useState, useEffect, useRef } from "react";
import Link from "next/link";
import Image from "next/image";
import { usePathname, useRouter } from "next/navigation";
import {
  LayoutDashboard,
  Users,
  Truck,
  Briefcase,
  Warehouse,
  Gem,
  Wallet,
  Shield,
  FileText,
  ChartNoAxesCombined,
  Lock,
  Route,
  Settings,
  Search,
  Bell,
  ClipboardList,
  Calendar,
  MapPin,
  GitBranch,
  Radar,
  LifeBuoy,
  Car,
  LogOut,
  ChevronUp,
  ChevronDown,
  Loader2,
} from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { organizationService } from "@/lib/services/organizationService";
import { PendingVerificationBanner } from "@/components/features/PendingVerificationBanner";
import { setOrgStatusCookie } from "@/lib/utils/orgStatusCookie";
import { ThemeToggle } from "@/components/ThemeToggle";

const NAV_SECTIONS = [
  {
    label: "MAIN",
    items: [
      { label: "Dashboard", href: "/", icon: <LayoutDashboard className="h-4 w-4" /> },
    ]
  },
  {
    label: "OPERATIONS",
    items: [
      { label: "Operations", href: "/operations/marketplace", icon: <Route className="h-4 w-4" /> },
      { label: "Control Tower", href: "/operations/control-tower", icon: <Radar className="h-4 w-4" /> },
      { label: "Dispatch", href: "/operations/dispatch", icon: <ClipboardList className="h-4 w-4" /> },
      { label: "Routes", href: "/operations/routes", icon: <GitBranch className="h-4 w-4" /> },
      { label: "Geofences", href: "/operations/geofences", icon: <MapPin className="h-4 w-4" /> },
    ]
  },
  {
    label: "RESOURCES",
    items: [
      { label: "Providers", href: "/providers", icon: <Users className="h-4 w-4" /> },
      { label: "Fleet", href: "/fleet/assets", icon: <Car className="h-4 w-4" /> },
      { label: "Workforce", href: "/workforce/shifts", icon: <Briefcase className="h-4 w-4" /> },
      { label: "Warehouse", href: "/warehouse", icon: <Warehouse className="h-4 w-4" /> },
    ]
  },
  {
    label: "COMMERCIAL",
    items: [
      { label: "CRM", href: "/crm/customers", icon: <Gem className="h-4 w-4" /> },
      { label: "Finance", href: "/finance", icon: <Wallet className="h-4 w-4" /> },
    ]
  },
  // {
  //   label: "COMPLIANCE & REPORTS",
  //   items: [
  //     { label: "Compliance", href: "/compliance", icon: <Shield className="h-4 w-4" /> },
  //     { label: "Reports", href: "/reports/library", icon: <ChartNoAxesCombined className="h-4 w-4" /> },
  //   ]
  // },
  // {
  //   label: "SYSTEM",
  //   items: [
  //     { label: "Governance", href: "/settings/staff", icon: <Lock className="h-4 w-4" /> },
  //     { label: "Settings", href: "/settings", icon: <Settings className="h-4 w-4" /> },
  //     { label: "Documents", href: "/documents", icon: <FileText className="h-4 w-4" /> },
  //   ]
  // },
];

const ALL_NAV_ITEMS = NAV_SECTIONS.flatMap(s => s.items);

function findActiveNavItem(pathname: string) {
  let best: (typeof ALL_NAV_ITEMS)[number] | null = null;

  for (const item of ALL_NAV_ITEMS) {
    const isMatch =
      pathname === item.href || pathname?.startsWith(item.href + "/");
    if (isMatch && (!best || item.href.length > best.href.length)) {
      best = item;
    }
  }

  return best;
}

const getPageName = (pathname: string) => {
  return findActiveNavItem(pathname)?.label ?? "Dashboard";
};

const ClassicHamburger = () => (
  <div className="relative w-6 h-5 flex flex-col items-center justify-between">
    <span className="block h-0.5 bg-current rounded-full w-4" />
    <span className="block h-0.5 bg-current rounded-full w-6" />
    <span className="block h-0.5 bg-current rounded-full w-5" />
  </div>
);

function initialsFrom(label: string | null | undefined): string {
  if (!label) return "U";
  const trimmed = label.trim();
  if (!trimmed) return "U";
  const parts = trimmed.split(/\s+/);
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[1][0]).toUpperCase();
}

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [isOpen, setIsOpen] = useState(false);
  const [isCollapsed, setIsCollapsed] = useState(false);
  const [isDesktop, setIsDesktop] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [isUserMenuOpen, setIsUserMenuOpen] = useState(false);
  const [isLoggingOut, setIsLoggingOut] = useState(false);
  const userMenuRef = useRef<HTMLDivElement>(null);

  const { _hasHydrated, accessToken, identityId, logout } = useAuthStore();
  const { organization, setOrganization, isPending, setLoading } = useOrganizationStore();

  const pageName = getPageName(pathname);
  const activeItem = findActiveNavItem(pathname);

  const displayName = organization?.tradingName || organization?.legalName || null;
  const displaySubtext = identityId ? `ID: ${identityId.slice(0, 8)}…` : "Signed in";
  const avatarInitials = initialsFrom(displayName);
  const displayRole = "Fleet Manager";

  useEffect(() => {
    if (!_hasHydrated) return;

    if (!accessToken) {
      router.replace("/sign-in");
      return;
    }

    const fetchOrganization = async () => {
      try {
        setLoading(true);
        const org = await organizationService.getCurrentOrganization();
        setOrganization(org);

        if (org?.status) {
          setOrgStatusCookie(org.status);
        }

        if (org?.status === "DRAFT") {
          router.replace("/setup");
          return;
        }
      } catch (error: any) {
        if (error?.response?.status === 401) {
          router.replace("/sign-in");
          return;
        }
        console.error("Failed to fetch organization:", error);
      } finally {
        setLoading(false);
        setIsLoading(false);
      }
    };

    fetchOrganization();
  }, [accessToken, _hasHydrated, router, setOrganization, setLoading]);

  useEffect(() => {
    const checkScreen = () => {
      setIsDesktop(window.innerWidth >= 1024);
    };
    checkScreen();
    window.addEventListener('resize', checkScreen);
    return () => window.removeEventListener('resize', checkScreen);
  }, []);

  useEffect(() => {
    if (isDesktop) {
      setIsOpen(false);
    }
  }, [isDesktop]);

  useEffect(() => {
    if (!isUserMenuOpen) return;

    const handleClickOutside = (event: MouseEvent) => {
      if (userMenuRef.current && !userMenuRef.current.contains(event.target as Node)) {
        setIsUserMenuOpen(false);
      }
    };
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setIsUserMenuOpen(false);
    };

    document.addEventListener("mousedown", handleClickOutside);
    document.addEventListener("keydown", handleEscape);
    return () => {
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("keydown", handleEscape);
    };
  }, [isUserMenuOpen]);

  const handleHamburgerClick = () => {
    if (isDesktop) {
      setIsCollapsed(!isCollapsed);
    } else {
      setIsOpen(!isOpen);
    }
  };

  const handleLogout = async () => {
    if (isLoggingOut) return;
    setIsLoggingOut(true);
    try {
      await logout();
    } finally {
      router.replace("/sign-in");
    }
  };

  if (!_hasHydrated || isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-surface-2">
        <div className="h-8 w-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    );
  }

  if (!accessToken) {
    return null;
  }

  if (organization?.status === "DRAFT") {
    router.replace("/setup");
    return null;
  }

  return (
    <div className="min-h-screen bg-surface-2 flex">
      <aside
        className={`
          fixed top-0 left-0 z-40
          ${isCollapsed ? "w-[68px]" : "w-[280px]"}
          h-screen
          bg-sidebar-bg/90 backdrop-blur-xl
          transform transition-all duration-300 ease-in-out
          ${isOpen ? "translate-x-0" : "-translate-x-full"}
          lg:translate-x-0
          px-3.5 py-5 text-sidebar-text
          flex flex-col
          shadow-xl
          border-r border-white/10
          overflow-y-auto
          scrollbar-hide
        `}
      >
        <div className={`flex items-center gap-1 px-2 pb-6 pt-14 lg:pt-5 flex-shrink-0 ${isCollapsed ? "justify-center" : ""}`}>
          <Image 
            src="/images/fleet-logo.png" 
            alt="KarryGo Fleet OS" 
            width={42} 
            height={42}
            className="rounded-xl object-contain flex-shrink-0"
          />
          {!isCollapsed && (
            <div className="min-w-0">
              <div className="font-display text-xl font-bold tracking-tight text-sidebar-text">
                KarryAm
              </div>
            
            </div>
          )}
        </div>

        <nav className="flex-1 space-y-4 overflow-y-auto py-4 scrollbar-hide">
          {NAV_SECTIONS.map((section, idx) => (
            <div key={idx}>
              {!isCollapsed && (
                <div className="px-3 text-[10px] font-semibold text-sidebar-text-secondary/70 tracking-[0.1em] uppercase">
                  {section.label}
                </div>
              )}
              <div className="space-y-0.5">
                {section.items.map((item) => {
                  const active = activeItem?.href === item.href;
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      onClick={() => setIsOpen(false)}
                      className={`
                        group flex items-center gap-3 rounded-xl px-3.5 py-2.5 text-[13px] font-medium transition-all duration-200
                        ${isCollapsed ? "justify-center px-0" : ""}
                        ${active 
                          ? "bg-sidebar-selected text-midnight shadow-lg shadow-aurora-teal/20" 
                          : "text-sidebar-text-secondary hover:bg-sidebar-hover hover:text-sidebar-text"
                        }
                      `}
                      title={isCollapsed ? item.label : ""}
                    >
                      <span className={`flex-shrink-0 ${active ? "text-midnight" : "text-sidebar-text-secondary/70 group-hover:text-sidebar-text"}`}>
                        {item.icon}
                      </span>
                      {!isCollapsed && <span>{item.label}</span>}
                    </Link>
                  );
                })}
              </div>
            </div>
          ))}
        </nav>

        <div
          ref={userMenuRef}
          className={`relative flex-shrink-0 border-t border-white/10 pt-4 ${isCollapsed ? "flex justify-center" : ""}`}
        >
          {isUserMenuOpen && (
            <div
              className={`
                absolute bottom-full mb-2 z-50
                ${isCollapsed ? "left-full ml-2 w-56" : "left-0 right-0"}
                bg-surface rounded-xl shadow-2xl border border-line overflow-hidden
                animate-in fade-in slide-in-from-bottom-2 duration-150
              `}
            >
              <div className="px-4 py-3 border-b border-line/50">
                <div className="text-[13px] font-semibold text-ink-950 truncate">
                  {displayName || "Signed in"}
                </div>
                <div className="text-[11px] text-ink-500 truncate">{displaySubtext}</div>
              </div>
              <button
                type="button"
                onClick={handleLogout}
                disabled={isLoggingOut}
                className="w-full flex items-center gap-2.5 px-4 py-2.5 text-[13px] font-medium text-red hover:bg-red-bg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {isLoggingOut ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <LogOut className="h-4 w-4" />
                )}
                <span>{isLoggingOut ? "Signing out…" : "Sign out"}</span>
              </button>
            </div>
          )}

          <button
            type="button"
            onClick={() => setIsUserMenuOpen((v) => !v)}
            className={`flex items-center gap-3 rounded-xl px-3 py-2.5 hover:bg-sidebar-hover transition-colors duration-200 cursor-pointer w-full text-left ${isCollapsed ? "justify-center" : ""}`}
            aria-haspopup="menu"
            aria-expanded={isUserMenuOpen}
          >
            <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-sidebar-selected text-[11px] font-bold text-midnight shadow-lg shadow-aurora-teal/20">
              {avatarInitials}
            </div>
            {!isCollapsed && (
              <>
                <div className="flex-1 min-w-0">
                  <div className="text-[12px] font-semibold truncate text-sidebar-text">
                    {displayName || "Signed-in user"}
                  </div>
                  <div className="text-[10px] text-sidebar-text-secondary/70 truncate">{displaySubtext}</div>
                </div>
                <ChevronUp
                  className={`h-3.5 w-3.5 text-sidebar-text-secondary/70 flex-shrink-0 transition-transform duration-150 ${isUserMenuOpen ? "" : "rotate-180"}`}
                />
              </>
            )}
          </button>
        </div>
      </aside>

      {isOpen && (
        <div
          className="lg:hidden fixed inset-0 z-30 bg-black/50 backdrop-blur-sm"
          onClick={() => setIsOpen(false)}
        />
      )}

      <div className={`flex-1 flex flex-col min-h-screen w-full transition-all duration-300 ${isCollapsed && isDesktop ? "lg:ml-[68px]" : "lg:ml-[280px]"}`}>
        <header className="sticky top-0 z-20 flex h-[70px] items-center justify-between px-4 sm:px-6 bg-surface/70 backdrop-blur-xl border-b border-line/20 rounded-b-3xl shadow-sm gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <button
              onClick={handleHamburgerClick}
              className="flex items-center justify-center w-9 h-9 rounded-lg hover:bg-surface-2/50 transition-colors flex-shrink-0 text-ink-950"
              aria-label="Toggle sidebar"
            >
              <ClassicHamburger />
            </button>
            <span className="font-display text-[18px] font-semibold text-ink-950 truncate">
              {pageName}
            </span>
          </div>

          <div className="hidden md:flex items-center gap-2 px-4 py-2 border border-line/50 rounded-xl bg-surface/50 backdrop-blur-sm text-ink-500 flex-1 max-w-[500px] mx-4">
            <Search className="h-4 w-4 text-ink-500 flex-shrink-0" />
            <input
              type="text"
              placeholder="Search fleet, provider, trip..."
              className="bg-transparent border-0 outline-0 w-full text-[13px] text-ink-900 placeholder:text-ink-500 font-sans"
            />
            <kbd className="text-[10px] text-ink-500 bg-surface-3 px-2 py-0.5 rounded font-mono flex-shrink-0">⌘K</kbd>
          </div>

          <div className="flex items-center gap-2 sm:gap-3 flex-shrink-0">
            <ThemeToggle />
            
            <button className="relative flex items-center justify-center h-9 w-9 rounded-lg hover:bg-surface-2/50 transition-colors flex-shrink-0">
              <Bell className="h-4 w-4 text-ink-950" />
              <span className="absolute -top-0.5 -right-0.5 h-2.5 w-2.5 rounded-full bg-red border-2 border-surface" />
            </button>

            {/* <div className="hidden sm:flex items-center gap-1.5 sm:gap-2 pl-2 sm:pl-3 border-l border-line/60">
              <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-midnight text-[11px] font-bold text-white">
                {avatarInitials}
              </div>
              <div className="hidden lg:block min-w-0 max-w-[140px]">
                <div className="text-[13px] font-semibold text-ink-950 truncate leading-tight">
                  {displayName || "Signed-in user"}
                </div>
                <div className="text-[11px] text-ink-500 truncate leading-tight">
                  {displayRole}
                </div>
              </div>
              <ChevronDown className="hidden lg:block h-3.5 w-3.5 text-ink-400 flex-shrink-0" />
            </div> */}

            <div className="flex sm:hidden h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-midnight text-[11px] font-bold text-white">
              {avatarInitials}
            </div>
          </div>
        </header>

        <main className="flex-1 overflow-y-auto p-4 sm:p-6 lg:p-8 w-full">
          {/* {isPending() && (
            <div className="mb-6">
              <PendingVerificationBanner organizationName={organization?.legalName} />
            </div>
          )} */}
          {children}
        </main>
      </div>
    </div>
  );
}