import { db, type OutboxEntry, type OutboxKind } from "./db";

/**
 * Seules ces opérations peuvent attendre le réseau. Une approbation, un rejet, un paiement ou une
 * publication de budget n'y figurent pas : une décision financière se prend en ligne.
 */
const OFFLINE_ALLOWED: RegExp[] = [
  /^PUT \/api\/v1\/daily-reports\/[^/]+$/,
  /^POST \/api\/v1\/daily-reports\/[^/]+\/submit$/,
  /^POST \/api\/v1\/projects\/[^/]+\/tasks$/,
  /^POST \/api\/v1\/tasks\/[^/]+\/progress$/,
  /^POST \/api\/v1\/projects\/[^/]+\/expenses$/,
  /^POST \/api\/v1\/expenses\/[^/]+\/submit$/,
  /^POST \/api\/v1\/attachments\/presign$/,
];

export function isAllowedOffline(method: string, path: string): boolean {
  return OFFLINE_ALLOWED.some((re) => re.test(`${method} ${path}`));
}

export class OfflineNotAllowedError extends Error {
  constructor(method: string, path: string) {
    super(`${method} ${path} exige une connexion`);
    this.name = "OfflineNotAllowedError";
  }
}

export interface EnqueueInput {
  kind: OutboxKind;
  label: string;
  entityId: string;
  method: "POST" | "PUT";
  path: string;
  body: unknown;
  usesReportVersion?: boolean;
  blobId?: string;
}

/** Place une opération dans la file. Refuse tout ce qui n'est pas explicitement autorisé. */
export async function enqueue(input: EnqueueInput): Promise<OutboxEntry> {
  if (!isAllowedOffline(input.method, input.path)) {
    throw new OfflineNotAllowedError(input.method, input.path);
  }
  const entry: OutboxEntry = {
    ...input,
    id: crypto.randomUUID(),
    state: "pending",
    attempts: 0,
    createdAt: Date.now(),
  };
  await db.outbox.add(entry);
  return entry;
}

export interface SyncDeps {
  fetch: typeof fetch;
  csrfToken: () => string | undefined;
}

export interface SyncResult {
  sent: number;
  /** Réseau absent ou serveur indisponible : on réessaiera plus tard. */
  interrupted: boolean;
  /** Session expirée : l'utilisateur doit se reconnecter, sa file reste intacte. */
  needsLogin: boolean;
}

class StopSync extends Error {
  constructor(readonly reason: "network" | "login") {
    super(reason);
  }
}

class Rejected extends Error {
  constructor(
    readonly state: "failed" | "conflict",
    message: string,
  ) {
    super(message);
  }
}

async function send(deps: SyncDeps, entry: OutboxEntry, path: string, body: unknown, ifMatch?: number): Promise<unknown> {
  const headers: Record<string, string> = { "Content-Type": "application/json", "Idempotency-Key": entry.id };
  const csrf = deps.csrfToken();
  if (csrf) headers["X-CSRF-Token"] = csrf;
  if (ifMatch !== undefined) headers["If-Match"] = String(ifMatch);
  let response: Response;
  try {
    response = await deps.fetch(path, { method: entry.method, headers, credentials: "same-origin", body: JSON.stringify(body) });
  } catch {
    throw new StopSync("network");
  }
  return handle(response);
}

async function handle(response: Response): Promise<unknown> {
  if (response.ok) {
    return response.status === 204 ? null : response.json().catch(() => null);
  }
  if (response.status === 401) throw new StopSync("login");
  if (response.status >= 500 || response.status === 429) throw new StopSync("network");
  const problem = (await response.json().catch(() => ({}))) as { detail?: string; code?: string };
  const message = problem.detail ?? `Erreur ${response.status}`;
  // Rien n'est écrasé silencieusement : la donnée a changé côté serveur, l'utilisateur tranchera.
  if (response.status === 409 && problem.code === "resource.stale") throw new Rejected("conflict", message);
  throw new Rejected("failed", message);
}

