"use client";

import { useCallback, useState } from "react";

const KEY = "cf-current-project";

function read(): string | null {
  try {
    return typeof localStorage === "undefined" ? null : localStorage.getItem(KEY);
  } catch {
    return null;
  }
}

/** Le chantier de travail, mémorisé sur l'appareil : le terrain travaille sur un chantier à la fois. */
export function useCurrentProject(): [string | null, (id: string) => void] {
  const [current, setCurrent] = useState<string | null>(read);
  const set = useCallback((id: string) => {
    setCurrent(id);
    try {
      localStorage.setItem(KEY, id);
    } catch {
      /* préférence non conservée, sans conséquence */
    }
  }, []);
  return [current, set];
}
