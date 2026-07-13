"use client";

import { useRef } from "react";

interface OtpInputProps {
  value: string[];
  onChange: (value: string[]) => void;
  length?: number;
}

export function OtpInput({ value, onChange, length = 6 }: OtpInputProps) {
  const inputRefs = useRef<(HTMLInputElement | null)[]>([]);

  const handleChange = (index: number, digit: string) => {
    const clean = digit.replace(/[^0-9]/g, "").slice(-1);
    const next = [...value];
    next[index] = clean;
    onChange(next);
    if (clean && index < length - 1) {
      inputRefs.current[index + 1]?.focus();
    }
  };

  const handleKeyDown = (
    index: number,
    e: React.KeyboardEvent<HTMLInputElement>
  ) => {
    if (e.key === "Backspace" && !value[index] && index > 0) {
      inputRefs.current[index - 1]?.focus();
    }
  };

  const handlePaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const pasted = e.clipboardData.getData("text").replace(/[^0-9]/g, "");
    if (!pasted) return;
    e.preventDefault();
    const next = pasted.slice(0, length).split("");
    while (next.length < length) next.push("");
    onChange(next);
    const lastFilled = Math.min(pasted.length, length) - 1;
    inputRefs.current[Math.max(lastFilled, 0)]?.focus();
  };

  return (
    <div className="flex gap-2.5">
      {Array.from({ length }).map((_, i) => (
        <input
          key={i}
          ref={(el) => {
            inputRefs.current[i] = el;
          }}
          type="text"
          inputMode="numeric"
          maxLength={1}
          value={value[i] ?? ""}
          onChange={(e) => handleChange(i, e.target.value)}
          onKeyDown={(e) => handleKeyDown(i, e)}
          onPaste={handlePaste}
          className="h-14 w-12 rounded-full border-2 border-brand-500 text-center text-lg font-semibold text-ink-900 focus:outline-none focus:ring-2 focus:ring-brand-500/30"
        />
      ))}
    </div>
  );
}