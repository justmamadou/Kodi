"use client";

import { useLiveQuery } from "dexie-react-hooks";
import { useCallback, useEffect } from "react";
import { csrfToken } from "../api";
import { db } from "./db";
import { flushOutbox, type SyncResult } from "./sync";

const deps = { fetch: (input: RequestInfo | URL, init?: RequestInit) => fetch(input, init), csrfToken };

/** Lance un passage de synchronisation depuis l'interface. */
export function syncNow(): Promise<SyncResult> {
  return flushOutbox(deps);
}

/** Synchronise au démarrage, au retour du réseau, et régulièrement ; expose le nombre d'éléments en attente. */
export function useAutoSync(): { pending: number; problems: number } {
  useEffect(() => {
    const run = () => {
      if (navigator.onLine) void syncNow();
    };
    run();
    window.addEventListener("online", run);
    const timer = window.setInterval(run, 30_000);
    return () => {
      window.removeEventListener("online", run);
      window.clearInterval(timer);
    };
  }, []);

  const pending = useLiveQuery(() => db.outbox.where("state").equals("pending").count(), [], 0);
  const problems = useLiveQuery(() => db.outbox.where("state").anyOf("failed", "conflict").count(), [], 0);
  return { pending, problems };
}

/** Après une action enfilée : tente l'envoi tout de suite si le réseau le permet. */
export function useFlushAfterEnqueue(): () => void {
  return useCallback(() => {
    if (navigator.onLine) void syncNow();
  }, []);
}
