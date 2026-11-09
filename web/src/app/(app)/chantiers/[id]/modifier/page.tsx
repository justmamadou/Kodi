"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import { Badge, Button, ErrorBanner, fieldError, Input, Loading, PROJECT_TONE, Select, Textarea } from "@/components/ui";
import type { Directory, Items, Project, StatusHistory } from "@/lib/api";
import { api } from "@/lib/api";
import { dateTime, PROJECT_STATUS_LABEL, ROLE_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";

// Mêmes transitions que le serveur : on ne propose que ce qui sera accepté.
const NEXT_STATUS: Record<string, string[]> = {
  draft: ["planned", "in_progress", "archived"],
  planned: ["draft", "in_progress", "archived"],
  in_progress: ["completed"],
  completed: ["in_progress", "archived"],
  archived: [],
};

export default function EditProjectPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const { data: project, error: loadError, loading, reload } = useResource<Project>(`/projects/${id}`);
  const directory = useResource<Items<Directory>>("/directory");
  const history = useResource<Items<StatusHistory>>(`/projects/${id}/status-history`);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  if (loading) return <Loading />;
  if (!project) return <ErrorBanner error={loadError} />;
  const managers = directory.data?.items.filter((p) => p.role === "manager" || p.role === "owner") ?? [];

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setError(null);
    try {
      await api(`/projects/${id}`, {
        method: "PATCH",
        ifMatch: project?.row_version,
        body: {
          reference: form.get("reference"),
          name: form.get("name"),
          client_name: form.get("client_name"),
          address: form.get("address"),
          description: form.get("description") || null,
          start_date: form.get("start_date") || null,
          planned_end_date: form.get("planned_end_date") || null,
          manager_id: form.get("manager_id") || null,
        },
      });
      router.push(`/chantiers/${id}`);
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

  async function transition(status: string) {
    if (status === "archived" && !window.confirm("Archiver ce chantier ? Il passera en lecture seule.")) return;
    setError(null);
    try {
      await api(`/projects/${id}/status`, { method: "POST", body: { status } });
      reload();
      history.reload();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <div style={{ maxWidth: 1120, width: "100%", display: "flex", flexDirection: "column", gap: 20 }}>
      <div>
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/chantiers" style={{ fontWeight: 600 }}>
            Chantiers
          </Link>
          <span className="muted"> / </span>
          <Link href={`/chantiers/${id}`} style={{ fontWeight: 600 }}>
            {project.name}
          </Link>
          <span className="muted"> / Modifier</span>
        </nav>
        <h1 className="page-h1" style={{ marginTop: 10 }}>
          Modifier le chantier
        </h1>
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
        <form key={project.row_version} onSubmit={submit} noValidate className="card" style={{ flex: "999 1 520px", minWidth: 0, padding: 24, display: "flex", flexDirection: "column", gap: 18 }}>
          <h2 className="sec-t">Informations du chantier</h2>
          <Input label="Nom du chantier" name="name" required defaultValue={project.name} error={fieldError(error, "name")} />
          <div className="g2">
            <Input label="Référence" name="reference" required defaultValue={project.reference} error={fieldError(error, "reference")} />
            <Input label="Client / maître d'ouvrage" name="client_name" required defaultValue={project.client_name} error={fieldError(error, "client_name")} />
          </div>
          <Input label="Adresse du chantier" name="address" required defaultValue={project.address} error={fieldError(error, "address")} />
          <Textarea label="Description" name="description" defaultValue={project.description ?? ""} />
          <div className="g2">
            <Input label="Date de démarrage" name="start_date" type="date" required defaultValue={project.start_date} error={fieldError(error, "start_date")} />
            <Input label="Date de fin prévue" name="planned_end_date" type="date" defaultValue={project.planned_end_date ?? ""} error={fieldError(error, "planned_end_date")} />
          </div>
          {managers.length > 0 && (
            <Select label="Conducteur responsable" name="manager_id" required defaultValue={project.manager_id} error={fieldError(error, "manager_id")}>
              {managers.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.full_name} — {ROLE_LABEL[m.role]}
                </option>
              ))}
            </Select>
          )}
          <ErrorBanner error={error} />
          <div style={{ display: "flex", justifyContent: "flex-end", gap: 12, flexWrap: "wrap" }}>
            <Link href={`/chantiers/${id}`} className="btn btn-g">
              Annuler
            </Link>
            <button type="submit" className="btn btn-p" disabled={busy || project.status === "archived"}>
              Enregistrer
            </button>
          </div>
        </form>
        <section className="card" style={{ flex: "1 1 300px", minWidth: 0, padding: 24, display: "flex", flexDirection: "column", gap: 14 }} aria-labelledby="h-cycle">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
            <h2 id="h-cycle" className="sec-t">
              Cycle de vie
            </h2>
            <Badge tone={PROJECT_TONE[project.status]}>{PROJECT_STATUS_LABEL[project.status]}</Badge>
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
            {NEXT_STATUS[project.status]?.map((s) => (
              <Button key={s} variant={s === "archived" ? "danger" : "secondary"} onClick={() => transition(s)}>
                Passer à « {PROJECT_STATUS_LABEL[s]} »
              </Button>
            ))}
          </div>
          <div>
            {history.data?.items.map((h) => (
              <div key={h.id} className="li" style={{ display: "block" }}>
                <strong>{PROJECT_STATUS_LABEL[h.to_status]}</strong>
                <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                  {h.actor_name} · {dateTime(h.created_at)}
                  {h.comment ? ` · ${h.comment}` : ""}
                </div>
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}
