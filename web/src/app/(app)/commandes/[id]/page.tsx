"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { type FormEvent, useState } from "react";
import { Badge, Banner, Button, Card, Empty, ErrorBanner, fieldError, Input, Loading, PO_TONE, Textarea } from "@/components/ui";
import type { Items, Material, POLine, PurchaseOrder } from "@/lib/api";
import { api } from "@/lib/api";
import { date, dateTime, fcfa, PO_EVENT_LABEL, PO_STATUS_LABEL, qty, today, unitLabel } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

interface DraftLine {
  key: string;
  rfq_item_id: string | null;
  material_id: string;
  label: string;
  unit: string;
  quantity: string;
  unit_price: string;
  tax_rate: string;
  comment: string | null;
}

const toDraft = (l: POLine): DraftLine => ({
  key: l.id,
  rfq_item_id: l.rfq_item_id,
  material_id: l.material_id ?? "",
  label: l.label,
  unit: l.unit,
  quantity: l.quantity_ordered.replace(".", ","),
  unit_price: String(l.unit_price ?? 0),
  tax_rate: (l.tax_rate ?? "0").replace(".", ","),
  comment: l.comment,
});

const dec = (v: string) => v.replace(/\s/g, "").replace(",", ".");

/** Brouillon : lignes, taxes, rattachement au catalogue et conditions, enregistrés en une fois. */
function DraftEditor({ po, onSaved }: { po: PurchaseOrder; onSaved: () => void }) {
  const materials = useResource<Items<Material>>("/materials");
  const [lines, setLines] = useState<DraftLine[]>(() => po.lines.map(toDraft));
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const catalog = (materials.data?.items ?? []).filter((m) => !m.archived_at);
  const update = (key: string, patch: Partial<DraftLine>) => setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)));

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const f = new FormData(event.currentTarget);
    setBusy(true);
    setError(null);
    try {
      await api(`/purchase-orders/${po.id}`, {
        method: "PUT",
        body: {
          expected_delivery_date: f.get("expected_delivery_date") || null,
          delivery_address: f.get("delivery_address") || null,
          payment_terms: f.get("payment_terms") || null,
          notes: f.get("notes") || null,
          lines: lines.map((l) => ({
            rfq_item_id: l.rfq_item_id,
            material_id: l.material_id || null,
            label: l.label,
            unit: l.unit,
            quantity: dec(l.quantity),
            unit_price: Number(l.unit_price.replace(/\D/g, "") || 0),
            tax_rate: dec(l.tax_rate) || "0",
            comment: l.comment,
          })),
        },
      });
      onSaved();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  const ht = lines.reduce((s, l) => s + Math.round(Number(dec(l.quantity)) * Number(l.unit_price || 0)), 0);
  const tax = lines.reduce((s, l) => s + Math.round((Math.round(Number(dec(l.quantity)) * Number(l.unit_price || 0)) * Number(dec(l.tax_rate) || 0)) / 100), 0);

  return (
    <form onSubmit={save} noValidate className="card wrap" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 16 }}>
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12, flexWrap: "wrap", alignItems: "center" }}>
        <h2 className="sec-t">Lignes et conditions (brouillon)</h2>
        <button type="button" className="btn btn-s" onClick={() => setLines((ls) => ls.map((l) => ({ ...l, tax_rate: "18" })))}>
          Appliquer la TVA 18 % partout
        </button>
      </div>
      {lines.map((l, i) => (
        <fieldset key={l.key} style={{ border: "1px solid #E5E5EA", borderRadius: 12, padding: 14, margin: 0, display: "flex", flexWrap: "wrap", gap: 12, minWidth: 0 }}>
          <legend className="lbl" style={{ padding: "0 6px" }}>
            {l.label}
          </legend>
          <div className="f" style={{ flex: "1 1 110px", minWidth: 0 }}>
            <label htmlFor={`q-${l.key}`}>Quantité ({unitLabel(l.unit)})</label>
            <input id={`q-${l.key}`} className="inp num" inputMode="decimal" value={l.quantity} onChange={(e) => update(l.key, { quantity: e.target.value })} />
            {fieldError(error, `lines.${i}.quantity`) && <span className="field-err">{fieldError(error, `lines.${i}.quantity`)}</span>}
          </div>
          <div className="f" style={{ flex: "1 1 130px", minWidth: 0 }}>
            <label htmlFor={`p-${l.key}`}>Prix unitaire HT (FCFA)</label>
            <input id={`p-${l.key}`} className="inp num" inputMode="numeric" value={l.unit_price} onChange={(e) => update(l.key, { unit_price: e.target.value })} />
          </div>
          <div className="f" style={{ flex: "1 1 90px", minWidth: 0 }}>
            <label htmlFor={`t-${l.key}`}>Taxe (%)</label>
            <input id={`t-${l.key}`} className="inp num" inputMode="decimal" value={l.tax_rate} onChange={(e) => update(l.key, { tax_rate: e.target.value })} />
          </div>
          {l.unit !== "forfait" && l.unit !== "jour" && l.unit !== "heure" && (
            <div className="f" style={{ flex: "2 1 200px", minWidth: 0 }}>
              <label htmlFor={`m-${l.key}`}>Article du catalogue (entrée en stock)</label>
              <select id={`m-${l.key}`} className="sel" style={{ width: "100%" }} value={l.material_id} onChange={(e) => update(l.key, { material_id: e.target.value })}>
                <option value="">Non stocké</option>
                {catalog
                  .filter((m) => m.unit === l.unit || m.id === l.material_id)
                  .map((m) => (
                    <option key={m.id} value={m.id}>
                      {m.name}
                    </option>
                  ))}
              </select>
            </div>
          )}
        </fieldset>
      ))}
      <div className="num" style={{ display: "flex", gap: 20, justifyContent: "flex-end", flexWrap: "wrap", fontSize: 14 }}>
        <span>HT {fcfa(ht)}</span>
        <span>Taxes {fcfa(tax)}</span>
        <strong>TTC {fcfa(ht + tax)} FCFA</strong>
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>
        <div style={{ flex: "1 1 160px", minWidth: 0 }}>
          <Input label="Livraison prévue" name="expected_delivery_date" type="date" defaultValue={po.expected_delivery_date ?? ""} />
        </div>
        <div style={{ flex: "2 1 260px", minWidth: 0 }}>
          <Input label="Adresse de livraison" name="delivery_address" defaultValue={po.delivery_address ?? ""} error={fieldError(error, "delivery_address")} />
        </div>
        <div style={{ flex: "1 1 100%", minWidth: 0 }}>
          <Input label="Conditions de paiement" name="payment_terms" defaultValue={po.payment_terms ?? ""} error={fieldError(error, "payment_terms")} />
        </div>
        <div style={{ flex: "1 1 100%", minWidth: 0 }}>
          <Textarea label="Notes pour le fournisseur" name="notes" defaultValue={po.notes ?? ""} />
        </div>
      </div>
      <ErrorBanner error={error} />
      <div>
        <Button type="submit" disabled={busy}>
          Enregistrer le brouillon
        </Button>
      </div>
    </form>
  );
}

