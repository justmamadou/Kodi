"use client";

import Link from "next/link";
import { useState } from "react";
import { IconAlert, IconCamera, IconCheck, IconChevron, IconDoc, IconDown, IconBox, IconPlus } from "@/components/icons";
import { Empty, ErrorBanner, Loading, Ring } from "@/components/ui";
import type { Dashboard, DailyReport, Items, Paged, Project, ProjectHealth, StockView, Task } from "@/lib/api";
import { api, NetworkError } from "@/lib/api";
import { useCurrentProject } from "@/lib/current-project";
import { fcfa, fcfaShort, longDate, mediumDate, monthLong, monthShort, pct, today } from "@/lib/format";
import { useIsDesktop, useOnline, useResource } from "@/lib/hooks";
import { db } from "@/lib/offline/db";
import { enqueue } from "@/lib/offline/sync";
import { syncNow } from "@/lib/offline/useSync";
import { can, useSession } from "@/lib/session";
import { useLiveQuery } from "dexie-react-hooks";

const SIGNAL = {
  critical: { label: "Dépense trop vite", color: "#D93025", ink: "#B3261E" },
  watch: { label: "À surveiller", color: "#C76A00", ink: "#9A5200" },
  good: { label: "Bon rythme", color: "#1A8F4C", ink: "#1A7F45" },
  none: { label: "Sans budget publié", color: "#AEAEB2", ink: "#6E6E73" },
} as const;

export default function HomePage() {
  const { user } = useSession();
  const desktop = useIsDesktop();
  // Maquette 03 pour le pilotage, maquette 08 pour le terrain.
  const field = user.role === "site_worker" || user.role === "viewer" || (user.role === "manager" && !desktop);
  return field ? <FieldHome /> : <FinanceHome />;
}

