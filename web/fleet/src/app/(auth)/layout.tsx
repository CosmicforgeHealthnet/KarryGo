"use client";

import Image from "next/image";
import { usePathname } from "next/navigation";

const AUTH_ROUTES_WITH_BRAND_PANEL = ["/sign-in", "/verify"];

function BrandPanel() {
  return (
    <div
      className="relative hidden w-[46%] overflow-hidden lg:block"
      style={{
        background:
          "linear-gradient(135deg, #24248f 0%, #2f30ac 50%, #3a3bc4 100%)",
      }}
    >
      {/* logo */}
      <div className="absolute left-8 top-8 flex items-center gap-2.5 xl:left-12 xl:top-10">
        <Image
          src="/images/fleet-logo.png"
          alt="KarryGo Fleet OS"
          width={36}
          height={36}
          className="rounded-lg object-contain"
        />
        <div>
          <div className="font-display text-[17px] font-semibold tracking-tight text-white">
            KarryGo
          </div>
          <div className="text-[9px] font-semibold tracking-[0.2em] text-white/60 uppercase">
            Fleet OS
          </div>
        </div>
      </div>

      <div className="relative flex h-full flex-col justify-between p-12 pt-28 xl:p-16 xl:pt-32">
        {/* headline */}
        <div className="max-w-[440px]">
          <h2 className="font-display text-[34px] font-bold leading-[1.15] tracking-[-0.02em] text-white xl:text-[40px]">
            Every vehicle,
            <br />
            every route,
            <br />
            <span className="text-accent">one control tower.</span>
          </h2>
          <p className="mt-4 text-[15px] font-normal leading-relaxed text-white/70">
            Smart fleet management starts here.
          </p>
        </div>

        {/* illustration — native colors, no filter */}
        <div className="fade-in-up relative -mb-12 -ml-16 self-start xl:-mb-16 xl:-ml-20">
          <Image
            src="/images/fleet-illustration.png"
            alt="KarryGo Fleet OS"
            width={1536}
            height={1024}
            className="block w-[520px] xl:w-[600px]"
            priority
          />
        </div>
      </div>
    </div>
  );
}

/**
 * Decorative backdrop for the form-side panel.
 * Replaces flat `bg-white` with a soft indigo-tinted gradient, a faint
 * dot-grid, and a glowing shield motif in the corner — quiet enough not
 * to compete with the form, present enough that the panel doesn't read
 * as blank/unfinished next to the brand panel.
 */
