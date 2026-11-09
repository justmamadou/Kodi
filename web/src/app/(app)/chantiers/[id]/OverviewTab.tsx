"use client";

import Link from "next/link";
import { IconWarn } from "@/components/icons";
import { Badge, Card, Empty, ErrorBanner, EXPENSE_TONE } from "@/components/ui";
import type { BudgetVersion, DailyReport, Expense, HistoryEntry, Items, Paged, Project, ProjectDetail } from "@/lib/api";
import { date, dayMonth, EXPENSE_STATUS_LABEL, fcfa, pct, shortName, WEATHER_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";
import { historyDetail, historyLabel } from "./HistoryTab";

export function OverviewTab({ project, onTab }: { project: Project; onTab: (tab: string) => void }) {
  const { user } = useSession();
  const finance = can.viewFinance(user.role);
  return (
    <>
      {finance && <Situation project={project} />}
      <div style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
        {finance && <BudgetByCategory projectId={project.id} />}
        <div style={{ flex: "1 1 320px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
          <LatestExpenses projectId={project.id} onTab={onTab} />
          <LatestReports projectId={project.id} onTab={onTab} />
          {finance && <LatestHistory projectId={project.id} />}
        </div>
      </div>
    </>
  );
}

function Kpi({ label, value, note, swatch }: { label: string; value: number; note: string; swatch?: string }) {
  return (
    <div>
      <div className="kl" style={{ display: "flex", alignItems: "center", gap: 6 }}>
        {swatch && <span style={{ width: 10, height: 10, borderRadius: 2, background: swatch }} />}
        {label}
      </div>
      <div className="kv">{fcfa(value)}</div>
      <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
        {note}
      </div>
    </div>
  );
}

function Situation({ project }: { project: Project }) {
  const { data, error } = useResource<ProjectDetail>(`/projects/${project.id}/financials`);
  const versions = useResource<Items<BudgetVersion>>(`/projects/${project.id}/budget/versions`);
  if (!data) return error ? <ErrorBanner error={error} /> : null;
  if (data.budget === 0) {
    return (
      <Card style={{ padding: 22 }}>
        <h2 className="sec-t">Situation financière</h2>
        <Empty>Aucun budget publié pour ce chantier. Publiez-en un dans l&apos;onglet Budget pour suivre les écarts.</Empty>
      </Card>
    );
  }
  const published = versions.data?.items.filter((v) => v.published_at) ?? [];
  const initial = published[published.length - 1];
  const current = published[0];
  const budgetNote = initial && current && initial.id !== current.id ? `Initial ${fcfa(initial.total_amount)} · ${current.total_amount >= initial.total_amount ? "+" : ""}${fcfa(current.total_amount - initial.total_amount)} le ${dayMonth(current.published_at)}` : `Version ${data.budget_version_no ?? 1}`;
  const paidPct = pct(data.paid, data.budget);
  const unpaidPct = pct(data.unpaid, data.budget);
  const exposedPct = pct(data.approved, data.budget);
  return (
    <section className="card" style={{ padding: 22 }} aria-labelledby="h-fin">
      <h2 id="h-fin" className="sec-t" style={{ marginBottom: 16 }}>
        Situation financière
      </h2>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(170px, 1fr))", gap: 20 }}>
        <Kpi label="Budget révisé" value={data.budget} note={budgetNote} />
        <Kpi label="Dépenses payées" value={data.paid} note={`${paidPct} % du budget`} swatch="#1D1D1F" />
        <Kpi label="Engagements ouverts" value={data.unpaid} note="Approuvé, pas encore réglé" swatch="#C76A00" />
        <Kpi label="Disponible prévisionnel" value={data.remaining} note={`${pct(data.remaining, data.budget)} % du budget`} swatch="#D2D2D7" />
        <Kpi label="En attente de validation" value={data.pending} note="Non compté dans les payées" />
      </div>
      <div role="img" aria-label={`Budget : ${paidPct} % payé, ${unpaidPct} % engagé, ${Math.max(0, 100 - exposedPct)} % disponible`} style={{ display: "flex", height: 14, borderRadius: 7, overflow: "hidden", marginTop: 20, background: "#D2D2D7", gap: 2 }}>
        <div style={{ width: `${Math.min(100, paidPct)}%`, background: "#1D1D1F" }} />
        <div style={{ width: `${Math.min(100, unpaidPct)}%`, background: "#C76A00" }} />
      </div>
      <div className="muted num" style={{ display: "flex", justifyContent: "space-between", gap: 8, marginTop: 6, fontSize: 12, whiteSpace: "normal" }}>
        <span>0</span>
        <span>
          Exposition {fcfa(data.approved)} FCFA · {exposedPct} %
        </span>
        <span>{fcfa(data.budget)}</span>
      </div>
      {(data.signal === "watch" || data.signal === "critical") && (
        <div className={data.signal === "critical" ? "err" : "warn"} style={{ display: "flex", gap: 12, alignItems: "flex-start", marginTop: 18, padding: "14px 16px", borderRadius: 10 }}>
          <IconWarn size={20} />
          <div style={{ fontSize: 14, color: "#1D1D1F" }}>
            <strong>Écart à examiner :</strong> {exposedPct} % du budget est exposé pour {data.declared_progress_pct} % d&apos;avancement déclaré
            {project.progress_updated_at ? ` (le ${dayMonth(project.progress_updated_at)})` : ""}. Ce signal invite à vérifier, il ne conclut pas à une anomalie.{" "}
            <a href="#h-bud" style={{ fontWeight: 600 }}>
              Voir le détail par catégorie
            </a>
          </div>
        </div>
      )}
    </section>
  );
}

function BudgetByCategory({ projectId }: { projectId: string }) {
  const { data } = useResource<ProjectDetail>(`/projects/${projectId}/financials`);
  const versions = useResource<Items<BudgetVersion>>(`/projects/${projectId}/budget/versions`);
  if (!data || data.categories.length === 0) return null;
  const current = versions.data?.items.find((v) => v.status === "published");
  return (
    <section className="card" style={{ flex: "999 1 560px", minWidth: 0 }} aria-labelledby="h-bud">
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 12, padding: "18px 16px 6px", flexWrap: "wrap" }}>
        <h2 id="h-bud" className="sec-t">
          Budget par catégorie
        </h2>
        <span className="muted" style={{ fontSize: 13 }}>
          En FCFA{current ? ` · version ${current.version_no} du ${date(current.published_at)}` : ""}
        </span>
      </div>
      <div style={{ overflowX: "auto" }}>
        <table className="tbl">
          <thead>
            <tr>
              <th>Catégorie</th>
              <th className="text-right">Prévu</th>
              <th className="text-right">Payé</th>
              <th className="text-right">Engagé</th>
              <th className="text-right">Disponible</th>
              <th className="text-right" style={{ minWidth: 120 }}>
                Consommé
              </th>
            </tr>
          </thead>
          <tbody>
            {data.categories.map((c) => (
              <tr key={c.category_id}>
                <td style={{ fontWeight: 600 }}>{c.name}</td>
                <td className="text-right num">{fcfa(c.budget)}</td>
                <td className="text-right num">{fcfa(c.paid)}</td>
                <td className="text-right num">{fcfa(c.unpaid)}</td>
                <td className="text-right num" style={c.remaining < 0 ? { color: "#B3261E", fontWeight: 600 } : undefined}>
                  {fcfa(c.remaining)}
                </td>
                <td>
                  <div style={{ height: 6, borderRadius: 3, background: "#F2F2F7" }} aria-label={`${pct(c.approved, c.budget)} % consommé`} role="img">
                    <div style={{ width: `${Math.min(100, pct(c.approved, c.budget))}%`, height: 6, borderRadius: 3, background: c.remaining < 0 ? "#D93025" : "#2B59E6" }} />
                  </div>
                </td>
              </tr>
            ))}
            <tr>
              <td style={{ fontWeight: 700 }}>Total</td>
              <td className="text-right num" style={{ fontWeight: 700 }}>
                {fcfa(data.budget)}
              </td>
              <td className="text-right num" style={{ fontWeight: 700 }}>
                {fcfa(data.paid)}
              </td>
              <td className="text-right num" style={{ fontWeight: 700 }}>
                {fcfa(data.unpaid)}
              </td>
              <td className="text-right num" style={{ fontWeight: 700 }}>
                {fcfa(data.remaining)}
              </td>
              <td />
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  );
}

