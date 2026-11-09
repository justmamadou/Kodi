import { beforeEach, describe, expect, it } from "vitest";
import { db } from "./db";
import { discard, enqueue, flushOutbox, isAllowedOffline, keepMine, OfflineNotAllowedError, retry, type SyncDeps } from "./sync";

type Call = { url: string; init: RequestInit };

/** Faux serveur : chaque réponse est décidée par `respond`, et tous les appels sont enregistrés. */
function server(respond: (url: string, init: RequestInit, n: number) => Response | Promise<Response>) {
  const calls: Call[] = [];
  const deps: SyncDeps = {
    csrfToken: () => "jeton-csrf",
    fetch: (async (url: string, init: RequestInit) => {
      calls.push({ url, init });
      return respond(url, init, calls.length);
    }) as unknown as typeof fetch,
  };
  return { deps, calls };
}

const json = (status: number, body: unknown = {}) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

const reportSave = (entityId: string) =>
  enqueue({
    kind: "report.save",
    label: "Rapport du 03/11",
    entityId,
    method: "PUT",
    path: `/api/v1/daily-reports/${entityId}`,
    body: { work_done: "Coffrage" },
    usesReportVersion: true,
  });

const reportSubmit = (entityId: string) =>
  enqueue({
    kind: "report.submit",
    label: "Soumission",
    entityId,
    method: "POST",
    path: `/api/v1/daily-reports/${entityId}/submit`,
    body: {},
  });

async function states() {
  return (await db.outbox.orderBy("seq").toArray()).map((e) => e.state);
}

beforeEach(async () => {
  await Promise.all([db.outbox.clear(), db.reportDrafts.clear(), db.blobs.clear()]);
});

describe("aucune approbation financière hors connexion", () => {
  it.each([
    ["POST", "/api/v1/expenses/abc/approve"],
    ["POST", "/api/v1/expenses/abc/reject"],
    ["POST", "/api/v1/expenses/abc/cancel"],
    ["POST", "/api/v1/expenses/abc/payments"],
    ["POST", "/api/v1/budget-versions/abc/publish"],
    ["POST", "/api/v1/budget-versions/abc/revise"],
    ["PATCH", "/api/v1/members/abc"],
    ["POST", "/api/v1/stock-movements/abc/post"],
  ])("refuse %s %s", async (method, path) => {
    expect(isAllowedOffline(method, path)).toBe(false);
    await expect(
      enqueue({ kind: "report.save", label: "x", entityId: "e", method: method as "POST", path, body: {} }),
    ).rejects.toBeInstanceOf(OfflineNotAllowedError);
    expect(await db.outbox.count()).toBe(0);
  });

  it.each([
    ["PUT", "/api/v1/daily-reports/abc"],
    ["POST", "/api/v1/daily-reports/abc/submit"],
    ["POST", "/api/v1/projects/abc/tasks"],
    ["POST", "/api/v1/tasks/abc/progress"],
    ["POST", "/api/v1/projects/abc/expenses"],
    ["POST", "/api/v1/expenses/abc/submit"],
  ])("autorise %s %s", (method, path) => {
    expect(isAllowedOffline(method, path)).toBe(true);
  });
});

