"use client";

import { useSearchParams } from "next/navigation";
import { type FormEvent, Suspense, useState } from "react";
import { IconAdjust, IconArrowDown, IconArrowUp, IconDownload } from "@/components/icons";
import { Badge, Button, Empty, ErrorBanner, fieldError, Input, Loading, SearchBox, Select, Soon } from "@/components/ui";
import type { Items, Material, Paged, Project, StockHistoryRow, StockRow, StockView } from "@/lib/api";
import { api } from "@/lib/api";
import { useCurrentProject } from "@/lib/current-project";
import { dayMonth, MOVEMENT_LABEL, qty, today, UNIT_LABEL } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

export default function MaterialsPage() {
  return (
    <Suspense fallback={<Loading />}>
      <Materials />
    </Suspense>
  );
}

const unit = (u: string) => UNIT_LABEL[u] ?? u;

function Materials() {
  const { user } = useSession();
  const params = useSearchParams();
  const [stored, setStored] = useCurrentProject();
  const projects = useResource<Paged<Project>>("/projects?limit=100");
  const list = projects.data?.items.filter((p) => p.status !== "archived") ?? [];
  const wanted = params.get("project");
  const [picked, setPicked] = useState<string | null>(wanted);
  const project = list.find((p) => p.id === (picked ?? stored)) ?? list[0];
  const [view, setView] = useState<"stock" | "moves" | "catalog">("stock");
  const [q, setQ] = useState("");

  const stock = useResource<StockView>(project ? `/projects/${project.id}/stock` : null);
  const recent = useResource<Items<StockHistoryRow>>(project ? `/projects/${project.id}/stock/history?limit=100` : null);
  const materials = useResource<Items<Material>>("/materials");

  function refresh() {
    stock.reload();
    recent.reload();
  }

  function exportCsv() {
    if (!stock.data || !project) return;
    const header = ["Article", "Unité", "Entrées", "Sorties", "Ajustements", "Stock théorique", "Dernier mouvement", "État"];
    const rows = stock.data.items.map((s) => [s.name, unit(s.unit), s.qty_in, s.qty_out, s.qty_adjust, s.on_hand, s.last_movement_on ?? "", s.is_low ? "Stock bas" : "Suffisant"]);
    // Le préfixe interdit à un tableur d'interpréter un nom d'article comme une formule.
    const cell = (v: string) => `"${(/^[=+\-@]/.test(v) ? `'${v}` : v).replace(/"/g, '""')}"`;
    const csv = `﻿${[header, ...rows].map((r) => r.map(cell).join(";")).join("\r\n")}`;
    const link = document.createElement("a");
    link.href = URL.createObjectURL(new Blob([csv], { type: "text/csv;charset=utf-8" }));
    link.download = `stock-${project.reference}.csv`;
    link.click();
    URL.revokeObjectURL(link.href);
  }

  if (projects.loading) return <Loading />;
  if (!project) return projects.error ? <ErrorBanner error={projects.error} /> : <Empty>Aucun chantier pour le moment.</Empty>;

  const needle = q.trim().toLowerCase();
  const items = (stock.data?.items ?? []).filter((s) => !needle || `${s.name} ${s.sku ?? ""}`.toLowerCase().includes(needle));
  const moves = recent.data?.items ?? [];
  const low = stock.data?.low_count ?? 0;

  return (
    <>
      <div className="wrap" style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h1 className="page-h1">Matériaux</h1>
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
            Stock théorique de {project.name} · calculé à partir des mouvements enregistrés
          </p>
        </div>
        <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
          <select
            className="sel"
            aria-label="Chantier"
            value={project.id}
            onChange={(e) => {
              setPicked(e.target.value);
              setStored(e.target.value);
            }}
          >
            {list.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <button type="button" className="btn btn-s" onClick={exportCsv} disabled={!stock.data}>
            <IconDownload />
            Exporter
          </button>
        </div>
      </div>
      <section className="wrap" aria-label="Résumé du stock" style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: 16 }}>
        <div className="card" style={{ padding: "18px 20px" }}>
          <div className="muted" style={{ fontSize: 13, fontWeight: 600 }}>
            Articles suivis
          </div>
          <div className="num" style={{ fontSize: 24, fontWeight: 700, marginTop: 8 }}>
            {stock.data?.items.length ?? "—"}
          </div>
        </div>
        <div className="card" style={{ padding: "18px 20px" }}>
          <div className="muted" style={{ fontSize: 13, fontWeight: 600 }}>
            Mouvements ce mois
          </div>
          <div className="num" style={{ fontSize: 24, fontWeight: 700, marginTop: 8 }}>
            {stock.data?.movements_this_month ?? "—"}
          </div>
        </div>
        <div className="card" style={{ padding: "18px 20px", background: low > 0 ? "#FFF1E0" : undefined }}>
          <div style={{ fontSize: 13, fontWeight: 600, color: low > 0 ? "#9A5200" : "#6E6E73" }}>Stock bas</div>
          <div className="num" style={{ fontSize: 24, fontWeight: 700, marginTop: 8 }}>
            {low} article{low > 1 ? "s" : ""}
          </div>
        </div>
        <div className="card" style={{ padding: "18px 20px" }}>
          <div className="muted" style={{ fontSize: 13, fontWeight: 600 }}>
            Transferts en cours
          </div>
          <div style={{ marginTop: 12 }}>
            <Soon />
          </div>
        </div>
      </section>
      <div className="wrap" style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
        <div style={{ flex: "999 1 560px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
          <section className="card" aria-label="Stock du chantier">
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: 16, flexWrap: "wrap" }}>
              <nav aria-label="Vue" style={{ display: "flex", gap: 4 }}>
                {(
                  [
                    ["stock", "Stock"],
                    ["moves", "Mouvements"],
                    ["catalog", "Catalogue"],
                  ] as const
                ).map(([id, label]) => (
                  <button key={id} type="button" className={`tab ${view === id ? "on" : ""}`} aria-pressed={view === id} onClick={() => setView(id)}>
                    {label}
                  </button>
                ))}
              </nav>
              {view === "stock" && <SearchBox label="Rechercher un article" placeholder="Rechercher un article…" value={q} onChange={setQ} />}
            </div>
            {view === "stock" && <StockTable loading={stock.loading} error={stock.error} items={items} />}
            {view === "moves" && (
              <div style={{ padding: "0 20px 8px" }}>
                {moves.length === 0 ? <Empty>Aucun mouvement sur ce chantier.</Empty> : moves.map((m) => <Move key={`${m.movement_id}-${m.material_name}`} move={m} />)}
              </div>
            )}
            {view === "catalog" && <Catalog onChange={materials.reload} />}
          </section>
          {view === "stock" && (
            <section className="card" style={{ padding: "16px 20px" }} aria-labelledby="h-mv">
              <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between" }}>
                <h2 id="h-mv" className="sec-t">
                  Mouvements récents
                </h2>
                <button type="button" className="link-btn" onClick={() => setView("moves")}>
                  Tout voir
                </button>
              </div>
              {moves.length === 0 ? (
                <Empty>Aucun mouvement sur ce chantier.</Empty>
              ) : (
                moves.slice(0, 3).map((m) => <Move key={`${m.movement_id}-${m.material_name}`} move={m} />)
              )}
            </section>
          )}
        </div>
        {can.moveStock(user.role) && <MovementForm key={project.id} project={project} materials={materials.data?.items.filter((m) => !m.archived_at) ?? []} onDone={refresh} />}
      </div>
    </>
  );
}

