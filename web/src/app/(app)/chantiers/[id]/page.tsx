"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useState } from "react";
import { IconCamera, IconDownload, IconPlus } from "@/components/icons";
import { Badge, ErrorBanner, Loading, PROJECT_TONE } from "@/components/ui";
import type { Project } from "@/lib/api";
import { date, PROJECT_STATUS_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";
import { BudgetTab } from "./BudgetTab";
import { DocumentsTab } from "./DocumentsTab";
import { ExpensesTab } from "./ExpensesTab";
import { HistoryTab } from "./HistoryTab";
import { OverviewTab } from "./OverviewTab";
import { ReportsTab } from "./ReportsTab";
import { StockTab } from "./StockTab";
import { TasksTab } from "./TasksTab";
import { TeamTab } from "./TeamTab";

const TABS = [
  { id: "overview", label: "Vue d'ensemble" },
  { id: "budget", label: "Budget" },
  { id: "expenses", label: "Dépenses" },
  { id: "stock", label: "Matériaux" },
  { id: "tasks", label: "Tâches" },
  { id: "reports", label: "Rapports journaliers" },
  { id: "team", label: "Équipe" },
  { id: "documents", label: "Documents" },
  { id: "history", label: "Historique" },
];

export default function ProjectPage() {
  const { id } = useParams<{ id: string }>();
  const { user } = useSession();
  const { data: project, error, stale, loading, reload } = useResource<Project>(`/projects/${id}`);
  const [tab, setTab] = useState("overview");

  if (loading) return <Loading />;
  if (!project) return <ErrorBanner error={error} />;

  const tabs = TABS.filter((t) => (t.id !== "budget" || can.viewBudget(user.role)) && (t.id !== "history" || can.viewFinance(user.role)));
  const readOnly = project.status === "archived";
  const meta = [project.client_name, project.address, project.planned_end_date ? `${date(project.start_date)} → ${date(project.planned_end_date)}` : `Depuis le ${date(project.start_date)}`, project.manager_name ? `Conducteur : ${project.manager_name}` : null, stale ? "données hors connexion" : null].filter(Boolean).join(" · ");

  return (
    <div className="wrap" style={{ display: "flex", flexDirection: "column", gap: 18 }}>
      <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
        <Link href="/chantiers" style={{ fontWeight: 600 }}>
          Chantiers
        </Link>
        <span className="muted"> / {project.name}</span>
      </nav>
      <div style={{ display: "flex", gap: 20, alignItems: "center", flexWrap: "wrap" }}>
        <div role="img" aria-label="Emplacement pour la photo du chantier" className="hide-sm" style={{ width: 132, height: 96, borderRadius: 10, background: "#E5E5EA", color: "#48484A", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 4, fontSize: 11, fontWeight: 600 }}>
          <IconCamera size={22} />
          Photo chantier
        </div>
        <div style={{ flex: "1 1 320px", minWidth: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
            <h1 className="page-h1">{project.name}</h1>
            <Badge tone={PROJECT_TONE[project.status]}>{PROJECT_STATUS_LABEL[project.status]}</Badge>
          </div>
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 14 }}>
            {meta}
          </p>
        </div>
        <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
          {can.exportData(user.role) && (
            <a href={`/api/v1/exports/projects/${id}/report.pdf`} className="btn btn-s">
              <IconDownload />
              Exporter
            </a>
          )}
          {can.manageProjects(user.role) && (
            <Link href={`/chantiers/${id}/modifier`} className="btn btn-s">
              Modifier
            </Link>
          )}
          {!readOnly && can.createExpense(user.role) && (
            <Link href={`/depenses/nouvelle?project=${id}`} className="btn btn-p">
              <IconPlus size={16} />
              Ajouter une dépense
            </Link>
          )}
        </div>
      </div>
      {readOnly && (
        <p role="status" className="mut" style={{ margin: 0, padding: "10px 14px", borderRadius: 10, fontSize: 14 }}>
          Ce chantier est archivé : il est en lecture seule et ses données sont conservées.
        </p>
      )}
      <div role="tablist" aria-label="Sections du chantier" className="tabs">
        {tabs.map((t) => (
          <button key={t.id} type="button" role="tab" aria-selected={tab === t.id} className={tab === t.id ? "on" : ""} onClick={() => setTab(t.id)}>
            {t.label}
          </button>
        ))}
      </div>
      {tab === "overview" && <OverviewTab project={project} onTab={setTab} />}
      {tab === "budget" && <BudgetTab project={project} onChange={reload} />}
      {tab === "expenses" && <ExpensesTab projectId={id} />}
      {tab === "stock" && <StockTab project={project} />}
      {tab === "tasks" && <TasksTab project={project} />}
      {tab === "reports" && <ReportsTab project={project} />}
      {tab === "team" && <TeamTab project={project} />}
      {tab === "documents" && <DocumentsTab projectId={id} />}
      {tab === "history" && <HistoryTab projectId={id} />}
    </div>
  );
}