/** Réception sur le chantier : quantités par ligne, bon de livraison, entrée en stock. */
function ReceiptForm({ po, onSaved }: { po: PurchaseOrder; onSaved: () => void }) {
  const open = po.lines.filter((l) => Number(l.quantity_remaining) > 0);
  const [quantities, setQuantities] = useState<Record<string, string>>(() => Object.fromEntries(open.map((l) => [l.id, l.quantity_remaining.replace(".", ",")])));
  const [receiptId, setReceiptId] = useState(() => crypto.randomUUID());
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const stockable = open.some((l) => l.material_id);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const f = new FormData(event.currentTarget);
    setBusy(true);
    setError(null);
    try {
      await api(`/purchase-orders/${po.id}/receipts`, {
        method: "POST",
        body: {
          id: receiptId,
          received_on: f.get("received_on"),
          delivery_note_ref: f.get("delivery_note_ref") || null,
          notes: f.get("notes") || null,
          to_stock: f.get("to_stock") === "on",
          lines: open.filter((l) => Number(dec(quantities[l.id] ?? "")) > 0).map((l) => ({ line_id: l.id, quantity: dec(quantities[l.id] ?? "") })),
        },
      });
      setReceiptId(crypto.randomUUID());
      onSaved();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} noValidate className="card wrap" style={{ padding: 20, display: "flex", flexDirection: "column", gap: 14 }}>
      <h2 className="sec-t">Enregistrer une réception</h2>
      <p className="hint" style={{ margin: 0 }}>
        Saisissez ce qui est réellement arrivé. Une ligne laissée à 0 reste à recevoir.
      </p>
      {open.map((l) => (
        <div key={l.id} style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap" }}>
          <div style={{ flex: "2 1 200px", minWidth: 0 }}>
            <div style={{ fontWeight: 600 }}>{l.label}</div>
            <div className="muted" style={{ fontSize: 13 }}>
              Reste {qty(l.quantity_remaining)} {unitLabel(l.unit)} sur {qty(l.quantity_ordered)}
            </div>
          </div>
          <div className="f" style={{ flex: "1 1 120px", minWidth: 0 }}>
            <label htmlFor={`r-${l.id}`}>Reçu ({unitLabel(l.unit)})</label>
            <input id={`r-${l.id}`} className="inp num" inputMode="decimal" value={quantities[l.id] ?? ""} onChange={(e) => setQuantities((q) => ({ ...q, [l.id]: e.target.value }))} />
          </div>
        </div>
      ))}
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap" }}>
        <div style={{ flex: "1 1 160px", minWidth: 0 }}>
          <Input label="Date de réception" name="received_on" type="date" defaultValue={today()} required error={fieldError(error, "received_on")} />
        </div>
        <div style={{ flex: "1 1 180px", minWidth: 0 }}>
          <Input label="N° du bon de livraison" name="delivery_note_ref" hint="Une même livraison ne s'enregistre qu'une fois." error={fieldError(error, "delivery_note_ref")} />
        </div>
        <div style={{ flex: "1 1 100%", minWidth: 0 }}>
          <Input label="Remarques" name="notes" placeholder="Sacs abîmés, livraison incomplète…" />
        </div>
      </div>
      {stockable && (
        <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 14 }}>
          <input type="checkbox" name="to_stock" defaultChecked style={{ width: 18, height: 18 }} /> Entrer en stock sur le chantier les articles du catalogue
        </label>
      )}
      <ErrorBanner error={error} />
      <div>
        <Button type="submit" disabled={busy}>
          Enregistrer la réception
        </Button>
      </div>
    </form>
  );
}

