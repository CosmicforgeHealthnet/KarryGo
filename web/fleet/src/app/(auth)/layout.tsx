"use client";

import Image from "next/image";
import { usePathname } from "next/navigation";

function RouteScene() {
  return (
    <svg
      viewBox="0 0 600 700"
      fill="none"
      className="absolute inset-0 h-full w-full"
      aria-hidden="true"
    >
      <path
        id="route"
        d="M70 610 C 140 560, 120 480, 200 440 C 300 390, 260 300, 360 260 C 430 232, 420 170, 500 120"
        stroke="url(#roadGradient)"
        strokeWidth="2"
        strokeLinecap="round"
        strokeDasharray="1 10"
      />

      {/* stop pins */}
      <g transform="translate(70 610)">
        <path
          d="M0 -20 C 8 -20 14 -14 14 -6 C 14 4 0 16 0 16 C 0 16 -14 4 -14 -6 C -14 -14 -8 -20 0 -20 Z"
          className="fill-accent"
        />
        <circle r="4" className="fill-navy-950" />
        <text x="20" y="4" className="fill-white/55 font-sans text-[11px]">
          Ikeja depot
        </text>
      </g>

      <g transform="translate(360 260)">
        {/* <path
          d="M0 -16 C 6.5 -16 11 -11 11 -5 C 11 3 0 13 0 13 C 0 13 -11 3 -11 -5 C -11 -11 -6.5 -16 0 -16 Z"
          className="fill-white/70"
        /> */}
        <circle r="3" className="fill-navy-950" />
        {/* <text x="16" y="4" className="fill-white/40 font-sans text-[11px]">
          Apapa
        </text> */}
      </g>

      <g transform="translate(500 120)">
        <path
          d="M0 -20 C 8 -20 14 -14 14 -6 C 14 4 0 16 0 16 C 0 16 -14 4 -14 -6 C -14 -14 -8 -20 0 -20 Z"
          className="fill-accent"
        />
        <circle r="4" className="fill-navy-950" />
        {/* <text x="20" y="4" className="fill-white/55 font-sans text-[11px]">
          Lagos Island
        </text> */}
      </g>

      {/* traveling vehicle */}
      <g className="motion-safe:block motion-reduce:hidden">
        <g>
          <animateMotion dur="7s" repeatCount="indefinite" rotate="auto" path="M70 610 C 140 560, 120 480, 200 440 C 300 390, 260 300, 360 260 C 430 232, 420 170, 500 120" />
          <circle r="11" className="fill-white/15" />
          <g transform="translate(-8 -6)">
            <rect x="0" y="2" width="10" height="7" rx="1.5" className="fill-white" />
            <path d="M10 4 H14 L16.5 7 V9 H10 Z" className="fill-white" />
            <circle cx="3.5" cy="9.5" r="1.6" className="fill-navy-950" />
            <circle cx="13" cy="9.5" r="1.6" className="fill-navy-950" />
          </g>
        </g>
      </g>

      <defs>
        <linearGradient id="roadGradient" x1="70" y1="610" x2="500" y2="120" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor="#0E91C7" stopOpacity="0.8" />
          <stop offset="1" stopColor="#ffffff" stopOpacity="0.25" />
        </linearGradient>
      </defs>
    </svg>
  );
}

function BrandPanel() {
  return (
    <div className="relative hidden w-[46%] overflow-hidden bg-navy-950 lg:block">
      {/* soft organic light instead of a repeating grid */}
      <div className="pointer-events-none absolute -left-24 -top-24 h-[420px] w-[420px] rounded-full bg-accent/25 blur-[110px]" />
      <div className="pointer-events-none absolute bottom-[-160px] right-[-100px] h-[380px] w-[380px] rounded-full bg-primary/40 blur-[100px]" />

      <RouteScene />

      <div className="relative flex h-full flex-col justify-between p-12 xl:p-16">
        <div />

        <div className="max-w-[440px]">
          <h2 className="font-display text-[34px] font-medium leading-[1.15] tracking-[-0.02em] text-white xl:text-[40px]">
            Every vehicle, every route, one control tower.
          </h2>
          <p className="mt-4 text-[15px] leading-relaxed text-sidebar-text-secondary/80">
            Sign in to dispatch, track compliance, and keep your whole fleet
            moving.
          </p>
        </div>

        {/* dashboard preview */}
        <div className="fade-in-up relative -mb-12 -mr-12 self-end xl:-mb-16 xl:-mr-16">
          <div className="overflow-hidden rounded-2xl border border-white/10 shadow-2xl shadow-black/40 ring-1 ring-white/5">
            <Image
              src="/images/dashboard-preview.png"
              alt="KarryGo Fleet OS dashboard"
              width={1366}
              height={643}
              className="block w-[480px] xl:w-[560px]"
              priority
            />
          </div>
        </div>
      </div>
    </div>
  );
}

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const isSignIn = pathname === "/sign-in";

  if (isSignIn) {
    return (
      <div className="flex min-h-screen bg-white">
        <div className="flex w-full flex-col justify-center px-6 sm:px-12 lg:w-[54%] lg:px-20 xl:px-28">
          <div className="mb-12 flex items-center gap-2.5">
            <Image
              src="/images/fleet-logo.png"
              alt="KarryGo Fleet OS"
              width={32}
              height={32}
              className="rounded-lg object-contain"
            />
            <div>
              <div className="font-display text-[16px] font-semibold tracking-tight text-ink-950">
                KarryGo
              </div>
              <div className="text-[9px] font-semibold tracking-[0.2em] text-ink-500 uppercase">
                Fleet OS
              </div>
            </div>
          </div>
          <div className="fade-in-up w-full max-w-[380px]">{children}</div>
        </div>
        <BrandPanel />
      </div>
    );
  }

  return (
    <div className="relative min-h-screen bg-white">
      <div className="absolute left-8 top-8 flex items-center gap-2.5">
        <Image
          src="/images/fleet-logo.png"
          alt="KarryGo Fleet OS"
          width={32}
          height={32}
          className="rounded-lg object-contain"
        />
        <div>
          <div className="font-display text-[16px] font-semibold tracking-tight text-ink-950">
            KarryGo
          </div>
          <div className="text-[9px] font-semibold tracking-[0.2em] text-ink-500 uppercase">
            Fleet OS
          </div>
        </div>
      </div>

      <div className="flex min-h-screen items-center justify-center px-6">
        <div className="fade-in-up w-full max-w-[380px]">{children}</div>
      </div>
    </div>
  );
}