"use client";

import { useLiveQuery } from "dexie-react-hooks";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { type ChangeEvent, useRef, useState } from "react";
import { IconBox, IconCamera, IconCard, IconClose, IconDocLines, IconOffline } from "@/components/icons";
import { ErrorBanner } from "@/components/ui";
import type { Paged, Project } from "@/lib/api";
import { useCurrentProject } from "@/lib/current-project";
import { useOnline, useResource } from "@/lib/hooks";
import { db } from "@/lib/offline/db";
import { compressPhoto } from "@/lib/offline/photos";
import { enqueue, retry } from "@/lib/offline/sync";
import { syncNow } from "@/lib/offline/useSync";
import { can, useSession } from "@/lib/session";

export default function AddPage() {
  const { user } = useSession();
  const router = useRouter();
  const online = useOnline();
  const { data } = useResource<Paged<Project>>("/projects?limit=100");
  const [stored] = useCurrentProject();
  const list = data?.items.filter((p) => p.status !== "archived") ?? [];
  const project = list.find((p) => p.id === stored) ?? list[0];
  const entries = useLiveQuery(() => db.outbox.where("state").anyOf("pending", "failed", "conflict").toArray(), [], []);
  const file = useRef<HTMLInputElement>(null);
  const [error, setError] = useState<unknown>(null);
  const [added, setAdded] = useState(false);

  async function addPhoto(event: ChangeEvent<HTMLInputElement>) {
    const picked = event.target.files?.[0];
    if (!picked || !project) return;
    setError(null);
    try {
      const blob = await compressPhoto(picked);
      const blobId = crypto.randomUUID();
      await db.blobs.put({ id: blobId, blob, mime: "image/jpeg", name: picked.name });
      await enqueue({
        kind: "photo.upload",
        label: `Photo · ${picked.name}`,
        entityId: crypto.randomUUID(),
        method: "POST",
        path: "/api/v1/attachments/presign",
        blobId,
        body: { id: crypto.randomUUID(), project_id: project.id, kind: "site_photo", mime_type: "image/jpeg", size_bytes: blob.size, original_name: picked.name },
      });
      setAdded(true);
      if (navigator.onLine) void syncNow();
    } catch (err) {
      setError(err);
    }
    event.target.value = "";
  }

  const close = () => (window.history.length > 1 ? router.back() : router.push("/"));
  const suffix = project ? `?project=${project.id}` : "";
  return (
    <div style={{ position: "fixed", inset: 0, zIndex: 40, display: "flex", alignItems: "flex-end", justifyContent: "center" }}>
      <div aria-hidden="true" onClick={close} style={{ position: "absolute", inset: 0, background: "rgba(16, 38, 74, 0.55)" }} />
      <div role="dialog" aria-modal="true" aria-labelledby="sheet-t" style={{ position: "relative", width: "100%", maxWidth: 480, maxHeight: "92dvh", overflowY: "auto", background: "#FFFFFF", borderRadius: "22px 22px 0 0", padding: "10px 16px 24px", display: "flex", flexDirection: "column", gap: 6 }}>
        <div aria-hidden="true" style={{ width: 40, height: 5, borderRadius: 3, background: "#D2D2D7", margin: "0 auto 8px" }} />
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "0 4px 6px" }}>
          <h2 id="sheet-t" style={{ margin: 0, fontSize: 18, fontWeight: 700 }}>
            {project ? `Ajouter sur ${project.name}` : "Ajouter"}
          </h2>
          <button type="button" aria-label="Fermer" onClick={close} style={{ width: 40, height: 40, border: 0, borderRadius: "50%", background: "#F5F5F7", color: "#1D1D1F", display: "flex", alignItems: "center", justifyContent: "center", cursor: "pointer" }}>
            <IconClose size={18} />
          </button>
        </div>
        {!online && (
          <div style={{ display: "flex", gap: 10, alignItems: "center", padding: "10px 12px", borderRadius: 10, background: "#F2F2F7", fontSize: 13, marginBottom: 6 }}>
            <span style={{ color: "#48484A" }}><IconOffline size={18} width={2} /></span>
            <span>
              <strong>Hors connexion.</strong> {entries.length} élément{entries.length > 1 ? "s" : ""} en attente, envoyé{entries.length > 1 ? "s" : ""} au retour du réseau.
            </span>
          </div>
        )}
        {can.createExpense(user.role) && (
          <Link href={`/depenses/nouvelle${suffix}`} className="it">
            <span className="ic" style={{ background: "#EBF0FD", color: "#2B59E6" }}><IconCard size={22} width={2} /></span>
            <span><span className="t">Dépense</span><span className="st">Montant, catégorie, photo du reçu</span></span>
          </Link>
        )}
        {can.writeReport(user.role) && (
          <Link href={`/rapports/nouveau${suffix}`} className="it">
            <span className="ic" style={{ background: "#E8F5EC", color: "#1A7F45" }}><IconDocLines size={22} /></span>
            <span><span className="t">Rapport journalier</span><span className="st">Travaux, effectif, photos du jour</span></span>
          </Link>
        )}
        {can.writeReport(user.role) && project && (
          <button type="button" className="it" onClick={() => file.current?.click()}>
            <span className="ic" style={{ background: "#FFF6D6", color: "#8A6400" }}><IconCamera size={22} /></span>
            <span><span className="t">Photo</span><span className="st">{added ? "Photo ajoutée à la file d'envoi" : "Ajoutée à la galerie du chantier"}</span></span>
          </button>
        )}
        <input ref={file} type="file" accept="image/*" capture="environment" onChange={addPhoto} style={{ display: "none" }} aria-label="Photo du chantier" />
        {can.moveStock(user.role) && (
          <Link href={`/materiaux${suffix}`} className="it">
            <span className="ic" style={{ background: "#F2F2F7", color: "#48484A" }}><IconBox size={22} width={2} /></span>
            <span><span className="t">Mouvement de stock</span><span className="st">Entrée, sortie ou ajustement</span></span>
          </Link>
        )}
        <ErrorBanner error={error} />
        {entries.length > 0 && (
          <div style={{ marginTop: 8, borderTop: "1px solid #F2F2F7", paddingTop: 12 }}>
            <div style={{ fontSize: 13, fontWeight: 700, color: "#6E6E73", padding: "0 4px 8px" }}>En attente d&apos;envoi</div>
            {entries.map((e) => (
              <div key={e.seq} style={{ display: "flex", alignItems: "center", gap: 10, padding: "8px 4px", fontSize: 14 }}>
                <span style={{ width: 8, height: 8, borderRadius: "50%", background: e.state === "pending" ? "#C76A00" : "#D93025", flex: "none" }} />
                <span style={{ flex: 1, minWidth: 0 }}>{e.label}</span>
                {e.state === "pending" ? (
                  <span style={{ fontSize: 12, color: "#9A5200", fontWeight: 600 }}>En attente</span>
                ) : (
                  <button type="button" onClick={() => (e.state === "failed" ? retry(e.seq!).then(syncNow) : router.push("/synchro"))} style={{ height: 32, padding: "0 10px", borderRadius: 8, border: "1px solid #F5C6C2", background: "#FFFFFF", color: "#B3261E", fontSize: 12, fontWeight: 600, cursor: "pointer", whiteSpace: "nowrap" }}>
                    {e.state === "failed" ? "Échec · Réessayer" : "Conflit · Voir"}
                  </button>
                )}
              </div>
            ))}
            <Link href="/synchro" style={{ display: "block", padding: "8px 4px", fontSize: 13, fontWeight: 600 }}>
              Voir tous les envois
            </Link>
          </div>
        )}
      </div>
    </div>
  );
}
