import Dexie, { type EntityTable } from "dexie";

/** États affichés : brouillon local → en attente d'envoi → synchronisé, ou échec / conflit. */
export type OutboxState = "pending" | "failed" | "conflict" | "synced";

export type OutboxKind =
  | "report.save"
  | "report.submit"
  | "task.create"
  | "task.progress"
  | "expense.create"
  | "expense.submit"
  | "photo.upload";

export interface OutboxEntry {
  seq?: number;
  /** Sert aussi d'Idempotency-Key : un rejeu ne crée jamais de doublon côté serveur. */
  id: string;
  kind: OutboxKind;
  label: string;
  /** Les opérations d'une même entité partent dans l'ordre ; un échec bloque les suivantes. */
  entityId: string;
  method: "POST" | "PUT";
  path: string;
  body: unknown;
  state: OutboxState;
  error?: string;
  attempts: number;
  /** La version vue par l'utilisateur est relue au moment de l'envoi (voir sync.ts). */
  usesReportVersion?: boolean;
  /** L'utilisateur a choisi de garder sa version après un conflit. */
  force?: boolean;
  blobId?: string;
  createdAt: number;
  syncedAt?: number;
}

export interface ReportDraft {
  id: string;
  projectId: string;
  reportDate: string;
  teamLabel: string;
  workDone: string;
  workforceCount: number | null;
  issues: string;
  weather: string | null;
  rowVersion?: number;
  updatedAt: number;
}

export interface StoredBlob {
  id: string;
  blob: Blob;
  mime: string;
  name: string;
}

export interface CacheEntry {
  key: string;
  value: unknown;
  at: number;
}

// Nom d'avant le changement de nom de l'application, conservé : le changer viderait la file d'envoi
// et les brouillons déjà présents sur les appareils.
export const db = new Dexie("chantierflow") as Dexie & {
  outbox: EntityTable<OutboxEntry, "seq">;
  reportDrafts: EntityTable<ReportDraft, "id">;
  blobs: EntityTable<StoredBlob, "id">;
  cache: EntityTable<CacheEntry, "key">;
};

db.version(1).stores({
  outbox: "++seq, id, state, entityId",
  reportDrafts: "id, projectId, updatedAt",
  blobs: "id",
  cache: "key",
});

/** Efface les données locales à la déconnexion (téléphone partagé sur un chantier). */
export async function clearLocalData(): Promise<void> {
  await Promise.all([db.outbox.clear(), db.reportDrafts.clear(), db.blobs.clear(), db.cache.clear()]);
}
