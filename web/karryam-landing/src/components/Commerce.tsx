"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { ArrowRight } from "lucide-react";

const vars = (o: Record<string, string | number>) => o as React.CSSProperties;

export default function Commerce() {
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
      id="commerce"
      className="relative bg-gradient-to-b from-[#F3F8FB] to-white text-midnight py-8 lg:py-20 overflow-hidden"
    >
      <style>{`
        @keyframes com-rise {
          from { opacity: 0; translate: 0 36px; }
          to { opacity: 1; translate: 0 0; }
        }
        @keyframes com-right {
          from { opacity: 0; translate: 56px 0; scale: 0.95; }
          to { opacity: 1; translate: 0 0; scale: 1; }
        }
        @keyframes com-breathe {
          0%, 100% { transform: scale(1); opacity: 0.75; }
          50% { transform: scale(1.12); opacity: 1; }
        }
        @keyframes com-shimmer {
          from { background-position: 0% 50%; }
          to { background-position: 200% 50%; }
        }

        .com-rise, .com-right { opacity: 0; }

        [data-inview="true"] .com-rise {
          animation: com-rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .com-right {
          animation: com-right 0.9s cubic-bezier(0.22, 1, 0.36, 1) 0.2s forwards;
        }

        .com-glow { animation: com-breathe 8s ease-in-out infinite; }

        .com-shimmer {
          background-image: linear-gradient(90deg, #0B1F44 0%, #0B1F44 35%, #00A28C 50%, #0B1F44 65%, #0B1F44 100%);
          background-size: 200% 100%;
          -webkit-background-clip: text;
          background-clip: text;
          color: transparent;
          animation: com-shimmer 7s linear infinite;
        }

        /* tilt on transform; entrance uses translate/scale so they never clash */
        .com-card {
          transform: perspective(1200px) rotateX(var(--rx, 0deg)) rotateY(var(--ry, 0deg));
          transition: transform 0.25s ease-out;
          will-change: transform;
        }

        @media (prefers-reduced-motion: reduce) {
          .com-rise, .com-right { opacity: 1; animation: none !important; }
          .com-glow { animation: none; }
          .com-shimmer { animation: none; background-image: none; color: inherit; }
          .com-card { transition: none; }
        }
      `}</style>

      <div className="mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-12 lg:gap-8 items-center">
        <div className="lg:col-span-5">
          <p
            className="com-rise text-[#00A28C] text-[11px] tracking-[0.24em] font-semibold uppercase"
            style={vars({ "--d": "0s" })}
          >
            More Than Deliveries
          </p>

          <h2
            className="com-rise mt-5 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight"
            style={vars({ "--d": "0.1s" })}
          >
            Food. Shop.
            <br />
            <span className="com-shimmer">Fulfilment intelligence.</span>
          </h2>

          <p
            className="com-rise mt-5 max-w-md text-sm sm:text-[15px] leading-relaxed text-midnight/70"
            style={vars({ "--d": "0.2s" })}
          >
            From your favourite meals to everyday essentials, Karryam connects
            commerce with intelligent logistics so you get what you need, when
            you need it.
          </p>

          <a
            href="#commerce"
            className="com-rise group mt-8 inline-flex items-center gap-2 rounded-full border border-aurora px-6 py-3 text-[13px] font-semibold text-[#00A28C] hover:bg-aurora hover:text-midnight transition-colors"
            style={vars({ "--d": "0.3s" })}
          >
            Explore commerce
            <ArrowRight
              size={14}
              strokeWidth={2.5}
              className="transition-transform duration-300 group-hover:translate-x-1"
            />
          </a>
        </div>

        <div
          className="lg:col-span-7 relative"
          onMouseMove={onMove}
          onMouseLeave={onLeave}
        >
          <div
            aria-hidden
            className="absolute left-1/2 top-1/2 h-[70%] w-[70%] -translate-x-1/2 -translate-y-1/2"
          >
            <div className="com-glow h-full w-full rounded-full bg-aurora/15 blur-3xl" />
          </div>

          <div ref={cardRef} className="com-card relative">
            <div className="com-right">
              <Image
                src="/images/commerce.png"
                alt="Karryam app with a delivery truck and grocery bags showing smart fulfilment"
                width={1600}
                height={1200}
                sizes="(min-width: 1024px) 58vw, 100vw"
                className="relative mx-auto w-full h-auto max-w-[760px] select-none"
              />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}