function FormPanelBackdrop() {
  return (
    <div className="pointer-events-none absolute inset-0 -z-10 overflow-hidden">
      {/* base tint — replaces pure white */}
      <div
        className="absolute inset-0"
        style={{
          background:
            "linear-gradient(160deg, #FBFBFE 0%, #F5F5FC 45%, #EFEFFA 100%)",
        }}
      />

      {/* organic wave/blob, bottom-right — sits under the glow, slightly
          more saturated than the base tint so the corner has depth
          instead of one flat gradient */}
      <svg
        viewBox="0 0 600 500"
        preserveAspectRatio="xMaxYMax slice"
        className="absolute -bottom-16 -right-16 h-[420px] w-[520px] xl:h-[500px] xl:w-[620px]"
        aria-hidden="true"
      >
        <path
          d="M600 90 C 480 40, 380 120, 400 230 C 418 328, 320 360, 340 440 C 355 490, 480 500, 600 470 Z"
          fill="#E4E3F9"
          opacity="0.55"
        />
        <path
          d="M600 180 C 500 150, 440 220, 470 300 C 495 365, 430 400, 460 460 C 475 490, 550 495, 600 480 Z"
          fill="#DAD8F6"
          opacity="0.5"
        />
      </svg>

      {/* dot grid, top-right */}
      <svg
        className="absolute right-0 top-0 h-[220px] w-[220px] text-primary/[0.14] xl:h-[260px] xl:w-[260px]"
        aria-hidden="true"
      >
        <defs>
          <pattern
            id="auth-dot-grid"
            width="18"
            height="18"
            patternUnits="userSpaceOnUse"
          >
            <circle cx="2" cy="2" r="1.6" fill="currentColor" />
          </pattern>
        </defs>
        <rect width="100%" height="100%" fill="url(#auth-dot-grid)" />
      </svg>

      {/* soft glow halo, bottom-right */}
      <div className="absolute bottom-4 right-6 h-[280px] w-[280px] rounded-full bg-white/70 blur-2xl xl:h-[340px] xl:w-[340px]" />
      <div className="absolute bottom-8 right-10 h-[220px] w-[220px] rounded-full bg-primary/[0.10] blur-3xl xl:h-[260px] xl:w-[260px]" />

      {/* glassy 3D shield badge — slightly tilted, compact proportions */}
      <svg
        viewBox="0 0 220 220"
        className="absolute bottom-10 right-4 h-[150px] w-[150px] xl:right-6 xl:h-[190px] xl:w-[190px]"
        aria-hidden="true"
      >
        <defs>
          <linearGradient id="auth-shield-fill" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#CDCBF5" />
            <stop offset="45%" stopColor="#9A97E6" />
            <stop offset="100%" stopColor="#6C68D6" />
          </linearGradient>
          <linearGradient id="auth-shield-highlight" x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#FFFFFF" stopOpacity="0.55" />
            <stop offset="100%" stopColor="#FFFFFF" stopOpacity="0" />
          </linearGradient>
          <linearGradient id="auth-check-fill" x1="0%" y1="0%" x2="0%" y2="100%">
            <stop offset="0%" stopColor="#FFFFFF" />
            <stop offset="100%" stopColor="#EDECFB" />
          </linearGradient>
          <filter id="auth-shield-shadow" x="-40%" y="-40%" width="180%" height="200%">
            <feDropShadow
              dx="0"
              dy="9"
              stdDeviation="9"
              floodColor="#4B48A8"
              floodOpacity="0.28"
            />
          </filter>
          <filter id="auth-check-shadow" x="-80%" y="-80%" width="260%" height="260%">
            <feDropShadow
              dx="0"
              dy="3"
              stdDeviation="4"
              floodColor="#2E2B7A"
              floodOpacity="0.4"
            />
          </filter>
        </defs>

        {/* shield body — shorter/wider proportions, tilted ~8deg */}
        <g transform="rotate(8 110 108)">
          <g filter="url(#auth-shield-shadow)">
            <path
              d="M110 28 L180 52 V102 C180 144 152 176 110 190 C68 176 40 144 40 102 V52 Z"
              fill="url(#auth-shield-fill)"
            />
            {/* top-left facet highlight, for the bevel/glass feel */}
            <path
              d="M110 28 L180 52 V102 C180 105 179 109 179 112 L110 34 L46 54 C44 53 43 52 40 52 Z"
              fill="url(#auth-shield-highlight)"
            />
          </g>

          {/* raised check badge — filled 3D shape, not a stroke */}
          <g filter="url(#auth-check-shadow)">
            <circle cx="110" cy="107" r="32" fill="#FFFFFF" fillOpacity="0.14" />
            <path
              d="M91 108 C 94 104, 100 104, 103 108 L 108 114 L 128 88 C 131 84, 137 84, 139 88 C 141 91, 141 95, 138 98 L 109 133 C 106 137, 100 137, 97 133 L 88 121 C 85 117, 86 111, 91 108 Z"
              fill="url(#auth-check-fill)"
            />
            {/* thin top-edge highlight on the check to sell the bevel */}
            <path
              d="M93 107 L107 122 L136 90"
              fill="none"
              stroke="#FFFFFF"
              strokeWidth="1.5"
              strokeOpacity="0.9"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </g>
        </g>
      </svg>
    </div>
  );
}

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const showBrandPanel = AUTH_ROUTES_WITH_BRAND_PANEL.includes(pathname);

  if (showBrandPanel) {
    return (
      <div className="flex min-h-screen">
        <BrandPanel />
        <div className="relative isolate flex w-full flex-col justify-center px-6 sm:px-12 lg:w-[54%] lg:px-20 xl:px-28">
          <FormPanelBackdrop />
          <div className="fade-in-up relative z-10 w-full max-w-[380px]">
            {children}
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="relative isolate min-h-screen">
      <FormPanelBackdrop />

      <div className="absolute left-8 top-8 z-10 flex items-center gap-2.5">
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

      <div className="relative z-10 flex min-h-screen items-center justify-center px-6">
        <div className="fade-in-up w-full max-w-[380px]">{children}</div>
      </div>
    </div>
  );
}