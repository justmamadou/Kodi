"use client";

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { cachedGet } from "./api";

function subscribeOnline(callback: () => void) {
  window.addEventListener("online", callback);
  window.addEventListener("offline", callback);
  return () => {
    window.removeEventListener("online", callback);
    window.removeEventListener("offline", callback);
  };
}

/** État du réseau, mis à jour en direct. */
export function useOnline(): boolean {
  return useSyncExternalStore(
    subscribeOnline,
    () => navigator.onLine,
    () => true,
  );
}

export interface Resource<T> {
  data: T | null;
  error: unknown;
  stale: boolean;
  loading: boolean;
  reload: () => void;
}

/** Charge une ressource, avec repli sur la dernière réponse connue hors connexion. */
export function useResource<T>(path: string | null): Resource<T> {
  const [state, setState] = useState<{ path: string | null; data: T | null; error: unknown; stale: boolean; loading: boolean }>({
    path,
    data: null,
    error: null,
    stale: false,
    loading: path !== null,
  });
  const [tick, setTick] = useState(0);

  useEffect(() => {
    if (path === null) return;
    let cancelled = false;
    cachedGet<T>(path).then(
      ({ data, stale }) => {
        if (!cancelled) setState({ path, data, error: null, stale, loading: false });
      },
      (error: unknown) => {
        if (!cancelled) setState((s) => ({ ...s, path, error, loading: false }));
      },
    );
    return () => {
      cancelled = true;
    };
  }, [path, tick]);

  const reload = useCallback(() => setTick((t) => t + 1), []);
  // Tant que la nouvelle ressource n'est pas arrivée, on n'affiche pas les données de la précédente.
  if (state.path !== path) return { data: null, error: null, stale: false, loading: path !== null, reload };
  return { data: state.data, error: state.error, stale: state.stale, loading: state.loading, reload };
}

function subscribeWide(callback: () => void) {
  const query = window.matchMedia("(min-width: 1024px)");
  query.addEventListener("change", callback);
  return () => query.removeEventListener("change", callback);
}

/** Vrai au format bureau : même point de rupture que la barre latérale. */
export function useIsDesktop(): boolean {
  return useSyncExternalStore(
    subscribeWide,
    () => window.matchMedia("(min-width: 1024px)").matches,
    () => true,
  );
}

function subscribeLocation(callback: () => void) {
  window.addEventListener("hashchange", callback);
  window.addEventListener("popstate", callback);
  return () => {
    window.removeEventListener("hashchange", callback);
    window.removeEventListener("popstate", callback);
  };
}

/**
 * Une partie de l'adresse courante, lue dans le navigateur seulement : null au rendu serveur, ce qui
 * permet aux pages statiques de ne pas afficher d'état erroné avant l'hydratation.
 */
export function useLocationPart(part: "hash" | "search"): string | null {
  return useSyncExternalStore(
    subscribeLocation,
    () => window.location[part],
    () => null,
  );
}
