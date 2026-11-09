"use client";

import { type FormEvent, type ReactNode, useEffect, useState } from "react";
import { IconAlertCircle, IconCheck, IconHelmet, IconInfo } from "@/components/icons";
import { Badge, Banner, ErrorBanner, fieldError, Loading } from "@/components/ui";
import type { Offer, SupplierPortal } from "@/lib/api";
import { api, ApiError } from "@/lib/api";
import { date, dateTime, fcfa, qty, RFQ_STATUS_LABEL, unitLabel } from "@/lib/format";
import { useLocationPart } from "@/lib/hooks";

interface Draft {
  prices: Record<string, string>;
  comments: Record<string, string>;
}

function draftFrom(offer?: Offer): Draft {
  const d: Draft = { prices: {}, comments: {} };
  for (const l of offer?.lines ?? []) {
    d.prices[l.rfq_item_id] = String(l.unit_price);
    d.comments[l.rfq_item_id] = l.comment ?? "";
  }
  return d;
}

const digits = (v: string) => v.replace(/\D/g, "");

/**
 * Page du fournisseur consulté, sans compte : le jeton arrive dans le fragment (#), jamais envoyé au
 * serveur web, et l'API le reçoit dans le corps des requêtes.
 */
export default function SupplierOfferPage() {
  const hash = useLocationPart("hash");
  const token = hash === null ? null : hash.replace(/^#/, "");
  const [view, setView] = useState<SupplierPortal | null>(null);
  const [loadError, setLoadError] = useState<unknown>(null);
  const [draft, setDraft] = useState<Draft>({ prices: {}, comments: {} });
  const [offerId, setOfferId] = useState(() => (typeof crypto !== "undefined" && "randomUUID" in crypto ? crypto.randomUUID() : ""));
  const [error, setError] = useState<unknown>(null);
  const [sent, setSent] = useState<Offer | null>(null);
  const [busy, setBusy] = useState(false);

  const [tick, setTick] = useState(0);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    api<SupplierPortal>("/supplier/rfq", { method: "POST", body: { token } }).then(
      (v) => {
        if (cancelled) return;
        setView(v);
        setDraft(draftFrom(v.offers[0]));
        setLoadError(null);
      },
      (err: unknown) => !cancelled && setLoadError(err),
    );
    return () => {
      cancelled = true;
    };
  }, [token, tick]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!view || !token) return;
    const form = new FormData(event.currentTarget);
    const lines = view.items
      .filter((i) => digits(draft.prices[i.id] ?? "") !== "")
      .map((i) => ({ rfq_item_id: i.id, unit_price: Number(digits(draft.prices[i.id] ?? "")), comment: draft.comments[i.id] || null }));
    const days = digits(String(form.get("delivery_days") ?? ""));
    setBusy(true);
    setError(null);
    try {
      const offer = await api<Offer>("/supplier/offers", {
        method: "POST",
        body: {
          token,
          id: offerId || undefined,
          lines,
          delivery_days: days === "" ? null : Number(days),
          valid_until: form.get("valid_until") || null,
          payment_terms: form.get("payment_terms") || null,
          contact_name: form.get("contact_name") || null,
          notes: form.get("notes") || null,
        },
      });
      setSent(offer);
      setOfferId(crypto.randomUUID());
      setTick((t) => t + 1);
      window.scrollTo({ top: 0, behavior: "smooth" });
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  const header = (
    <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
      <div style={{ width: 40, height: 40, borderRadius: 10, background: "#FFC21A", color: "#1D1D1F", display: "flex", alignItems: "center", justifyContent: "center" }}>
        <IconHelmet size={22} width={2} />
      </div>
      <div style={{ fontSize: 20, fontWeight: 700 }}>Kodi</div>
    </div>
  );

  const shell = (children: ReactNode) => (
    <main style={{ minHeight: "100dvh", display: "flex", flexDirection: "column", alignItems: "center", padding: "32px 16px 56px", gap: 24 }}>
      {header}
      <div style={{ width: "100%", maxWidth: 880, display: "flex", flexDirection: "column", gap: 20, minWidth: 0 }}>{children}</div>
    </main>
  );

  if (token === null || (token && !view && !loadError)) return shell(<Loading />);
  if (!token || loadError) {
    const gone = !token || (loadError instanceof ApiError && loadError.code === "rfq_link.invalid");
    return shell(
      <section className="card" style={{ padding: 28, display: "flex", flexDirection: "column", gap: 16 }}>
        <h1 style={{ margin: 0, fontSize: 26, fontWeight: 700 }}>Lien non valable</h1>
        {gone ? (
          <Banner tone="err" icon={<IconAlertCircle size={18} />}>
            Ce lien de consultation n&apos;est plus valable : il a peut-être été remplacé par un lien plus récent. Demandez-en un nouveau à l&apos;entreprise qui vous a consulté.
          </Banner>
        ) : (
          <ErrorBanner error={loadError} />
        )}
      </section>,
    );
  }
  if (!view) return shell(<Loading />);

  const total = view.items.reduce((sum, i) => {
    const p = digits(draft.prices[i.id] ?? "");
    return p ? sum + Math.round(Number(i.quantity) * Number(p)) : sum;
  }, 0);

  return shell(
    <>
      <section className="card" style={{ padding: 28, display: "flex", flexDirection: "column", gap: 14 }}>
        <div className="muted" style={{ fontSize: 14 }}>
          {view.organization_name} consulte <strong style={{ color: "#1D1D1F" }}>{view.supplier_name}</strong>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <h1 style={{ margin: 0, fontSize: 26, fontWeight: 700, letterSpacing: "-0.02em" }}>{view.title}</h1>
          <Badge tone={view.open ? "blue" : view.status === "cancelled" ? "red" : "neutral"}>{view.open ? "Ouverte aux offres" : RFQ_STATUS_LABEL[view.status]}</Badge>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(170px, 1fr))", gap: 16 }}>
          <div>
            <div className="kl">Demande</div>
            <div style={{ fontWeight: 600, marginTop: 4 }}>{view.reference}</div>
          </div>
          <div>
            <div className="kl">Réponse avant le</div>
            <div style={{ fontWeight: 600, marginTop: 4 }}>{date(view.response_deadline)} inclus</div>
          </div>
          <div>
            <div className="kl">Livraison souhaitée</div>
            <div style={{ fontWeight: 600, marginTop: 4 }}>{date(view.delivery_date)}</div>
          </div>
          <div>
            <div className="kl">Lieu de livraison</div>
            <div style={{ fontWeight: 600, marginTop: 4 }}>{view.delivery_place}</div>
          </div>
        </div>
        {view.notes && <p style={{ margin: 0, fontSize: 14, whiteSpace: "pre-wrap" }}>{view.notes}</p>}
        {sent && (
          <Banner tone="ok" icon={<IconCheck size={18} />}>
            Offre envoyée (version {sent.version}, {fcfa(sent.total_amount)} FCFA). Vous pouvez la corriger tant que la demande est ouverte : chaque envoi crée une nouvelle version.
          </Banner>
        )}
        {view.status === "cancelled" && <Banner tone="err">Demande annulée : {view.cancel_reason}</Banner>}
        {view.status === "closed" && <Banner tone="info">La consultation est close : elle n&apos;accepte plus d&apos;offre.</Banner>}
        {view.status === "published" && !view.open && <Banner tone="warn">La date limite est passée : la demande n&apos;accepte plus d&apos;offre.</Banner>}
      </section>

      <form onSubmit={submit} noValidate className="card" style={{ padding: 28, display: "flex", flexDirection: "column", gap: 18 }}>
        <div>
          <h2 className="sec-t">{view.open ? "Votre offre" : "Lignes demandées"}</h2>
          {view.open && (
            <p className="hint" style={{ margin: "4px 0 0" }}>
              Prix unitaires en FCFA. Laissez vide une ligne que vous ne fournissez pas.
            </p>
          )}
        </div>
        {view.items.map((item) => {
          const price = digits(draft.prices[item.id] ?? "");
          return (
            <div key={item.id} style={{ display: "flex", flexWrap: "wrap", gap: 12, paddingBottom: 16, borderBottom: "1px solid #F2F2F7", alignItems: "flex-end" }}>
              <div style={{ flex: "3 1 240px", minWidth: 0 }}>
                <div style={{ fontWeight: 600 }}>
                  {item.position}. {item.label}
                </div>
                <div className="muted" style={{ fontSize: 13 }}>
                  {qty(item.quantity)} {unitLabel(item.unit)}
                  {item.needed_by ? ` · pour le ${date(item.needed_by)}` : ""}
                  {item.specifications ? ` · ${item.specifications}` : ""}
                </div>
              </div>
              {view.open && (
                <>
                  <div className="f" style={{ flex: "1 1 140px", minWidth: 0 }}>
                    <label htmlFor={`p-${item.id}`}>Prix unitaire (FCFA)</label>
                    <input
                      id={`p-${item.id}`}
                      className="inp num"
                      inputMode="numeric"
                      value={price ? fcfa(Number(price)) : ""}
                      onChange={(e) => setDraft((d) => ({ ...d, prices: { ...d.prices, [item.id]: digits(e.target.value) } }))}
                      style={{ textAlign: "right" }}
                    />
                  </div>
                  <div style={{ flex: "1 1 120px", minWidth: 0, textAlign: "right", paddingBottom: 10 }} className="num">
                    {price ? `${fcfa(Math.round(Number(item.quantity) * Number(price)))} FCFA` : <span className="muted">—</span>}
                  </div>
                  <div className="f" style={{ flex: "1 1 100%", minWidth: 0 }}>
                    <label htmlFor={`c-${item.id}`}>Commentaire</label>
                    <input id={`c-${item.id}`} className="inp" value={draft.comments[item.id] ?? ""} placeholder="Marque, délai propre à cette ligne…" onChange={(e) => setDraft((d) => ({ ...d, comments: { ...d.comments, [item.id]: e.target.value } }))} />
                  </div>
                </>
              )}
            </div>
          );
        })}
        {view.open && (
          <>
            <div className="num" style={{ display: "flex", justifyContent: "space-between", gap: 12, fontSize: 18, fontWeight: 700, flexWrap: "wrap" }}>
              <span>Total</span>
              <span>{fcfa(total)} FCFA</span>
            </div>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
              <div className="f" style={{ flex: "1 1 180px" }}>
                <label htmlFor="delivery_days">Délai de livraison (jours)</label>
                <input id="delivery_days" name="delivery_days" className="inp num" inputMode="numeric" defaultValue={view.offers[0]?.delivery_days ?? ""} />
                {fieldError(error, "delivery_days") && <span className="field-err">{fieldError(error, "delivery_days")}</span>}
              </div>
              <div className="f" style={{ flex: "1 1 180px" }}>
                <label htmlFor="valid_until">Offre valable jusqu&apos;au</label>
                <input id="valid_until" name="valid_until" type="date" className="inp" defaultValue={view.offers[0]?.valid_until ?? ""} />
              </div>
              <div className="f" style={{ flex: "1 1 100%" }}>
                <label htmlFor="payment_terms">Conditions de paiement</label>
                <input id="payment_terms" name="payment_terms" className="inp" placeholder="Ex. : 50 % à la commande, solde à la livraison" defaultValue={view.offers[0]?.payment_terms ?? ""} />
              </div>
              <div className="f" style={{ flex: "1 1 220px" }}>
                <label htmlFor="contact_name">Votre nom</label>
                <input id="contact_name" name="contact_name" className="inp" autoComplete="name" defaultValue={view.offers[0]?.contact_name ?? ""} />
              </div>
              <div className="f" style={{ flex: "1 1 100%" }}>
                <label htmlFor="notes">Remarques</label>
                <textarea id="notes" name="notes" className="inp" defaultValue={view.offers[0]?.notes ?? ""} />
              </div>
            </div>
            {fieldError(error, "lines") && (
              <span role="alert" className="field-err">
                {fieldError(error, "lines")}
              </span>
            )}
            {!fieldError(error, "lines") && <ErrorBanner error={error} />}
            <button type="submit" className="btn btn-p" disabled={busy} style={{ height: 48, fontSize: 15 }}>
              {busy ? "Envoi…" : view.offers.length > 0 ? "Envoyer une nouvelle version" : "Envoyer mon offre"}
            </button>
            <p className="hint" style={{ margin: 0, display: "flex", gap: 6, alignItems: "flex-start" }}>
              <IconInfo size={14} /> Ce lien vous est personnel. Seule l&apos;entreprise qui vous consulte voit votre offre.
            </p>
          </>
        )}
      </form>

      {view.offers.length > 0 && (
        <section className="card" style={{ padding: 28 }}>
          <h2 className="sec-t" style={{ marginBottom: 12 }}>
            Vos offres envoyées
          </h2>
          {view.offers.map((o) => (
            <div key={o.id} className="num" style={{ display: "flex", justifyContent: "space-between", gap: 12, padding: "8px 0", borderTop: "1px solid #F2F2F7", fontSize: 14, flexWrap: "wrap" }}>
              <span>
                Version {o.version} · {dateTime(o.submitted_at)}
              </span>
              <strong>{fcfa(o.total_amount)} FCFA</strong>
            </div>
          ))}
        </section>
      )}
    </>,
  );
}
