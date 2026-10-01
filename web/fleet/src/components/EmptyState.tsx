"use client";

import Image from "next/image";
import Link from "next/link";
import type { ReactNode } from "react";

export interface EmptyStateProps {
  /** Path to the illustration PNG under /public/images. */
  image: string;
  /** Short, actionable title. Usually 2–5 words. */
  title: string;
  /** One or two sentences explaining what will appear here and when. */
  description: string;
  /** Optional action link. If both actionHref and actionLabel are set, a button renders. */
  actionHref?: string;
  /** Optional action label. Required if actionHref is set. */
  actionLabel?: string;
  /** Optional custom action. Takes precedence over actionHref/actionLabel. */
  action?: ReactNode;
  /** Optional className for the outer wrapper, for embedding inside cards. */
  className?: string;
}

/**
 * Standard empty state used across the app.
 *
 * Visual pattern:
 *  - illustration at 200×200
 *  - short title
 *  - one-or-two-sentence description
 *  - optional call-to-action
 *
 * Designed to sit inside a rounded, bordered card without further wrapping.
 * When embedding inside a table, pass className="py-8" or similar to adjust
 * vertical rhythm.
 */
export default function EmptyState({
  image,
  title,
  description,
  actionHref,
  actionLabel,
  action,
  className,
}: EmptyStateProps) {
  return (
    <div
      className={
        "flex flex-col items-center justify-center text-center py-10 px-6 " +
        (className ?? "")
      }
    >
      <div className="relative h-[180px] w-[180px] sm:h-[200px] sm:w-[200px]">
        <Image
          src={image}
          alt=""
          fill
          sizes="200px"
          className="object-contain opacity-90"
        />
      </div>
      <h4 className="mt-4 text-sm font-semibold text-ink-950">{title}</h4>
      <p className="mt-1.5 max-w-[300px] text-xs leading-relaxed text-ink-500">
        {description}
      </p>
      {action ? (
        <div className="mt-5">{action}</div>
      ) : actionHref && actionLabel ? (
        <Link
          href={actionHref}
          className="mt-5 inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
        >
          {actionLabel}
        </Link>
      ) : null}
    </div>
  );
}