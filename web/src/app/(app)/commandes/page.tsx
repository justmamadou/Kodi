"use client";

import Link from "next/link";
import { useState } from "react";
import { IconChevron } from "@/components/icons";
import { Badge, Empty, ErrorBanner, Loading, PO_TONE, SearchBox } from "@/components/ui";
import type { Items, POSummary } from "@/lib/api";
import { date, fcfa, PO_STATUS_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";

const TABS: { id: string; label: string; statuses: string[] }[] = [
  { id: "", label: "Toutes", statuses: [] },
  { id: "todo", label: "À traiter", statuses: ["draft", "submitted", "approved"] },
  { id: "delivery", label: "En livraison", statuses: ["sent", "partially_received"] },
  { id: "done", label: "Reçues", statuses: ["received"] },
  { id: "cancelled", label: "Annulées", statuses: ["cancelled"] },
];

/** Bons de commande des chantiers visibles. Les montants n'apparaissent qu'aux rôles qui voient les prix. */
export default function OrdersPage() {
  const { data, error, loading } = useResource<Items<POSummary>>("/purchase-orders");
  const [tab, setTab] = useState("");
  const [q, setQ] = useState("");

  const all = data?.items ?? [];
  const needle = q.trim().toLowerCase();
  const matching = all.filter((o) => !needle || `${o.reference} ${o.supplier_name} ${o.project_name}`.toLowerCase().includes(needle));
  const inTab = (id: string) => {
    const t = TABS.find((x) => x.id === id);
    return matching.filter((o) => !t || t.statuses.length === 0 || t.statuses.includes(o.status));
  };
  const visible = inTab(tab);
  const prices = all.some((o) => o.total_amount !== null);

  return (
    <>
      <div className="wrap">
        <h1 className="page-h1">Commandes</h1>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
          Bons de commande issus des offres retenues : approbation, envoi au fournisseur, réceptions sur le chantier.
        </p>
      </div>
      <section className="card wrap" aria-label="Liste des bons de commande">
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, padding: 16 }}>
          <nav aria-label="Filtrer par étape" style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
            {TABS.map((t) => (
              <button key={t.id} type="button" className={`tab ${tab === t.id ? "on" : ""}`} aria-pressed={tab === t.id} onClick={() => setTab(t.id)}>
                {t.label} ({inTab(t.id).length})
              </button>
            ))}
          </nav>
          <div style={{ marginLeft: "auto", maxWidth: "100%" }}>
            <SearchBox label="Rechercher une commande" placeholder="Numéro, fournisseur, chantier…" value={q} onChange={setQ} />
          </div>
        </div>
        {loading ? (
          <Loading />
        ) : error ? (
          <div style={{ padding: 16 }}>
            <ErrorBanner error={error} />
          </div>
        ) : visible.length === 0 ? (
          <Empty>{all.length === 0 ? "Aucune commande. Elles se génèrent depuis l'offre retenue d'une demande de prix." : "Aucune commande ne correspond à ces filtres."}</Empty>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table className="tbl">
              <thead>
                <tr>
                  <th>Commande</th>
                  <th>Statut</th>
                  <th className="hide-sm">Chantier</th>
                  <th className="hide-sm">Livraison prévue</th>
                  {prices && <th className="text-right hide-sm">Total TTC (FCFA)</th>}
                  <th>
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visible.map((o) => (
                  <tr key={o.id}>
                    <td>
                      <Link href={`/commandes/${o.id}`} style={{ fontWeight: 600, color: "#1D1D1F" }}>
                        {o.reference} · {o.supplier_name}
                      </Link>
                      <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                        {o.lines_received}/{o.line_count} ligne{o.line_count > 1 ? "s" : ""} reçue{o.lines_received > 1 ? "s" : ""}
                      </div>
                    </td>
                    <td>
                      <Badge tone={PO_TONE[o.status]}>{PO_STATUS_LABEL[o.status]}</Badge>
                    </td>
                    <td className="hide-sm">{o.project_name}</td>
                    <td className="num hide-sm">{date(o.expected_delivery_date)}</td>
                    {prices && <td className="num text-right hide-sm">{o.total_amount !== null ? fcfa(o.total_amount) : "—"}</td>}
                    <td>
                      <Link href={`/commandes/${o.id}`} aria-label={`Ouvrir ${o.reference}`} style={{ color: "#6E6E73", display: "inline-flex" }}>
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
    </>
  );
}
