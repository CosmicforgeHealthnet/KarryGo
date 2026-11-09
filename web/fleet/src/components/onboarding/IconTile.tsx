import React from "react";

type Tone = "brand" | "accent" | "red" | "green" | "amber";

const TONES: Record<Tone, string> = {
  brand: "bg-primary-bg text-label-accent",
  accent: "bg-primary-bg text-label-accent",
  green: "bg-green-bg text-label-accent",
  red: "bg-red-bg text-red",
  amber: "bg-amber-bg text-amber",
};

// Flat Karryam icon tile (vendor app style): a quiet tinted square with a thin
// outlined icon. Shared by every onboarding page.
export function IconTile({
  icon,
  theme = "brand",
  muted = false,
  size = "md",
}: {
  icon: React.ReactNode;
  theme?: Tone;
  muted?: boolean;
  size?: "md" | "lg";
}) {
  const box = size === "lg" ? "h-14 w-14 rounded-2xl" : "h-10 w-10 rounded-xl";
  const glyph = size === "lg" ? "[&>svg]:h-6 [&>svg]:w-6" : "[&>svg]:h-[18px] [&>svg]:w-[18px]";
  const tone = muted ? "bg-surface-3 text-ink-400" : TONES[theme] ?? TONES.brand;
  return (
    <div className={`flex flex-shrink-0 items-center justify-center ${tone} ${box}`}>
      <span className={`flex ${glyph}`}>
        {React.isValidElement(icon)
          ? React.cloneElement(icon as React.ReactElement<{ strokeWidth?: number }>, { strokeWidth: 1.75 })
          : icon}
      </span>
    </div>
  );
}
