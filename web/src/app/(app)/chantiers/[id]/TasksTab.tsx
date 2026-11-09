"use client";

import { type FormEvent, useState } from "react";
import { Badge, Button, Card, Empty, ErrorBanner, Input, Loading, Select, type Tone } from "@/components/ui";
import type { Items, Project, ProjectMember, Task } from "@/lib/api";
import { date, PRIORITY_LABEL, TASK_STATUS_LABEL } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { enqueue } from "@/lib/offline/sync";
import { syncNow } from "@/lib/offline/useSync";
import { can, useSession } from "@/lib/session";
import { PhasesCard } from "./PhasesCard";

const STATUS_TONE: Record<string, Tone> = { todo: "neutral", in_progress: "blue", blocked: "red", done: "green", cancelled: "neutral" };
const NEXT: Record<string, { to: string; label: string }[]> = {
  todo: [{ to: "in_progress", label: "Démarrer" }],
  in_progress: [
    { to: "done", label: "Terminer" },
    { to: "blocked", label: "Bloquer" },
  ],
  blocked: [{ to: "in_progress", label: "Reprendre" }],
  done: [{ to: "in_progress", label: "Rouvrir" }],
  cancelled: [],
};

export function TasksTab({ project }: { project: Project }) {
  const { user } = useSession();
  const online = useOnline();
  const [status, setStatus] = useState("");
  const [assignee, setAssignee] = useState("");
  const query = new URLSearchParams();
  if (status) query.set("status", status);
  if (assignee) query.set("assignee_id", assignee);
  const { data, error, loading, reload } = useResource<Items<Task>>(`/projects/${project.id}/tasks?${query}`);
  const members = useResource<Items<ProjectMember>>(`/projects/${project.id}/members`);
  const [title, setTitle] = useState("");
  const [priority, setPriority] = useState("medium");
  const [due, setDue] = useState("");
  const [owner, setOwner] = useState("");
  const [actionError, setActionError] = useState<unknown>(null);
  const [queued, setQueued] = useState(false);
  const editable = project.status !== "archived";

  /** Création et avancement passent par la file : ils fonctionnent hors connexion et ne s'écrasent pas. */
  async function afterEnqueue() {
    if (navigator.onLine) {
      await syncNow();
      reload();
      setQueued(false);
    } else {
      setQueued(true);
    }
  }

  async function create(event: FormEvent) {
    event.preventDefault();
    setActionError(null);
    try {
      const id = crypto.randomUUID();
      await enqueue({
        kind: "task.create",
        label: `Tâche · ${title}`,
        entityId: id,
        method: "POST",
        path: `/api/v1/projects/${project.id}/tasks`,
        body: { id, title, priority, due_date: due || null, assignee_ids: owner ? [owner] : [] },
      });
      setTitle("");
      setDue("");
      setOwner("");
      await afterEnqueue();
    } catch (err) {
      setActionError(err);
    }
  }

  async function progress(task: Task, body: { status?: string; progress_pct?: number }, label: string) {
    setActionError(null);
    try {
      await enqueue({ kind: "task.progress", label: `${label} · ${task.title}`, entityId: task.id, method: "POST", path: `/api/v1/tasks/${task.id}/progress`, body });
      await afterEnqueue();
    } catch (err) {
      setActionError(err);
    }
  }

  const tasks = data?.items ?? [];
  return (
    <div className="flex flex-col gap-4">
      <PhasesCard project={project} />
      <Card className="p-6">
        <div className="mb-3 grid gap-3 sm:grid-cols-2">
          <Select label="Statut" value={status} onChange={(e) => setStatus(e.target.value)}>
            <option value="">Tous</option>
            {Object.entries(TASK_STATUS_LABEL).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </Select>
          <Select label="Responsable" value={assignee} onChange={(e) => setAssignee(e.target.value)}>
            <option value="">Tous</option>
            {members.data?.items.map((m) => (
              <option key={m.user_id} value={m.user_id}>
                {m.full_name}
              </option>
            ))}
          </Select>
        </div>
        {loading ? (
          <Loading />
        ) : error ? (
          <ErrorBanner error={error} />
        ) : tasks.length === 0 ? (
          <Empty>Aucune tâche.</Empty>
        ) : (
          <ul className="divide-y divide-line">
            {tasks.map((t) => {
              const mayAct = can.manageTasks(user.role) || t.assignee_ids.includes(user.id);
              return (
                <li key={t.id} className="py-3">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <span className="min-w-0 font-semibold">{t.title}</span>
                    <span className="flex items-center gap-1">
                      {t.priority !== "medium" && <Badge tone={t.priority === "urgent" ? "red" : "neutral"}>{PRIORITY_LABEL[t.priority]}</Badge>}
                      <Badge tone={STATUS_TONE[t.status]}>{TASK_STATUS_LABEL[t.status]}</Badge>
                    </span>
                  </div>
                  <p className="text-xs text-muted">
                    {t.progress_pct} %{t.due_date ? ` · échéance ${date(t.due_date)}` : ""}
                    {t.assignee_ids.length > 0 ? ` · ${t.assignee_ids.map((id) => members.data?.items.find((m) => m.user_id === id)?.full_name ?? "Responsable").join(", ")}` : ""}
                  </p>
                  <progress className="mt-1 h-1.5 w-full" max={100} value={t.progress_pct} aria-label={`Avancement de ${t.title}`} />
                  {editable && mayAct && user.role !== "viewer" && (
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                      {NEXT[t.status]?.map((n) => (
                        <Button key={n.to} variant="secondary" onClick={() => progress(t, { status: n.to }, n.label)}>
                          {n.label}
                        </Button>
                      ))}
                      {t.status === "in_progress" && (
                        <Select
                          label="Avancement"
                          value={String(t.progress_pct)}
                          onChange={(e) => progress(t, { progress_pct: Number(e.target.value) }, `Avancement ${e.target.value} %`)}
                        >
                          {[0, 10, 20, 30, 40, 50, 60, 70, 80, 90].map((p) => (
                            <option key={p} value={p}>
                              {p} %
                            </option>
                          ))}
                          {t.progress_pct % 10 !== 0 && <option value={t.progress_pct}>{t.progress_pct} %</option>}
                        </Select>
                      )}
                    </div>
                  )}
                </li>
              );
            })}
          </ul>
        )}
      </Card>
      {editable && can.manageTasks(user.role) && (
        <Card className="p-6">
          <h2 className="mb-3 text-lg font-bold">Nouvelle tâche</h2>
          <form onSubmit={create} className="grid gap-3 sm:grid-cols-2">
            <Input label="Titre" required minLength={2} value={title} onChange={(e) => setTitle(e.target.value)} />
            <Select label="Priorité" value={priority} onChange={(e) => setPriority(e.target.value)}>
              {Object.entries(PRIORITY_LABEL).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </Select>
            <Input label="Échéance" type="date" value={due} onChange={(e) => setDue(e.target.value)} />
            <Select label="Responsable" value={owner} onChange={(e) => setOwner(e.target.value)}>
              <option value="">Non affectée</option>
              {members.data?.items.map((m) => (
                <option key={m.user_id} value={m.user_id}>
                  {m.full_name}
                </option>
              ))}
            </Select>
            <div className="sm:col-span-2">
              <Button type="submit" disabled={title.trim().length < 2}>
                Créer la tâche
              </Button>
            </div>
          </form>
        </Card>
      )}
      <ErrorBanner error={actionError} />
      {(queued || !online) && (
        <p role="status" className="text-sm text-warn-ink">
          Hors connexion : vos changements de tâches partiront au retour du réseau (voir Envois).
        </p>
      )}
    </div>
  );
}