async function uploadPhoto(deps: SyncDeps, entry: OutboxEntry): Promise<void> {
  const presigned = (await send(deps, entry, entry.path, entry.body)) as { attachment: { id: string; status: string }; upload_url: string };
  if (presigned.attachment.status !== "uploaded") {
    const stored = entry.blobId ? await db.blobs.get(entry.blobId) : undefined;
    if (!stored) throw new Rejected("failed", "La photo n'est plus disponible sur cet appareil.");
    let put: Response;
    try {
      put = await deps.fetch(presigned.upload_url, { method: "PUT", body: stored.blob });
    } catch {
      throw new StopSync("network");
    }
    if (!put.ok) throw new StopSync("network");
    const headers: Record<string, string> = {};
    const csrf = deps.csrfToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
    let complete: Response;
    try {
      complete = await deps.fetch(`/api/v1/attachments/${presigned.attachment.id}/complete`, {
        method: "POST",
        headers,
        credentials: "same-origin",
      });
    } catch {
      throw new StopSync("network");
    }
    await handle(complete);
  }
  if (entry.blobId) await db.blobs.delete(entry.blobId);
}

async function process(deps: SyncDeps, entry: OutboxEntry): Promise<void> {
  if (entry.kind === "photo.upload") return uploadPhoto(deps, entry);

  let ifMatch: number | undefined;
  if (entry.usesReportVersion && !entry.force) {
    // Relue à l'envoi : deux enregistrements en file se suivent sans se contredire.
    ifMatch = (await db.reportDrafts.get(entry.entityId))?.rowVersion;
  }
  const result = (await send(deps, entry, entry.path, entry.body, ifMatch)) as { row_version?: number } | null;
  if (entry.kind.startsWith("report.") && result?.row_version !== undefined) {
    await db.reportDrafts.update(entry.entityId, { rowVersion: result.row_version });
  }
}

let running: Promise<SyncResult> | null = null;

/** Rejoue la file dans l'ordre. Un seul passage à la fois, même si plusieurs déclencheurs arrivent. */
export function flushOutbox(deps: SyncDeps): Promise<SyncResult> {
  running ??= run(deps).finally(() => {
    running = null;
  });
  return running;
}

async function run(deps: SyncDeps): Promise<SyncResult> {
  const result: SyncResult = { sent: 0, interrupted: false, needsLogin: false };
  const blocked = new Set<string>();
  const entries = await db.outbox.orderBy("seq").toArray();
  for (const entry of entries) {
    if (entry.state === "failed" || entry.state === "conflict") {
      blocked.add(entry.entityId);
      continue;
    }
    if (entry.state !== "pending") continue;
    // L'opération précédente sur cette entité a échoué : soumettre sans elle n'aurait pas de sens.
    if (blocked.has(entry.entityId)) continue;

    try {
      await process(deps, entry);
      await db.outbox.update(entry.seq!, { state: "synced", syncedAt: Date.now(), error: undefined, attempts: entry.attempts + 1 });
      result.sent++;
    } catch (err) {
      if (err instanceof StopSync) {
        await db.outbox.update(entry.seq!, { attempts: entry.attempts + 1 });
        result.interrupted = err.reason === "network";
        result.needsLogin = err.reason === "login";
        return result;
      }
      if (err instanceof Rejected) {
        await db.outbox.update(entry.seq!, { state: err.state, error: err.message, attempts: entry.attempts + 1 });
        blocked.add(entry.entityId);
        continue;
      }
      throw err;
    }
  }
  await pruneSynced();
  return result;
}

/** L'historique « synchronisé » reste visible 24 h, sans jamais grossir indéfiniment. */
async function pruneSynced(): Promise<void> {
  const threshold = Date.now() - 24 * 60 * 60 * 1000;
  await db.outbox
    .where("state")
    .equals("synced")
    .filter((e) => (e.syncedAt ?? 0) < threshold)
    .delete();
}

/** Réessayer une opération en échec. */
export async function retry(seq: number): Promise<void> {
  await db.outbox.update(seq, { state: "pending", error: undefined });
}

/** Après un conflit : l'utilisateur garde explicitement sa version. */
export async function keepMine(seq: number): Promise<void> {
  await db.outbox.update(seq, { state: "pending", error: undefined, force: true });
}

/** Abandonner une opération et celles qui en dépendent. */
export async function discard(seq: number): Promise<void> {
  const entry = await db.outbox.get(seq);
  if (!entry) return;
  const dependents = await db.outbox
    .where("entityId")
    .equals(entry.entityId)
    .filter((e) => e.state !== "synced" && (e.seq ?? 0) >= seq)
    .toArray();
  await db.transaction("rw", db.outbox, db.blobs, async () => {
    for (const d of dependents) {
      if (d.blobId) await db.blobs.delete(d.blobId);
      await db.outbox.delete(d.seq!);
    }
  });
}
