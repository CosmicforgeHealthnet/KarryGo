"use client";

import { useEffect, useState, useCallback } from "react";

export function useOtpTimer(initialSeconds = 251) {
  const [secondsLeft, setSecondsLeft] = useState(initialSeconds);

  useEffect(() => {
    if (secondsLeft <= 0) return;
    const id = setInterval(() => {
      setSecondsLeft((s) => Math.max(s - 1, 0));
    }, 1000);
    return () => clearInterval(id);
  }, [secondsLeft]);

  const reset = useCallback(() => setSecondsLeft(initialSeconds), [initialSeconds]);

  const minutes = Math.floor(secondsLeft / 60);
  const seconds = secondsLeft % 60;
  const formatted = `${String(minutes).padStart(2, "0")}:${String(
    seconds
  ).padStart(2, "0")}`;

  return { secondsLeft, formatted, canResend: secondsLeft === 0, reset };
}