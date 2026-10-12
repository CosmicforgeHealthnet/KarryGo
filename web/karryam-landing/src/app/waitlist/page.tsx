// web/karryam-landing/src/app/waitlist/page.tsx
import type { Metadata } from "next";
import Image from "next/image";
import WaitlistForm from "@/components/WaitlistForm";

export const metadata: Metadata = {
  title: "Join the Karryam waitlist",
  description:
    "Join the Karryam waitlist for customers, providers, vendors, fleets, and API partners.",
};

export default function WaitlistPage() {
  return (
    <main className="relative isolate min-h-[100svh] overflow-hidden bg-midnight text-white">
      {/* Ambient background */}
      <div aria-hidden className="pointer-events-none absolute inset-0 -z-10">
        <div className="absolute left-1/2 top-[-12rem] h-[34rem] w-[60rem] max-w-[140vw] -translate-x-1/2 rounded-full bg-aurora/20 blur-[120px] motion-safe:animate-pulse [animation-duration:6s]" />
        <div className="absolute bottom-[-14rem] right-[-8rem] h-[26rem] w-[26rem] rounded-full bg-aurora/10 blur-[110px]" />
        <div className="absolute inset-0 bg-[linear-gradient(to_right,rgba(255,255,255,0.05)_1px,transparent_1px),linear-gradient(to_bottom,rgba(255,255,255,0.05)_1px,transparent_1px)] bg-[size:56px_56px] [mask-image:radial-gradient(ellipse_70%_60%_at_50%_30%,black,transparent_75%)]" />
      </div>

      {/* Fluid page frame */}
      <div className="mx-auto flex w-full max-w-[clamp(20rem,92vw,56rem)] flex-col items-center px-[clamp(1rem,4vw,3rem)] pb-[clamp(3rem,8vw,7rem)] pt-[clamp(5rem,12vw,9rem)] text-center">
        {/* Logo */}
        <Image
          src="/images/logo.png"
          alt="Karryam"
          width={160}
          height={48}
          priority
          className="h-[clamp(2rem,5vw,2.75rem)] w-auto select-none"
        />

        <span className="mt-[clamp(1.5rem,4vw,2.5rem)] inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[0.05] px-[clamp(0.75rem,2vw,1rem)] py-[clamp(0.35rem,1vw,0.5rem)] text-[clamp(0.7rem,1.4vw,0.8rem)] font-medium text-white/80 backdrop-blur">
          <span className="relative flex h-2 w-2">
            <span className="absolute inline-flex h-full w-full rounded-full bg-aurora opacity-60 motion-safe:animate-ping" />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-aurora" />
          </span>
          Early access is open
        </span>

        <h1 className="mt-[clamp(1.5rem,4vw,2.25rem)] max-w-[22ch] text-[clamp(1.75rem,5vw,2.75rem)] font-bold leading-[1.12] tracking-tight text-white">
          Be first in line when Karryam launches
        </h1>

        <p className="mt-[clamp(1rem,3vw,1.5rem)] max-w-[46ch] text-[clamp(0.9rem,1.7vw,1.0625rem)] leading-[1.6] text-white/65">
          Tell us who you are and we&apos;ll email you the moment Karryam goes
          live in your area.
        </p>

        <div className="mt-[clamp(2.5rem,7vw,4.5rem)] w-full">
          <WaitlistForm />
        </div>
      </div>
    </main>
  );
}