export default function PurchaseOrderPage() {
  const { id } = useParams<{ id: string }>();
  const { user } = useSession();
  const { data: po, error, loading, reload } = useResource<PurchaseOrder>(`/purchase-orders/${id}`);
  const [pending, setPending] = useState<{ action: string; label: string; required: boolean } | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  if (loading) return <Loading />;
  if (!po) return <ErrorBanner error={error} />;

  async function act(action: string, body?: unknown) {
    setBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      const out = await api<{ emailed?: boolean }>(`/purchase-orders/${id}/${action}`, { method: "POST", body });
      if (action === "send" && po) setNotice(out.emailed ? `Bon de commande envoyé par email à ${po.supplier_email}.` : "Le fournisseur n'a pas d'email : transmettez-lui le bon de commande (WhatsApp, impression).");
      setPending(null);
      reload();
    } catch (err) {
      setActionError(err);
    } finally {
      setBusy(false);
    }
  }

  function confirm(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) void act(pending.action, { comment: new FormData(event.currentTarget).get("comment") || null });
  }

  const manage = can.managePO(user.role);
  const approver = can.approvePO(user.role);
  const mine = po.created_by === user.id;
  const lastReturn = [...po.events].reverse().find((e) => e.event === "returned");
  const received = po.lines.filter((l) => Number(l.quantity_remaining) === 0).length;
  const cancellable = manage && ["draft", "submitted", "approved", "sent"].includes(po.status);

  return (
    <>
      <div className="wrap">
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/commandes" style={{ fontWeight: 600 }}>
            Commandes
          </Link>
          <span className="muted"> / {po.reference}</span>
        </nav>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", marginTop: 10 }}>
          <h1 className="page-h1">
            {po.reference} · {po.supplier_name}
          </h1>
          <Badge tone={PO_TONE[po.status]}>{PO_STATUS_LABEL[po.status]}</Badge>
        </div>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 14 }}>
          <Link href={`/chantiers/${po.project_id}`} style={{ fontWeight: 600 }}>
            {po.project_name}
          </Link>
          {po.rfq_id && (
            <>
              {" · "}
              <Link href={`/achats/${po.rfq_id}`}>{po.rfq_reference}</Link>
            </>
          )}
          {" · "}préparée par {po.created_by_name} le {date(po.created_at)}
        </p>
      </div>

      <Card className="wrap p-6">
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: 20 }}>
          {po.total_amount !== null && (
            <div>
              <div className="kl">Total TTC (FCFA)</div>
              <div className="kv">{fcfa(po.total_amount)}</div>
            </div>
          )}
          <div>
            <div className="kl">Livraison prévue</div>
            <div className="kv" style={{ fontSize: 18 }}>
              {date(po.expected_delivery_date)}
            </div>
          </div>
          <div>
            <div className="kl">Lignes reçues</div>
            <div className="kv" style={{ fontSize: 18 }}>
              {received} / {po.lines.length}
            </div>
          </div>
          <div style={{ minWidth: 0 }}>
            <div className="kl">Fournisseur</div>
            <div style={{ marginTop: 8, fontSize: 14, overflowWrap: "anywhere" }}>
              <Link href={`/fournisseurs/${po.supplier_id}`} style={{ fontWeight: 600 }}>
                {po.supplier_name}
              </Link>
              <div className="muted">{[po.supplier_phone, po.supplier_email].filter(Boolean).join(" · ")}</div>
            </div>
          </div>
        </div>
        {(po.delivery_address || po.payment_terms || po.notes) && (
          <p style={{ margin: "16px 0 0", fontSize: 14 }}>
            {[po.delivery_address && `Livraison : ${po.delivery_address}`, po.payment_terms && `Paiement : ${po.payment_terms}`, po.notes].filter(Boolean).join(" · ")}
          </p>
        )}
        <div style={{ display: "flex", flexDirection: "column", gap: 12, marginTop: 16 }}>
          {po.status === "cancelled" && <Banner tone="err">Annulée : {po.cancel_reason}</Banner>}
          {po.status === "draft" && lastReturn && <Banner tone="warn">Renvoyée par {lastReturn.actor_name} : {lastReturn.comment}</Banner>}
          {po.status === "submitted" && approver && mine && <Banner tone="info">Vous avez préparé cette commande : une autre personne doit l&apos;approuver.</Banner>}
          {notice && <Banner tone="ok">{notice}</Banner>}
        </div>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 16 }}>
          {po.status === "draft" && manage && (
            <Button disabled={busy} onClick={() => void act("submit")}>
              Soumettre à l&apos;approbation
            </Button>
          )}
          {po.status === "submitted" && approver && !mine && (
            <>
              <Button disabled={busy} onClick={() => setPending({ action: "approve", label: "Commentaire d'approbation", required: false })}>
                Approuver
              </Button>
              <Button variant="secondary" disabled={busy} onClick={() => setPending({ action: "return", label: "Motif du renvoi", required: true })}>
                Renvoyer en brouillon
              </Button>
            </>
          )}
          {po.status === "approved" && manage && (
            <Button disabled={busy} onClick={() => void act("send")}>
              Envoyer au fournisseur
            </Button>
          )}
          {cancellable && (
            <Button variant="danger" disabled={busy} onClick={() => setPending({ action: "cancel", label: "Motif de l'annulation", required: true })}>
              Annuler la commande
            </Button>
          )}
        </div>
        {pending && (
          <form onSubmit={confirm} noValidate style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap", marginTop: 16 }}>
            <div style={{ flex: "1 1 260px", minWidth: 0 }}>
              <Input label={pending.label} name="comment" required={pending.required} error={fieldError(actionError, "comment")} />
            </div>
            <Button type="submit" variant={pending.action === "cancel" ? "danger" : "primary"} disabled={busy}>
              Confirmer
            </Button>
            <Button variant="ghost" onClick={() => setPending(null)}>
              Retour
            </Button>
          </form>
        )}
        {actionError && !fieldError(actionError, "comment") ? (
          <div style={{ marginTop: 12 }}>
            <ErrorBanner error={actionError} />
          </div>
        ) : null}
      </Card>

      {po.status === "draft" && manage && po.prices_visible ? (
        <DraftEditor key={po.events.length} po={po} onSaved={reload} />
      ) : (
        <section className="card wrap" aria-labelledby="h-lines">
          <h2 id="h-lines" className="sec-t" style={{ padding: 16 }}>
            Lignes
          </h2>
          <div style={{ overflowX: "auto" }}>
            <table className="tbl">
              <thead>
                <tr>
                  <th>Désignation</th>
                  <th className="text-right">Commandé</th>
                  <th className="text-right">Reçu</th>
                  <th className="text-right">Restant</th>
                  {po.prices_visible && <th className="text-right hide-sm">PU HT</th>}
                  {po.prices_visible && <th className="text-right hide-sm">Taxe</th>}
                  {po.prices_visible && <th className="text-right">Montant HT</th>}
                </tr>
              </thead>
              <tbody>
                {po.lines.map((l) => (
                  <tr key={l.id}>
                    <td>
                      <div style={{ fontWeight: 600 }}>{l.label}</div>
                      {l.material_id && (
                        <div className="muted" style={{ fontSize: 12 }}>
                          Article du catalogue
                        </div>
                      )}
                    </td>
                    <td className="num text-right">
                      {qty(l.quantity_ordered)} {unitLabel(l.unit)}
                    </td>
                    <td className="num text-right">{qty(l.quantity_received)}</td>
                    <td className="num text-right" style={Number(l.quantity_remaining) > 0 && po.status !== "cancelled" ? { fontWeight: 600, color: "#9A5200" } : undefined}>
                      {qty(l.quantity_remaining)}
                    </td>
                    {po.prices_visible && <td className="num text-right hide-sm">{fcfa(l.unit_price ?? 0)}</td>}
                    {po.prices_visible && <td className="num text-right hide-sm">{(l.tax_rate ?? "0").replace(".", ",")} %</td>}
                    {po.prices_visible && <td className="num text-right">{fcfa(l.amount ?? 0)}</td>}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {po.prices_visible && po.subtotal_amount !== null && (
            <div className="num" style={{ display: "flex", gap: 20, justifyContent: "flex-end", flexWrap: "wrap", padding: 16, fontSize: 14 }}>
              <span>HT {fcfa(po.subtotal_amount)}</span>
              <span>Taxes {fcfa(po.tax_amount ?? 0)}</span>
              <strong>TTC {fcfa(po.total_amount ?? 0)} FCFA</strong>
            </div>
          )}
        </section>
      )}

      {(po.status === "sent" || po.status === "partially_received") && can.receivePO(user.role) && <ReceiptForm key={po.receipts.length} po={po} onSaved={reload} />}

      <section className="card wrap" aria-labelledby="h-rec">
        <h2 id="h-rec" className="sec-t" style={{ padding: 16 }}>
          Réceptions
        </h2>
        {po.receipts.length === 0 ? (
          <Empty>Aucune réception.</Empty>
        ) : (
          po.receipts.map((r) => (
            <div key={r.id} style={{ borderTop: "1px solid #F2F2F7", padding: "12px 16px" }}>
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
                <strong>{date(r.received_on)}</strong>
                {r.delivery_note_ref && <span className="muted">BL {r.delivery_note_ref}</span>}
                <span className="muted">· {r.received_by_name}</span>
                {r.stock_movement_id && <Badge tone="green">Entrée en stock</Badge>}
              </div>
              <ul style={{ margin: "6px 0 0", paddingLeft: 18, fontSize: 14 }}>
                {r.lines.map((rl) => {
                  const line = po.lines.find((l) => l.id === rl.line_id);
                  return (
                    <li key={rl.line_id}>
                      {line?.label} : {qty(rl.quantity)} {line ? unitLabel(line.unit) : ""}
                    </li>
                  );
                })}
              </ul>
              {r.notes && (
                <p className="muted" style={{ margin: "6px 0 0", fontSize: 13 }}>
                  {r.notes}
                </p>
              )}
            </div>
          ))
        )}
      </section>

      <section className="card wrap" aria-labelledby="h-hist">
        <h2 id="h-hist" className="sec-t" style={{ padding: 16 }}>
          Historique
        </h2>
        <ol style={{ listStyle: "none", margin: 0, padding: "0 16px 16px", display: "flex", flexDirection: "column", gap: 10 }}>
          {po.events.map((e) => (
            <li key={e.id} style={{ display: "flex", gap: 12, flexWrap: "wrap", fontSize: 14 }}>
              <span className="num muted" style={{ minWidth: 130 }}>
                {dateTime(e.created_at)}
              </span>
              <span style={{ flex: "1 1 220px", minWidth: 0 }}>
                <strong>{PO_EVENT_LABEL[e.event] ?? e.event}</strong> · {e.actor_name}
                {e.comment ? <span className="muted"> : {e.comment}</span> : null}
              </span>
            </li>
          ))}
        </ol>
      </section>
    </>
  );
}
