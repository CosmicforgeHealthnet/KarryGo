"use client";

import { useEffect, useRef, useState, type CSSProperties } from "react";
import Image from "next/image";
import { ArrowRight, ChevronDown, MapPin } from "lucide-react";
import { FEATURED_LOCATIONS, NIGERIA_STATES_AND_FCT } from "@/lib/nigeria-locations";

type MotionState = "idle" | "armed" | "in";

const delay = (ms: number): CSSProperties => ({ ["--d" as string]: ms });
const index = (i: number): CSSProperties => ({ ["--i" as string]: i });

/* All motion lives here so this stays a single file. No brand colours are
   defined: the glow/ping/shadow use the same aurora teal already in your
   section gradient (0,194,168). */
const MOTION_CSS = `
.launch { --launch-ease: cubic-bezier(0.16, 1, 0.3, 1); --launch-aurora: 0, 194, 168; }

.launch-reveal {
  transition: opacity .9s var(--launch-ease), transform .9s var(--launch-ease), filter .9s var(--launch-ease);
  transition-delay: calc(var(--d, 0) * 1ms);
}
.launch[data-state="armed"] .launch-reveal { opacity: 0; transform: translate3d(0,24px,0); filter: blur(6px); }

.launch-map-wrap {
  transition: opacity 1.4s var(--launch-ease), transform 1.4s var(--launch-ease), filter 1.4s var(--launch-ease);
  transition-delay: 200ms;
}
.launch[data-state="armed"] .launch-map-wrap { opacity: 0; transform: translate3d(32px,0,0) scale(.94); filter: blur(10px); }

.launch-city {
  transition: opacity .7s var(--launch-ease), transform .7s var(--launch-ease), border-color .3s ease, background-color .3s ease;
  transition-delay: calc(650ms + var(--i, 0) * 110ms), calc(650ms + var(--i, 0) * 110ms), 0s, 0s;
}
.launch[data-state="armed"] .launch-city { opacity: 0; transform: translate3d(28px,0,0); }
.launch[data-state="in"] .launch-city:hover,
.launch[data-state="idle"] .launch-city:hover {
  transition-delay: 0s;
  transform: translate3d(-4px,0,0);
  border-color: rgba(var(--launch-aurora), .45);
  background-color: rgba(var(--launch-aurora), .06);
}

.launch[data-state="in"] .launch-map { animation: launch-float 10s ease-in-out 1.6s infinite alternate; will-change: transform; }
@keyframes launch-float { from { transform: translate3d(0,0,0); } to { transform: translate3d(0,-8px,0); } }

.launch[data-state="in"] .launch-glow { animation: launch-breathe 7s ease-in-out 1s infinite; }
@keyframes launch-breathe { 0%,100% { opacity: .55; } 50% { opacity: 1; } }

.launch-pin { position: relative; display: inline-flex; }
.launch-pin::after {
  content: ""; position: absolute; inset: -5px; border-radius: 9999px;
  border: 1px solid rgba(var(--launch-aurora), .55); opacity: 0; pointer-events: none;
}
.launch[data-state="in"] .launch-pin::after {
  animation: launch-ping 3s cubic-bezier(0,0,.2,1) infinite;
  animation-delay: calc(1.6s + var(--i, 0) * 450ms);
}
@keyframes launch-ping {
  0% { transform: scale(.6); opacity: .8; }
  60%,100% { transform: scale(1.9); opacity: 0; }
}

.launch-cta { transition: transform .35s var(--launch-ease), box-shadow .35s var(--launch-ease), filter .25s ease; }
.launch-cta:hover { transform: translate3d(0,-2px,0); filter: brightness(1.08); box-shadow: 0 12px 32px -10px rgba(var(--launch-aurora), .6); }
.launch-cta:active { transform: translate3d(0,0,0) scale(.98); box-shadow: 0 4px 14px -6px rgba(var(--launch-aurora), .5); transition-duration: .1s; }
.launch-arrow { transition: transform .35s var(--launch-ease); }
.launch-cta:hover .launch-arrow, .launch-cta:focus-visible .launch-arrow { transform: translate3d(4px,0,0); }

@media (prefers-reduced-motion: reduce) {
  .launch-reveal, .launch-map-wrap, .launch-city, .launch-cta, .launch-arrow {
    transition: none !important; opacity: 1 !important; filter: none !important;
  }
  .launch-reveal, .launch-map-wrap, .launch-city { transform: none !important; }
  .launch-map, .launch-glow, .launch-pin::after { animation: none !important; }
}
`;

