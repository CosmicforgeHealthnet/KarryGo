"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import type { LucideIcon } from "lucide-react";
import {
  ArrowRight,
  Car,
  Package,
  ShoppingBag,
  Truck,
  UtensilsCrossed,
} from "lucide-react";

type Service = {
  title: string;
  text: string;
  image: string;
  alt: string;
  icon: LucideIcon;
  chip: string;
};

const SERVICES: Service[] = [
  {
    title: "Ride",
    text: "Fast, safe and reliable city and intercity rides.",
    image: "/images/ride.png",
    alt: "White car driving through a city",
    icon: Car,
    chip: "bg-midnight/10 text-midnight",
  },
  {
    title: "Send",
    text: "Track your parcels in real time.",
    image: "/images/send.png",
    alt: "Dispatch rider on a motorbike",
    icon: Package,
    chip: "bg-aurora/15 text-aurora-ink",
  },
  {
    title: "Truck",
    text: "Heavy loads. Smarter logistics.",
    image: "/images/truck.png",
    alt: "Blue freight truck on the road",
    icon: Truck,
    chip: "bg-indigo/10 text-indigo",
  },
  {
    title: "Food",
    text: "Your favourite meals, delivered.",
    image: "/images/food.png",
    alt: "Plated meal ready for delivery",
    icon: UtensilsCrossed,
    chip: "bg-aurora/15 text-aurora-ink",
  },
  {
    title: "Shop",
    text: "Everyday essentials, instantly.",
    image: "/images/shop.png",
    alt: "Shopping bags with fresh groceries",
    icon: ShoppingBag,
    chip: "bg-midnight/10 text-midnight",
  },
];

// Desktop 3D arc: rotateY in degrees and translateZ in px per card.
// Positive rotateY on the left cards turns them inward toward the center.
// Flip the signs of ry to tilt the other way.
const TILT = [
  { ry: 16, tz: 24 },
  { ry: 8, tz: 8 },
  { ry: 0, tz: 0 },
  { ry: -8, tz: 8 },
  { ry: -16, tz: 24 },
];

const vars = (o: Record<string, string | number>) => o as React.CSSProperties;

