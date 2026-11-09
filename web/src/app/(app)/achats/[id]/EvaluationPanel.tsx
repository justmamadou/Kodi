"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import { IconCheck } from "@/components/icons";
import { Badge, Banner, Button, ErrorBanner, fieldError, Input, PO_TONE } from "@/components/ui";
import type { EvaluatedOffer, PurchaseOrder, RFQDetail } from "@/lib/api";
import { api } from "@/lib/api";
import { date, fcfa, PO_STATUS_LABEL } from "@/lib/format";
import { can, useSession } from "@/lib/session";

function Score({ value }: { value: number }) {
  return (
    <span className="num" style={{ fontWeight: 600 }}>
      {value.toLocaleString("fr-FR", { maximumFractionDigits: 1 })}
    </span>
  );
}

/**
 * Comparaison des dernières offres sur les critères pondérés de la demande, sélection de l'offre et
 * génération du bon de commande.
 */
export function EvaluationPanel({ rfq, onChange }: { rfq: RFQDetail; onChange: () => void }) {
  const router = useRouter();
  const { user } = useSession();
  const ev = rfq.evaluation;
  const [editing, setEditing] = useState(false);
  const [choosing, setChoosing] = useState<EvaluatedOffer | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  if (!ev || ev.offers.length === 0) return null;

  const awarded = rfq.awarded_offer_id !== null;
  const canAward = can.awardRFQ(user.role) && rfq.status === "closed";

  async function call(method: string, path: string, body?: unknown) {
    setBusy(true);
    setError(null);
    try {
      const out = await api<unknown>(path, { method, body });
      setEditing(false);
      setChoosing(null);
      onChange();
      return out;
    } catch (err) {
      setError(err);
      return null;
    } finally {
      setBusy(false);
    }
  }

  function saveCriteria(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const f = new FormData(event.currentTarget);
    void call("PUT", `/rfqs/${rfq.id}/criteria`, {
      weight_price: Number(f.get("weight_price")),
      weight_delivery: Number(f.get("weight_delivery")),
      weight_coverage: Number(f.get("weight_coverage")),
    });
  }

  function award(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!choosing) return;
    void call("POST", `/rfqs/${rfq.id}/award`, { offer_id: choosing.offer_id, reason: new FormData(event.currentTarget).get("reason") || null });
  }

  async function createOrder() {
    const po = (await call("POST", `/rfqs/${rfq.id}/purchase-order`)) as PurchaseOrder | null;
    if (po) router.push(`/commandes/${po.id}`);
  }

  return (
    <section className="card wrap" aria-labelledby="h-eval">
      <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: 12, padding: 16, flexWrap: "wrap" }}>
        <div>
          <h2 id="h-eval" className="sec-t">
            Évaluation des offres
          </h2>
          <p className="hint" style={{ margin: "4px 0 0" }}>
            Prix {ev.weight_price} % · délai {ev.weight_delivery} % · couverture des lignes {ev.weight_coverage} %. Chaque critère est noté sur 100.
          </p>
        </div>
        {can.manageRFQ(user.role) && !awarded && rfq.status !== "cancelled" && !editing && (
          <Button variant="secondary" onClick={() => setEditing(true)}>
            Modifier les critères
          </Button>
        )}
      </div>
      {editing && (
        <form onSubmit={saveCriteria} noValidate style={{ display: "flex", flexWrap: "wrap", gap: 12, padding: "0 16px 16px", alignItems: "flex-end" }}>
          {(
            [
              ["weight_price", "Prix (%)", ev.weight_price],
              ["weight_delivery", "Délai (%)", ev.weight_delivery],
              ["weight_coverage", "Couverture (%)", ev.weight_coverage],
            ] as const
          ).map(([name, label, value]) => (
            <div key={name} style={{ flex: "1 1 120px", minWidth: 0 }}>
              <Input label={label} name={name} inputMode="numeric" defaultValue={value} error={fieldError(error, name)} />
            </div>
          ))}
          <Button type="submit" disabled={busy}>
            Appliquer
          </Button>
          <Button variant="ghost" onClick={() => setEditing(false)}>
            Annuler
          </Button>
        </form>
      )}
      <div style={{ overflowX: "auto" }}>
        <table className="tbl">
          <thead>
            <tr>
              <th>Rang</th>
              <th>Fournisseur</th>
              <th className="text-right">Score</th>
              <th className="text-right hide-sm">Prix</th>
              <th className="text-right hide-sm">Délai</th>
              <th className="text-right hide-sm">Couverture</th>
              <th className="text-right">Total (FCFA)</th>
              {canAward && !rfq.order && (
                <th>
                  <span className="sr-only">Actions</span>
                </th>
              )}
            </tr>
          </thead>
          <tbody>
            {ev.offers.map((o) => (
              <tr key={o.offer_id} style={o.selected ? { background: "#E8F5EC" } : undefined}>
                <td className="num">{o.rank}</td>
                <td>
                  <div style={{ fontWeight: 600 }}>
                    {o.supplier_name} {o.selected && <Badge tone="green">Retenue</Badge>}
                  </div>
                  <div className="muted" style={{ fontSize: 12 }}>
                    {[
                      `v${o.version}`,
                      o.delivery_days !== null ? `livraison en ${o.delivery_days} j` : "délai non communiqué",
                      `${o.quoted_lines}/${o.total_lines} lignes`,
                      o.valid_until ? `valable jusqu'au ${date(o.valid_until)}` : null,
                      o.payment_terms,
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </div>
                  {o.expired && (
                    <div style={{ fontSize: 12, color: "#9A5200", fontWeight: 600 }}>Validité de l&apos;offre dépassée</div>
                  )}
                </td>
                <td className="text-right">
                  <Score value={o.score} />
                </td>
                <td className="text-right hide-sm">
                  <Score value={o.price_score} />
                </td>
                <td className="text-right hide-sm">
                  <Score value={o.delivery_score} />
                </td>
                <td className="text-right hide-sm">
                  <Score value={o.coverage_score} />
                </td>
                <td className="num text-right">{fcfa(o.total_amount)}</td>
                {canAward && !rfq.order && (
                  <td className="text-right">
                    {!o.selected && (
                      <Button variant="secondary" disabled={busy} onClick={() => setChoosing(o)}>
                        Retenir
                      </Button>
                    )}
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 12, padding: 16 }}>
        {rfq.status === "published" && can.awardRFQ(user.role) && <Banner tone="info">Clôturez la consultation pour retenir une offre : plus aucun fournisseur ne pourra alors répondre.</Banner>}
        {choosing && (
          <form onSubmit={award} noValidate style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap" }}>
            <div style={{ flex: "1 1 280px", minWidth: 0 }}>
              <Input label={`Motif du choix de ${choosing.supplier_name}`} name="reason" hint={choosing.rank > 1 ? "Conseillé : cette offre n'est pas la mieux notée." : "Facultatif."} error={fieldError(error, "reason")} />
            </div>
            <Button type="submit" disabled={busy}>
              Retenir cette offre
            </Button>
            <Button variant="ghost" onClick={() => setChoosing(null)}>
              Annuler
            </Button>
          </form>
        )}
        {awarded && (
          <Banner tone="ok" icon={<IconCheck size={18} />}>
            Offre retenue le {date(rfq.awarded_at)}
            {rfq.award_reason ? ` : ${rfq.award_reason}` : "."}
          </Banner>
        )}
        {rfq.order ? (
          <p style={{ margin: 0, fontSize: 14 }}>
            Bon de commande{" "}
            <Link href={`/commandes/${rfq.order.id}`} style={{ fontWeight: 600 }}>
              {rfq.order.reference}
            </Link>{" "}
            <Badge tone={PO_TONE[rfq.order.status]}>{PO_STATUS_LABEL[rfq.order.status]}</Badge>
          </p>
        ) : (
          awarded && (
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
              {can.managePO(user.role) && (
                <Button disabled={busy} onClick={() => void createOrder()}>
                  Générer le bon de commande
                </Button>
              )}
              {can.awardRFQ(user.role) && (
                <Button variant="ghost" disabled={busy} onClick={() => void call("DELETE", `/rfqs/${rfq.id}/award`)}>
                  Retirer la sélection
                </Button>
              )}
            </div>
          )
        )}
        {error && !fieldError(error, "reason") && !fieldError(error, "weight_price") ? <ErrorBanner error={error} /> : null}
        {fieldError(error, "weight_price") && <ErrorBanner error={error} />}
      </div>
    </section>
  );
}
