"use client";

import { Card, Empty, ErrorBanner, Loading } from "@/components/ui";
import type { HistoryEntry, Items } from "@/lib/api";
import { dateTime, fcfa, PROJECT_STATUS_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";

const LABELS: Record<string, string> = {
  "project.created": "Chantier créé",
  "project.updated": "Chantier modifié",
  "project.status_changed": "Statut modifié",
  "project.progress_updated": "Avancement déclaré",
  "project.member_assigned": "Membre affecté",
  "project.member_removed": "Membre retiré",
  "budget.published": "Budget publié",
  "budget.revision_created": "Révision du budget ouverte",
  "budget.discarded": "Brouillon de budget abandonné",
  "expense.created": "Dépense saisie",
  "expense.submitted": "Dépense soumise",
  "expense.approved": "Dépense approuvée",
  "expense.rejected": "Dépense rejetée",
  "expense.cancelled": "Dépense annulée",
  "expense.back_to_draft": "Dépense repassée en brouillon",
  "payment.recorded": "Paiement enregistré",
  "stock.posted": "Mouvement de stock enregistré",
  "report.submitted": "Rapport publié",
  "report.reopened": "Rapport rouvert",
  "attachment.uploaded": "Document ajouté",
};

const ENTITY: Record<string, string> = { project: "Chantier", budget_version: "Budget", expense: "Dépense", payment: "Paiement", project_member: "Équipe", stock_movement: "Stock", task: "Tâche", daily_report: "Rapport" };

export function historyLabel(h: HistoryEntry): string {
  return LABELS[h.action] ?? ENTITY[h.entity_type] ?? "Action";
}

/** Une ligne lisible à partir des valeurs auditées, uniquement quand elles sont présentes. */
export function historyDetail(h: HistoryEntry): string | null {
  const after = h.after ?? {};
  const before = h.before ?? {};
  const parts: string[] = [];
  if (typeof after.description === "string") parts.push(after.description);
  if (typeof after.amount === "number") parts.push(`${fcfa(after.amount)} FCFA`);
  if (typeof after.total_amount === "number") parts.push(`${typeof before.total_amount === "number" ? `${fcfa(before.total_amount)} → ` : ""}${fcfa(after.total_amount)} FCFA`);
  if (typeof after.status === "string" && h.entity_type === "project") parts.push(`${typeof before.status === "string" ? `${PROJECT_STATUS_LABEL[before.status]} → ` : ""}${PROJECT_STATUS_LABEL[after.status] ?? after.status}`);
  if (typeof after.declared_progress_pct === "number") parts.push(`${after.declared_progress_pct} %`);
  if (typeof after.decision_comment === "string") parts.push(`Motif : ${after.decision_comment}`);
  return parts.length > 0 ? parts.join(" · ") : null;
}

export function HistoryTab({ projectId }: { projectId: string }) {
  const { data, error, loading } = useResource<Items<HistoryEntry>>(`/projects/${projectId}/history`);
  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  return (
    <Card style={{ padding: "18px 20px" }}>
      <h2 className="sec-t" style={{ marginBottom: 6 }}>
        Historique
      </h2>
      {data.items.length === 0 ? (
        <Empty>Aucune action enregistrée.</Empty>
      ) : (
        data.items.map((h) => (
          <div key={h.id} className="li" style={{ display: "block" }}>
            <strong>{historyLabel(h)}</strong>
            {historyDetail(h) ? ` : ${historyDetail(h)}` : ""}
            <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
              {h.actor_name ?? "Système"} · {dateTime(h.created_at)}
            </div>
          </div>
        ))
      )}
    </Card>
  );
}