function SectionHead({ id, title, onMore }: { id: string; title: string; onMore: () => void }) {
  return (
    <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
      <h2 id={id} className="sec-t">
        {title}
      </h2>
      <button type="button" className="link-btn" onClick={onMore}>
        Tout voir
      </button>
    </div>
  );
}

export function expenseBadge(e: Expense) {
  if (e.status === "approved" && e.payment_status === "paid") return <Badge tone="green">Payée</Badge>;
  return <Badge tone={EXPENSE_TONE[e.status]}>{EXPENSE_STATUS_LABEL[e.status]}</Badge>;
}

function LatestExpenses({ projectId, onTab }: { projectId: string; onTab: (tab: string) => void }) {
  const { data } = useResource<Paged<Expense>>(`/expenses?project_id=${projectId}&limit=4`);
  return (
    <section className="card" style={{ padding: "18px 20px" }} aria-labelledby="h-dep">
      <SectionHead id="h-dep" title="Dernières dépenses" onMore={() => onTab("expenses")} />
      {data?.items.length === 0 && <Empty>Aucune dépense pour le moment.</Empty>}
      {data?.items.map((e) => (
        <Link key={e.id} href={`/depenses/${e.id}`} className="li" style={{ color: "#1D1D1F" }}>
          <span style={{ minWidth: 0 }}>
            {e.description}
            <span className="muted" style={{ display: "block", fontSize: 12 }}>
              {dayMonth(e.expense_date)} · {shortName(e.created_by_name)}
            </span>
          </span>
          <span style={{ textAlign: "right", flex: "none" }}>
            <span className="num" style={{ fontWeight: 600, display: "block", marginBottom: 4 }}>
              {fcfa(e.amount)}
            </span>
            {expenseBadge(e)}
          </span>
        </Link>
      ))}
    </section>
  );
}

