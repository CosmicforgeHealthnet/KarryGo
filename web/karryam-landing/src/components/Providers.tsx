"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { ArrowRight } from "lucide-react";

const vars = (o: Record<string, string | number>) => o as React.CSSProperties;

export default function Providers() {
  const sectionRef = useRef<HTMLElement>(null);
  const cardRef = useRef<HTMLDivElement>(null);
  const [inView, setInView] = useState(false);

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

  const canTilt = () =>
    typeof window !== "undefined" &&
    window.matchMedia(
      "(hover: hover) and (min-width: 1024px) and (prefers-reduced-motion: no-preference)"
    ).matches;

  const onMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const card = cardRef.current;
    if (!card || !canTilt()) return;
    const r = e.currentTarget.getBoundingClientRect();
    const px = (e.clientX - r.left) / r.width - 0.5;
    const py = (e.clientY - r.top) / r.height - 0.5;
    card.style.setProperty("--ry", `${(px * 9).toFixed(2)}deg`);
    card.style.setProperty("--rx", `${(-py * 7).toFixed(2)}deg`);
  };

  const onLeave = () => {
    const card = cardRef.current;
    if (!card) return;
    card.style.setProperty("--ry", "0deg");
    card.style.setProperty("--rx", "0deg");
  };

  return (
    <section
      ref={sectionRef}
      data-inview={inView}
      id="providers"
      className="relative bg-white text-midnight py-8 lg:py-20 overflow-hidden"
    >
      <style>{`
        @keyframes prv-rise {
          from { opacity: 0; translate: 0 36px; }
          to { opacity: 1; translate: 0 0; }
        }
        @keyframes prv-zoom {
          from { opacity: 0; translate: 0 40px; scale: 0.93; }
          to { opacity: 1; translate: 0 0; scale: 1; }
        }
        @keyframes prv-sweep {
          from { translate: -120% 0; }
          to { translate: 420% 0; }
        }

        .prv-rise, .prv-zoom { opacity: 0; }

        [data-inview="true"] .prv-rise {
          animation: prv-rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .prv-zoom {
          animation: prv-zoom 0.9s cubic-bezier(0.22, 1, 0.36, 1) 0.45s forwards;
        }
        [data-inview="true"] .prv-sweep {
          animation: prv-sweep 1.4s ease-in-out 1.3s 1 forwards;
        }

        /* tilt on transform; entrance uses translate/scale so they never clash */
        .prv-card {
          transform: perspective(1200px) rotateX(var(--rx, 0deg)) rotateY(var(--ry, 0deg));
          transition: transform 0.25s ease-out;
          will-change: transform;
        }

        @media (prefers-reduced-motion: reduce) {
          .prv-rise, .prv-zoom { opacity: 1; animation: none !important; }
          .prv-sweep { display: none; }
          .prv-card { transition: none; }
        }
      `}</style>

      <div className="mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-10 lg:gap-8 items-center">
        {/* Providers */}
        <div className="lg:col-span-4">
          <p
            className="prv-rise text-[#00A28C] text-[11px] tracking-[0.24em] font-semibold uppercase"
            style={vars({ "--d": "0s" })}
          >
            For Providers
          </p>

          <h2
            className="prv-rise mt-5 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight"
            style={vars({ "--d": "0.1s" })}
          >
            More trips.
            <br />
            More opportunities.
          </h2>

          <p
            className="prv-rise mt-5 max-w-sm text-sm sm:text-[15px] leading-relaxed text-midnight/70"
            style={vars({ "--d": "0.2s" })}
          >
            Join as a driver, rider or vendor and be part of a growing movement.
          </p>

          <a
            href="/waitlist"
            className="prv-rise group mt-8 inline-flex items-center gap-2 rounded-full bg-aurora px-6 py-3 text-[13px] font-semibold text-midnight shadow-[0_0_24px_-6px_rgba(0,194,168,0.7)] hover:brightness-110 hover:shadow-[0_0_34px_-4px_rgba(0,194,168,0.9)] transition"
            style={vars({ "--d": "0.3s" })}
          >
            Join as a Provider
            <ArrowRight
              size={14}
              strokeWidth={2.5}
              className="transition-transform duration-300 group-hover:translate-x-1"
            />
          </a>
        </div>

        {/* Image */}
        <div
          className="lg:col-span-4"
          onMouseMove={onMove}
          onMouseLeave={onLeave}
        >
          <div ref={cardRef} className="prv-card">
            <div className="prv-zoom">
              <div className="relative overflow-hidden rounded-3xl shadow-[0_24px_60px_-20px_rgba(11,31,68,0.35)] ring-1 ring-midnight/5">
                <Image
                  src="/images/providers.png"
                  alt="Smiling Karryam food vendors working in a busy kitchen"
                  width={1200}
                  height={800}
                  sizes="(min-width: 1024px) 32vw, 100vw"
                  className="w-full h-auto select-none"
                />
                {/* one-time light sweep */}
                <span
                  aria-hidden
                  className="prv-sweep pointer-events-none absolute inset-y-0 -left-1/3 w-1/4 -skew-x-12 bg-gradient-to-r from-transparent via-white/40 to-transparent"
                  style={{ translate: "-120% 0" }}
                />
              </div>
            </div>
          </div>
        </div>

        {/* Fleets */}
        <div className="lg:col-span-4 lg:pl-6">
          <p
            className="prv-rise text-[#00A28C] text-[11px] tracking-[0.24em] font-semibold uppercase"
            style={vars({ "--d": "0.6s" })}
          >
            For Fleets
          </p>

          <h2
            className="prv-rise mt-5 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight"
            style={vars({ "--d": "0.7s" })}
          >
            Smarter logistics.
            <br />
            Smarter business.
          </h2>

          <p
            className="prv-rise mt-5 max-w-sm text-sm sm:text-[15px] leading-relaxed text-midnight/70"
            style={vars({ "--d": "0.8s" })}
          >
            Manage your fleet, staff and operations with powerful logistics
            tools.
          </p>

          <a
            href="#fleet"
            className="prv-rise group mt-8 inline-flex items-center gap-2 rounded-full border border-aurora px-6 py-3 text-[13px] font-semibold text-[#00A28C] hover:bg-aurora hover:text-midnight transition-colors"
            style={vars({ "--d": "0.9s" })}
          >
            Explore Fleet
            <ArrowRight
              size={14}
              strokeWidth={2.5}
              className="transition-transform duration-300 group-hover:translate-x-1"
            />
          </a>
        </div>
      </div>
    </section>
  );
}