"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { type FormEvent, useState } from "react";
import { IconCheck, IconCopy } from "@/components/icons";
import { Badge, Banner, Button, Card, Empty, ErrorBanner, fieldError, Input, Loading, RFQ_TONE } from "@/components/ui";
import type { Items, Offer, RFQDetail, Supplier, SupplierLink, WithLinks } from "@/lib/api";
import { api } from "@/lib/api";
import { date, dateTime, fcfa, qty, RFQ_STATUS_LABEL, unitLabel } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";
import { EvaluationPanel } from "./EvaluationPanel";

function whatsapp(link: SupplierLink, message: string): string | null {
  const digits = (link.phone ?? "").replace(/\D/g, "");
  return digits ? `https://wa.me/${digits}?text=${encodeURIComponent(message)}` : null;
}

/** Liens tout juste émis : seul moment où ils sont lisibles, le serveur n'en garde que le hash. */
function LinksPanel({ links, rfq, onClose }: { links: SupplierLink[]; rfq: RFQDetail; onClose: () => void }) {
  const { organization } = useSession();
  const [copied, setCopied] = useState<string | null>(null);

  async function copy(link: SupplierLink) {
    try {
      await navigator.clipboard.writeText(link.url);
      setCopied(link.supplier_id);
    } catch {
      setCopied(null);
    }
  }

  return (
    <Card className="wrap p-6">
      <h2 className="sec-t">Liens à transmettre</h2>
      <p className="muted" style={{ margin: "6px 0 16px", fontSize: 14 }}>
        Chaque lien est personnel. Il ne s&apos;affichera plus après avoir quitté cette page : en cas de perte, émettez-en un nouveau.
      </p>
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        {links.map((l) => {
          const message = `Bonjour, ${organization.name} vous consulte pour la demande de prix ${rfq.reference} : « ${rfq.title} ». Déposez votre offre avant le ${date(rfq.response_deadline)} : ${l.url}`;
          const wa = whatsapp(l, message);
          return (
            <div key={l.supplier_id} style={{ display: "flex", flexDirection: "column", gap: 8, paddingBottom: 16, borderBottom: "1px solid #F2F2F7" }}>
              <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                <strong>{l.supplier_name}</strong>
                {l.emailed ? (
                  <span className="bdg ok" style={{ whiteSpace: "normal", overflowWrap: "anywhere" }}>
                    Envoyé à {l.email}
                  </span>
                ) : (
                  <Badge>À transmettre</Badge>
                )}
              </div>
              <input className="inp" readOnly value={l.url} aria-label={`Lien de ${l.supplier_name}`} onFocus={(e) => e.currentTarget.select()} style={{ fontSize: 13 }} />
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                <button type="button" className="btn btn-s" onClick={() => void copy(l)}>
                  {copied === l.supplier_id ? <IconCheck size={16} /> : <IconCopy size={16} />}
                  {copied === l.supplier_id ? "Copié" : "Copier le lien"}
                </button>
                {wa && (
                  <a className="btn btn-s" href={wa} target="_blank" rel="noopener noreferrer">
                    Envoyer par WhatsApp
                  </a>
                )}
              </div>
            </div>
          );
        })}
      </div>
      <button type="button" className="btn btn-g" style={{ marginTop: 12, padding: 0 }} onClick={onClose}>
        J&apos;ai transmis les liens
      </button>
    </Card>
  );
}