function FinanceHome() {
  const { user } = useSession();
  const [period, setPeriod] = useState<"week" | "month" | "all">("all");
  const { data, error, stale, loading } = useResource<Dashboard>(`/dashboard?period=${period}`);
  const first = user.full_name.split(" ")[0];
  if (loading && !data) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  const { totals, to_process: proc } = data;
  const paidPct = pct(totals.paid, totals.budget);
  const orderedPct = pct(totals.unpaid, totals.budget);
  const atRisk = data.projects.filter((p) => p.signal === "critical");
  const taskCount = (proc.pending_count > 0 ? 1 : 0) + atRisk.length + (proc.missing_reports.length > 0 ? 1 : 0);
  const active = data.projects.filter((p) => p.status === "in_progress").length;

  return (
    <div style={{ maxWidth: 1120, width: "100%", margin: "0 auto", display: "flex", flexDirection: "column", gap: 24 }}>
      <header style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <div className="s2" style={{ fontSize: 15, fontWeight: 500 }}>
            {stale ? "Dernières données connues : pas de réseau" : longDate()}
          </div>
          <h1 className="home-h1" style={{ margin: "2px 0 0", fontSize: 44, fontWeight: 700, letterSpacing: "-0.035em", lineHeight: 1.1 }}>
            Bonjour, {first}.
          </h1>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <div className="seg" role="group" aria-label="Période des paiements">
            {(
              [
                ["week", "Semaine"],
                ["month", "Mois"],
                ["all", "Tout"],
              ] as const
            ).map(([value, label]) => (
              <button key={value} type="button" className={period === value ? "on" : ""} aria-pressed={period === value} onClick={() => setPeriod(value)}>
                {label}
              </button>
            ))}
          </div>
          {can.manageProjects(user.role) && (
            <Link href="/chantiers/nouveau" className="btn btn-p">
              <IconPlus size={15} width={2.6} />
              Nouveau chantier
            </Link>
          )}
        </div>
      </header>

      <section className="card card-lg hero-card" style={{ padding: "36px 40px", display: "flex", flexWrap: "wrap", gap: 40, alignItems: "center" }} aria-labelledby="ap-h">
        <div style={{ flex: "1 1 380px", minWidth: 0 }}>
          <div id="ap-h" className="s2" style={{ fontSize: 15, fontWeight: 500 }}>
            Disponible sur vos chantiers
          </div>
          <div className="num hero-num" style={{ fontSize: 88, fontWeight: 700, letterSpacing: "-0.045em", lineHeight: 1, marginTop: 10, color: totals.remaining < 0 ? "#B3261E" : undefined }}>
            {fcfaShort(totals.remaining)}
            <span className="s2" style={{ fontSize: 28, fontWeight: 600, letterSpacing: "-0.02em", marginLeft: 8 }}>
              FCFA
            </span>
          </div>
          <div className="s2" style={{ fontSize: 15, marginTop: 10 }}>
            sur {fcfaShort(totals.budget)} de budget · {active} chantier{active > 1 ? "s" : ""} actif{active > 1 ? "s" : ""}
          </div>
          <div role="img" aria-label={`Payé ${paidPct} %, commandé ${orderedPct} %, disponible ${Math.max(0, 100 - paidPct - orderedPct)} %`} style={{ display: "flex", height: 10, borderRadius: 5, overflow: "hidden", gap: 3, marginTop: 28, background: "#E8E8ED" }}>
            <span style={{ width: `${Math.min(100, paidPct)}%`, background: "#1D1D1F" }} />
            <span style={{ width: `${Math.min(100, orderedPct)}%`, background: "#2B59E6" }} />
          </div>
          <div style={{ display: "flex", gap: 28, marginTop: 14, fontSize: 14, flexWrap: "wrap" }}>
            <Legend color="#1D1D1F" label={period === "all" ? "Payé" : period === "week" ? "Payé cette semaine" : "Payé ce mois-ci"} value={fcfaShort(data.period_paid)} />
            <Legend color="#2B59E6" label="Commandé" value={fcfaShort(totals.unpaid)} />
            <Legend color="#D2D2D7" label="Disponible" value={fcfaShort(totals.remaining)} />
          </div>
        </div>
        <PaidChart months={data.paid_by_month} />
      </section>

      <div style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
        <section className="card card-lg" style={{ flex: "1 1 340px", minWidth: 0, padding: "24px 24px 10px" }} aria-labelledby="ap-t">
          <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
            <h2 id="ap-t" className="h2">
              À traiter
            </h2>
            <span className="s2 num" style={{ fontSize: 14 }}>
              {taskCount}
            </span>
          </div>
          <div style={{ marginTop: 8 }}>
            {taskCount === 0 && <Empty>Rien à traiter pour le moment.</Empty>}
            {proc.pending_count > 0 && (
              <ToDo href="/depenses" color="#2B59E6" icon={<IconCheck size={18} />} title={`${proc.pending_count} dépense${proc.pending_count > 1 ? "s" : ""} à valider`} text={`${fcfa(proc.pending_amount)} FCFA${proc.duplicate_count > 0 ? ` · ${proc.duplicate_count} doublon${proc.duplicate_count > 1 ? "s" : ""} possible${proc.duplicate_count > 1 ? "s" : ""}` : ""}`} />
            )}
            {atRisk.map((p) => (
              <ToDo key={p.project_id} href={`/chantiers/${p.project_id}`} color="#D93025" icon={<IconAlert size={18} />} title={`${p.name} à ${pct(p.approved, p.budget)} % du budget`} text={`Pour ${p.declared_progress_pct} % d'avancement`} />
            ))}
            {proc.missing_reports.length > 0 && (
              <ToDo href="/rapports" color="#C76A00" icon={<IconDoc size={18} />} title={proc.missing_reports.length > 1 ? `${proc.missing_reports.length} rapports manquants` : "Rapport manquant"} text={`${proc.missing_reports.map((m) => m.name).join(", ")} · hier`} />
            )}
          </div>
        </section>
        <section className="card card-lg" style={{ flex: "999 1 520px", minWidth: 0, padding: "24px 28px 10px" }} aria-labelledby="ap-c">
          <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
            <h2 id="ap-c" className="h2">
              Vos chantiers
            </h2>
            <Link href="/chantiers" style={{ fontSize: 14, fontWeight: 500 }}>
              Tout afficher
            </Link>
          </div>
          <div style={{ marginTop: 8 }}>
            {data.projects.length === 0 && <Empty>Aucun chantier pour le moment.</Empty>}
            {sortByRisk(data.projects).map((p) => (
              <ProjectRow key={p.project_id} project={p} />
            ))}
          </div>
        </section>
      </div>
      <style>{`@media (max-width: 720px) { .home-h1 { font-size: 34px !important; } .hero-card { padding: 22px !important; gap: 24px !important; } .hero-num { font-size: 56px !important; white-space: normal !important; } }`}</style>
    </div>
  );
}

