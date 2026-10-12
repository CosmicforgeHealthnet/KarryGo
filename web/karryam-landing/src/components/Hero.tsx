import Image from "next/image";
import { ArrowRight, Globe2, Layers, ShieldCheck } from "lucide-react";

const TRUST = [
  { icon: ShieldCheck, title: "Secure", sub: "& Trusted" },
  { icon: Globe2, title: "Built for", sub: "Africa" },
  { icon: Layers, title: "Scalable", sub: "Platform" },
];

export default function Hero() {
  return (
    <section
      id="home"
      className="relative bg-midnight text-white overflow-hidden flex items-center lg:min-h-[100svh]"
    >
      {/* Background depth */}
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_90%_40%_at_50%_80%,rgba(0,194,168,0.06),transparent_70%)] lg:bg-[radial-gradient(ellipse_60%_60%_at_70%_50%,rgba(0,194,168,0.06),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_50%_40%_at_0%_0%,rgba(45,58,91,0.25),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-0 opacity-[0.06] [background-image:radial-gradient(#fff_1px,transparent_1px)] [background-size:28px_28px] [mask-image:linear-gradient(to_bottom,black,transparent_80%)]"
      />
      <div
        aria-hidden
        className="absolute inset-x-0 bottom-0 h-28 bg-gradient-to-t from-midnight to-transparent"
      />

      {/* pt-24 clears the fixed navbar (h-16 mobile / h-20 desktop) */}
      <div className="relative mx-auto w-full max-w-[1440px] px-5 sm:px-8 lg:px-12 pt-24 pb-8 lg:pb-12 grid lg:grid-cols-12 gap-6 sm:gap-10 lg:gap-4 items-center">
        {/* Left: copy */}
        <div className="hero-copy lg:col-span-5 flex flex-col">
          <p className="text-aurora text-[10px] sm:text-[11px] tracking-[0.24em] font-semibold uppercase">
            Movement Intelligence
          </p>

          <h1 className="mt-4 sm:mt-5 text-[clamp(1.9rem,6vw,2.75rem)] lg:text-[clamp(2rem,3.1vw,3.25rem)] font-bold leading-[1.1] tracking-tight">
            One platform for
            <br className="hidden lg:block" /> how people, goods
            <br className="hidden lg:block" /> and businesses{" "}
            <span className="text-aurora">move.</span>
          </h1>

          <p className="mt-4 sm:mt-5 max-w-[26rem] text-sm leading-relaxed text-white/70">
            Ride, Send, Truck, Food and Shop — all in one ecosystem. Built for
            customers, providers, vendors, fleets and businesses. Launching
            soon.
          </p>

          {/* Inline waitlist form: stacked on phones, pill from sm up */}
          <form
            action="/waitlist"
            method="get"
            className="mt-6 sm:mt-7 flex w-full max-w-[26rem] flex-col gap-1.5 rounded-2xl border border-white/15 bg-white/[0.06] p-1.5 backdrop-blur focus-within:border-aurora/60 transition-colors sm:flex-row sm:items-center sm:gap-0 sm:rounded-full sm:p-1"
          >
            <label htmlFor="hero-email" className="sr-only">
              Email address
            </label>
            <input
              id="hero-email"
              type="email"
              name="email"
              required
              placeholder="Enter your email address"
              className="min-w-0 flex-1 bg-transparent px-4 py-3 sm:py-2.5 text-[13px] text-white placeholder:text-white/45 outline-none"
            />
            <button
              type="submit"
              className="shrink-0 inline-flex w-full items-center justify-center gap-2 rounded-full bg-aurora px-5 py-3 sm:w-auto sm:py-2.5 text-[13px] font-semibold text-midnight hero-cta hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-aurora"
            >
              Join the Waitlist
              <ArrowRight aria-hidden size={14} strokeWidth={2.5} className="hero-cta-arrow" />
            </button>
          </form>

          {/* Trust row */}
          <ul className="mt-6 sm:mt-8 flex flex-wrap items-center gap-x-5 sm:gap-x-7 gap-y-3">
            {TRUST.map(({ icon: Icon, title, sub }) => (
              <li key={title} className="flex items-center gap-2.5">
                <span className="grid h-8 w-8 place-items-center rounded-full border border-aurora/40 bg-aurora/10 text-aurora">
                  <Icon size={15} strokeWidth={2} />
                </span>
                <span className="text-[11px] leading-tight text-white/75">
                  {title}
                  <br />
                  {sub}
                </span>
              </li>
            ))}
          </ul>
        </div>

        {/* Right: hero image, blended into the background */}
        <div className="hero-art lg:col-span-7 relative">
          <Image
            src="/images/hero-movement-map-v2.png"
            alt="Karryam app connecting Ride, Send, Truck, Food and Shop with providers, vendors, fleets and API partners"
            width={1672}
            height={941}
            priority
            sizes="(min-width: 1024px) 58vw, 100vw"
            className="relative mx-auto w-full max-w-[640px] h-auto lg:max-w-none lg:w-[108%] lg:-ml-[4%] select-none"
          />
        </div>
      </div>
    </section>
  );
}