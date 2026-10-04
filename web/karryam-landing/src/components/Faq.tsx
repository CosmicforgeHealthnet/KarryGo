import { ArrowRight, ChevronDown } from "lucide-react";

const FAQS = [
  {
    q: "When will Karryam launch?",
    a: "We are launching soon. Join the waitlist and we will email you as soon as we go live in your city.",
  },
  {
    q: "Is Karryam available in my city?",
    a: "We are starting in selected Nigerian markets, beginning with Lagos, Abuja and Port Harcourt, with more cities to follow. Join the waitlist so we know where to launch next.",
  },
  {
    q: "How do I join the waitlist?",
    a: "Enter your email address on this page and tap Join the Waitlist. You will be asked to verify your email, and that is all it takes.",
  },
  {
    q: "What are the benefits of joining early?",
    a: "Waitlist members hear about the launch first, so you will know the moment Karryam is available where you are.",
  },
];

export default function Faq() {
  return (
    <section
      id="faq"
      className="relative scroll-mt-20 bg-white text-midnight py-16 lg:py-20"
    >
      <div className="mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-10 lg:gap-12 items-start">
        <div className="lg:col-span-4">
          <h2 className="text-[clamp(1.5rem,2.2vw,2.1rem)] font-bold leading-[1.15] tracking-tight">
            Frequently Asked Questions
          </h2>

          <p className="mt-4 max-w-sm text-sm leading-relaxed text-midnight/70">
            Quick answers to common questions about Karryam.
          </p>

          <a
            href="/waitlist"
            className="mt-7 inline-flex items-center gap-2 rounded-full border border-aurora px-5 py-2.5 text-[12px] font-semibold text-[#00A28C] hover:bg-aurora hover:text-midnight transition-colors"
          >
            Join the waitlist
            <ArrowRight size={13} strokeWidth={2.5} />
          </a>
        </div>

        <div className="lg:col-span-8 divide-y divide-midnight/10 border-y border-midnight/10">
          {FAQS.map(({ q, a }) => (
            <details key={q} className="group py-1">
              <summary className="flex cursor-pointer list-none items-center justify-between gap-6 py-4 text-[14px] font-semibold marker:hidden [&::-webkit-details-marker]:hidden">
                {q}
                <ChevronDown
                  size={18}
                  strokeWidth={2}
                  className="shrink-0 text-[#00A28C] transition-transform duration-200 group-open:rotate-180"
                />
              </summary>
              <p className="max-w-2xl pb-4 text-[13px] leading-relaxed text-midnight/70">
                {a}
              </p>
            </details>
          ))}
        </div>
      </div>
    </section>
  );
}