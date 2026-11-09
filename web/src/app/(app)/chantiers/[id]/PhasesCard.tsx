"use client";

import { type FormEvent, useState } from "react";
import { Badge, Button, Card, Empty, ErrorBanner, Input } from "@/components/ui";
import type { Items, Phase, Project } from "@/lib/api";
import { api } from "@/lib/api";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

const NEXT: Record<string, string> = { pending: "in_progress", in_progress: "done", done: "pending" };
const LABEL: Record<string, string> = { pending: "À venir", in_progress: "En cours", done: "Terminée" };

export function PhasesCard({ project }: { project: Project }) {
  const { user } = useSession();
  const { data, error, reload } = useResource<Items<Phase>>(`/projects/${project.id}/phases`);
  const [name, setName] = useState("");
  const [actionError, setActionError] = useState<unknown>(null);
  const editable = can.manageProjects(user.role) && project.status !== "archived";
  const phases = data?.items ?? [];

  async function run(fn: () => Promise<unknown>) {
    setActionError(null);
    try {
      await fn();
      reload();
    } catch (err) {
      setActionError(err);
    }
  }

  function move(index: number, delta: number) {
    const ids = phases.map((p) => p.id);
    const other = index + delta;
    const a = ids[index];
    const b = ids[other];
    if (a === undefined || b === undefined) return;
    ids[index] = b;
    ids[other] = a;
    void run(() => api(`/projects/${project.id}/phases/order`, { method: "PUT", body: { phase_ids: ids } }));
  }

  return (
    <Card style={{ padding: "18px 20px" }}>
      <h2 className="sec-t">Phases</h2>
      <ErrorBanner error={error} />
      {phases.length === 0 ? (
        <Empty>Aucune phase pour le moment.</Empty>
      ) : (
        phases.map((p, i) => (
          <div key={p.id} className="li" style={{ flexWrap: "wrap" }}>
            <span style={{ fontWeight: 600, minWidth: 0 }}>
              {p.position}. {p.name}
            </span>
            <span style={{ display: "flex", alignItems: "center", gap: 6 }}>
              <Badge tone={p.status === "done" ? "green" : p.status === "in_progress" ? "blue" : "neutral"}>{LABEL[p.status]}</Badge>
              {editable && (
                <>
                  <Button variant="secondary" style={{ height: 34 }} aria-label={`Changer le statut de ${p.name}`} onClick={() => run(() => api(`/projects/${project.id}/phases/${p.id}`, { method: "PATCH", body: { status: NEXT[p.status] } }))}>
                    Suivant
                  </Button>
                  <Button variant="secondary" style={{ height: 34, padding: "0 10px" }} aria-label={`Monter ${p.name}`} disabled={i === 0} onClick={() => move(i, -1)}>
                    ↑
                  </Button>
                  <Button variant="secondary" style={{ height: 34, padding: "0 10px" }} aria-label={`Descendre ${p.name}`} disabled={i === phases.length - 1} onClick={() => move(i, 1)}>
                    ↓
                  </Button>
                </>
              )}
            </span>
          </div>
        ))
      )}
      {editable && (
        <form
          style={{ display: "flex", alignItems: "flex-end", gap: 8, marginTop: 12 }}
          onSubmit={(e: FormEvent) => {
            e.preventDefault();
            void run(() => api(`/projects/${project.id}/phases`, { method: "POST", body: { name } }).then(() => setName("")));
          }}
        >
          <div style={{ flex: 1, minWidth: 0 }}>
            <Input label="Nouvelle phase" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <Button type="submit" variant="secondary" disabled={name.trim().length < 2}>
            Ajouter
          </Button>
        </form>
      )}
      <ErrorBanner error={actionError} />
    </Card>
  );
}
