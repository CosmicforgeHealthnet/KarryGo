"use client";

import { ChevronDown } from "lucide-react";

const COUNTRIES = [
  { code: "NG", dial: "+234", flag: "🇳🇬" },
  { code: "GH", dial: "+233", flag: "🇬🇭" },
  { code: "KE", dial: "+254", flag: "🇰🇪" },
];

interface PhoneInputProps {
  value: string;
  onChange: (value: string) => void;
  dialCode: string;
  onDialCodeChange: (dial: string) => void;
}

export function PhoneInput({
  value,
  onChange,
  dialCode,
  onDialCodeChange,
}: PhoneInputProps) {
  return (
    <div className="flex overflow-hidden rounded-xl border border-ink-100 focus-within:border-brand-500">
      <div className="relative flex items-center gap-1.5 border-r border-ink-100 bg-white pl-3 pr-2">
        <span aria-hidden className="text-lg leading-none">
          {COUNTRIES.find((c) => c.dial === dialCode)?.flag}
        </span>
        <span className="text-sm font-medium text-ink-900">{dialCode}</span>
        <ChevronDown className="h-3.5 w-3.5 text-ink-500" />
        <select
          aria-label="Country code"
          value={dialCode}
          onChange={(e) => onDialCodeChange(e.target.value)}
          className="absolute inset-0 cursor-pointer opacity-0"
        >
          {COUNTRIES.map((c) => (
            <option key={c.code} value={c.dial}>
              {c.flag} {c.dial}
            </option>
          ))}
        </select>
      </div>
      <input
        type="tel"
        inputMode="numeric"
        placeholder="8067735987"
        value={value}
        onChange={(e) => onChange(e.target.value.replace(/[^0-9]/g, ""))}
        className="w-full px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:outline-none"
      />
    </div>
  );
}