describe("synchronisation", () => {
  it("envoie dans l'ordre, avec l'idempotence et le jeton CSRF", async () => {
    const a = await reportSave("r1");
    await reportSubmit("r1");
    const { deps, calls } = server(() => json(200, { row_version: 2 }));

    const result = await flushOutbox(deps);

    expect(result).toEqual({ sent: 2, interrupted: false, needsLogin: false });
    expect(calls.map((c) => c.url)).toEqual(["/api/v1/daily-reports/r1", "/api/v1/daily-reports/r1/submit"]);
    const headers = calls[0]!.init.headers as Record<string, string>;
    expect(headers["Idempotency-Key"]).toBe(a.id);
    expect(headers["X-CSRF-Token"]).toBe("jeton-csrf");
    expect(await states()).toEqual(["synced", "synced"]);
  });

  it("sans réseau : garde tout en attente et réutilise la même clé au rejeu", async () => {
    const entry = await reportSave("r1");
    const down = server(() => {
      throw new TypeError("Failed to fetch");
    });
    const first = await flushOutbox(down.deps);
    expect(first).toMatchObject({ sent: 0, interrupted: true });
    expect(await states()).toEqual(["pending"]);

    const up = server(() => json(200, { row_version: 1 }));
    await flushOutbox(up.deps);
    expect((up.calls[0]!.init.headers as Record<string, string>)["Idempotency-Key"]).toBe(entry.id);
    expect(await states()).toEqual(["synced"]);
  });

  it("un serveur en panne interrompt sans marquer d'échec définitif", async () => {
    await reportSave("r1");
    const { deps } = server(() => json(503));
    expect(await flushOutbox(deps)).toMatchObject({ interrupted: true });
    expect(await states()).toEqual(["pending"]);
  });

  it("une session expirée demande de se reconnecter, sans rien perdre", async () => {
    await reportSave("r1");
    const { deps } = server(() => json(401));
    expect(await flushOutbox(deps)).toMatchObject({ needsLogin: true, sent: 0 });
    expect(await states()).toEqual(["pending"]);
  });

  it("une erreur métier marque l'échec et bloque la suite de la même entité, pas les autres", async () => {
    await reportSave("r1");
    await reportSubmit("r1");
    await reportSave("r2");
    const { deps, calls } = server((url) =>
      url.endsWith("/r1") ? json(422, { code: "validation.failed", detail: "Travaux manquants" }) : json(200, { row_version: 1 }),
    );

    const result = await flushOutbox(deps);

    expect(result.sent).toBe(1);
    expect(calls.map((c) => c.url)).toEqual(["/api/v1/daily-reports/r1", "/api/v1/daily-reports/r2"]);
    expect(await states()).toEqual(["failed", "pending", "synced"]);
    const failed = await db.outbox.orderBy("seq").first();
    expect(failed?.error).toBe("Travaux manquants");
  });

  it("réessayer relance l'opération et débloque la suite", async () => {
    const failed = await reportSave("r1");
    await reportSubmit("r1");
    const bad = server(() => json(422, { detail: "Non" }));
    await flushOutbox(bad.deps);
    const stored = await db.outbox.where("id").equals(failed.id).first();
    await retry(stored!.seq!);

    const good = server(() => json(200, { row_version: 1 }));
    expect((await flushOutbox(good.deps)).sent).toBe(2);
    expect(await states()).toEqual(["synced", "synced"]);
  });

  it("ne rejoue jamais une opération déjà synchronisée", async () => {
    await reportSave("r1");
    const once = server(() => json(200, { row_version: 1 }));
    await flushOutbox(once.deps);
    const twice = server(() => json(200));
    await flushOutbox(twice.deps);
    expect(twice.calls).toHaveLength(0);
  });

  it("deux déclencheurs simultanés ne font qu'un seul passage", async () => {
    await reportSave("r1");
    const { deps, calls } = server(() => json(200, { row_version: 1 }));
    await Promise.all([flushOutbox(deps), flushOutbox(deps), flushOutbox(deps)]);
    expect(calls).toHaveLength(1);
  });
});

