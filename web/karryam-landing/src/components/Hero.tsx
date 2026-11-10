import Image from "next/image";
import { ArrowRight, Play } from "lucide-react";

export default function Hero() {
  return (
    <section id="home" className="relative overflow-hidden bg-white text-midnight">

      <div className="relative mx-auto w-full max-w-[1440px] px-5 pt-28 sm:px-8 lg:min-h-[min(100svh,880px)] lg:px-12 lg:pt-0">
        {/* Left: copy (vertically centred on desktop) */}
        <div className="hero-copy relative z-10 flex max-w-[720px] flex-col lg:min-h-[min(100svh,880px)] lg:justify-center lg:pb-24 lg:pt-28">
          <span className="inline-flex w-fit items-center gap-2.5 rounded-full bg-aurora/10 px-4 py-2 text-[13px] font-semibold text-midnight">
            <span className="h-2 w-2 rounded-full bg-aurora" />
            Movement Intelligence
          </span>

          <h1 className="mt-6 text-[clamp(2.3rem,4.6vw,4rem)] font-bold leading-[1.06] tracking-[-0.035em]">
            One platform for how people, goods and businesses{" "}
            <span className="text-aurora">move.</span>
          </h1>

          <p className="mt-6 text-[clamp(1.1rem,1.6vw,1.4rem)] font-medium text-indigo/80">
            Ride. Send. Shop. Food. Truck.
          </p>

          <div className="mt-9 flex flex-wrap items-center gap-x-6 gap-y-4">
            <a
              href="/waitlist"
              className="hero-cta inline-flex items-center gap-2.5 rounded-full bg-aurora px-8 py-4 text-[16px] font-semibold text-white shadow-[0_12px_28px_-10px_rgba(0,194,168,0.55)] hover:brightness-105 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-aurora"
            >
              Join the Waitlist
              <ArrowRight aria-hidden size={18} strokeWidth={2.25} className="hero-cta-arrow" />
            </a>
            <a
              href="#how-it-works"
              className="group inline-flex items-center gap-3.5 text-[16px] font-semibold text-midnight focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-aurora"
            >
              <span className="grid h-14 w-14 place-items-center rounded-full border border-midnight/15 bg-white transition group-hover:border-aurora">
                <Play aria-hidden size={18} className="ml-0.5 fill-midnight text-midnight" />
              </span>
              Watch Demo
            </a>
          </div>
        </div>

      </div>

      {/* Right: lifestyle photo. Its left side is white, so on desktop it
          sits behind the copy and the white merges with the page. */}
      <div className="relative -mt-2 sm:mt-4 lg:absolute lg:inset-y-0 lg:right-0 lg:mt-0 lg:w-[min(74%,1180px)]">
        <div className="hero-photo relative aspect-[5/4] w-full overflow-hidden sm:aspect-[16/10] lg:absolute lg:bottom-0 lg:aspect-auto lg:h-full">
          <Image
            src="/images/hero-lifestyle-v2.jpg"
            alt="A woman ordering on the Karryam app, with a Karryam rider, car and truck in the city behind her"
            fill
            priority
            sizes="(min-width: 1024px) min(74vw, 1180px), 100vw"
            className="select-none object-cover object-[92%_bottom] mix-blend-multiply lg:object-contain lg:object-right-bottom"
          />
        </div>
      </div>

      {/* Curved base: white sweep with a teal line, closing the photo. */}
      <svg
        aria-hidden
        viewBox="0 0 1440 120"
        preserveAspectRatio="none"
        className="absolute inset-x-0 bottom-0 z-10 h-[34px] w-full sm:h-[64px] lg:h-[110px]"
      >
        <path d="M0,95 C320,40 620,30 900,62 C1130,88 1300,98 1440,80 L1440,120 L0,120 Z" fill="#fff" />
        <path d="M0,95 C320,40 620,30 900,62" fill="none" stroke="#00C2A8" strokeWidth="5" strokeLinecap="round" />
      </svg>
    </section>
  );
}
