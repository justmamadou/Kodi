"use client";

import Link from "next/link";
import { useState } from "react";
import { IconDownload, IconDocLines } from "@/components/icons";
import { Badge, Empty, ErrorBanner, Loading, type Tone } from "@/components/ui";
import type { Overview, Paged, Project, ProjectHealth } from "@/lib/api";
import { date, dayMonth, fcfa, fcfaShort, pct, qty, shortName, today, UNIT_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

type Tab = "summary" | "finance" | "reports" | "stock";

function signal(p: ProjectHealth): { label: string; tone: Tone } {
  if (p.signal === "critical") return { label: p.threshold_reached ? `Seuil ${p.threshold_reached} % dépassé` : "Dépense sans budget", tone: "red" };
  if (p.signal === "watch") return { label: "Écart à examiner", tone: "orange" };
  if (p.signal === "good") return { label: "Conforme", tone: "green" };
  return { label: "Sans budget publié", tone: "neutral" };
}

/** Numéro de semaine ISO. */
function isoWeek(day: string): number {
  const d = new Date(`${day}T00:00:00Z`);
  const thursday = new Date(d);
  thursday.setUTCDate(d.getUTCDate() + 3 - ((d.getUTCDay() + 6) % 7));
  const firstThursday = new Date(Date.UTC(thursday.getUTCFullYear(), 0, 4));
  return 1 + Math.round(((thursday.getTime() - firstThursday.getTime()) / 86_400_000 - 3 + ((firstThursday.getUTCDay() + 6) % 7)) / 7);
}

export default function ReportsPage() {
  const { user } = useSession();
  const [tab, setTab] = useState<Tab>("summary");
  const [projectId, setProjectId] = useState("");
  const { data, error, loading } = useResource<Overview>(`/reports/overview${projectId ? `?project_id=${projectId}` : ""}`);
  const projects = useResource<Paged<Project>>("/projects?limit=100");
  const active = projects.data?.items.filter((p) => p.status === "in_progress") ?? [];

  const tabs: [Tab, string][] = [
    ["summary", "Synthèse"],
    ["finance", "Financier"],
    ["reports", "Rapports journaliers"],
    ["stock", "Matériaux"],
  ];

  return (
    <>
      <div className="wrap" style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h1 className="page-h1">Rapports</h1>
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
            Synthèse financière et opérationnelle · {active.length} chantier{active.length > 1 ? "s" : ""} actif{active.length > 1 ? "s" : ""}
          </p>
        </div>
        {can.exportData(user.role) && (
          <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
            <a href={`/api/v1/exports/expenses.xlsx${projectId ? `?project_id=${projectId}` : ""}`} className="btn btn-s">
              <IconDownload />
              XLSX
            </a>
            <a href={projectId ? `/api/v1/exports/projects/${projectId}/report.pdf` : "/api/v1/exports/portfolio.pdf"} className="btn btn-p">
              <IconDocLines />
              Générer le PDF
            </a>
          </div>
        )}
      </div>
      <div className="wrap" style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12 }}>
        <nav aria-label="Type de rapport" style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
          {tabs.map(([id, label]) => (
            <button key={id} type="button" className={`tab ${tab === id ? "on" : ""}`} aria-pressed={tab === id} onClick={() => setTab(id)}>
              {label}
            </button>
          ))}
        </nav>
        <div style={{ marginLeft: "auto", display: "flex", gap: 10, flexWrap: "wrap", maxWidth: "100%" }}>
          <select className="sel" aria-label="Chantiers" value={projectId} onChange={(e) => setProjectId(e.target.value)}>
            <option value="">Tous les chantiers actifs</option>
            {projects.data?.items.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </div>
      </div>
      {loading ? <Loading /> : !data ? <ErrorBanner error={error} /> : <Content tab={tab} data={data} />}
    </>
  );
}

function Content({ tab, data }: { tab: Tab; data: Overview }) {
  return (
    <>
      {(tab === "summary" || tab === "finance") && <Comparison data={data} />}
      {tab === "summary" && (
        <div className="wrap" style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
          <Categories data={data} />
          <Week data={data} />
        </div>
      )}
      {tab === "finance" && (
        <div className="wrap">
          <Categories data={data} />
        </div>
      )}
      {tab === "reports" && (
        <div className="wrap" style={{ display: "flex" }}>
          <Week data={data} />
        </div>
      )}
      {(tab === "summary" || tab === "reports") && <Latest data={data} />}
      {tab === "stock" && <Stock data={data} />}
    </>
  );
}

