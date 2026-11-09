"use client";

import Link from "next/link";
import { useState } from "react";
import { IconChevron, IconPlus } from "@/components/icons";
import { Badge, Empty, ErrorBanner, Loading, SearchBox } from "@/components/ui";
import type { Items, Supplier } from "@/lib/api";
import { SUPPLIER_ACTIVITY_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

/** Pas de maquette dédiée : même grammaire que les listes Chantiers et Demandes de prix. */
export default function SuppliersPage() {
  const { user } = useSession();
  const { data, error, loading } = useResource<Items<Supplier>>("/suppliers?include_archived=true");
  const [archived, setArchived] = useState(false);
  const [activity, setActivity] = useState("");
  const [q, setQ] = useState("");

  const all = data?.items ?? [];
  const needle = q.trim().toLowerCase();
  const matching = all.filter(
    (s) => (!activity || s.activities?.includes(activity)) && (!needle || `${s.name} ${s.legal_name ?? ""} ${s.ninea ?? ""} ${s.phone ?? ""} ${s.email ?? ""} ${s.city ?? ""}`.toLowerCase().includes(needle)),
  );
  const visible = matching.filter((s) => s.is_active !== archived);
  const count = (arch: boolean) => matching.filter((s) => s.is_active !== arch).length;

  return (
    <>
      <div className="wrap" style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h1 className="page-h1">Fournisseurs</h1>
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
            Fiches légales, contacts, conditions commerciales et historique des consultations et achats.
          </p>
        </div>
        {can.createSupplier(user.role) && (
          <Link href="/fournisseurs/nouveau" className="btn btn-p">
            <IconPlus size={16} />
            Nouveau fournisseur
          </Link>
        )}
      </div>
      <section className="card wrap" aria-label="Répertoire des fournisseurs">
        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12, padding: 16 }}>
          <nav aria-label="Filtrer par état" style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
            <button type="button" className={`tab ${!archived ? "on" : ""}`} aria-pressed={!archived} onClick={() => setArchived(false)}>
              Actifs ({count(false)})
            </button>
            <button type="button" className={`tab ${archived ? "on" : ""}`} aria-pressed={archived} onClick={() => setArchived(true)}>
              Archivés ({count(true)})
            </button>
          </nav>
          <div style={{ marginLeft: "auto", display: "flex", gap: 10, flexWrap: "wrap", maxWidth: "100%" }}>
            <SearchBox label="Rechercher un fournisseur" placeholder="Nom, NINEA, téléphone, ville…" value={q} onChange={setQ} />
            <select className="sel" aria-label="Activité" value={activity} onChange={(e) => setActivity(e.target.value)}>
              <option value="">Toutes les activités</option>
              {Object.entries(SUPPLIER_ACTIVITY_LABEL).map(([k, label]) => (
                <option key={k} value={k}>
                  {label}
                </option>
              ))}
            </select>
          </div>
        </div>
        {loading ? (
          <Loading />
        ) : error ? (
          <div style={{ padding: 16 }}>
            <ErrorBanner error={error} />
          </div>
        ) : visible.length === 0 ? (
          <Empty>{all.length === 0 ? "Aucun fournisseur pour le moment." : "Aucun fournisseur ne correspond à ces filtres."}</Empty>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table className="tbl">
              <thead>
                <tr>
                  <th>Fournisseur</th>
                  <th className="hide-sm">Activités</th>
                  <th>Téléphone</th>
                  <th className="hide-sm">Ville</th>
                  <th>
                    <span className="sr-only">Actions</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {visible.map((s) => (
                  <tr key={s.id}>
                    <td>
                      <Link href={`/fournisseurs/${s.id}`} style={{ fontWeight: 600, color: "#1D1D1F" }}>
                        {s.name}
                      </Link>
                      {(s.legal_name || s.ninea) && (
                        <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                          {[s.legal_name, s.ninea ? `NINEA ${s.ninea}` : null].filter(Boolean).join(" · ")}
                        </div>
                      )}
                    </td>
                    <td className="hide-sm">
                      <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
                        {(s.activities ?? []).map((a) => (
                          <Badge key={a}>{SUPPLIER_ACTIVITY_LABEL[a] ?? a}</Badge>
                        ))}
                      </div>
                    </td>
                    <td className="num muted">{s.phone ?? "—"}</td>
                    <td className="muted hide-sm">{s.city ?? "—"}</td>
                    <td>
                      <Link href={`/fournisseurs/${s.id}`} aria-label={`Ouvrir ${s.name}`} style={{ color: "#6E6E73", display: "inline-flex" }}>
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