/** Dernière version de chaque fournisseur, comparée ligne à ligne ; le prix le plus bas est signalé. */
function Comparison({ rfq }: { rfq: RFQDetail }) {
  const latest = new Map<string, Offer>();
  for (const o of rfq.offers) if (!latest.has(o.rfq_supplier_id)) latest.set(o.rfq_supplier_id, o);
  const offers = [...latest.values()].sort((a, b) => a.total_amount - b.total_amount);
  if (offers.length === 0) return <Empty>Aucune offre reçue pour le moment.</Empty>;

  const price = (o: Offer, itemId: string) => o.lines.find((l) => l.rfq_item_id === itemId);
  return (
    <div style={{ overflowX: "auto" }}>
      <table className="tbl">
        <thead>
          <tr>
            <th>Ligne</th>
            {offers.map((o) => (
              <th key={o.id} className="text-right" style={{ minWidth: 150 }}>
                {o.supplier_name}
                <div className="muted" style={{ fontSize: 11, fontWeight: 500 }}>
                  version {o.version} · {dateTime(o.submitted_at)}
                </div>
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rfq.items.map((item) => {
            const quoted = offers.map((o) => price(o, item.id)?.unit_price).filter((p): p is number => p !== undefined);
            const best = quoted.length > 1 ? Math.min(...quoted) : undefined;
            return (
              <tr key={item.id}>
                <td>
                  <div style={{ fontWeight: 600 }}>{item.label}</div>
                  <div className="muted" style={{ fontSize: 12 }}>
                    {qty(item.quantity)} {unitLabel(item.unit)}
                  </div>
                </td>
                {offers.map((o) => {
                  const line = price(o, item.id);
                  if (!line) {
                    return (
                      <td key={o.id} className="text-right muted" style={{ fontSize: 13 }}>
                        Non chiffré
                      </td>
                    );
                  }
                  const isBest = best !== undefined && line.unit_price === best;
                  return (
                    <td key={o.id} className="num text-right" style={isBest ? { background: "#E8F5EC" } : undefined}>
                      <div style={{ fontWeight: 600, color: isBest ? "#1A7F45" : undefined }}>{fcfa(line.amount)}</div>
                      <div className="muted" style={{ fontSize: 12 }}>
                        {fcfa(line.unit_price)} / {unitLabel(item.unit)}
                      </div>
                      {line.comment && (
                        <div className="muted" style={{ fontSize: 12, whiteSpace: "normal" }}>
                          {line.comment}
                        </div>
                      )}
                    </td>
                  );
                })}
              </tr>
            );
          })}
          <tr>
            <td style={{ fontWeight: 700 }}>Total (FCFA)</td>
            {offers.map((o) => (
              <td key={o.id} className="num text-right" style={{ fontWeight: 700 }}>
                {fcfa(o.total_amount)}
                {o.lines.length < rfq.items.length && (
                  <div className="muted" style={{ fontSize: 11, fontWeight: 500 }}>
                    {o.lines.length} ligne{o.lines.length > 1 ? "s" : ""} sur {rfq.items.length}
                  </div>
                )}
              </td>
            ))}
          </tr>
          <tr>
            <td className="muted">Conditions</td>
            {offers.map((o) => (
              <td key={o.id} className="text-right muted" style={{ fontSize: 12, whiteSpace: "normal" }}>
                {[o.delivery_days !== null ? `Livraison en ${o.delivery_days} j` : null, o.valid_until ? `Valable jusqu'au ${date(o.valid_until)}` : null, o.payment_terms]
                  .filter(Boolean)
                  .join(" · ") || "—"}
              </td>
            ))}
          </tr>
        </tbody>
      </table>
    </div>
  );
}

export default function RFQDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { user } = useSession();
  const { data: rfq, error, loading, reload } = useResource<RFQDetail>(`/rfqs/${id}`);
  const manage = can.manageRFQ(user.role);
  const directory = useResource<Items<Supplier>>(manage ? "/suppliers" : null);
  const [links, setLinks] = useState<SupplierLink[]>([]);
  const [actionError, setActionError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [cancelling, setCancelling] = useState(false);

  if (loading) return <Loading />;
  if (!rfq) return <ErrorBanner error={error} />;

  async function act(path: string, body?: unknown) {
    setBusy(true);
    setActionError(null);
    try {
      const out = await api<WithLinks | RFQDetail>(`/rfqs/${id}${path}`, { method: "POST", body });
      if ("links" in out) setLinks((current) => [...out.links, ...current.filter((l) => !out.links.some((n) => n.supplier_id === l.supplier_id))]);
      setCancelling(false);
      reload();
    } catch (err) {
      setActionError(err);
    } finally {
      setBusy(false);
    }
  }

  function cancel(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    void act("/cancel", { reason: new FormData(event.currentTarget).get("reason") });
  }

  const deadlinePassed = rfq.status === "published" && !rfq.open;
  const consulted = new Set(rfq.suppliers.map((s) => s.supplier_id));
  const addable = (directory.data?.items ?? []).filter((s) => !consulted.has(s.id));
  const respondents = rfq.suppliers.filter((s) => s.offer_count > 0).length;

  return (
    <>
      <div className="wrap">
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/achats" style={{ fontWeight: 600 }}>
            Demandes de prix
          </Link>
          <span className="muted"> / {rfq.reference}</span>
        </nav>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", marginTop: 10 }}>
          <h1 className="page-h1">{rfq.title}</h1>
          <Badge tone={RFQ_TONE[rfq.status]}>{RFQ_STATUS_LABEL[rfq.status]}</Badge>
        </div>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 14 }}>
          {rfq.reference} ·{" "}
          <Link href={`/chantiers/${rfq.project_id}`} style={{ fontWeight: 600 }}>
            {rfq.project_name}
          </Link>{" "}
          · créée par {rfq.created_by_name} le {date(rfq.created_at)}
        </p>
      </div>

      <Card className="wrap p-6">
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: 20 }}>
          <div>
            <div className="kl">Date limite de réponse</div>
            <div className="kv" style={{ fontSize: 18 }}>
              {date(rfq.response_deadline)}
            </div>
          </div>
          <div>
            <div className="kl">Livraison souhaitée</div>
            <div className="kv" style={{ fontSize: 18 }}>
              {date(rfq.delivery_date)}
            </div>
          </div>
          <div>
            <div className="kl">Réponses</div>
            <div className="kv" style={{ fontSize: 18 }}>
              {respondents} / {rfq.suppliers.length}
            </div>
          </div>
          <div>
            <div className="kl">Versions d&apos;offre reçues</div>
            <div className="kv" style={{ fontSize: 18 }}>
              {rfq.offers.length}
            </div>
          </div>
        </div>
        {rfq.notes && (
          <p style={{ margin: "16px 0 0", fontSize: 14, whiteSpace: "pre-wrap" }}>
            <span className="kl">Précisions : </span>
            {rfq.notes}
          </p>
        )}
        <div style={{ display: "flex", flexDirection: "column", gap: 12, marginTop: 16 }}>
          {rfq.status === "cancelled" && <Banner tone="err">Annulée le {date(rfq.cancelled_at)} : {rfq.cancel_reason}</Banner>}
          {rfq.status === "closed" && <Banner tone="ok">Clôturée le {date(rfq.closed_at)}. Les offres reçues restent consultables ci-dessous.</Banner>}
          {deadlinePassed && <Banner tone="warn">Date limite dépassée : les fournisseurs ne peuvent plus déposer d&apos;offre. Clôturez la consultation pour l&apos;archiver.</Banner>}
          {rfq.status === "draft" && <Banner tone="info">Brouillon : les fournisseurs ne voient rien tant que la demande n&apos;est pas publiée.</Banner>}
        </div>
        {manage && (rfq.status === "draft" || rfq.status === "published") && (
          <div className="mt-3 flex flex-wrap gap-2" style={{ marginTop: 16 }}>
            {rfq.status === "draft" && (
              <>
                <Button disabled={busy} onClick={() => void act("/publish")}>
                  Publier aux fournisseurs
                </Button>
                <Link href={`/achats/${id}/modifier`} className="btn btn-s">
                  Modifier
                </Link>
              </>
            )}
            {rfq.status === "published" && (
              <Button disabled={busy} onClick={() => void act("/close")}>
                Clôturer la consultation
              </Button>
            )}
            {!cancelling && (
              <Button variant="danger" disabled={busy} onClick={() => setCancelling(true)}>
                Annuler la demande
              </Button>
            )}
          </div>
        )}
        {cancelling && (
          <form onSubmit={cancel} noValidate style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap", marginTop: 16 }}>
            <div style={{ flex: "1 1 260px", minWidth: 0 }}>
              <Input label="Motif de l'annulation" name="reason" required hint="Visible des fournisseurs consultés." error={fieldError(actionError, "reason")} />
            </div>
            <Button type="submit" variant="danger" disabled={busy}>
              Confirmer l&apos;annulation
            </Button>
            <Button type="button" variant="ghost" onClick={() => setCancelling(false)}>
              Retour
            </Button>
          </form>
        )}
        {actionError && !fieldError(actionError, "reason") ? (
          <div style={{ marginTop: 12 }}>
            <ErrorBanner error={actionError} />
          </div>
        ) : null}
      </Card>

      {links.length > 0 && <LinksPanel links={links} rfq={rfq} onClose={() => setLinks([])} />}

      {rfq.status !== "draft" && <EvaluationPanel rfq={rfq} onChange={reload} />}

      {rfq.status !== "draft" && (
        <section className="card wrap" aria-labelledby="h-comp">
          <div style={{ padding: "16px 16px 0" }}>
            <h2 id="h-comp" className="sec-t">
              Comparatif ligne par ligne
            </h2>
            <p className="hint" style={{ margin: "4px 0 12px" }}>
              Dernière version de chaque fournisseur. En vert, le prix unitaire le plus bas de la ligne.
            </p>
          </div>
          <Comparison rfq={rfq} />
        </section>
      )}

      <section className="card wrap" aria-labelledby="h-items">
        <h2 id="h-items" className="sec-t" style={{ padding: 16 }}>
          Lignes demandées
        </h2>
        {rfq.items.length === 0 ? (
          <Empty>Aucune ligne : modifiez le brouillon pour en ajouter.</Empty>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table className="tbl">
              <thead>
                <tr>
                  <th>Désignation</th>
                  <th className="text-right">Quantité</th>
                  <th className="hide-sm">Pour le</th>
                </tr>
              </thead>
              <tbody>
                {rfq.items.map((item) => (
                  <tr key={item.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>
                        {item.label}
                        {item.kind === "service" && (
                          <span className="muted" style={{ fontWeight: 500 }}>
                            {" "}
                            · prestation
                          </span>
                        )}
                      </div>
                      {item.specifications && (
                        <div className="muted" style={{ fontSize: 12 }}>
                          {item.specifications}
                        </div>
                      )}
                    </td>
                    <td className="num text-right">
                      {qty(item.quantity)} {unitLabel(item.unit)}
                    </td>
                    <td className="num hide-sm">{date(item.needed_by)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <section className="card wrap" aria-labelledby="h-sup">
        <h2 id="h-sup" className="sec-t" style={{ padding: 16 }}>
          Fournisseurs consultés
        </h2>
        {rfq.suppliers.length === 0 ? (
          <Empty>Aucun fournisseur choisi.</Empty>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table className="tbl">
              <thead>
                <tr>
                  <th>Fournisseur</th>
                  <th className="hide-sm">Contact</th>
                  <th>Réponse</th>
                  {manage && rfq.status === "published" && rfq.open && (
                    <th>
                      <span className="sr-only">Actions</span>
                    </th>
                  )}
                </tr>
              </thead>
              <tbody>
                {rfq.suppliers.map((s) => {
                  const last = rfq.offers.find((o) => o.rfq_supplier_id === s.id);
                  return (
                    <tr key={s.id}>
                      <td style={{ fontWeight: 600 }}>{s.supplier_name}</td>
                      <td className="muted hide-sm" style={{ fontSize: 13 }}>
                        {[s.supplier_phone, s.supplier_email].filter(Boolean).join(" · ") || "—"}
                      </td>
                      <td>
                        {last ? (
                          <span style={{ fontSize: 13 }}>
                            <Badge tone="green">{fcfa(last.total_amount)} FCFA</Badge>{" "}
                            <span className="muted">
                              {s.offer_count > 1 ? `${s.offer_count} versions` : "1 version"} · {dateTime(last.submitted_at)}
                            </span>
                          </span>
                        ) : rfq.status === "draft" ? (
                          <span className="muted">—</span>
                        ) : (
                          <span className="muted" style={{ fontSize: 13 }}>
                            Pas encore de réponse
                          </span>
                        )}
                      </td>
                      {manage && rfq.status === "published" && rfq.open && (
                        <td className="text-right">
                          <button type="button" className="btn btn-g" disabled={busy} onClick={() => void act(`/suppliers/${s.supplier_id}/link`)}>
                            Nouveau lien
                          </button>
                        </td>
                      )}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        {manage && rfq.status === "published" && rfq.open && addable.length > 0 && (
          <div style={{ padding: 16 }}>
            <select
              className="sel"
              aria-label="Consulter un autre fournisseur"
              value=""
              disabled={busy}
              onChange={(e) => e.target.value && void act(`/suppliers/${e.target.value}`)}
              style={{ width: "100%", maxWidth: 420 }}
            >
              <option value="">+ Consulter un autre fournisseur…</option>
              {addable.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                </option>
              ))}
            </select>
          </div>
        )}
      </section>

      {rfq.offers.length > 0 && (
        <section className="card wrap" aria-labelledby="h-hist">
          <h2 id="h-hist" className="sec-t" style={{ padding: 16 }}>
            Historique des offres
          </h2>
          <div style={{ display: "flex", flexDirection: "column" }}>
            {rfq.offers.map((o) => (
              <details key={o.id} style={{ borderTop: "1px solid #F2F2F7", padding: "12px 16px" }}>
                <summary style={{ cursor: "pointer", display: "flex", gap: 8, flexWrap: "wrap", alignItems: "baseline" }}>
                  <strong>{o.supplier_name}</strong>
                  <span className="muted" style={{ fontSize: 13 }}>
                    version {o.version} · {dateTime(o.submitted_at)}
                    {o.contact_name ? ` · ${o.contact_name}` : ""}
                  </span>
                  <span className="num" style={{ marginLeft: "auto", fontWeight: 600 }}>
                    {fcfa(o.total_amount)} FCFA
                  </span>
                </summary>
                <ul style={{ margin: "10px 0 0", paddingLeft: 18, fontSize: 13 }}>
                  {o.lines.map((l) => {
                    const item = rfq.items.find((i) => i.id === l.rfq_item_id);
                    return (
                      <li key={l.rfq_item_id}>
                        {item?.label} : {qty(l.quantity)} × {fcfa(l.unit_price)} = <span className="num">{fcfa(l.amount)}</span>
                        {l.comment ? ` (${l.comment})` : ""}
                      </li>
                    );
                  })}
                </ul>
                {(o.notes || o.payment_terms || o.delivery_days !== null || o.valid_until) && (
                  <p className="muted" style={{ margin: "8px 0 0", fontSize: 13 }}>
                    {[o.delivery_days !== null ? `Livraison en ${o.delivery_days} j` : null, o.valid_until ? `valable jusqu'au ${date(o.valid_until)}` : null, o.payment_terms, o.notes].filter(Boolean).join(" · ")}
                  </p>
                )}
              </details>
            ))}
          </div>
        </section>
      )}
    </>
  );
}
