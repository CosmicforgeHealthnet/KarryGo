import { type InputHTMLAttributes, forwardRef } from "react";
import { cn } from "@/lib/utils";

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  ({ className, label, id, ...props }, ref) => {
    return (
      <div>
        {label && (
          <label
            htmlFor={id}
            className="mb-2 block text-sm font-medium text-ink-900"
          >
            {label}
          </label>
        )}
        <input
          ref={ref}
          id={id}
          className={cn(
            "w-full rounded-xl border border-ink-100 px-4 py-4 text-sm text-ink-900 placeholder:text-ink-300 focus:border-brand-500 focus:outline-none",
            className
          )}
          {...props}
        />
      </div>
    );
  }
);
Input.displayName = "Input";