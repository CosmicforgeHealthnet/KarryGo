// src/app/(auth)/layout.tsx

"use client";

import Image from "next/image";
import { usePathname } from "next/navigation";

const AUTH_ROUTES_WITH_BRAND_PANEL = ["/sign-in", "/verify", "/sign-up"];

function BrandPanel() {
  return (
    <div
      className="relative hidden w-[46%] overflow-hidden lg:block"
      style={{
        // NOTE: intentionally fixed purple — same in light and dark.
        background:
          "linear-gradient(135deg, #24248f 0%, #2f30ac 50%, #3a3bc4 100%)",
      }}
    >
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
            KarryAm
          </div>
        
        </div>
      </div>

      <div className="relative flex h-full flex-col justify-between p-12 pt-28 xl:p-16 xl:pt-32">
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

function FormPanelBackdrop() {
  return (
    <div className="pointer-events-none absolute inset-0 -z-10 overflow-hidden">
      <div
        className="absolute inset-0"
        style={{
          background:
            "linear-gradient(160deg, var(--auth-backdrop-from) 0%, var(--auth-backdrop-via) 45%, var(--auth-backdrop-to) 100%)",
        }}
      />

      <svg
        viewBox="0 0 600 500"
        preserveAspectRatio="xMaxYMax slice"
        className="absolute -bottom-16 -right-16 h-[420px] w-[520px] xl:h-[500px] xl:w-[620px]"
        aria-hidden="true"
      >
        <path
          d="M600 90 C 480 40, 380 120, 400 230 C 418 328, 320 360, 340 440 C 355 490, 480 500, 600 470 Z"
          fill="var(--auth-backdrop-blob-1)"
          opacity="0.55"
        />
        <path
          d="M600 180 C 500 150, 440 220, 470 300 C 495 365, 430 400, 460 460 C 475 490, 550 495, 600 480 Z"
          fill="var(--auth-backdrop-blob-2)"
          opacity="0.5"
        />
      </svg>

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

      <div
        className="absolute bottom-4 right-6 h-[280px] w-[280px] rounded-full blur-2xl xl:h-[340px] xl:w-[340px]"
        style={{ background: "var(--auth-backdrop-fade)" }}
      />
      <div className="absolute bottom-8 right-10 h-[220px] w-[220px] rounded-full bg-primary/[0.10] blur-3xl xl:h-[260px] xl:w-[260px]" />
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
            KarryAm
          </div>
          
        </div>
      </div>

      <div className="relative z-10 flex min-h-screen items-center justify-center px-6">
        <div className="fade-in-up w-full max-w-[380px]">{children}</div>
      </div>
    </div>
  );
}