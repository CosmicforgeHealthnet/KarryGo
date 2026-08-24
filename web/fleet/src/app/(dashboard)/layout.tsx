// src/app/(dashboard)/layout.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import Image from "next/image";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Users,
  Truck,
  Briefcase,
  Warehouse,
  Gem,
  Package,
  Wallet,
  Shield,
  Layers,
  MessageSquare,
  FileText,
  ChartNoAxesCombined,
  Lock,
  Smartphone,
  Route,
  Settings,
  UserCog,
  FileCheck,
  Search,
  Bell,
  ClipboardList,
  Calendar,
  MapPin,
  GitBranch,
  Radar,
  LifeBuoy,
  AlertTriangle,
  Car,
  Wrench,
  Fuel,
  Gauge,
  Bike,
} from "lucide-react";

// ===== NAVIGATION SECTIONS =====
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
      { label: "Trips", href: "/operations/trips", icon: <Calendar className="h-4 w-4" /> },
      { label: "Rescue", href: "/operations/rescue", icon: <LifeBuoy className="h-4 w-4" /> },
      // { label: "Alerts", href: "/operations/alerts", icon: <AlertTriangle className="h-4 w-4" /> },
      { label: "Geofences", href: "/operations/geofences", icon: <MapPin className="h-4 w-4" /> },
    ]
  },
  {
    label: "RESOURCES",
    items: [
      { label: "Providers", href: "/providers", icon: <Users className="h-4 w-4" /> },
      { label: "Fleet", href: "/fleet/assets", icon: <Car className="h-4 w-4" /> },
      { label: "Workforce", href: "/workforce", icon: <Briefcase className="h-4 w-4" /> },
      { label: "Warehouse", href: "/warehouse", icon: <Warehouse className="h-4 w-4" /> },
    ]
  },
  {
    label: "COMMERCIAL",
    items: [
      { label: "CRM", href: "/crm/customers", icon: <Gem className="h-4 w-4" /> },
      // { label: "Private Ops", href: "/private-ops/orders", icon: <Package className="h-4 w-4" /> },
      { label: "Finance", href: "/finance", icon: <Wallet className="h-4 w-4" /> },
    ]
  },
  {
    label: "COMPLIANCE & REPORTS",
    items: [
      { label: "Compliance", href: "/compliance", icon: <Shield className="h-4 w-4" /> },
      { label: "Reports", href: "/reports/library", icon: <ChartNoAxesCombined className="h-4 w-4" /> },
    ]
  },
  {
    label: "SYSTEM",
    items: [
      { label: "Governance", href: "/settings/staff", icon: <Lock className="h-4 w-4" /> },
      { label: "Settings", href: "/settings", icon: <Settings className="h-4 w-4" /> },
      { label: "Documents", href: "/documents", icon: <FileText className="h-4 w-4" /> },
    ]
  },
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
    <span className="block h-0.5 bg-ink-950 rounded-full w-4" />
    <span className="block h-0.5 bg-ink-950 rounded-full w-6" />
    <span className="block h-0.5 bg-ink-950 rounded-full w-5" />
  </div>
);

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const [isOpen, setIsOpen] = useState(false);
  const [isCollapsed, setIsCollapsed] = useState(false);
  const [isDesktop, setIsDesktop] = useState(false);
  const pageName = getPageName(pathname);
  const activeItem = findActiveNavItem(pathname);

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

  const handleHamburgerClick = () => {
    if (isDesktop) {
      setIsCollapsed(!isCollapsed);
    } else {
      setIsOpen(!isOpen);
    }
  };

  return (
    <div className="min-h-screen bg-[#fafafc] flex">
      {/* ===== SIDEBAR ===== */}
      <aside
        className={`
          fixed top-0 left-0 z-40
          ${isCollapsed ? "w-[68px]" : "w-[280px]"}
          h-screen
          bg-[#141455]/90 backdrop-blur-xl
          transform transition-all duration-300 ease-in-out
          ${isOpen ? "translate-x-0" : "-translate-x-full"}
          lg:translate-x-0
          px-3.5 py-5 text-white
          flex flex-col
          shadow-xl
          border-r border-white/10
          overflow-y-auto
          scrollbar-hide
        `}
      >
        {/* Logo */}
        <div className={`flex items-center gap-3 px-2 pb-6 pt-14 lg:pt-5 flex-shrink-0 ${isCollapsed ? "justify-center" : ""}`}>
          <Image 
            src="/images/fleet-logo.png" 
            alt="KarryGo Fleet OS" 
            width={42} 
            height={42}
            className="rounded-xl object-contain flex-shrink-0"
          />
          {!isCollapsed && (
            <div className="min-w-0">
              <div className="font-display text-xl font-bold tracking-tight text-white">
                KarryGo
              </div>
              <div className="text-[9px] font-semibold tracking-[0.2em] text-[#0E91C7]/80 uppercase">
                Fleet OS
              </div>
            </div>
          )}
        </div>

        {/* Navigation */}
        <nav className="flex-1 space-y-4 overflow-y-auto py-4 scrollbar-hide">
          {NAV_SECTIONS.map((section, idx) => (
            <div key={idx}>
              {!isCollapsed && (
                <div className="px-3 text-[10px] font-semibold text-[#E8E8F8]/60 tracking-[0.1em] uppercase">
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
                          ? "bg-[#2B2CAA] text-white shadow-lg shadow-[#2B2CAA]/20" 
                          : "text-[#E8E8F8] hover:bg-[#1E1F7A] hover:text-white"
                        }
                      `}
                      title={isCollapsed ? item.label : ""}
                    >
                      <span className={`flex-shrink-0 ${active ? "text-[#0E91C7]" : "text-[#E8E8F8]/60 group-hover:text-white"}`}>
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

        {/* User */}
        <div className={`flex-shrink-0 border-t border-white/10 pt-4 ${isCollapsed ? "flex justify-center" : ""}`}>
          <div className={`flex items-center gap-3 rounded-xl px-3 py-2.5 hover:bg-[#1E1F7A] transition-colors duration-200 cursor-pointer ${isCollapsed ? "justify-center" : ""}`}>
            <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-[#2B2CAA] text-[11px] font-bold text-white shadow-lg shadow-[#2B2CAA]/20">
              U
            </div>
            {!isCollapsed && (
              <div className="flex-1 min-w-0">
                <div className="text-[12px] font-semibold truncate text-white">Signed-in user</div>
                <div className="text-[10px] text-[#E8E8F8]/60 truncate">Role placeholder</div>
              </div>
            )}
          </div>
        </div>
      </aside>

      {/* Mobile overlay */}
      {isOpen && (
        <div
          className="lg:hidden fixed inset-0 z-30 bg-black/50 backdrop-blur-sm"
          onClick={() => setIsOpen(false)}
        />
      )}

      {/* ===== MAIN CONTENT ===== */}
      <div className={`flex-1 flex flex-col min-h-screen w-full transition-all duration-300 ${isCollapsed && isDesktop ? "lg:ml-[68px]" : "lg:ml-[280px]"}`}>
        {/* ===== HEADER ===== */}
        <header className="sticky top-0 z-20 flex h-[70px] items-center justify-between px-4 sm:px-6 bg-white/70 backdrop-blur-xl border-b border-white/20 rounded-b-3xl shadow-sm">
          <div className="flex items-center gap-3">
            <button
              onClick={handleHamburgerClick}
              className="flex items-center justify-center w-9 h-9 rounded-lg hover:bg-surface-2/50 transition-colors"
              aria-label="Toggle sidebar"
            >
              <ClassicHamburger />
            </button>
            <span className="font-display text-[18px] font-semibold text-ink-950">
              {pageName}
            </span>
          </div>

          <div className="hidden md:flex items-center gap-2 px-4 py-2 border border-line/50 rounded-xl bg-white/50 backdrop-blur-sm text-ink-500 flex-1 max-w-[500px] mx-4">
            <Search className="h-4 w-4 text-ink-500" />
            <input
              type="text"
              placeholder="Search fleet, provider, trip..."
              className="bg-transparent border-0 outline-0 w-full text-[13px] text-ink-900 placeholder:text-ink-500 font-sans"
            />
            <kbd className="text-[10px] text-ink-500 bg-surface-3 px-2 py-0.5 rounded font-mono">⌘K</kbd>
          </div>

          <div className="flex items-center gap-2">
            <button className="relative flex items-center justify-center h-9 w-9 rounded-lg hover:bg-surface-2/50 transition-colors">
              <Bell className="h-4 w-4 text-navy-950" />
              <span className="absolute -top-0.5 -right-0.5 h-2.5 w-2.5 rounded-full bg-red border-2 border-white" />
            </button>
          </div>
        </header>

        <main className="flex-1 overflow-y-auto p-4 sm:p-6 lg:p-8 w-full">
          {children}
        </main>
      </div>
    </div>
  );
}