const RISK = { critical: 0, watch: 1, good: 2, none: 3 };
function sortByRisk(projects: ProjectHealth[]) {
  return [...projects].sort((a, b) => RISK[a.signal] - RISK[b.signal] || pct(b.approved, b.budget) - pct(a.approved, a.budget));
}

function Legend({ color, label, value }: { color: string; label: string; value: string }) {
  return (
    <div>
      <div className="s2" style={{ fontSize: 13 }}>
        <span className="dot" style={{ background: color }} />
        {label}
      </div>
      <div className="num" style={{ fontWeight: 600, marginTop: 2 }}>
        {value}
      </div>
    </div>
  );
}

function ToDo({ href, color, icon, title, text }: { href: string; color: string; icon: React.ReactNode; title: string; text: string }) {
  return (
    <Link href={href} className="dli">
      <span className="sq" style={{ background: color }}>
        {icon}
      </span>
      <span style={{ flex: 1, minWidth: 0 }}>
        <span style={{ display: "block", fontSize: 15, fontWeight: 600 }}>{title}</span>
        <span className="s2" style={{ display: "block", fontSize: 13 }}>
          {text}
        </span>
      </span>
      <span style={{ color: "#AEAEB2" }}>
        <IconChevron />
      </span>
    </Link>
  );
}