export default function Services() {
  const ref = useRef<HTMLElement>(null);
  const [inView, setInView] = useState(false);

  useEffect(() => {
    const el = ref.current;
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

  return (
    <section
      ref={ref}
      data-inview={inView}
      id="services"
      className="relative bg-white text-midnight py-20 lg:py-20 overflow-hidden"
    >
      <style>{`
        @keyframes svc-rise {
          from { opacity: 0; translate: 0 16px; }
          to { opacity: 1; translate: 0 0; }
        }
        @keyframes svc-float {
          0%, 100% { transform: translateY(0); }
          50% { transform: translateY(-2px); }
        }
        @keyframes svc-shimmer {
          from { background-position: 0% 50%; }
          to { background-position: 200% 50%; }
        }

        .svc-shimmer {
          background-image: linear-gradient(90deg, #0B1F44 0%, #0B1F44 35%, #00A28C 50%, #0B1F44 65%, #0B1F44 100%);
          background-size: 200% 100%;
          -webkit-background-clip: text;
          background-clip: text;
          color: transparent;
          animation: svc-shimmer 7s linear infinite;
        }

        .svc-text, .svc-card { opacity: 0; }

        [data-inview="true"] .svc-text {
          animation: svc-rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .svc-card {
          animation: svc-rise 0.8s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .svc-float {
          animation: svc-float 5s ease-in-out infinite;
          animation-delay: calc(1.6s + var(--f, 0s));
        }

        .svc-card { transform: none; }

        @media (min-width: 1024px) and (hover: hover) and (pointer: fine) {
          .svc-list { perspective: 1400px; perspective-origin: 50% 40%; }
          .svc-card {
            transform: rotateY(var(--ry, 0deg)) translateZ(var(--tz, 0px));
            transition: transform 0.5s cubic-bezier(0.22, 1, 0.36, 1);
          }
          .svc-card:hover {
            transform: rotateY(0deg) translateZ(20px);
            z-index: 10;
          }
        }

        @media (prefers-reduced-motion: reduce) {
          .svc-text, .svc-card { opacity: 1; animation: none !important; }
          .svc-float { animation: none !important; }
          .svc-shimmer { animation: none; background-image: none; color: inherit; }
          .svc-card { transition: none; translate: none; }
          .svc-card:hover { transform: rotateY(var(--ry, 0deg)) translateZ(var(--tz, 0px)); }
        }
      `}</style>

      <div className="mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-12 lg:gap-8 items-center">
        <div className="lg:col-span-4">
          <p
            className="svc-text text-aurora-ink text-[11px] tracking-[0.24em] font-semibold uppercase"
            style={vars({ "--d": "0s" })}
          >
            The Services
          </p>

          <h2
            className="svc-text mt-5 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight"
            style={vars({ "--d": "0.1s" })}
          >
            Five services.
            <br />
            <span className="svc-shimmer">One ecosystem.</span>
          </h2>

          <p
            className="svc-text mt-5 max-w-md text-sm sm:text-[15px] leading-relaxed text-midnight/70"
            style={vars({ "--d": "0.2s" })}
          >
            From everyday rides to heavy-duty logistics, Karryam gives you the
            freedom to move, buy, sell and grow —{" "}
            <span className="font-semibold text-midnight">
              all in one app.
            </span>
          </p>

          <a
            href="#ecosystem"
            className="svc-text mt-8 inline-flex items-center gap-2 rounded-full border border-aurora px-6 py-3 text-[13px] font-semibold text-aurora-ink hover:bg-aurora hover:text-midnight transition-colors duration-200 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-aurora-ink"
            style={vars({ "--d": "0.3s" })}
          >
            Explore all services
            <ArrowRight size={14} strokeWidth={2.5} />
          </a>
        </div>

        <ul className="svc-list lg:col-span-8 flex lg:grid lg:grid-cols-5 gap-3 lg:gap-4 overflow-x-auto lg:overflow-visible snap-x snap-mandatory -mx-6 px-6 lg:mx-0 lg:px-0 py-6 lg:py-8">
          {SERVICES.map(({ title, text, image, alt, icon: Icon, chip }, i) => (
            <li
              key={title}
              className="svc-card group snap-start shrink-0 w-[62%] sm:w-[34%] lg:w-auto"
              style={vars({
                "--d": `${0.35 + i * 0.12}s`,
                "--ry": `${TILT[i].ry}deg`,
                "--tz": `${TILT[i].tz}px`,
              })}
            >
              <article
                className="svc-float h-full rounded-2xl bg-white p-2 ring-1 ring-midnight/5 shadow-[0_10px_30px_-12px_rgba(11,31,68,0.22)] transition-shadow duration-300 group-hover:shadow-[0_24px_50px_-14px_rgba(11,31,68,0.24)]"
                style={vars({ "--f": `${i * 0.45}s` })}
              >
                <div className="relative aspect-[4/4.6] overflow-hidden rounded-xl">
                  <Image
                    src={image}
                    alt={alt}
                    fill
                    sizes="(min-width: 1024px) 12vw, 62vw"
                    className="object-cover transition-transform duration-500 motion-safe:group-hover:scale-[1.035]"
                  />
                  {/* shine sweep */}
                  <span
                    aria-hidden
                    className="pointer-events-none absolute inset-y-0 -left-1/2 w-1/2 -skew-x-12 bg-gradient-to-r from-transparent via-white/15 to-transparent -translate-x-full transition-transform duration-700 ease-out group-hover:translate-x-[300%]"
                  />
                </div>

                <div className="px-1.5 pt-3 pb-1.5">
                  <div className="flex items-center gap-2">
                    <span
                      className={`grid h-6 w-6 shrink-0 place-items-center rounded-md ${chip}`}
                    >
                      <Icon size={13} strokeWidth={2} />
                    </span>
                    <h3 className="text-sm font-semibold leading-none">
                      {title}
                    </h3>
                  </div>

                  <p className="mt-2.5 text-[11px] leading-snug text-midnight/60">
                    {text}
                  </p>

                  <div className="mt-3 flex justify-end">
                    <span
                      aria-hidden
                      className="grid h-6 w-6 place-items-center rounded-full bg-midnight text-white transition-all duration-300 group-hover:bg-aurora group-hover:text-midnight group-hover:translate-x-1"
                    >
                      <ArrowRight size={11} strokeWidth={2.5} />
                    </span>
                  </div>
                </div>
              </article>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}