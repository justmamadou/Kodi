"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { IconDownload, IconImage, IconPlus, IconWarn } from "@/components/icons";
import { Badge, Empty, ErrorBanner, EXPENSE_TONE, Loading } from "@/components/ui";
import type { Category, Expense, ExpenseCounts, ExpenseDetail, Items, Paged, Project } from "@/lib/api";
import { api } from "@/lib/api";
import { date, dateTime, dayMonth, EXPENSE_STATUS_LABEL, fcfa, METHOD_LABEL, monthEnd, monthYear, PAYMENT_STATUS_LABEL, shortName, today } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

type Tab = "pending" | "approved" | "paid" | "rejected" | "all";

const TAB_QUERY: Record<Tab, Record<string, string>> = {
  pending: { status: "pending" },
  approved: { status: "approved", payment: "unpaid" },
  paid: { payment: "paid" },
  rejected: { status: "rejected" },
  all: {},
};

/** Les douze derniers mois, du plus récent au plus ancien. */
function lastMonths(): string[] {
  const [y, m] = today().split("-").map(Number);
  return Array.from({ length: 12 }, (_, i) => {
    const d = new Date(Date.UTC(y ?? 2026, (m ?? 1) - 1 - i, 1));
    return d.toISOString().slice(0, 7);
  });
}

function control(e: Expense) {
  if (e.possible_duplicate) return <Badge tone="orange">Doublon possible</Badge>;
  return e.has_receipt ? <Badge tone="green">Justificatif OK</Badge> : <Badge tone="red">Sans justificatif</Badge>;
}

function statusBadge(e: Expense) {
  if (e.status === "approved" && e.payment_status === "paid") return <Badge tone="green">Payée</Badge>;
  return <Badge tone={EXPENSE_TONE[e.status]}>{EXPENSE_STATUS_LABEL[e.status]}</Badge>;
}

