"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { type ChangeEvent, type FormEvent, Suspense, useEffect, useRef, useState } from "react";
import { IconBack, IconCamera, IconOffline } from "@/components/icons";
import { ErrorBanner, Loading } from "@/components/ui";
import type { DailyReport, Items, Paged, Project, Task } from "@/lib/api";
import { api } from "@/lib/api";
import { useCurrentProject } from "@/lib/current-project";
import { date, today, WEATHER_LABEL } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { db } from "@/lib/offline/db";
import { compressPhoto } from "@/lib/offline/photos";
import { enqueue } from "@/lib/offline/sync";
import { syncNow } from "@/lib/offline/useSync";
import { can, useSession } from "@/lib/session";

const AUTOSAVE_KEY = "report-form";

interface Form {
  id: string;
  projectId: string;
  reportDate: string;
  teamLabel: string;
  workDone: string;
  workforceCount: string;
  issues: string;
  weather: string;
  rowVersion?: number;
  doneTaskIds: string[];
}

interface PendingPhoto {
  id: string;
  name: string;
  size: number;
  url: string;
}

function blankForm(projectId = ""): Form {
  return { id: crypto.randomUUID(), projectId, reportDate: today(), teamLabel: "", workDone: "", workforceCount: "", issues: "", weather: "", doneTaskIds: [] };
}

export default function NewReportPage() {
  return (
    <Suspense fallback={<Loading />}>
      <ReportForm />
    </Suspense>
  );
}