describe("conflits : rien n'est écrasé silencieusement", () => {
  const seedDraft = (rowVersion: number) =>
    db.reportDrafts.put({
      id: "r1",
      projectId: "p1",
      reportDate: "2026-11-03",
      teamLabel: "",
      workDone: "Coffrage",
      workforceCount: null,
      issues: "",
      weather: null,
      rowVersion,
      updatedAt: Date.now(),
    });

  it("envoie la version vue, et signale le conflit si elle a changé", async () => {
    await seedDraft(3);
    await reportSave("r1");
    const { deps, calls } = server(() => json(409, { code: "resource.stale", detail: "Modifié entre-temps" }));

    await flushOutbox(deps);

    expect((calls[0]!.init.headers as Record<string, string>)["If-Match"]).toBe("3");
    const entry = await db.outbox.orderBy("seq").first();
    expect(entry?.state).toBe("conflict");
    expect(entry?.error).toBe("Modifié entre-temps");
  });

  it("garder ma version renvoie sans If-Match, sur décision explicite", async () => {
    await seedDraft(3);
    await reportSave("r1");
    await flushOutbox(server(() => json(409, { code: "resource.stale", detail: "Modifié" })).deps);
    const entry = await db.outbox.orderBy("seq").first();
    await keepMine(entry!.seq!);

    const { deps, calls } = server(() => json(200, { row_version: 5 }));
    await flushOutbox(deps);

    expect((calls[0]!.init.headers as Record<string, string>)["If-Match"]).toBeUndefined();
    expect((await db.reportDrafts.get("r1"))?.rowVersion).toBe(5);
  });

  it("la version suit les enregistrements successifs de la file", async () => {
    await seedDraft(1);
    await reportSave("r1");
    await reportSave("r1");
    let version = 1;
    const { deps, calls } = server(() => json(200, { row_version: ++version }));
    await flushOutbox(deps);
    expect((calls[0]!.init.headers as Record<string, string>)["If-Match"]).toBe("1");
    expect((calls[1]!.init.headers as Record<string, string>)["If-Match"]).toBe("2");
  });

  it("abandonner retire l'opération et ce qui en dépend", async () => {
    await reportSave("r1");
    await reportSubmit("r1");
    await reportSave("r2");
    const first = await db.outbox.orderBy("seq").first();
    await discard(first!.seq!);
    expect((await db.outbox.toArray()).map((e) => e.entityId)).toEqual(["r2"]);
  });
});

describe("photos", () => {
  const photo = () =>
    enqueue({
      kind: "photo.upload",
      label: "Photo",
      entityId: "r1",
      method: "POST",
      path: "/api/v1/attachments/presign",
      body: { id: "a1" },
      blobId: "b1",
    });

  it("dépose le fichier via l'URL pré-signée, confirme, puis libère l'appareil", async () => {
    await db.blobs.put({ id: "b1", blob: new Blob(["jpeg"]), mime: "image/jpeg", name: "p.jpg" });
    await photo();
    const { deps, calls } = server((url) => {
      if (url.endsWith("/presign")) return json(201, { attachment: { id: "a1", status: "pending" }, upload_url: "https://s3.test/put" });
      return json(200, {});
    });

    expect((await flushOutbox(deps)).sent).toBe(1);

    expect(calls.map((c) => c.url)).toEqual(["/api/v1/attachments/presign", "https://s3.test/put", "/api/v1/attachments/a1/complete"]);
    const putHeaders = calls[1]!.init.headers as Record<string, string> | undefined;
    expect(putHeaders?.["X-CSRF-Token"]).toBeUndefined(); // le jeton ne part jamais vers le stockage
    expect(await db.blobs.count()).toBe(0);
  });

  it("le stockage injoignable : la photo reste sur l'appareil pour plus tard", async () => {
    await db.blobs.put({ id: "b1", blob: new Blob(["jpeg"]), mime: "image/jpeg", name: "p.jpg" });
    await photo();
    const { deps } = server((url) => {
      if (url.endsWith("/presign")) return json(201, { attachment: { id: "a1", status: "pending" }, upload_url: "https://s3.test/put" });
      throw new TypeError("Failed to fetch");
    });
    expect(await flushOutbox(deps)).toMatchObject({ interrupted: true, sent: 0 });
    expect(await db.blobs.count()).toBe(1);
    expect(await states()).toEqual(["pending"]);
  });
});