function Comparison({ data }: { data: Overview }) {
  const { totals } = data;
  return (
    <section className="card wrap" aria-labelledby="h-comp">
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 12, padding: "18px 16px 6px", flexWrap: "wrap" }}>
        <h2 id="h-comp" className="sec-t">
          Comparatif par chantier
        </h2>
        <span className="muted" style={{ fontSize: 13 }}>
          En FCFA · au {date(today())}
        </span>
      </div>
      {data.projects.length === 0 ? (
        <Empty>Aucun chantier sur cette sélection.</Empty>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table className="tbl">
            <thead>
              <tr>
                <th>Chantier</th>
                <th className="text-right">Budget révisé</th>
                <th className="text-right hide-sm">Payé</th>
                <th className="text-right hide-sm">Engagé</th>
                <th className="text-right hide-sm">Disponible</th>
                <th className="text-right">Exposé</th>
                <th className="text-right hide-sm">Avancement</th>
                <th className="hide-sm">Signal</th>
              </tr>
            </thead>
            <tbody>
              {data.projects.map((p) => {
                const s = signal(p);
                return (
                  <tr key={p.project_id}>
                    <td>
                      <Link href={`/chantiers/${p.project_id}`} style={{ fontWeight: 600, color: "#1D1D1F" }}>
                        {p.name}
                      </Link>
                    </td>
                    <td className="text-right num">{fcfa(p.budget)}</td>
                    <td className="text-right num hide-sm">{fcfa(p.paid)}</td>
                    <td className="text-right num hide-sm">{fcfa(p.unpaid)}</td>
                    <td className="text-right num hide-sm">{fcfa(p.remaining)}</td>
                    <td className="text-right num" style={{ fontWeight: 600 }}>
                      {pct(p.approved, p.budget)} %
                    </td>
                    <td className="text-right num hide-sm">{p.declared_progress_pct} %</td>
                    <td className="hide-sm">
                      <Badge tone={s.tone}>{s.label}</Badge>
                    </td>
                  </tr>
                );
              })}
              <tr>
                <td style={{ fontWeight: 700 }}>Total</td>
                <td className="text-right num" style={{ fontWeight: 700 }}>
                  {fcfa(totals.budget)}
                </td>
                <td className="text-right num hide-sm" style={{ fontWeight: 700 }}>
                  {fcfa(totals.paid)}
                </td>
                <td className="text-right num hide-sm" style={{ fontWeight: 700 }}>
                  {fcfa(totals.unpaid)}
                </td>
                <td className="text-right num hide-sm" style={{ fontWeight: 700 }}>
                  {fcfa(totals.remaining)}
                </td>
                <td className="text-right num" style={{ fontWeight: 700 }}>
                  {pct(totals.approved, totals.budget)} %
                </td>
                <td className="hide-sm" />
                <td className="hide-sm" />
              </tr>
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function Categories({ data }: { data: Overview }) {
  const total = data.paid_by_category.reduce((sum, c) => sum + c.paid, 0);
  const max = Math.max(1, ...data.paid_by_category.map((c) => c.paid));
  return (
    <section className="card" style={{ flex: "999 1 520px", minWidth: 0, padding: 20 }} aria-labelledby="h-cat">
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 12, flexWrap: "wrap" }}>
        <h2 id="h-cat" className="sec-t">
          Dépenses payées par catégorie
        </h2>
        <span className="muted" style={{ fontSize: 13 }}>
          {fcfa(total)} FCFA
        </span>
      </div>
      <div style={{ marginTop: 14 }}>
        {data.paid_by_category.length === 0 && <Empty>Aucun paiement enregistré.</Empty>}
        {data.paid_by_category.map((c) => (
          <div key={c.name} className="cat-bar">
            <span style={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis" }}>{c.name}</span>
            <div style={{ height: 14, borderRadius: 4, background: "#F2F2F7" }}>
              <div style={{ width: `${(c.paid * 100) / max}%`, height: 14, borderRadius: 4, background: "#2B59E6" }} />
            </div>
            <span className="num" style={{ fontWeight: 600, textAlign: "right" }}>
              {fcfaShort(c.paid)} · {pct(c.paid, total)} %
            </span>
          </div>
        ))}
      </div>
    </section>
  );
}

function Week({ data }: { data: Overview }) {
  const now = today();
  const elapsed = data.week_days.filter((d) => d <= now);
  const received = data.week_projects.reduce((sum, p) => sum + p.reported.filter((d) => data.week_days.includes(d)).length, 0);
  const expected = elapsed.length * data.week_projects.length;
  const first = data.week_days[0];
  return (
    <section className="card" style={{ flex: "1 1 380px", minWidth: 0, padding: 20 }} aria-labelledby="h-rj">
      <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 12 }}>
        <h2 id="h-rj" className="sec-t">
          Rapports journaliers
        </h2>
        <span className="muted" style={{ fontSize: 13 }}>
          {first ? `Semaine ${isoWeek(first)} · lun. → sam.` : ""}
        </span>
      </div>
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginTop: 12 }}>
        <span className="num" style={{ fontSize: 30, fontWeight: 700 }}>
          {received} / {expected}
        </span>
        <span className="muted" style={{ fontSize: 14 }}>
          rapports reçus
        </span>
      </div>
      <div style={{ marginTop: 14, display: "flex", flexDirection: "column", gap: 12, fontSize: 14 }}>
        {data.week_projects.length === 0 && <Empty>Aucun chantier en cours.</Empty>}
        {data.week_projects.map((p) => {
          const done = p.reported.filter((d) => data.week_days.includes(d)).length;
          return (
            <div key={p.project_id} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
              <span style={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis" }}>{p.name}</span>
              <span className="dots" role="img" aria-label={`${done} sur ${elapsed.length}`}>
                {data.week_days.map((d) => {
                  const ok = p.reported.includes(d);
                  // Un jour à venir, ou aujourd'hui sans rapport, n'est pas encore un rapport manquant.
                  const missing = !ok && d < now;
                  return <span key={d} className="d" style={{ background: ok ? "#1A8F4C" : "#E5E5EA", outline: missing ? "1px dashed #9A5200" : undefined }} />;
                })}
              </span>
            </div>
          );
        })}
      </div>
      <p className="muted" style={{ margin: "14px 0 0", fontSize: 12, display: "flex", gap: 14, flexWrap: "wrap" }}>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
          <span style={{ width: 10, height: 10, borderRadius: 2, background: "#1A8F4C" }} />
          Reçu
        </span>
        <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
          <span style={{ width: 10, height: 10, borderRadius: 2, background: "#E5E5EA", outline: "1px dashed #9A5200" }} />
          Manquant
        </span>
      </p>
    </section>
  );
}