function ReportForm() {
  const params = useSearchParams();
  const router = useRouter();
  const online = useOnline();
  const editId = params.get("id");
  const { user } = useSession();
  const [stored] = useCurrentProject();
  const presetProject = params.get("project") ?? stored ?? "";
  const projects = useResource<Paged<Project>>("/projects?limit=100");
  const fileInput = useRef<HTMLInputElement>(null);
  const [newTask, setNewTask] = useState<string | null>(null);
  const [form, setForm] = useState<Form | null>(null);
  const [photos, setPhotos] = useState<PendingPhoto[]>([]);
  const [error, setError] = useState<unknown>(null);
  const [fields, setFields] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [saved, setSaved] = useState(false);
  const tasks = useResource<Items<Task>>(form?.projectId ? `/projects/${form.projectId}/tasks` : null);

  // Le brouillon survit à une fermeture de l'application ou à une coupure de batterie.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      let initial: Form | null = null;
      if (editId) {
        const report = await api<DailyReport>(`/daily-reports/${editId}`);
        initial = {
          ...blankForm(report.project_id),
          id: report.id,
          reportDate: report.report_date,
          teamLabel: report.team_label,
          workDone: report.work_done,
          workforceCount: report.workforce_count?.toString() ?? "",
          issues: report.issues ?? "",
          weather: report.weather ?? "",
          rowVersion: report.row_version,
        };
      } else {
        const stored = await db.cache.get(AUTOSAVE_KEY);
        initial = (stored?.value as Form | undefined) ?? blankForm(presetProject);
      }
      if (!cancelled) setForm(initial);
    })().catch((err: unknown) => setError(err));
    return () => {
      cancelled = true;
    };
  }, [editId, presetProject]);

  useEffect(() => {
    if (form && !editId) void db.cache.put({ key: AUTOSAVE_KEY, value: form, at: Date.now() });
  }, [form, editId]);

  if (!form) return error ? <ErrorBanner error={error} /> : <Loading />;
  const update = (patch: Partial<Form>) => {
    setForm({ ...form, ...patch });
    setSaved(false);
  };

  async function addPhotos(event: ChangeEvent<HTMLInputElement>) {
    if (!form) return;
    setError(null);
    try {
      for (const file of Array.from(event.target.files ?? [])) {
        const blob = await compressPhoto(file);
        const id = crypto.randomUUID();
        await db.blobs.put({ id, blob, mime: "image/jpeg", name: file.name });
        setPhotos((current) => [...current, { id, name: file.name, size: blob.size, url: URL.createObjectURL(blob) }]);
      }
    } catch (err) {
      setError(err);
    }
    event.target.value = "";
  }

  function validate(f: Form): boolean {
    const problems: Record<string, string> = {};
    if (!f.projectId) problems.projectId = "Choisissez un chantier.";
    if (!f.reportDate) problems.reportDate = "La date est obligatoire.";
    if (f.workDone.trim().length < 2) problems.workDone = "Décrivez les travaux réalisés.";
    if (f.workforceCount !== "" && (!Number.isInteger(Number(f.workforceCount)) || Number(f.workforceCount) < 0)) {
      problems.workforceCount = "Indiquez un nombre entier positif.";
    }
    setFields(problems);
    return Object.keys(problems).length === 0;
  }

  /** Enregistre puis met en file : la saisie ne dépend jamais de l'état du réseau. */
  async function save(submit: boolean) {
    if (!form || !validate(form)) return;
    setBusy(true);
    setError(null);
    try {
      await db.reportDrafts.put({
        id: form.id,
        projectId: form.projectId,
        reportDate: form.reportDate,
        teamLabel: form.teamLabel,
        workDone: form.workDone,
        workforceCount: form.workforceCount === "" ? null : Number(form.workforceCount),
        issues: form.issues,
        weather: form.weather || null,
        rowVersion: form.rowVersion,
        updatedAt: Date.now(),
      });
      const label = `Rapport du ${date(form.reportDate)}`;
      await enqueue({
        kind: "report.save",
        label,
        entityId: form.id,
        method: "PUT",
        path: `/api/v1/daily-reports/${form.id}`,
        usesReportVersion: true,
        body: {
          project_id: form.projectId,
          report_date: form.reportDate,
          team_label: form.teamLabel,
          work_done: form.workDone,
          workforce_count: form.workforceCount === "" ? null : Number(form.workforceCount),
          issues: form.issues || null,
          weather: form.weather || null,
          tasks: form.doneTaskIds.map((id) => ({ task_id: id, done_that_day: true })),
        },
      });
      for (const photo of photos) {
        await enqueue({
          kind: "photo.upload",
          label: `Photo · ${photo.name}`,
          entityId: form.id,
          method: "POST",
          path: "/api/v1/attachments/presign",
          blobId: photo.id,
          body: { id: crypto.randomUUID(), daily_report_id: form.id, kind: "site_photo", mime_type: "image/jpeg", size_bytes: photo.size, original_name: photo.name },
        });
      }
      if (submit) {
        await enqueue({ kind: "report.submit", label: `Soumission · ${label}`, entityId: form.id, method: "POST", path: `/api/v1/daily-reports/${form.id}/submit`, body: {} });
      }
      setPhotos([]);
      if (navigator.onLine) await syncNow();
      if (submit) {
        await db.cache.delete(AUTOSAVE_KEY);
        const waiting = await db.outbox.where("state").anyOf("pending", "failed", "conflict").count();
        router.push(waiting > 0 ? "/ajouter" : "/");
        return;
      }
      setSaved(true);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  /** Une tâche créée ici est cochée d'office : elle décrit ce qui vient d'être fait. */
  async function addTask() {
    if (!form || !newTask || newTask.trim().length < 2) return;
    setError(null);
    try {
      const id = crypto.randomUUID();
      await enqueue({ kind: "task.create", label: `Tâche · ${newTask.trim()}`, entityId: id, method: "POST", path: `/api/v1/projects/${form.projectId}/tasks`, body: { id, title: newTask.trim(), priority: "medium", due_date: form.reportDate, assignee_ids: [] } });
      setNewTask(null);
      if (navigator.onLine) await syncNow();
      tasks.reload();
    } catch (err) {
      setError(err);
    }
  }

  const list = projects.data?.items.filter((p) => p.status !== "archived") ?? [];
  const projectId = form.projectId || list[0]?.id || "";
  if (projectId && !form.projectId) setForm({ ...form, projectId });
  const openTasks = tasks.data?.items.filter((t) => t.status !== "cancelled" && t.status !== "done") ?? [];
  const count = form.workforceCount === "" ? 0 : Number(form.workforceCount) || 0;
  const err = (key: string) =>
    fields[key] ? (
      <span role="alert" className="field-err">
        {fields[key]}
      </span>
    ) : null;

  return (
    <main style={{ maxWidth: 560, margin: "0 auto", minHeight: "100dvh", display: "flex", flexDirection: "column" }}>
      <header style={{ display: "flex", alignItems: "center", gap: 8, padding: "12px 8px", background: "#FFFFFF", borderBottom: "1px solid #E5E5EA" }}>
        <Link href="/" aria-label="Retour" style={{ width: 44, height: 44, display: "flex", alignItems: "center", justifyContent: "center", color: "#1D1D1F" }}>
          <IconBack />
        </Link>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700, flex: 1 }}>Rapport du jour</h1>
        {!online ? (
          <span style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 10px", marginRight: 8, borderRadius: 999, background: "#F2F2F7", fontSize: 12, fontWeight: 600, color: "#48484A" }}>
            <IconOffline size={14} />
            Hors ligne
          </span>
        ) : (
          (saved || form.workDone.trim() !== "") && (
            <span role="status" style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 10px", marginRight: 8, borderRadius: 999, background: "#E8F5EC", fontSize: 12, fontWeight: 600, color: "#1A7F45" }}>
              <span style={{ width: 7, height: 7, borderRadius: "50%", background: "#1A8F4C" }} />
              Brouillon enregistré
            </span>
          )
        )}
      </header>
      <form
        className="m-form"
        noValidate
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          void save(true);
        }}
        style={{ flex: 1, padding: 16, display: "flex", flexDirection: "column", gap: 20 }}
      >
        <div style={{ display: "grid", gridTemplateColumns: "1.4fr 1fr", gap: 10 }}>
          <div className="f" style={{ minWidth: 0 }}>
            <label htmlFor="ch">
              Chantier <span className="req">*</span>
            </label>
            <select id="ch" className="inp" value={form.projectId} onChange={(e) => update({ projectId: e.target.value, doneTaskIds: [] })} aria-invalid={fields.projectId ? true : undefined}>
              {list.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
            {err("projectId")}
          </div>
          <div className="f" style={{ minWidth: 0 }}>
            <label htmlFor="dt">
              Date <span className="req">*</span>
            </label>
            <input id="dt" type="date" className="inp" value={form.reportDate} max={today()} onChange={(e) => update({ reportDate: e.target.value })} aria-invalid={fields.reportDate ? true : undefined} />
            {err("reportDate")}
          </div>
        </div>
        <div className="f">
          <label htmlFor="tr">
            Travaux réalisés <span className="req">*</span>
          </label>
          <textarea id="tr" className="inp" value={form.workDone} onChange={(e) => update({ workDone: e.target.value })} aria-invalid={fields.workDone ? true : undefined} />
          {err("workDone")}
        </div>
        <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
          <legend className="lg" style={{ marginBottom: 8 }}>
            Tâches du jour
          </legend>
          {openTasks.length > 0 ? (
            <div className="box">
              {openTasks.map((t) => {
                const done = form.doneTaskIds.includes(t.id);
                return (
                  <label key={t.id} className="row-m">
                    <input type="checkbox" checked={done} onChange={(e) => update({ doneTaskIds: e.target.checked ? [...form.doneTaskIds, t.id] : form.doneTaskIds.filter((id) => id !== t.id) })} style={{ width: 22, height: 22, accentColor: "#1A8F4C", flex: "none" }} />
                    <span style={{ flex: 1, minWidth: 0 }}>{t.title}</span>
                    <span style={{ fontSize: 12, fontWeight: 600, color: done ? "#1A7F45" : "#9A5200", whiteSpace: "nowrap" }}>{done ? "Terminée" : "Non terminée"}</span>
                  </label>
                );
              })}
            </div>
          ) : (
            <p className="hint" style={{ margin: 0 }}>
              Aucune tâche en cours sur ce chantier.
            </p>
          )}
          {can.manageTasks(user.role) &&
            (newTask === null ? (
              <button type="button" onClick={() => setNewTask("")} style={{ marginTop: 8, height: 44, border: 0, background: "transparent", color: "#2B59E6", fontWeight: 600, fontSize: 15, padding: 0, cursor: "pointer" }}>
                + Ajouter une tâche
              </button>
            ) : (
              <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
                <input aria-label="Titre de la tâche" className="inp" style={{ flex: 1, minWidth: 0 }} value={newTask} onChange={(e) => setNewTask(e.target.value)} />
                <button type="button" className="mbtn" style={{ padding: "0 14px", background: "#2B59E6", color: "#FFFFFF" }} onClick={addTask}>
                  Ajouter
                </button>
              </div>
            ))}
        </fieldset>
        <div className="f">
          <label htmlFor="nb">Nombre d&apos;ouvriers présents</label>
          <div style={{ display: "flex", alignItems: "center", width: 180, border: "1px solid #D2D2D7", borderRadius: 12, background: "#FFFFFF", overflow: "hidden" }}>
            <button type="button" aria-label="Retirer un ouvrier" onClick={() => update({ workforceCount: String(Math.max(0, count - 1)) })} style={{ width: 56, height: 50, border: 0, background: "#F5F5F7", fontSize: 22, fontWeight: 600, cursor: "pointer" }}>
              −
            </button>
            <input id="nb" inputMode="numeric" value={form.workforceCount} onChange={(e) => update({ workforceCount: e.target.value.replace(/\D/g, "") })} style={{ width: 68, height: 50, border: 0, textAlign: "center", fontSize: 20, fontWeight: 700, outline: "none" }} />
            <button type="button" aria-label="Ajouter un ouvrier" onClick={() => update({ workforceCount: String(count + 1) })} style={{ width: 56, height: 50, border: 0, background: "#F5F5F7", fontSize: 22, fontWeight: 600, cursor: "pointer" }}>
              +
            </button>
          </div>
          <span style={{ fontSize: 13, color: "#6E6E73" }}>Indicatif, sans lien avec la paie.</span>
          {err("workforceCount")}
        </div>
        <div className="f">
          <label htmlFor="ob">Observations et difficultés</label>
          <textarea id="ob" className="inp" value={form.issues} onChange={(e) => update({ issues: e.target.value })} />
        </div>
        <div>
          <div className="lg" style={{ marginBottom: 8 }}>
            Photos du chantier <span style={{ fontWeight: 500, color: "#6E6E73" }}>({photos.length})</span>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(3, minmax(0, 1fr))", gap: 8 }}>
            {photos.map((p, i) => (
              <div key={p.id} role="img" aria-label={`Photo ${i + 1} : ${p.name}`} className="ph">
                {/* eslint-disable-next-line @next/next/no-img-element -- aperçu local d'un Blob */}
                <img src={p.url} alt="" />
              </div>
            ))}
            <button type="button" aria-label="Prendre une photo" onClick={() => fileInput.current?.click()} style={{ aspectRatio: 1, borderRadius: 12, border: "1.5px dashed #2B59E6", background: "#F2F5FE", color: "#2B59E6", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 4, fontSize: 12, fontWeight: 600, cursor: "pointer" }}>
              <IconCamera size={22} />
              Photo
            </button>
          </div>
          <input ref={fileInput} type="file" accept="image/*" capture="environment" multiple onChange={addPhotos} aria-label="Photos du chantier" style={{ display: "none" }} />
          <p style={{ margin: "8px 0 0", fontSize: 13, color: "#6E6E73" }}>Photos compressées avant envoi. La position GPS est retirée.</p>
        </div>
        <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
          <legend className="lg" style={{ marginBottom: 8 }}>
            Météo <span style={{ fontWeight: 500, color: "#6E6E73" }}>(facultatif)</span>
          </legend>
          <div className="chips">
            {Object.entries(WEATHER_LABEL).map(([value, label]) => (
              <button key={value} type="button" className={`chip ${form.weather === value ? "on" : ""}`} aria-pressed={form.weather === value} onClick={() => update({ weather: form.weather === value ? "" : value })}>
                {label}
              </button>
            ))}
          </div>
        </fieldset>
        <ErrorBanner error={error} />
        <div style={{ marginTop: "auto", display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 10, paddingTop: 8 }}>
          <button type="button" className="mbtn" disabled={busy} onClick={() => save(false)} style={{ borderColor: "#D2D2D7", background: "#FFFFFF", color: "#1D1D1F" }}>
            Brouillon
          </button>
          <button type="submit" className="mbtn" disabled={busy} style={{ background: "#2B59E6", color: "#FFFFFF" }}>
            Publier le rapport
          </button>
        </div>
      </form>
    </main>
  );
}
