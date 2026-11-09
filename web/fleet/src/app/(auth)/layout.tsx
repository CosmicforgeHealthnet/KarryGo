// src/app/(auth)/layout.tsx

"use client";

import Image from "next/image";
import { KarryamLogo } from "@/components/brand/KarryamLogo";

// Photo panel: the Karryam truck, a teal wash for legible type, the headline.
function BrandPanel() {
  return (
    <aside className="relative hidden flex-1 overflow-hidden bg-[#0B2E33] lg:block">
      <Image
        src="/images/signin-truck.jpg"
        alt="A Karryam delivery truck on the highway"
        fill
        priority
        sizes="56vw"
        className="object-cover object-[46%_center]"
      />
      {/* Teal wash: strong behind the headline, clear over the truck. */}
      <div
        className="absolute inset-0"
        style={{
          background:
            "linear-gradient(180deg, rgba(7,42,48,0.92) 0%, rgba(7,42,48,0.70) 38%, rgba(7,42,48,0.10) 62%, rgba(7,42,48,0) 100%)",
        }}
      />
      {/* Brand shape: a rounded square turned 36deg, mostly off the top-right
          corner; its left corner sits 267px in from the edge, 150px down. */}
      <div
        className="absolute h-[520px] w-[520px] rotate-[36deg] rounded-[72px]"
        style={{
          right: -327,
          top: -168,
          // Translucent and fading toward its left point, so it reads as light
          // in the sky rather than a solid tile.
          background:
            "linear-gradient(225deg, rgba(25,199,183,0.55) 0%, rgba(25,181,168,0.32) 45%, rgba(14,143,134,0.06) 100%)",
          boxShadow: "inset 0 0 0 1px rgba(25,199,183,0.18)",
        }}
      />

      <div className="relative pl-[121px] pr-12 pt-[min(196px,17vh)]">
        <h2 className="whitespace-nowrap text-[clamp(40px,3.65vw,61px)] font-bold leading-[1.0] tracking-[-0.035em] text-white">
          Run your fleet,
          <br />
          <span className="text-[#19C7B7]">deliver more.</span>
        </h2>
        <span className="mt-8 block h-[4px] w-20 rounded-full bg-[#19C7B7]" />
        <p className="mt-9 max-w-[480px] text-[clamp(17px,1.25vw,21px)] leading-[1.55] text-white/90">
          Manage vehicles, drivers and deliveries in one connected platform.
        </p>
      </div>
    </aside>
  );
}

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="auth flex min-h-screen bg-[var(--surface)]">
      <div className="flex min-h-screen w-full flex-col px-6 py-8 sm:px-12 lg:w-[44%] lg:flex-none lg:px-16 lg:pt-[min(55px,5vh)] xl:px-24">
        <KarryamLogo height={45} />
        <main className="flex flex-1 items-center py-12 lg:items-start lg:pb-10 lg:pt-[min(106px,7vh)]">
          <div className="fade-in-up w-full max-w-[540px]">{children}</div>
        </main>
      </div>
      <BrandPanel />
    </div>
  );
}