export default function Launch() {
  const ref = useRef<HTMLElement>(null);
  const [state, setState] = useState<MotionState>("idle");
  const [showAllLocations, setShowAllLocations] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;

    const rect = el.getBoundingClientRect();
    if (rect.top < window.innerHeight * 0.8 && rect.bottom > 0) {
      setState("in");
      return;
    }

    setState("armed");

    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setState("in");
          io.disconnect();
        }
      },
      { threshold: 0.2, rootMargin: "0px 0px -8% 0px" }
    );

    io.observe(el);
    return () => io.disconnect();
  }, []);

  return (
    <section
      ref={ref}
      id="launch"
      data-state={state}
      className="launch relative scroll-mt-20 overflow-hidden bg-midnight text-white py-16 lg:py-16"
    >
      <style dangerouslySetInnerHTML={{ __html: MOTION_CSS }} />

      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_55%_70%_at_70%_50%,rgba(0,194,168,0.08),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_45%_45%_at_0%_100%,rgba(45,58,91,0.22),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-0 opacity-[0.05] [background-image:radial-gradient(#fff_1px,transparent_1px)] [background-size:28px_28px] [mask-image:linear-gradient(to_bottom,transparent,black_30%,black_70%,transparent)]"
      />

      <div className="relative mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-8 lg:gap-4 items-center">
        <div className="launch-copy lg:col-span-4">
          <p
            className="launch-reveal text-aurora text-[11px] tracking-[0.24em] font-semibold uppercase"
            style={delay(0)}
          >
            Launching Soon
          </p>

          <h2
            className="launch-reveal mt-5 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight"
            style={delay(110)}
          >
            Nigeria,
            <br />
            <span className="text-aurora">get ready.</span>
          </h2>

          <p
            className="launch-reveal mt-5 max-w-md text-sm sm:text-[15px] leading-relaxed text-white/70"
            style={delay(220)}
          >
            Karryam is launching soon in selected Nigerian markets. Be among the
            first to move, send, sell and grow with us.
          </p>

          <div className="launch-reveal mt-8" style={delay(340)}>
            <a
              href="/waitlist"
              className="launch-cta inline-flex items-center gap-2 rounded-full bg-aurora px-6 py-3 text-[13px] font-semibold text-midnight shadow-sm focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-aurora"
            >
              Join the waitlist
              <ArrowRight
                aria-hidden
                size={14}
                strokeWidth={2.5}
                className="launch-arrow"
              />
            </a>
          </div>
        </div>

        <div className="lg:col-span-8 relative">
          <div
            aria-hidden
            className="launch-glow absolute left-1/2 top-1/2 h-[70%] w-[70%] -translate-x-1/2 -translate-y-1/2 rounded-full bg-aurora/10 blur-3xl"
          />

          <div className="launch-map-wrap relative">
            <Image
              src="/images/nigeria-map.png"
              alt="Glowing map of Nigeria with Karryam launch cities"
              width={1200}
              height={1000}
              sizes="(min-width: 1024px) 60vw, 100vw"
              className="launch-map relative h-auto w-full select-none lg:-ml-6"
            />
          </div>

          <ul className="relative mt-6 flex flex-col gap-2.5 sm:absolute sm:right-0 sm:top-1/2 sm:mt-0 sm:w-[200px] sm:-translate-y-1/2">
            {FEATURED_LOCATIONS.map((location, i) => (
              <li
                key={location}
                style={index(i)}
                className="launch-city relative overflow-hidden flex items-center gap-3 rounded-xl border border-white/10 bg-midnight/70 px-4 py-2.5 backdrop-blur"
              >
                <span className="launch-pin" aria-hidden>
                  <MapPin size={14} className="text-aurora" strokeWidth={2} />
                </span>
                <span className="text-[13px] font-semibold text-white">{location}</span>
              </li>
            ))}
            <li
              style={index(FEATURED_LOCATIONS.length)}
              className="launch-city relative overflow-hidden rounded-xl border border-white/10 bg-midnight/70 backdrop-blur"
            >
              <button
                type="button"
                aria-expanded={showAllLocations}
                aria-controls="launch-all-states"
                onClick={() => setShowAllLocations((value) => !value)}
                className="flex w-full cursor-pointer items-center gap-3 px-4 py-2.5 text-left text-[13px] text-white/70 transition-colors hover:text-white focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-aurora"
              >
                <ChevronDown
                  size={14}
                  className={`shrink-0 text-aurora transition-transform ${showAllLocations ? "rotate-180" : ""}`}
                  aria-hidden
                />
                <span>{showAllLocations ? "Show less" : "and more..."}</span>
              </button>
            </li>
          </ul>
          <div
            id="launch-all-states"
            hidden={!showAllLocations}
            className="relative mt-6 rounded-2xl border border-white/15 bg-midnight/90 px-4 py-5 sm:px-6"
          >
            <h3 className="text-base font-semibold text-white">
              All 36 Nigerian states + FCT
            </h3>
            <p className="mt-2 text-xs leading-relaxed text-white/65">
              Explore locations across Nigeria. This list is for geographic interest,
              not a promise that Karryam services are already live in every state.
            </p>
            <ul className="mt-4 grid grid-cols-2 gap-x-4 gap-y-2 text-xs text-white/85 sm:grid-cols-3 lg:grid-cols-4">
              {NIGERIA_STATES_AND_FCT.map((location) => (
                <li key={location} className="break-words leading-relaxed">
                  {location}
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </section>
  );
}