function StockTable({ loading, error, items }: { loading: boolean; error: unknown; items: StockRow[] }) {
  if (loading) return <Loading />;
  if (error) return <ErrorBanner error={error} />;
  return (
    <>
      {items.length === 0 ? (
        <Empty>Aucun article sur ce chantier. Enregistrez une première entrée.</Empty>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table className="tbl">
            <thead>
              <tr>
                <th>Article</th>
                <th className="hide-sm">Unité</th>
                <th className="r hide-sm" style={{ textAlign: "right" }}>
                  Entrées
                </th>
                <th className="r hide-sm" style={{ textAlign: "right" }}>
                  Sorties
                </th>
                <th className="r hide-sm" style={{ textAlign: "right" }}>
                  Ajustements
                </th>
                <th className="r" style={{ textAlign: "right" }}>
                  Stock théorique
                </th>
                <th className="hide-sm">Dernier mouvement</th>
                <th>État</th>
              </tr>
            </thead>
            <tbody>
              {items.map((s) => (
                <tr key={s.material_id}>
                  <td style={{ fontWeight: 600 }}>{s.name}</td>
                  <td className="muted hide-sm">{unit(s.unit)}</td>
                  <td className="r num hide-sm">{qty(s.qty_in)}</td>
                  <td className="r num hide-sm">{qty(s.qty_out)}</td>
                  <td className="r num hide-sm">{qty(s.qty_adjust)}</td>
                  <td className="r num" style={{ fontWeight: 700 }}>
                    {qty(s.on_hand)}
                  </td>
                  <td className="muted num hide-sm" style={{ fontSize: 13 }}>
                    {s.last_movement_on ? `${dayMonth(s.last_movement_on)} · ${(MOVEMENT_LABEL[s.last_movement_type ?? ""] ?? "").toLowerCase()}` : "—"}
                  </td>
                  <td>{s.is_low ? <Badge tone="orange">Stock bas</Badge> : s.low_stock_threshold ? <Badge tone="green">Suffisant</Badge> : <Badge>Sans seuil</Badge>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <p className="muted" style={{ margin: 0, padding: "12px 16px 16px", fontSize: 12, borderTop: "1px solid #F2F2F7" }}>
        Stock théorique = entrées − sorties ± ajustements. Ce n&apos;est pas un inventaire physique : un écart avec le comptage sur site signale une perte, une erreur de saisie ou une consommation non déclarée.
      </p>
    </>
  );
}

function Move({ move: m }: { move: StockHistoryRow }) {
  const style = m.movement_type === "in" ? { background: "#E8F5EC", color: "#1A7F45" } : m.movement_type === "out" ? { background: "#F2F2F7", color: "#48484A" } : { background: "#EBF0FD", color: "#1F48C8" };
  return (
    <div className="mv">
      <span className="mi" style={style}>
        {m.movement_type === "in" ? <IconArrowDown size={17} /> : m.movement_type === "out" ? <IconArrowUp size={17} /> : <IconAdjust size={17} />}
      </span>
      <span style={{ flex: 1, minWidth: 0 }}>
        <strong>{MOVEMENT_LABEL[m.movement_type]}</strong> · {qty(m.quantity)} {unit(m.unit)} de {m.material_name}
        <span className="muted" style={{ display: "block", fontSize: 13 }}>
          {[m.reason, m.author_name].filter(Boolean).join(" · ")}
        </span>
      </span>
      <span className="muted num" style={{ fontSize: 13 }}>
        {dayMonth(m.occurred_on)}
      </span>
    </div>
  );
}

function MovementForm({ project, materials, onDone }: { project: Project; materials: Material[]; onDone: () => void }) {
  const online = useOnline();
  const [type, setType] = useState<"in" | "out" | "adjustment">("in");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [formKey, setFormKey] = useState(0);
  const readOnly = project.status === "archived";

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setError(null);
    try {
      const movement = await api<{ id: string }>(`/projects/${project.id}/stock-movements`, {
        method: "POST",
        body: {
          movement_type: type,
          occurred_on: form.get("occurred_on"),
          reason: form.get("reason") || null,
          items: [{ material_id: form.get("material_id"), quantity: String(form.get("quantity")).replace(",", ".") }],
        },
      });
      await api(`/stock-movements/${movement.id}/post`, { method: "POST" });
      setFormKey((k) => k + 1);
      onDone();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="card" style={{ flex: "1 1 320px", minWidth: 0, padding: 20, display: "flex", flexDirection: "column", gap: 16 }} aria-labelledby="h-new">
      <h2 id="h-new" className="sec-t">
        Nouveau mouvement
      </h2>
      <div className="seg3" role="group" aria-label="Type de mouvement">
        {(
          [
            ["in", "Entrée"],
            ["out", "Sortie"],
            ["adjustment", "Ajust."],
          ] as const
        ).map(([value, label]) => (
          <button key={value} type="button" className={type === value ? "on" : ""} aria-pressed={type === value} onClick={() => setType(value)}>
            {label}
          </button>
        ))}
        <button type="button" disabled title="Bientôt disponible">
          Transfert
        </button>
      </div>
      {!online ? (
        <p style={{ margin: 0, fontSize: 14, color: "#9A5200" }}>Connexion requise : un mouvement posté vérifie le stock disponible.</p>
      ) : (
        <form key={formKey} onSubmit={submit} noValidate style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          <Select label="Article" name="material_id" required defaultValue="" error={fieldError(error, "items")}>
            <option value="">Choisir…</option>
            {materials.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name} ({unit(m.unit)})
              </option>
            ))}
          </Select>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 12 }}>
            <Input label="Quantité" name="quantity" inputMode="decimal" required hint={type === "adjustment" ? "Négative pour retirer." : undefined} />
            <Input label="Date" name="occurred_on" type="date" required defaultValue={today()} max={today()} error={fieldError(error, "occurred_on")} />
          </div>
          <div className="f">
            <label htmlFor="mmv">
              Motif{type === "adjustment" && <span className="req"> *</span>}
            </label>
            <textarea id="mmv" name="reason" className="inp" style={{ height: 70 }} />
            {fieldError(error, "reason") && <span className="field-err">{fieldError(error, "reason")}</span>}
          </div>
          <ErrorBanner error={error} />
          <button type="submit" className="btn btn-p" style={{ height: 42 }} disabled={busy || readOnly}>
            Enregistrer le mouvement
          </button>
        </form>
      )}
    </section>
  );
}

function Catalog({ onChange }: { onChange: () => void }) {
  const { user } = useSession();
  const [archived, setArchived] = useState(false);
  const [q, setQ] = useState("");
  const query = new URLSearchParams();
  if (archived) query.set("include_archived", "true");
  if (q.trim()) query.set("q", q.trim());
  const { data, error, loading, reload } = useResource<Items<Material>>(`/materials?${query}`);
  const [actionError, setActionError] = useState<unknown>(null);
  const [formKey, setFormKey] = useState(0);
  const editable = can.manageCatalog(user.role);

  function changed() {
    reload();
    onChange();
  }

  async function create(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setActionError(null);
    try {
      const threshold = String(form.get("low_stock_threshold") ?? "").replace(",", ".").trim();
      await api("/materials", { method: "POST", body: { name: form.get("name"), unit: form.get("unit"), sku: form.get("sku") || null, low_stock_threshold: threshold || null } });
      setFormKey((k) => k + 1);
      changed();
    } catch (err) {
      setActionError(err);
    }
  }

  async function archive(id: string) {
    if (!window.confirm("Archiver ce matériau ? Il restera dans l'historique, mais ne pourra plus être mouvementé.")) return;
    setActionError(null);
    try {
      await api(`/materials/${id}/archive`, { method: "POST" });
      changed();
    } catch (err) {
      setActionError(err);
    }
  }

  return (
    <div style={{ padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 14 }}>
      <div style={{ display: "flex", gap: 12, alignItems: "center", flexWrap: "wrap" }}>
        <SearchBox label="Rechercher dans le catalogue" placeholder="Nom ou référence" value={q} onChange={setQ} />
        <label className="ck-line">
          <input type="checkbox" checked={archived} onChange={(e) => setArchived(e.target.checked)} /> Inclure les archivés
        </label>
      </div>
      {loading ? (
        <Loading />
      ) : error ? (
        <ErrorBanner error={error} />
      ) : data?.items.length === 0 ? (
        <Empty>Aucun matériau.</Empty>
      ) : (
        <div>
          {data?.items.map((m) => (
            <div key={m.id} className="li">
              <span style={{ minWidth: 0 }}>
                <span style={{ fontWeight: 600, display: "block" }}>{m.name}</span>
                <span className="muted" style={{ fontSize: 12 }}>
                  {[unit(m.unit), m.sku, m.low_stock_threshold ? `Seuil : ${qty(m.low_stock_threshold)}` : null].filter(Boolean).join(" · ")}
                </span>
              </span>
              {m.archived_at ? (
                <Badge>Archivé</Badge>
              ) : (
                editable && (
                  <Button variant="secondary" style={{ height: 34 }} onClick={() => archive(m.id)}>
                    Archiver
                  </Button>
                )
              )}
            </div>
          ))}
        </div>
      )}
      {editable && (
        <form key={formKey} onSubmit={create} noValidate style={{ display: "flex", flexDirection: "column", gap: 12, borderTop: "1px solid #F2F2F7", paddingTop: 14 }}>
          <h3 style={{ margin: 0, fontSize: 15, fontWeight: 700 }}>Nouveau matériau</h3>
          <div className="g2" style={{ gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))" }}>
            <Input label="Nom" name="name" required placeholder="Ciment CPJ 45" error={fieldError(actionError, "name")} />
            <Select label="Unité" name="unit" required defaultValue="" error={fieldError(actionError, "unit")}>
              <option value="">Choisir…</option>
              {Object.entries(UNIT_LABEL).map(([value, label]) => (
                <option key={value} value={value}>
                  {label}
                </option>
              ))}
            </Select>
            <Input label="Référence" name="sku" error={fieldError(actionError, "sku")} />
            <Input label="Seuil de stock bas" name="low_stock_threshold" inputMode="decimal" error={fieldError(actionError, "low_stock_threshold")} />
          </div>
          <ErrorBanner error={actionError} />
          <div>
            <Button type="submit">Ajouter au catalogue</Button>
          </div>
        </form>
      )}
    </div>
  );
}