function Latest({ data }: { data: Overview }) {
  return (
    <section className="card wrap" aria-labelledby="h-last">
      <div style={{ padding: "18px 16px 6px" }}>
        <h2 id="h-last" className="sec-t">
          Derniers rapports journaliers
        </h2>
      </div>
      {data.latest_reports.length === 0 ? (
        <Empty>Aucun rapport publié.</Empty>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table className="tbl">
            <thead>
              <tr>
                <th>Date</th>
                <th>Chantier</th>
                <th>Travaux réalisés</th>
                <th className="text-right hide-sm">Ouvriers</th>
                <th className="text-right hide-sm">Photos</th>
                <th className="hide-sm">Difficultés</th>
                <th className="hide-sm">Auteur</th>
              </tr>
            </thead>
            <tbody>
              {data.latest_reports.map((r) => (
                <tr key={r.id}>
                  <td className="num">{dayMonth(r.report_date)}</td>
                  <td>{r.project_name}</td>
                  <td>
                    <Link href={`/rapports/${r.id}`} style={{ color: "#1D1D1F" }}>
                      {r.work_done}
                    </Link>
                  </td>
                  <td className="text-right num hide-sm">{r.workforce_count ?? "—"}</td>
                  <td className="text-right num hide-sm">{r.photo_count}</td>
                  <td className="hide-sm">{r.issues ? <Badge tone="orange">{r.issues}</Badge> : <span className="muted">—</span>}</td>
                  <td className="hide-sm">{shortName(r.author_name)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}

function Stock({ data }: { data: Overview }) {
  return (
    <section className="card wrap" aria-labelledby="h-stock">
      <div style={{ padding: "18px 16px 6px" }}>
        <h2 id="h-stock" className="sec-t">
          Stock théorique par chantier
        </h2>
      </div>
      {data.stock.length === 0 ? (
        <Empty>Aucun mouvement de stock enregistré.</Empty>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table className="tbl">
            <thead>
              <tr>
                <th>Chantier</th>
                <th>Article</th>
                <th className="text-right">Stock théorique</th>
                <th>État</th>
              </tr>
            </thead>
            <tbody>
              {data.stock.map((s) => (
                <tr key={`${s.project_id}-${s.material_id}`}>
                  <td>{s.project_name}</td>
                  <td style={{ fontWeight: 600 }}>{s.name}</td>
                  <td className="text-right num" style={{ fontWeight: 700 }}>
                    {qty(s.on_hand)} {UNIT_LABEL[s.unit] ?? s.unit}
                  </td>
                  <td>{s.is_low ? <Badge tone="orange">Stock bas</Badge> : <Badge tone="green">Suffisant</Badge>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