function LatestReports({ projectId, onTab }: { projectId: string; onTab: (tab: string) => void }) {
  const { data } = useResource<Items<DailyReport>>(`/projects/${projectId}/daily-reports`);
  const reports = data?.items.slice(0, 2);
  return (
    <section className="card" style={{ padding: "18px 20px" }} aria-labelledby="h-rap">
      <SectionHead id="h-rap" title="Rapports journaliers" onMore={() => onTab("reports")} />
      {reports?.length === 0 && <Empty>Aucun rapport pour le moment.</Empty>}
      {reports?.map((r) => (
        <Link key={r.id} href={`/rapports/${r.id}`} className="li" style={{ color: "#1D1D1F" }}>
          <span style={{ minWidth: 0 }}>
            {date(r.report_date)} · {r.team_label || r.work_done.slice(0, 40)}
            <span className="muted" style={{ display: "block", fontSize: 12 }}>
              {[r.workforce_count !== null ? `${r.workforce_count} ouvriers` : null, r.weather ? WEATHER_LABEL[r.weather] : null].filter(Boolean).join(" · ")}
            </span>
          </span>
          <Badge tone={r.status === "submitted" ? "green" : "orange"}>{r.status === "submitted" ? "Publié" : "Brouillon"}</Badge>
        </Link>
      ))}
    </section>
  );
}

function LatestHistory({ projectId }: { projectId: string }) {
  const { data } = useResource<Items<HistoryEntry>>(`/projects/${projectId}/history?limit=3`);
  return (
    <section className="card" style={{ padding: "18px 20px" }} aria-labelledby="h-his">
      <h2 id="h-his" className="sec-t" style={{ marginBottom: 6 }}>
        Historique
      </h2>
      {data?.items.length === 0 && <Empty>Aucune action enregistrée.</Empty>}
      {data?.items.map((h) => (
        <div key={h.id} className="li" style={{ display: "block" }}>
          <strong>{historyLabel(h)}</strong>
          {historyDetail(h) ? ` : ${historyDetail(h)}` : ""}
          <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
            {shortName(h.actor_name)} · {date(h.created_at)}
          </div>
        </div>
      ))}
    </section>
  );
}