function ProjectRow({ project: p }: { project: ProjectHealth }) {
  const signal = SIGNAL[p.signal];
  return (
    <Link href={`/chantiers/${p.project_id}`} className="ch">
      <Ring pct={p.declared_progress_pct} color={signal.color} />
      <span style={{ minWidth: 0 }}>
        <span style={{ display: "block", fontSize: 16, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis" }}>{p.name}</span>
        <span className="s2" style={{ display: "block", fontSize: 13 }}>
          {p.manager_name} · budget {pct(p.approved, p.budget)} %
        </span>
      </span>
      <span style={{ fontSize: 13, fontWeight: 600, color: signal.ink, whiteSpace: "nowrap" }}>
        <span className="dot" style={{ background: signal.color }} />
        {signal.label}
      </span>
    </Link>
  );
}

function PaidChart({ months }: { months: { month: string; paid: number }[] }) {
  const W = 400;
  const H = 150;
  const max = Math.max(1, ...months.map((m) => m.paid));
  const points = months.map((m, i) => ({ x: 10 + (i * (W - 20)) / Math.max(1, months.length - 1), y: 140 - (m.paid / max) * 120 }));
  const line = points.reduce((d, p, i) => {
    if (i === 0) return `M${p.x} ${p.y}`;
    const prev = points[i - 1]!;
    const mid = (prev.x + p.x) / 2;
    return `${d} C ${mid} ${prev.y}, ${mid} ${p.y}, ${p.x} ${p.y}`;
  }, "");
  const last = months[months.length - 1];
  const previous = months[months.length - 2];
  const lastPoint = points[points.length - 1];
  const growth = last && previous && previous.paid > 0 ? Math.round(((last.paid - previous.paid) * 100) / previous.paid) : null;
  return (
    <figure style={{ flex: "1 1 360px", minWidth: 0, margin: 0 }}>
      <figcaption style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 8, flexWrap: "wrap" }}>
        <span className="s2" style={{ fontSize: 13, fontWeight: 500 }}>
          Payé par mois
        </span>
        {last && (
          <span style={{ fontSize: 13, fontWeight: 600 }}>
            {monthLong(last.month)} <span className="num">{fcfaShort(last.paid)}</span>{" "}
            {growth !== null && <span style={{ color: growth >= 0 ? "#1A7F45" : "#B3261E" }}>{growth >= 0 ? "+" : ""}{growth} %</span>}
          </span>
        )}
      </figcaption>
      <svg viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`Paiements mensuels : ${months.map((m) => `${monthLong(m.month)} ${fcfaShort(m.paid)}`).join(", ")}`} style={{ width: "100%", height: "auto", marginTop: 12, overflow: "visible" }}>
        <line x1="0" y1="140" x2={W} y2="140" stroke="#EDEDF0" strokeWidth="1" />
        <line x1="0" y1="80" x2={W} y2="80" stroke="#F3F3F5" strokeWidth="1" />
        <line x1="0" y1="20" x2={W} y2="20" stroke="#F3F3F5" strokeWidth="1" />
        {lastPoint && (
          <>
            <path d={`${line} L${lastPoint.x} 140 L10 140 Z`} fill="#2B59E6" fillOpacity="0.08" />
            <path d={line} fill="none" stroke="#2B59E6" strokeWidth="3" strokeLinecap="round" />
            <circle cx={lastPoint.x} cy={lastPoint.y} r="6" fill="#FFFFFF" stroke="#2B59E6" strokeWidth="3" />
          </>
        )}
      </svg>
      <div className="s2" style={{ display: "flex", justifyContent: "space-between", fontSize: 12, marginTop: 6 }}>
        {months.map((m) => (
          <span key={m.month}>{monthShort(m.month)}</span>
        ))}
      </div>
    </figure>
  );
}

