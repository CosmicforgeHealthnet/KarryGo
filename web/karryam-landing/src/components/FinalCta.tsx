import { Apple, ArrowRight, Play } from "lucide-react";

export default function FinalCta() {
  return (
    <section
      id="join"
      className="relative scroll-mt-20 overflow-hidden bg-midnight text-white py-16 lg:py-20"
    >
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_60%_80%_at_85%_100%,rgba(0,194,168,0.22),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_45%_60%_at_0%_0%,rgba(45,58,91,0.25),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-x-0 bottom-0 h-24 opacity-30 [background-image:repeating-linear-gradient(90deg,rgba(0,194,168,0.5)_0_2px,transparent_2px_14px)] [mask-image:linear-gradient(to_top,black,transparent)]"
      />

      <div className="relative mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-10 lg:gap-8 items-center">
        <div className="lg:col-span-6">
          <h2 className="text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight">
            Be part of <span className="text-aurora">what&apos;s next.</span>
          </h2>

          <p className="mt-4 max-w-md text-sm sm:text-[15px] leading-relaxed text-white/70">
            Join the waitlist and be the first to move, send, sell and grow with
            Karryam.
          </p>

          <form
            action="/waitlist"
            method="get"
            className="final-cta-form mt-7 flex w-full max-w-[28rem] items-center rounded-full border border-white/15 bg-white/[0.06] p-1 backdrop-blur focus-within:border-aurora/60 transition-colors"
          >
            <label htmlFor="cta-email" className="sr-only">
              Email address
            </label>
            <input
              id="cta-email"
              type="email"
              name="email"
              required
              autoComplete="email"
              placeholder="Enter your email address"
              className="min-w-0 flex-1 bg-transparent px-4 py-2.5 text-[13px] text-white placeholder:text-white/45 outline-none"
            />
            <button
              type="submit"
              className="final-cta-button shrink-0 inline-flex items-center gap-2 rounded-full bg-aurora px-5 py-2.5 text-[13px] font-semibold text-midnight shadow-sm hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-aurora"
            >
              Join the Waitlist
              <ArrowRight aria-hidden size={14} strokeWidth={2.5} className="interaction-arrow" />
            </button>
          </form>
        </div>

        <div className="lg:col-span-6 flex flex-wrap items-center gap-3 lg:justify-end">
          <span
            aria-disabled="true"
            className="inline-flex items-center gap-3 rounded-xl border border-white/15 bg-white/[0.06] px-4 py-2.5 backdrop-blur"
          >
            <Apple size={22} strokeWidth={1.75} />
            <span className="leading-tight">
              <span className="block text-[10px] text-white/60">
                Coming soon on the
              </span>
              <span className="block text-[14px] font-semibold">App Store</span>
            </span>
          </span>

          <span
            aria-disabled="true"
            className="inline-flex items-center gap-3 rounded-xl border border-white/15 bg-white/[0.06] px-4 py-2.5 backdrop-blur"
          >
            <Play size={20} strokeWidth={1.75} />
            <span className="leading-tight">
              <span className="block text-[10px] text-white/60">
                Coming soon on
              </span>
              <span className="block text-[14px] font-semibold">
                Google Play
              </span>
            </span>
          </span>
        </div>
      </div>
    </section>
  );
}