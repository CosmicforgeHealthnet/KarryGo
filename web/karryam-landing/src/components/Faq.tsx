"use client";

import { useEffect, useRef, useState, useId } from "react";
import { ArrowRight, ChevronDown } from "lucide-react";

const vars = (o: Record<string, string | number>) => o as React.CSSProperties;

const FAQS = [
  {
    q: "When will Karryam launch?",
    a: "We are launching soon. Join the waitlist and we will email you as soon as we go live in your city.",
  },
  {
    q: "Is Karryam available in my city?",
    a: "We are starting in selected Nigerian markets, highlighting Lagos, Abuja (FCT), Port Harcourt (Rivers State) and Oyo State. Other states may follow; being listed does not mean a service is live there. Join the waitlist for updates.",
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
  const sectionRef = useRef<HTMLElement>(null);
  const [inView, setInView] = useState(false);
  const [open, setOpen] = useState<Set<number>>(new Set());
  const uid = useId();

  useEffect(() => {
    const el = sectionRef.current;
    if (!el) return;
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setInView(true);
          io.disconnect();
        }
      },
      { threshold: 0.2 }
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  const toggle = (i: number) =>
    setOpen((prev) => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i);
      else next.add(i);
      return next;
    });

  return (
    <section
      ref={sectionRef}
      data-inview={inView}
      id="faq"
      className="relative scroll-mt-20 bg-white text-midnight py-16 lg:py-20"
    >
      <style>{`
        @keyframes fq-rise {
          from { opacity: 0; translate: 0 28px; }
          to { opacity: 1; translate: 0 0; }
        }

        @keyframes fq-shimmer {
          from { background-position: 0% 50%; }
          to { background-position: 200% 50%; }
        }

        .fq-shimmer {
          background-image: linear-gradient(90deg, #0B1F44 0%, #0B1F44 35%, #00A28C 50%, #0B1F44 65%, #0B1F44 100%);
          background-size: 200% 100%;
          -webkit-background-clip: text;
          background-clip: text;
          color: transparent;
          animation: fq-shimmer 7s linear infinite;
        }

        .fq-rise { opacity: 0; }

        [data-inview="true"] .fq-rise {
          animation: fq-rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }

        /* smooth accordion: animate grid row 0fr -> 1fr, no JS height math */
        .fq-panel {
          display: grid;
          grid-template-rows: 0fr;
          opacity: 0;
          transition:
            grid-template-rows 0.45s cubic-bezier(0.22, 1, 0.36, 1),
            opacity 0.35s ease;
        }
        .fq-panel[data-open="true"] {
          grid-template-rows: 1fr;
          opacity: 1;
        }
        .fq-panel > div { overflow: hidden; }

        .fq-q { transition: color 0.25s ease; }
        .fq-btn:hover .fq-q { color: #00A28C; }
        .fq-btn[aria-expanded="true"] .fq-q { color: #00A28C; }

        @media (prefers-reduced-motion: reduce) {
          .fq-rise { opacity: 1; animation: none !important; }
          .fq-shimmer { animation: none; background-image: none; color: inherit; }
          .fq-panel { transition: none; }
          .fq-q { transition: none; }
        }
      `}</style>

      <div className="mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-10 lg:gap-12 items-start">
        <div className="lg:col-span-4">
          <h2
            className="fq-rise text-[clamp(1.5rem,2.2vw,2.1rem)] font-bold leading-[1.15] tracking-tight"
            style={vars({ "--d": "0s" })}
          >
            Frequently Asked
            <br />
            <span className="fq-shimmer">Questions</span>
          </h2>

          <p
            className="fq-rise mt-4 max-w-sm text-sm leading-relaxed text-midnight/70"
            style={vars({ "--d": "0.1s" })}
          >
            Quick answers to common questions about Karryam.
          </p>

          <a
            href="/waitlist"
            className="fq-rise group mt-7 inline-flex items-center gap-2 rounded-full border border-aurora px-5 py-2.5 text-[12px] font-semibold text-aurora-ink hover:bg-aurora hover:text-midnight transition-colors"
            style={vars({ "--d": "0.2s" })}
          >
            Join the waitlist
            <ArrowRight
              aria-hidden
              size={13}
              strokeWidth={2.5}
              className="transition-transform duration-300 group-hover:translate-x-1"
            />
          </a>
        </div>

        <div className="lg:col-span-8 divide-y divide-midnight/10 border-y border-midnight/10">
          {FAQS.map(({ q, a }, i) => {
            const isOpen = open.has(i);
            const btnId = `${uid}-q-${i}`;
            const panelId = `${uid}-a-${i}`;
            return (
              <div
                key={q}
                className="fq-rise py-1"
                style={vars({ "--d": `${0.15 + i * 0.1}s` })}
              >
                <h3>
                  <button
                    type="button"
                    id={btnId}
                    aria-expanded={isOpen}
                    aria-controls={panelId}
                    onClick={() => toggle(i)}
                    className="fq-btn flex w-full cursor-pointer items-center justify-between gap-6 py-4 text-left text-[14px] font-semibold focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-aurora"
                  >
                    <span className="fq-q">{q}</span>
                    <ChevronDown
                      aria-hidden
                      size={18}
                      strokeWidth={2}
                      className={`shrink-0 text-aurora-ink transition-transform duration-300 ease-out ${
                        isOpen ? "rotate-180" : ""
                      }`}
                    />
                  </button>
                </h3>

                <div
                  id={panelId}
                  role="region"
                  aria-labelledby={btnId}
                  aria-hidden={!isOpen}
                  data-open={isOpen}
                  className="fq-panel"
                >
                  <div>
                    <p className="max-w-2xl pb-4 text-[13px] leading-relaxed text-midnight/70">
                      {a}
                    </p>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}