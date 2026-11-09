"use client";

import { useParams } from "next/navigation";
import { useState } from "react";
import { Badge, Button, Card, ErrorBanner, Input, LinkButton, Loading, PageTitle } from "@/components/ui";
import type { DailyReport } from "@/lib/api";
import { api } from "@/lib/api";
import { date, WEATHER_LABEL } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

interface Detail extends DailyReport {
  tasks: { task_id: string; title: string; done_that_day: boolean }[];
  attachments: { id: string; original_name: string | null }[];
}

export default function ReportPage() {
  const { id } = useParams<{ id: string }>();
  const { user } = useSession();
  const online = useOnline();
  const { data: r, error, loading, reload } = useResource<Detail>(`/daily-reports/${id}`);
  const [actionError, setActionError] = useState<unknown>(null);
  const [reason, setReason] = useState("");
  const [photos, setPhotos] = useState<Record<string, string>>({});

  if (loading) return <Loading />;
  if (!r) return <ErrorBanner error={error} />;

  async function run(fn: () => Promise<unknown>) {
    setActionError(null);
    try {
      await fn();
      reload();
    } catch (err) {
      setActionError(err);
    }
  }

  async function show(attachmentId: string) {
    try {
      const { url } = await api<{ url: string }>(`/attachments/${attachmentId}/url`);
      setPhotos((p) => ({ ...p, [attachmentId]: url }));
    } catch (err) {
      setActionError(err);
    }
  }

  const submitted = r.status === "submitted";
  return (
    <>
      <PageTitle
        title={`Rapport du ${date(r.report_date)}`}
        subtitle={r.team_label || undefined}
        action={<Badge tone={submitted ? "green" : "orange"}>{submitted ? "Soumis" : "Brouillon"}</Badge>}
      />
      <Card className="mb-4">
        <h2 className="text-sm font-medium text-muted">Travaux réalisés</h2>
        <p className="mt-1 whitespace-pre-wrap">{r.work_done}</p>
        {r.issues && (
          <>
            <h2 className="mt-3 text-sm font-medium text-muted">Observations et difficultés</h2>
            <p className="mt-1 whitespace-pre-wrap">{r.issues}</p>
          </>
        )}
        <p className="mt-3 text-sm text-muted">
          {r.workforce_count !== null ? `${r.workforce_count} ouvrier(s)` : "Effectif non renseigné"}
          {r.weather ? ` · ${WEATHER_LABEL[r.weather]}` : ""}
        </p>
      </Card>
      {r.tasks.length > 0 && (
        <Card className="mb-4">
          <h2 className="mb-2 text-lg font-bold">Tâches du jour</h2>
          <ul className="flex flex-col gap-1 text-sm">
            {r.tasks.map((t) => (
              <li key={t.task_id}>
                {t.done_that_day ? "✓" : "·"} {t.title}
              </li>
            ))}
          </ul>
        </Card>
      )}
      {r.attachments.length > 0 && (
        <Card className="mb-4">
          <h2 className="mb-2 text-lg font-bold">Photos</h2>
          <ul className="grid gap-3 sm:grid-cols-2">
            {r.attachments.map((a) => (
              <li key={a.id}>
                {photos[a.id] ? (
                  // eslint-disable-next-line @next/next/no-img-element -- URL pré-signée de courte durée, hors optimiseur d'images
                  <img src={photos[a.id]} alt={a.original_name ?? "Photo du chantier"} className="w-full rounded-xl" />
                ) : (
                  <Button variant="secondary" onClick={() => show(a.id)}>
                    Voir {a.original_name ?? "la photo"}
                  </Button>
                )}
              </li>
            ))}
          </ul>
        </Card>
      )}
      <ErrorBanner error={actionError} />
      {!submitted && can.writeReport(user.role) && (
        <div className="mt-3 flex flex-wrap gap-2">
          <LinkButton href={`/rapports/nouveau?id=${r.id}`} variant="secondary">
            Modifier
          </LinkButton>
          <Button disabled={!online} onClick={() => run(() => api(`/daily-reports/${id}/submit`, { method: "POST" }))}>
            Soumettre
          </Button>
        </div>
      )}
      {submitted && can.manageProjects(user.role) && (
        <Card className="p-6 wrap">
          <h2 className="mb-2 text-lg font-bold">Rouvrir le rapport</h2>
          <p className="mb-3 text-sm text-muted">Un rapport soumis est verrouillé. Sa réouverture est tracée dans le journal d&apos;audit.</p>
          <Input label="Motif" required value={reason} onChange={(e) => setReason(e.target.value)} />
          <Button className="mt-3" variant="secondary" disabled={reason.trim().length < 3 || !online} onClick={() => run(() => api(`/daily-reports/${id}/reopen`, { method: "POST", body: { reason } }))}>
            Rouvrir
          </Button>
        </Card>
      )}
    </>
  );
}