/** Maquette 08 : l'accueil du terrain, centré sur un chantier. */
function FieldHome() {
  const { user } = useSession();
  const online = useOnline();
  const projects = useResource<Paged<Project>>("/projects?limit=100");
  const [stored, setCurrent] = useCurrentProject();
  const list = projects.data?.items.filter((p) => p.status !== "archived") ?? [];
  const project = list.find((p) => p.id === stored) ?? list[0];
  const pending = useLiveQuery(() => db.outbox.where("state").anyOf("pending", "failed", "conflict").count(), [], 0);

  if (projects.loading) return <Loading />;
  if (!project) {
    return projects.error ? <ErrorBanner error={projects.error} /> : <Empty>Aucun chantier ne vous est affecté pour le moment.</Empty>;
  }
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 22, maxWidth: 560, width: "100%", margin: "0 auto" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
        <span className="s2" style={{ fontSize: 15, fontWeight: 500 }}>
          {mediumDate()}
        </span>
        {online && pending === 0 ? (
          <span style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13, fontWeight: 600, color: "#1A7F45" }}>
            <span style={{ width: 7, height: 7, borderRadius: "50%", background: "#1A8F4C" }} />À jour
          </span>
        ) : (
          <Link href="/ajouter" style={{ display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13, fontWeight: 600, color: "#9A5200" }}>
            <span style={{ width: 7, height: 7, borderRadius: "50%", background: "#C76A00" }} />
            {pending > 0 ? `${pending} en attente` : "Hors connexion"}
          </Link>
        )}
      </div>
      <div style={{ position: "relative", marginTop: -10 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <span style={{ fontSize: 34, fontWeight: 700, letterSpacing: "-0.035em", lineHeight: 1.15 }}>{project.name}</span>
          {list.length > 1 && (
            <span style={{ color: "#2B59E6", marginTop: 6 }}>
              <IconDown size={20} />
            </span>
          )}
        </div>
        {list.length > 1 && (
          <select aria-label="Changer de chantier" value={project.id} onChange={(e) => setCurrent(e.target.value)} style={{ position: "absolute", inset: 0, opacity: 0, width: "100%", cursor: "pointer" }}>
            {list.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        )}
      </div>
      <FieldProject key={project.id} project={project} canSeeBudget={can.viewFinance(user.role)} />
    </div>
  );
}

function FieldProject({ project, canSeeBudget }: { project: Project; canSeeBudget: boolean }) {
  const { user } = useSession();
  const online = useOnline();
  const [progress, setProgress] = useState(project.declared_progress_pct);
  const [editing, setEditing] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const tasks = useResource<Items<Task>>(`/projects/${project.id}/tasks`);
  const stock = useResource<StockView>(`/projects/${project.id}/stock`);
  const reports = useResource<Items<DailyReport>>(`/projects/${project.id}/daily-reports?date_from=${today()}&date_to=${today()}`);
  const [done, setDone] = useState<Record<string, boolean>>({});

  async function save(value: number) {
    setError(null);
    try {
      await api(`/projects/${project.id}/progress`, { method: "POST", body: { progress_pct: value } });
      setProgress(value);
      setEditing(false);
    } catch (err) {
      setError(err instanceof NetworkError ? new Error("Connexion requise pour déclarer l'avancement.") : err);
    }
  }

  async function toggle(task: Task, checked: boolean) {
    setDone((d) => ({ ...d, [task.id]: checked }));
    setError(null);
    try {
      await enqueue({
        kind: "task.progress",
        label: `${checked ? "Terminer" : "Rouvrir"} · ${task.title}`,
        entityId: task.id,
        method: "POST",
        path: `/api/v1/tasks/${task.id}/progress`,
        body: { status: checked ? "done" : "in_progress" },
      });
      if (navigator.onLine) {
        await syncNow();
        tasks.reload();
      }
    } catch (err) {
      setError(err);
    }
  }

  const open = (tasks.data?.items ?? []).filter((t) => t.status !== "cancelled" && (t.status !== "done" || t.due_date === today() || done[t.id] !== undefined));
  const isDone = (t: Task) => done[t.id] ?? t.status === "done";
  const doneCount = open.filter(isDone).length;
  const report = reports.data?.items[0];
  const reportLabel = report ? (report.status === "submitted" ? "Envoyé" : "Brouillon") : "À envoyer";
  const mine = can.manageTasks(user.role);

  return (
    <>
      <section className="grp" style={{ padding: 20, display: "flex", alignItems: "center", gap: 20, marginTop: -6 }} aria-label="Avancement du chantier">
        <Ring pct={progress} color="#2B59E6" size={96} stroke={11} track="#EEF0F5" text={`${progress}%`} fontSize={22} />
        <div style={{ flex: 1 }}>
          <div style={{ fontSize: 17, fontWeight: 600 }}>Avancement</div>
          {canSeeBudget && <BudgetUsed projectId={project.id} />}
          {can.writeReport(user.role) && !editing && (
            <button type="button" disabled={!online} onClick={() => setEditing(true)} style={{ marginTop: 10, padding: 0, border: 0, background: "transparent", color: online ? "#1F48C8" : "#AEAEB2", fontSize: 15, fontWeight: 600, cursor: online ? "pointer" : "not-allowed" }}>
              Mettre à jour
            </button>
          )}
          {editing && (
            <div style={{ marginTop: 10, display: "flex", alignItems: "center", gap: 8 }}>
              <input aria-label="Avancement en pour cent" type="range" min={0} max={100} step={5} defaultValue={progress} onChange={(e) => setProgress(Number(e.target.value))} style={{ flex: 1, minWidth: 0 }} />
              <button type="button" onClick={() => save(progress)} style={{ border: 0, background: "transparent", color: "#1F48C8", fontSize: 15, fontWeight: 600, cursor: "pointer" }}>
                OK
              </button>
            </div>
          )}
        </div>
      </section>
      <ErrorBanner error={error} />
      {can.createExpense(user.role) && (
        <Link href={`/depenses/nouvelle?project=${project.id}`} style={{ display: "flex", alignItems: "center", gap: 16, padding: "18px 20px", borderRadius: 20, background: "#2B59E6", color: "#FFFFFF", boxShadow: "0 10px 24px rgba(43,89,230,.28)" }}>
          <span style={{ width: 50, height: 50, borderRadius: "50%", background: "rgba(255,255,255,.2)", display: "flex", alignItems: "center", justifyContent: "center", flex: "none" }}>
            <IconCamera size={24} />
          </span>
          <span style={{ flex: 1 }}>
            <span style={{ display: "block", fontSize: 18, fontWeight: 700, letterSpacing: "-0.01em" }}>Ajouter une dépense</span>
            <span style={{ display: "block", fontSize: 14, opacity: 0.88, marginTop: 2 }}>Photographiez le reçu, c&apos;est tout</span>
          </span>
        </Link>
      )}
      <section aria-labelledby="ap-today">
        <h2 id="ap-today" className="hdr" style={{ margin: 0 }}>
          Aujourd&apos;hui · {doneCount} sur {open.length}
        </h2>
        <div className="grp">
          {open.length === 0 && <p className="muted" style={{ padding: "16px", margin: 0, fontSize: 15 }}>Aucune tâche en cours sur ce chantier.</p>}
          {open.map((t) => {
            const allowed = mine || t.assignee_ids.includes(user.id);
            return (
              <label key={t.id} className="cell">
                <input type="checkbox" className="ck" checked={isDone(t)} disabled={!allowed || t.status === "blocked"} onChange={(e) => toggle(t, e.target.checked)} />
                <span className={isDone(t) ? "s2" : undefined} style={{ flex: 1, textDecoration: isDone(t) ? "line-through" : undefined }}>
                  {t.title}
                </span>
              </label>
            );
          })}
        </div>
      </section>
      <div className="grp">
        {can.writeReport(user.role) && (
          <Link href={report?.status === "submitted" ? `/rapports/${report.id}` : `/rapports/nouveau?project=${project.id}`} className="cell">
            <span style={{ width: 30, height: 30, borderRadius: 8, background: "#1A8F4C", color: "#FFFFFF", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <IconDoc size={17} width={2.2} />
            </span>
            <span style={{ flex: 1 }}>Rapport du jour</span>
            <span className="s2" style={{ fontSize: 15 }}>
              {reportLabel}
            </span>
            <span style={{ color: "#C7C7CC" }}>
              <IconChevron size={14} width={3} />
            </span>
          </Link>
        )}
        <Link href={`/materiaux?project=${project.id}`} className="cell">
          <span style={{ width: 30, height: 30, borderRadius: 8, background: "#8E8E93", color: "#FFFFFF", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <IconBox size={17} width={2.2} />
          </span>
          <span style={{ flex: 1 }}>Stock</span>
          <span className="s2" style={{ fontSize: 15 }}>
            {stock.data ? (stock.data.low_count > 0 ? `${stock.data.low_count} bas` : "Suffisant") : ""}
          </span>
          <span style={{ color: "#C7C7CC" }}>
            <IconChevron size={14} width={3} />
          </span>
        </Link>
      </div>
    </>
  );
}

function BudgetUsed({ projectId }: { projectId: string }) {
  const { data } = useResource<ProjectHealth>(`/projects/${projectId}/financials`);
  if (!data) return null;
  return (
    <div className="s2" style={{ fontSize: 14, marginTop: 2 }}>
      Budget utilisé : {pct(data.approved, data.budget)} %
    </div>
  );
}
