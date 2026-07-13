import { type ButtonHTMLAttributes, forwardRef } from "react";
import { cn } from "@/lib/utils";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "primary" | "social" | "social-dark";
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "primary", disabled, ...props }, ref) => {
    return (
      <button
        ref={ref}
        disabled={disabled}
        className={cn(
          "w-full rounded-xl px-6 py-4 text-base font-semibold transition-colors duration-150 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand-600 disabled:cursor-not-allowed",
          variant === "primary" &&
            !disabled &&
            "bg-brand-500 text-white hover:bg-brand-600",
          variant === "primary" && disabled && "bg-brand-200 text-white",
          variant === "social" &&
            "flex items-center justify-center gap-3 border border-ink-100 bg-white text-ink-700 hover:bg-ink-100",
          variant === "social-dark" &&
            "flex items-center justify-center gap-3 bg-ink-900 text-white hover:bg-black",
          className
        )}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";