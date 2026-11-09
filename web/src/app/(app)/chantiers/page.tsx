"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import { IconChevron, IconPlus } from "@/components/icons";
import { Badge, Empty, ErrorBanner, Loading, PROJECT_TONE, SearchBox } from "@/components/ui";
import type { Dashboard, Project } from "@/lib/api";
import { fetchAll } from "@/lib/api";
import { dateShort, fcfa, pct, PROJECT_STATUS_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

const TABS: { id: string; label: string }[] = [
  { id: "", label: "Tous" },
  { id: "in_progress", label: "En cours" },
  { id: "planned", label: "Planifiés" },
  { id: "completed", label: "Terminés" },
  { id: "archived", label: "Archivés" },
];

function plural(n: number, one: string, many: string) {
  return `${n} ${n > 1 ? many : one}`;
}

export default function ProjectsPage() {
  const { user } = useSession();
  const finance = can.viewFinance(user.role);
  const [state, setState] = useState<{ items: Project[]; error: unknown; loading: boolean }>({ items: [], error: null, loading: true });
  // Le signal de dérive vient du serveur : l'écran ne recalcule aucune règle financière.
  const dashboard = useResource<Dashboard>(finance ? "/dashboard" : null);
  const [tab, setTab] = useState("");
  const [q, setQ] = useState("");
  const [manager, setManager] = useState("");
  const [sort, setSort] = useState(finance ? "exposed" : "name");

  useEffect(() => {
    let cancelled = false;
    fetchAll<Project>("/projects").then(
      ({ items }) => !cancelled && setState({ items, error: null, loading: false }),
      (error: unknown) => !cancelled && setState({ items: [], error, loading: false }),
    );
    return () => {
      cancelled = true;
    };
  }, []);

  const managers = useMemo(() => [...new Set(state.items.map((p) => p.manager_name ?? ""))].filter(Boolean).sort(), [state.items]);
  const signals = useMemo(() => new Map(dashboard.data?.projects.map((p) => [p.project_id, p.signal])), [dashboard.data]);

  const exposed = (p: Project) => pct(p.approved ?? 0, p.budget ?? 0);
  const needle = q.trim().toLowerCase();
  const matching = state.items.filter((p) => (!manager || p.manager_name === manager) && (!needle || `${p.name} ${p.client_name} ${p.address} ${p.city ?? ""} ${p.reference}`.toLowerCase().includes(needle)));
  const count = (status: string) => (status ? matching.filter((p) => p.status === status).length : matching.filter((p) => p.status !== "archived").length);
  const visible = matching
    .filter((p) => (tab ? p.status === tab : p.status !== "archived"))
    .sort((a, b) => {
      if (sort === "end") return (a.planned_end_date ?? "9999").localeCompare(b.planned_end_date ?? "9999");
      if (sort === "name") return a.name.localeCompare(b.name, "fr");
      return exposed(b) - exposed(a);
    });
  const all = state.items.filter((p) => p.status !== "archived");
  const by = (s: string) => all.filter((p) => p.status === s).length;

  const barColor = (p: Project) => {
    if (p.status === "completed") return "#48484A";
    const signal = signals.get(p.id);
    return signal === "critical" ? "#D93025" : signal === "watch" ? "#C76A00" : "#2B59E6";
  };

  return (
    <>
      <div className="wrap" style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h1 className="page-h1">Chantiers</h1>
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
            {plural(all.length, "chantier", "chantiers")} · {by("in_progress")} en cours, {plural(by("planned"), "planifié", "planifiés")}, {plural(by("completed"), "terminé", "terminés")}
          </p>
        </div>
        {can.manageProjects(user.role) && (
          <Link href="/chantiers/nouveau" className="btn btn-p">
            <IconPlus size={16} />
            Nouveau chantier
          </Link>
        )}
      </div>
      <section className="card wrap" aria-label="Liste des chantiers">
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, padding: 16 }}>
          <nav aria-label="Filtrer par statut" style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
            {TABS.map((t) => (
              <button key={t.id} type="button" className={`tab ${tab === t.id ? "on" : ""}`} aria-pressed={tab === t.id} onClick={() => setTab(t.id)}>
                {t.label} ({count(t.id)})
              </button>
            ))}
          </nav>
          <div style={{ marginLeft: "auto", display: "flex", gap: 10, flexWrap: "wrap", maxWidth: "100%" }}>
            <SearchBox label="Rechercher un chantier" placeholder="Nom, client, quartier…" value={q} onChange={setQ} />
            <select className="sel" aria-label="Responsable" value={manager} onChange={(e) => setManager(e.target.value)}>
              <option value="">Tous les responsables</option>
              {managers.map((m) => (
                <option key={m}>{m}</option>
              ))}
            </select>
            <select className="sel" aria-label="Trier par" value={sort} onChange={(e) => setSort(e.target.value)}>
              {finance && <option value="exposed">Trier : budget exposé</option>}
              <option value="end">Trier : date de fin</option>
              <option value="name">Trier : nom</option>
            </select>
          </div>
        </div>
        {state.loading ? (
          <Loading />
        ) : state.error ? (
          <div style={{ padding: 16 }}>
            <ErrorBanner error={state.error} />
          </div>
        ) : visible.length === 0 ? (
          <Empty>{state.items.length === 0 ? "Aucun chantier pour le moment." : "Aucun chantier ne correspond à ces filtres."}</Empty>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table className="tbl">
              <thead>
                <tr>
                  <th>Chantier</th>
                  <th>Statut</th>
                  <th className="hide-sm">Responsable</th>
                  <th className="hide-sm">Période</th>
                  {finance && <th className="hide-sm">Budget révisé (FCFA)</th>}
                  {finance && (
                    <th className="hide-sm" style={{ minWidth: 170 }}>
                      Budget exposé
                    </th>
                  )}
                  <th className="hide-sm">Avancement</th>
                  <th>
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visible.map((p) => (
                  <tr key={p.id}>
                    <td>
                      <Link href={`/chantiers/${p.id}`} style={{ fontWeight: 600, color: "#1D1D1F" }}>
                        {p.name}
                      </Link>
                      <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                        {p.client_name} · {p.city ?? p.address}
                      </div>
                    </td>
                    <td>
                      <Badge tone={PROJECT_TONE[p.status]}>{PROJECT_STATUS_LABEL[p.status]}</Badge>
                    </td>
                    <td className="hide-sm">{p.manager_name}</td>
                    <td className="num muted hide-sm" style={{ fontSize: 13 }}>
                      {p.planned_end_date ? `${dateShort(p.start_date)} → ${dateShort(p.planned_end_date)}` : `Depuis le ${dateShort(p.start_date)}`}
                    </td>
                    {finance && <td className="num hide-sm">{p.budget ? fcfa(p.budget) : "—"}</td>}
                    {finance && (
                      <td className="hide-sm">
                        {p.status === "planned" || p.status === "draft" ? (
                          <span className="muted" style={{ fontSize: 13 }}>
                            Pas encore démarré
                          </span>
                        ) : p.budget ? (
                          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                            <div style={{ flex: 1, height: 8, borderRadius: 4, background: "#F2F2F7" }}>
                              <div style={{ width: `${Math.min(100, exposed(p))}%`, height: 8, borderRadius: 4, background: barColor(p) }} />
                            </div>
                            <span className="num" style={{ fontSize: 13, fontWeight: 600 }}>
                              {exposed(p)} %
                            </span>
                          </div>
                        ) : (
                          <span className="muted" style={{ fontSize: 13 }}>
                            Sans budget publié
                          </span>
                        )}
                      </td>
                    )}
                    <td className="num hide-sm">{p.declared_progress_pct} %</td>
                    <td>
                      <Link href={`/chantiers/${p.id}`} aria-label={`Ouvrir ${p.name}`} style={{ color: "#6E6E73", display: "inline-flex" }}>
                        <IconChevron size={18} width={2} />
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
      <p className="muted wrap" style={{ margin: 0, fontSize: 13 }}>
        Un chantier terminé peut être archivé : il passe en lecture seule et ses données sont conservées.
      </p>
    </>
  );
}
