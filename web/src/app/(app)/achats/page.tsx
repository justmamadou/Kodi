"use client";

import Link from "next/link";
import { useState } from "react";
import { IconChevron, IconPlus } from "@/components/icons";
import { Badge, Empty, ErrorBanner, Loading, RFQ_TONE, SearchBox } from "@/components/ui";
import type { Items, RFQSummary } from "@/lib/api";
import { date, RFQ_STATUS_LABEL, rfqRef, today } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

const TABS: { id: string; label: string }[] = [
  { id: "", label: "Toutes" },
  { id: "draft", label: "Brouillons" },
  { id: "published", label: "Publiées" },
  { id: "closed", label: "Clôturées" },
  { id: "cancelled", label: "Annulées" },
];

/** Pas de maquette dédiée : même grammaire que la liste des chantiers. */
export default function RFQListPage() {
  const { user } = useSession();
  const { data, error, loading } = useResource<Items<RFQSummary>>("/rfqs");
  const [tab, setTab] = useState("");
  const [q, setQ] = useState("");

  const needle = q.trim().toLowerCase();
  const all = data?.items ?? [];
  const matching = all.filter((r) => !needle || `${rfqRef(r.number)} ${r.title} ${r.project_name}`.toLowerCase().includes(needle));
  const visible = matching.filter((r) => !tab || r.status === tab);
  const count = (status: string) => (status ? matching.filter((r) => r.status === status).length : matching.length);
  const waiting = all.filter((r) => r.status === "published").length;

  return (
    <>
      <div className="wrap" style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h1 className="page-h1">Demandes de prix</h1>
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
            Consultez plusieurs fournisseurs et comparez leurs offres, ligne par ligne.{waiting > 0 ? ` ${waiting} consultation${waiting > 1 ? "s" : ""} en cours.` : ""}
          </p>
        </div>
        {can.manageRFQ(user.role) && (
          <Link href="/achats/nouvelle" className="btn btn-p">
            <IconPlus size={16} />
            Nouvelle demande
          </Link>
        )}
      </div>
      <section className="card wrap" aria-label="Liste des demandes de prix">
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, padding: 16 }}>
          <nav aria-label="Filtrer par statut" style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
            {TABS.map((t) => (
              <button key={t.id} type="button" className={`tab ${tab === t.id ? "on" : ""}`} aria-pressed={tab === t.id} onClick={() => setTab(t.id)}>
                {t.label} ({count(t.id)})
              </button>
            ))}
          </nav>
          <div style={{ marginLeft: "auto", maxWidth: "100%" }}>
            <SearchBox label="Rechercher une demande" placeholder="Numéro, objet, chantier…" value={q} onChange={setQ} />
          </div>
        </div>
        {loading ? (
          <Loading />
        ) : error ? (
          <div style={{ padding: 16 }}>
            <ErrorBanner error={error} />
          </div>
        ) : visible.length === 0 ? (
          <Empty>{all.length === 0 ? "Aucune demande de prix pour le moment." : "Aucune demande ne correspond à ces filtres."}</Empty>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table className="tbl">
              <thead>
                <tr>
                  <th>Demande</th>
                  <th>Statut</th>
                  <th className="hide-sm">Chantier</th>
                  <th className="hide-sm">Date limite</th>
                  <th className="hide-sm">Réponses</th>
                  <th>
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visible.map((r) => {
                  const late = r.status === "published" && r.response_deadline !== null && r.response_deadline < today();
                  return (
                    <tr key={r.id}>
                      <td>
                        <Link href={`/achats/${r.id}`} style={{ fontWeight: 600, color: "#1D1D1F" }}>
                          {r.title}
                        </Link>
                        <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                          {rfqRef(r.number)} · {r.item_count} ligne{r.item_count > 1 ? "s" : ""}
                        </div>
                      </td>
                      <td>
                        <Badge tone={RFQ_TONE[r.status]}>{RFQ_STATUS_LABEL[r.status]}</Badge>
                      </td>
                      <td className="hide-sm">{r.project_name}</td>
                      <td className="num hide-sm" style={late ? { color: "#9A5200" } : undefined}>
                        {date(r.response_deadline)}
                        {late && " · dépassée"}
                      </td>
                      <td className="num hide-sm">{r.status === "draft" ? "—" : `${r.responded_count} / ${r.supplier_count}`}</td>
                      <td>
                        <Link href={`/achats/${r.id}`} aria-label={`Ouvrir ${r.title}`} style={{ color: "#6E6E73", display: "inline-flex" }}>
                          <IconChevron size={18} width={2} />
                        </Link>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </>
  );
}
