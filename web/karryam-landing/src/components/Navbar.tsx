"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import { ArrowRight, Menu, X } from "lucide-react";

const NAV_ITEMS = [
  { label: "Home", id: "home" },
  { label: "Services", id: "services" },
  { label: "For Providers", id: "providers" },
  { label: "For Businesses", id: "businesses" },
  { label: "About", id: "about" },
];

export default function Navbar() {
  const [scrolled, setScrolled] = useState(false);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState("home");

  useEffect(() => {
    const onScroll = () => {
      setScrolled(window.scrollY > 24);

      // scroll-spy: last section whose top has passed the header
      let current = "home";
      for (const { id } of NAV_ITEMS) {
        const el = document.getElementById(id);
        if (el && el.getBoundingClientRect().top <= 120) current = id;
      }
      setActive(current);
    };
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  // Mobile menu: lock page scroll, close on Escape, close if viewport grows to desktop
  useEffect(() => {
    if (!open) return;

    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    const mq = window.matchMedia("(min-width: 1024px)");
    const onMq = () => {
      if (mq.matches) setOpen(false);
    };

    window.addEventListener("keydown", onKey);
    mq.addEventListener("change", onMq);

    return () => {
      document.body.style.overflow = prevOverflow;
      window.removeEventListener("keydown", onKey);
      mq.removeEventListener("change", onMq);
    };
  }, [open]);

  return (
    <header
      className={`fixed inset-x-0 top-0 z-50 transition-all duration-300 ${
        scrolled || open
          ? "bg-midnight/85 backdrop-blur-xl border-b border-white/5"
          : "bg-transparent border-b border-transparent"
      }`}
    >
      <nav className="mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12 h-16 lg:h-20 flex items-center justify-between lg:grid lg:grid-cols-[1fr_auto_1fr]">
        {/* Logo */}
        <a
          href="#home"
          className="flex items-center justify-self-start"
          aria-label="Karryam home"
        >
          <Image
            src="/images/logo.png"
            alt="Karryam"
            width={100}
            height={60}
            priority
            className="h-7 sm:h-8 lg:h-10 w-auto select-none"
          />
        </a>

        {/* Center nav */}
        <ul className="hidden lg:flex items-center gap-8 xl:gap-10 text-[13px] font-medium">
          {NAV_ITEMS.map(({ label, id }) => {
            const isActive = active === id;
            return (
              <li key={id} className="relative">
                <a
                  href={`#${id}`}
                  aria-current={isActive ? "page" : undefined}
                  className={`py-2 whitespace-nowrap transition-colors ${
                    isActive
                      ? "text-aurora"
                      : "text-white/75 hover:text-white"
                  }`}
                >
                  {label}
                </a>
                {isActive && (
                  <span className="absolute -bottom-1 left-0 right-0 h-px bg-aurora/70" />
                )}
              </li>
            );
          })}
        </ul>

        {/* Right */}
        <div className="flex items-center gap-3 lg:gap-6 justify-self-end">
          <a
            href="/login"
            className="hidden sm:inline text-[13px] font-medium text-white/80 hover:text-white transition-colors"
          >
            Login
          </a>
          <a
            href="/waitlist"
            className="hidden sm:inline-flex items-center gap-2 whitespace-nowrap bg-aurora text-midnight font-semibold text-[13px] px-4 lg:px-5 py-2.5 rounded-full shadow-[0_0_24px_-4px_rgba(0,194,168,0.6)] hover:brightness-110 transition"
          >
            Join the Waitlist
            <ArrowRight size={14} strokeWidth={2.5} />
          </a>
          <button
            onClick={() => setOpen(!open)}
            className="lg:hidden text-white p-2 -mr-2"
            aria-label="Toggle menu"
            aria-expanded={open}
          >
            {open ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>
      </nav>

      {/* Mobile sheet */}
      {open && (
        <div className="lg:hidden border-t border-white/5 max-h-[calc(100dvh-4rem)] overflow-y-auto overscroll-contain">
          <ul className="px-5 sm:px-8 py-5 flex flex-col gap-1 text-white/90">
            {NAV_ITEMS.map(({ label, id }) => (
              <li key={id}>
                <a
                  href={`#${id}`}
                  onClick={() => setOpen(false)}
                  className={`block py-3 text-base ${
                    active === id ? "text-aurora" : ""
                  }`}
                >
                  {label}
                </a>
              </li>
            ))}
            <li className="pt-3 pb-2 flex flex-col gap-3">
            
              <a
                href="/waitlist"
                className="flex items-center justify-center gap-2 bg-aurora text-midnight font-semibold px-5 py-3 rounded-full"
              >
                Join the Waitlist
                <ArrowRight size={16} strokeWidth={2.5} />
              </a>
            </li>
          </ul>
        </div>
      )}
    </header>
  );
}