export default function ExpensesPage() {
  const { user } = useSession();
  const approver = can.approve(user.role);
  const [tab, setTab] = useState<Tab>(approver ? "pending" : "all");
  const [projectId, setProjectId] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [month, setMonth] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const panel = useRef<HTMLElement>(null);

  const query = new URLSearchParams({ limit: "100", ...TAB_QUERY[tab] });
  if (projectId) query.set("project_id", projectId);
  if (categoryId) query.set("category_id", categoryId);
  if (month) {
    query.set("date_from", `${month}-01`);
    query.set("date_to", monthEnd(month));
  }
  const { data, error, stale, loading, reload } = useResource<Paged<Expense>>(`/expenses?${query}`);
  const counts = useResource<ExpenseCounts>("/expenses/counts");
  const projects = useResource<Paged<Project>>("/projects?limit=100");
  const categories = useResource<Items<Category>>("/categories");
  const items = data?.items ?? [];
  const selected = items.find((e) => e.id === selectedId) ?? items[0];
  const c = counts.data;

  const tabs: { id: Tab; label: string }[] = [
    { id: "pending", label: `À valider (${c?.pending ?? 0})` },
    { id: "approved", label: `Approuvées (${c?.approved ?? 0})` },
    { id: "paid", label: `Payées (${c?.paid ?? 0})` },
    { id: "rejected", label: `Rejetées (${c?.rejected ?? 0})` },
    { id: "all", label: "Toutes" },
  ];

  function refresh() {
    reload();
    counts.reload();
  }

  function select(id: string) {
    setSelectedId(id);
    if (!window.matchMedia("(min-width: 1024px)").matches) panel.current?.scrollIntoView({ behavior: "smooth", block: "start" });
  }

  return (
    <>
      <div className="wrap" style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h1 className="page-h1">Dépenses</h1>
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
            {stale ? "Dernières données connues : pas de réseau" : c ? `${c.pending} dépense${c.pending > 1 ? "s" : ""} à valider · ${fcfa(c.pending_amount)} FCFA` : " "}
          </p>
        </div>
        <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
          {can.exportData(user.role) && (
            <a href={`/api/v1/exports/expenses.xlsx${projectId ? `?project_id=${projectId}` : ""}`} className="btn btn-s">
              <IconDownload />
              Exporter XLSX
            </a>
          )}
          {can.exportData(user.role) && (
            <a href={projectId ? `/api/v1/exports/projects/${projectId}/report.pdf` : "/api/v1/exports/portfolio.pdf"} className="btn btn-s">
              Rapport PDF
            </a>
          )}
          {can.createExpense(user.role) && !can.exportData(user.role) && (
            <Link href="/depenses/nouvelle" className="btn btn-p">
              <IconPlus size={16} />
              Nouvelle dépense
            </Link>
          )}
        </div>
      </div>
      <div className="wrap" style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12 }}>
        <nav aria-label="Filtrer par statut" style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
          {tabs.map((t) => (
            <button key={t.id} type="button" className={`tab ${tab === t.id ? "on" : ""}`} aria-pressed={tab === t.id} onClick={() => setTab(t.id)}>
              {t.label}
            </button>
          ))}
        </nav>
        <div style={{ marginLeft: "auto", display: "flex", gap: 10, flexWrap: "wrap", maxWidth: "100%" }}>
          <select className="sel" aria-label="Chantier" value={projectId} onChange={(e) => setProjectId(e.target.value)}>
            <option value="">Tous les chantiers</option>
            {projects.data?.items.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <select className="sel" aria-label="Catégorie" value={categoryId} onChange={(e) => setCategoryId(e.target.value)}>
            <option value="">Toutes catégories</option>
            {categories.data?.items.map((cat) => (
              <option key={cat.id} value={cat.id}>
                {cat.name}
              </option>
            ))}
          </select>
          <select className="sel" aria-label="Période" value={month} onChange={(e) => setMonth(e.target.value)}>
            <option value="">Toutes les périodes</option>
            {lastMonths().map((m) => (
              <option key={m} value={m}>
                {monthYear(m)}
              </option>
            ))}
          </select>
        </div>
      </div>
      <div className="wrap" style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
        <section className="card" style={{ flex: "999 1 520px", minWidth: 0 }} aria-label="Liste des dépenses">
          {loading ? (
            <Loading />
          ) : error ? (
            <div style={{ padding: 16 }}>
              <ErrorBanner error={error} />
            </div>
          ) : items.length === 0 ? (
            <Empty>Aucune dépense ici.</Empty>
          ) : (
            <div style={{ overflowX: "auto" }}>
              <table className="tbl">
                <thead>
                  <tr>
                    <th>Date</th>
                    <th>Dépense</th>
                    <th className="hide-sm">Chantier</th>
                    <th className="hide-sm">Saisie par</th>
                    <th className="text-right">Montant (FCFA)</th>
                    <th className="hide-sm">{tab === "pending" ? "Contrôle" : "Statut"}</th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((e) => (
                    <tr key={e.id} className={`click ${selected?.id === e.id ? "sel" : ""}`} aria-selected={selected?.id === e.id} tabIndex={0} onClick={() => select(e.id)} onKeyDown={(k) => k.key === "Enter" && select(e.id)}>
                      <td className="num">{dayMonth(e.expense_date)}</td>
                      <td>
                        <span style={{ fontWeight: 600 }}>{e.description}</span>
                        <div className="muted" style={{ fontSize: 12 }}>
                          {e.category_name}
                          {e.supplier_name ? ` · ${e.supplier_name}` : ""}
                        </div>
                      </td>
                      <td className="hide-sm">{e.project_name}</td>
                      <td className="hide-sm">{shortName(e.created_by_name)}</td>
                      <td className="text-right num" style={{ fontWeight: 600 }}>
                        {fcfa(e.amount)}
                      </td>
                      <td className="hide-sm">{tab === "pending" ? control(e) : statusBadge(e)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </section>
        <section ref={panel} className="card" style={{ flex: "1 1 360px", minWidth: 0, padding: 20, display: "flex", flexDirection: "column", gap: 16, scrollMarginTop: 16 }} aria-labelledby="h-det">
          {selected ? <DecisionPanel key={selected.id} id={selected.id} onChange={refresh} onSelect={setSelectedId} /> : <Empty>Sélectionnez une dépense pour voir son détail.</Empty>}
        </section>
      </div>
    </>
  );
}

function Receipt({ attachment }: { attachment: ExpenseDetail["attachments"][number] }) {
  const [url, setUrl] = useState<string | null>(null);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    let cancelled = false;
    api<{ url: string }>(`/attachments/${attachment.id}/url`).then(
      (r) => !cancelled && setUrl(r.url),
      () => !cancelled && setFailed(true),
    );
    return () => {
      cancelled = true;
    };
  }, [attachment.id]);
  const image = attachment.mime_type.startsWith("image/");
  const frame: React.CSSProperties = { height: 150, borderRadius: 10, background: "#F2F2F7", border: "1px dashed #C7C7CC", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 6, color: "#48484A", fontSize: 13, fontWeight: 600, overflow: "hidden" };
  if (failed) {
    return <div style={frame}>Justificatif indisponible hors connexion</div>;
  }
  return (
    <a href={url ?? undefined} target="_blank" rel="noopener noreferrer" style={frame} aria-label="Agrandir le justificatif">
      {url && image ? (
        // eslint-disable-next-line @next/next/no-img-element -- URL signée et temporaire du stockage objet
        <img src={url} alt="Reçu photographié" style={{ width: "100%", height: "100%", objectFit: "cover" }} />
      ) : (
        <>
          <IconImage size={24} />
          {attachment.original_name ?? "Justificatif"} · Ouvrir
        </>
      )}
    </a>
  );
}

function Dt({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div style={{ display: "flex", justifyContent: "space-between", gap: 16, padding: "7px 0", borderBottom: "1px solid #F2F2F7", fontSize: 14 }}>
      <dt className="muted" style={{ flex: "none" }}>
        {label}
      </dt>
      <dd style={{ margin: 0, textAlign: "right", fontWeight: 500 }}>{children}</dd>
    </div>
  );
}

function DecisionPanel({ id, onChange, onSelect }: { id: string; onChange: () => void; onSelect: (id: string) => void }) {
  const { user } = useSession();
  const online = useOnline();
  const { data: e, error, loading } = useResource<ExpenseDetail>(`/expenses/${id}`);
  const [comment, setComment] = useState("");
  const [actionError, setActionError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  if (loading) return <Loading />;
  if (!e) return <ErrorBanner error={error} />;
  const canDecide = e.status === "pending" && can.approve(user.role);
  const mine = e.created_by === user.id;
  const receipt = e.attachments.find((a) => a.kind === "receipt") ?? e.attachments[0];
  const submitted = e.approvals.find((a) => a.event === "submitted");
  const duplicate = e.duplicates[0];

  async function decide(action: "approve" | "reject") {
    if (action === "reject" && comment.trim() === "") {
      setActionError(new Error("Indiquez le motif du rejet."));
      return;
    }
    setBusy(true);
    setActionError(null);
    try {
      await api(`/expenses/${id}/${action}`, { method: "POST", body: comment.trim() ? { comment: comment.trim() } : {} });
      onChange();
    } catch (err) {
      setActionError(err);
      setBusy(false);
    }
  }

  const declared = e.declared_payment_method ? METHOD_LABEL[e.declared_payment_method] : null;
  return (
    <>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
        <div style={{ minWidth: 0 }}>
          <h2 id="h-det" className="sec-t">
            {e.description}
          </h2>
          <div className="muted" style={{ fontSize: 13, marginTop: 4 }}>
            {submitted ? `Soumise le ${dateTime(submitted.created_at)}` : `Dépense du ${date(e.expense_date)}`}
          </div>
        </div>
        {statusBadge({ ...e, status: e.status })}
      </div>
      <div className="num" style={{ fontSize: 28, fontWeight: 700, letterSpacing: "-0.01em" }}>
        {fcfa(e.amount)} <span style={{ fontSize: 15 }}>FCFA</span>
      </div>
      {receipt ? (
        <Receipt key={receipt.id} attachment={receipt} />
      ) : (
        <div className="err" style={{ padding: "12px 14px", borderRadius: 10, fontSize: 14 }}>
          Aucun justificatif joint à cette dépense.
        </div>
      )}
      <dl style={{ margin: 0 }}>
        <Dt label="Chantier">{e.project_name}</Dt>
        <Dt label="Catégorie">{e.category_name}</Dt>
        {e.supplier_name && <Dt label="Fournisseur">{e.supplier_name}</Dt>}
        <Dt label="Date">{date(e.expense_date)}</Dt>
        <Dt label="Paiement">
          {declared ? `${declared} · ` : ""}
          {e.declared_paid_on && e.payment_status === "unpaid" ? `déclarée payée le ${date(e.declared_paid_on)}` : PAYMENT_STATUS_LABEL[e.payment_status]}
        </Dt>
        <Dt label="Saisie par">{e.created_by_name}</Dt>
      </dl>
      {e.comment && (
        <p style={{ margin: 0, fontSize: 14 }}>
          <span className="muted">Commentaire : </span>
          {e.comment}
        </p>
      )}
      {e.decision_comment && (
        <p style={{ margin: 0, fontSize: 14 }}>
          <span className="muted">Motif : </span>
          {e.decision_comment}
        </p>
      )}
      {duplicate && (
        <div className="warn" style={{ display: "flex", gap: 10, padding: "12px 14px", borderRadius: 10, fontSize: 14 }}>
          <IconWarn size={18} />
          <div style={{ color: "#1D1D1F" }}>
            Une dépense identique existe : même montant, date, fournisseur et chantier.{" "}
            <button type="button" className="link-btn" style={{ fontSize: 14 }} onClick={() => onSelect(duplicate.id)}>
              Comparer les deux
            </button>
          </div>
        </div>
      )}
      {canDecide && !online && <p style={{ margin: 0, fontSize: 13, color: "#9A5200" }}>Connexion requise pour valider une dépense.</p>}
      {canDecide && online && mine && <p style={{ margin: 0, fontSize: 13, color: "#9A5200" }}>Vous avez saisi cette dépense : une autre personne doit la valider.</p>}
      {canDecide && online && !mine && (
        <>
          <div className="f">
            <label htmlFor="com">Commentaire (obligatoire en cas de rejet)</label>
            <textarea id="com" className="inp" style={{ height: 72 }} value={comment} onChange={(ev) => setComment(ev.target.value)} />
          </div>
          <ErrorBanner error={actionError} />
          <div style={{ display: "flex", gap: 10 }}>
            <button type="button" className="btn btn-d" style={{ flex: 1 }} disabled={busy} onClick={() => decide("reject")}>
              Rejeter
            </button>
            <button type="button" className="btn btn-p" style={{ flex: 1 }} disabled={busy} onClick={() => decide("approve")}>
              Approuver
            </button>
          </div>
          <p className="muted" style={{ margin: 0, fontSize: 12 }}>
            Une dépense rejetée reste dans l&apos;historique mais n&apos;entre pas dans les totaux payés.
          </p>
        </>
      )}
      <Link href={`/depenses/${id}`} style={{ fontSize: 13, fontWeight: 600 }}>
        Ouvrir la fiche complète
      </Link>
    </>
  );
}
