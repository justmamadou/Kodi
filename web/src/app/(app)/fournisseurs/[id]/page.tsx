"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { type FormEvent, type ReactNode, useState } from "react";
import { Badge, Banner, Button, Card, Empty, ErrorBanner, EXPENSE_TONE, fieldError, Input, Loading, PO_TONE, RFQ_TONE, Select, Textarea } from "@/components/ui";
import type { Items, SupplierContact, SupplierDetail, SupplierHistory, SupplierTerms } from "@/lib/api";
import { api } from "@/lib/api";
import { date, dateTime, EXPENSE_STATUS_LABEL, fcfa, LEGAL_FORM_LABEL, METHOD_LABEL, PO_STATUS_LABEL, poRef, RFQ_STATUS_LABEL, rfqRef, SUPPLIER_ACTIVITY_LABEL, today } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div style={{ minWidth: 0 }}>
      <div className="kl">{label}</div>
      <div style={{ marginTop: 4, fontSize: 15, overflowWrap: "anywhere" }}>{children || <span className="muted">—</span>}</div>
    </div>
  );
}

function termsSummary(t: SupplierTerms): string[] {
  return [
    t.payment_method ? `Paiement : ${METHOD_LABEL[t.payment_method] ?? t.payment_method}` : null,
    t.payment_delay_days !== null ? `à ${t.payment_delay_days} jours` : null,
    t.advance_pct !== null ? `acompte ${t.advance_pct} %` : null,
    t.discount_pct !== null ? `remise ${t.discount_pct.replace(".", ",")} %` : null,
    t.delivery_days !== null ? `livraison en ${t.delivery_days} j` : null,
    t.delivery_included === null ? null : t.delivery_included ? "livraison comprise" : "livraison en sus",
    t.minimum_order_amount !== null ? `minimum de commande ${fcfa(t.minimum_order_amount)} FCFA` : null,
    t.credit_limit !== null ? `encours autorisé ${fcfa(t.credit_limit)} FCFA` : null,
  ].filter((x): x is string => x !== null);
}

const num = (v: FormDataEntryValue | null) => {
  const s = String(v ?? "").replace(/\s/g, "");
  return s === "" ? null : Number(s);
};

function ContactForm({ contact, onSave, onCancel, error, busy }: { contact?: SupplierContact; onSave: (body: unknown) => void; onCancel: () => void; error: unknown; busy: boolean }) {
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const f = new FormData(event.currentTarget);
    onSave({ full_name: f.get("full_name"), job_title: f.get("job_title"), phone: f.get("phone"), email: f.get("email"), notes: f.get("notes"), is_primary: f.get("is_primary") === "on" });
  }
  return (
    <form onSubmit={submit} noValidate style={{ display: "flex", flexWrap: "wrap", gap: 12, padding: 16, borderTop: "1px solid #F2F2F7" }}>
      <div style={{ flex: "1 1 200px", minWidth: 0 }}>
        <Input label="Nom" name="full_name" required defaultValue={contact?.full_name} error={fieldError(error, "full_name")} />
      </div>
      <div style={{ flex: "1 1 160px", minWidth: 0 }}>
        <Input label="Fonction" name="job_title" defaultValue={contact?.job_title ?? ""} error={fieldError(error, "job_title")} />
      </div>
      <div style={{ flex: "1 1 160px", minWidth: 0 }}>
        <Input label="Téléphone" name="phone" type="tel" defaultValue={contact?.phone ?? ""} error={fieldError(error, "phone")} />
      </div>
      <div style={{ flex: "1 1 200px", minWidth: 0 }}>
        <Input label="Email" name="email" type="email" defaultValue={contact?.email ?? ""} error={fieldError(error, "email")} />
      </div>
      <label style={{ flex: "1 1 100%", display: "flex", alignItems: "center", gap: 8, fontSize: 14 }}>
        <input type="checkbox" name="is_primary" defaultChecked={contact?.is_primary} style={{ width: 18, height: 18 }} /> Contact principal
      </label>
      <div style={{ flex: "1 1 100%", display: "flex", gap: 8 }}>
        <Button type="submit" disabled={busy}>
          {contact ? "Enregistrer le contact" : "Ajouter le contact"}
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Annuler
        </Button>
      </div>
    </form>
  );
}

export default function SupplierDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { user } = useSession();
  const edit = can.editSupplier(user.role);
  const trade = can.supplierTrade(user.role);
  const { data: s, error, loading, reload } = useResource<SupplierDetail>(`/suppliers/${id}`);
  const history = useResource<SupplierHistory>(trade ? `/suppliers/${id}/history` : null);
  const [showTerms, setShowTerms] = useState(false);
  const termsHistory = useResource<Items<SupplierTerms & { created_by_name: string }>>(trade && showTerms ? `/suppliers/${id}/terms` : null);
  const [contactForm, setContactForm] = useState<string | null>(null); // "new" ou l'id du contact modifié
  const [termsForm, setTermsForm] = useState(false);
  const [actionError, setActionError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  if (loading) return <Loading />;
  if (!s) return <ErrorBanner error={error} />;

  async function act(method: string, path: string, body?: unknown, done?: () => void) {
    setBusy(true);
    setActionError(null);
    try {
      await api(path, { method, body });
      done?.();
      reload();
      if (path.endsWith("/terms")) termsHistory.reload();
    } catch (err) {
      setActionError(err);
    } finally {
      setBusy(false);
    }
  }

  function saveTerms(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const f = new FormData(event.currentTarget);
    const included = String(f.get("delivery_included") ?? "");
    void act(
      "POST",
      `/suppliers/${id}/terms`,
      {
        effective_from: f.get("effective_from") || null,
        payment_method: f.get("payment_method") || null,
        payment_delay_days: num(f.get("payment_delay_days")),
        advance_pct: num(f.get("advance_pct")),
        discount_pct: String(f.get("discount_pct") ?? "").replace(",", ".").trim() || null,
        delivery_days: num(f.get("delivery_days")),
        delivery_included: included === "" ? null : included === "yes",
        minimum_order_amount: num(f.get("minimum_order_amount")),
        credit_limit: num(f.get("credit_limit")),
        notes: f.get("notes") || null,
      },
      () => setTermsForm(false),
    );
  }

  const active = s.is_active;
  const h = history.data;

  return (
    <>
      <div className="wrap">
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/fournisseurs" style={{ fontWeight: 600 }}>
            Fournisseurs
          </Link>
          <span className="muted"> / {s.name}</span>
        </nav>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", marginTop: 10 }}>
          <h1 className="page-h1">{s.name}</h1>
          {!active && <Badge tone="orange">Archivé</Badge>}
        </div>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 14 }}>
          {[s.legal_name, s.legal_form ? LEGAL_FORM_LABEL[s.legal_form] : null, s.city].filter(Boolean).join(" · ") || "Fiche à compléter"}
        </p>
      </div>

      <Card className="wrap p-6">
        {!active && (
          <div style={{ marginBottom: 16 }}>
            <Banner tone="warn">Archivé le {date(s.archived_at)} : hors du répertoire et des nouvelles demandes de prix. Son historique reste consultable.</Banner>
          </div>
        )}
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))", gap: 20 }}>
          <Field label="Raison sociale">{s.legal_name}</Field>
          <Field label="Forme juridique">{s.legal_form ? LEGAL_FORM_LABEL[s.legal_form] : null}</Field>
          <Field label="NINEA">{s.ninea}</Field>
          <Field label="RCCM">{s.rccm}</Field>
          <Field label="Téléphone">{s.phone && <a href={`tel:${s.phone}`}>{s.phone}</a>}</Field>
          <Field label="Email">{s.email && <a href={`mailto:${s.email}`}>{s.email}</a>}</Field>
          <Field label="Adresse">{[s.address, s.city].filter(Boolean).join(", ")}</Field>
          <Field label="Activités">
            {(s.activities ?? []).length > 0 && (
              <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
                {s.activities?.map((a) => (
                  <Badge key={a}>{SUPPLIER_ACTIVITY_LABEL[a] ?? a}</Badge>
                ))}
              </div>
            )}
          </Field>
        </div>
        {s.notes && <p style={{ margin: "16px 0 0", fontSize: 14, whiteSpace: "pre-wrap" }}>{s.notes}</p>}
        {edit && (
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 16 }}>
            {active ? (
              <>
                <Link href={`/fournisseurs/${id}/modifier`} className="btn btn-s">
                  Modifier la fiche
                </Link>
                <Button variant="danger" disabled={busy} onClick={() => void act("POST", `/suppliers/${id}/archive`)}>
                  Archiver
                </Button>
              </>
            ) : (
              <Button disabled={busy} onClick={() => void act("POST", `/suppliers/${id}/restore`)}>
                Restaurer
              </Button>
            )}
          </div>
        )}
        {actionError && !contactForm && !termsForm ? (
          <div style={{ marginTop: 12 }}>
            <ErrorBanner error={actionError} />
          </div>
        ) : null}
      </Card>

      <section className="card wrap" aria-labelledby="h-contacts">
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: 16, flexWrap: "wrap" }}>
          <h2 id="h-contacts" className="sec-t">
            Contacts
          </h2>
          {edit && active && contactForm === null && (
            <Button variant="secondary" onClick={() => setContactForm("new")}>
              Ajouter un contact
            </Button>
          )}
        </div>
        {s.contacts.length === 0 && contactForm !== "new" && <Empty>Aucun contact enregistré.</Empty>}
        {s.contacts.map((c) =>
          contactForm === c.id ? (
            <ContactForm key={c.id} contact={c} busy={busy} error={actionError} onCancel={() => setContactForm(null)} onSave={(body) => void act("PATCH", `/supplier-contacts/${c.id}`, body, () => setContactForm(null))} />
          ) : (
            <div key={c.id} style={{ display: "flex", gap: 12, padding: "12px 16px", borderTop: "1px solid #F2F2F7", flexWrap: "wrap", alignItems: "center" }}>
              <div style={{ flex: "1 1 220px", minWidth: 0 }}>
                <div style={{ fontWeight: 600 }}>
                  {c.full_name} {c.is_primary && <Badge tone="blue">Principal</Badge>}
                </div>
                <div className="muted" style={{ fontSize: 13, overflowWrap: "anywhere" }}>
                  {[c.job_title, c.phone, c.email].filter(Boolean).join(" · ")}
                </div>
              </div>
              <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                {c.phone && (
                  <a className="btn btn-s" href={`tel:${c.phone}`}>
                    Appeler
                  </a>
                )}
                {edit && active && (
                  <>
                    <Button variant="ghost" onClick={() => setContactForm(c.id)}>
                      Modifier
                    </Button>
                    <Button variant="ghost" disabled={busy} onClick={() => void act("DELETE", `/supplier-contacts/${c.id}`)}>
                      Retirer
                    </Button>
                  </>
                )}
              </div>
            </div>
          ),
        )}
        {contactForm === "new" && <ContactForm busy={busy} error={actionError} onCancel={() => setContactForm(null)} onSave={(body) => void act("POST", `/suppliers/${id}/contacts`, body, () => setContactForm(null))} />}
        {actionError && contactForm && !fieldError(actionError, "full_name") && !fieldError(actionError, "phone") && !fieldError(actionError, "email") ? (
          <div style={{ padding: "0 16px 16px" }}>
            <ErrorBanner error={actionError} />
          </div>
        ) : null}
      </section>

      {trade && (
        <section className="card wrap" aria-labelledby="h-terms">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: 16, flexWrap: "wrap" }}>
            <h2 id="h-terms" className="sec-t">
              Conditions commerciales
            </h2>
            {edit && active && !termsForm && (
              <Button variant="secondary" onClick={() => setTermsForm(true)}>
                {s.terms ? "Nouvelles conditions" : "Renseigner les conditions"}
              </Button>
            )}
          </div>
          {s.terms ? (
            <div style={{ padding: "0 16px 16px" }}>
              <p style={{ margin: 0, fontSize: 15 }}>{termsSummary(s.terms).join(" · ") || "—"}</p>
              {s.terms.notes && (
                <p className="muted" style={{ margin: "6px 0 0", fontSize: 14 }}>
                  {s.terms.notes}
                </p>
              )}
              <p className="hint" style={{ margin: "8px 0 0" }}>
                Version {s.terms.version}, en vigueur depuis le {date(s.terms.effective_from)}.{" "}
                {s.terms.version > 1 && (
                  <button type="button" className="btn btn-g" style={{ padding: 0, height: "auto", minHeight: 0, color: "#2B59E6" }} onClick={() => setShowTerms((v) => !v)}>
                    {showTerms ? "Masquer l'historique" : "Voir l'historique"}
                  </button>
                )}
              </p>
            </div>
          ) : (
            !termsForm && <Empty>Aucune condition commerciale enregistrée.</Empty>
          )}
          {showTerms && termsHistory.data && (
            <div style={{ overflowX: "auto" }}>
              <table className="tbl">
                <thead>
                  <tr>
                    <th>Version</th>
                    <th>Conditions</th>
                    <th className="hide-sm">Saisie</th>
                  </tr>
                </thead>
                <tbody>
                  {termsHistory.data.items.map((t) => (
                    <tr key={t.id}>
                      <td className="num">
                        v{t.version} · {date(t.effective_from)}
                      </td>
                      <td style={{ whiteSpace: "normal" }}>{termsSummary(t).join(" · ") || t.notes || "—"}</td>
                      <td className="muted hide-sm" style={{ fontSize: 13 }}>
                        {t.created_by_name}, {dateTime(t.created_at)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {termsForm && (
            <form onSubmit={saveTerms} noValidate style={{ display: "flex", flexWrap: "wrap", gap: 12, padding: 16, borderTop: "1px solid #F2F2F7" }}>
              <div style={{ flex: "1 1 160px", minWidth: 0 }}>
                <Input label="En vigueur à partir du" name="effective_from" type="date" defaultValue={today()} error={fieldError(actionError, "effective_from")} />
              </div>
              <div style={{ flex: "1 1 160px", minWidth: 0 }}>
                <Select label="Moyen de paiement" name="payment_method" defaultValue={s.terms?.payment_method ?? ""}>
                  <option value="">—</option>
                  {Object.entries(METHOD_LABEL).map(([k, label]) => (
                    <option key={k} value={k}>
                      {label}
                    </option>
                  ))}
                </Select>
              </div>
              <div style={{ flex: "1 1 140px", minWidth: 0 }}>
                <Input label="Délai de paiement (jours)" name="payment_delay_days" inputMode="numeric" defaultValue={s.terms?.payment_delay_days ?? ""} error={fieldError(actionError, "payment_delay_days")} />
              </div>
              <div style={{ flex: "1 1 120px", minWidth: 0 }}>
                <Input label="Acompte (%)" name="advance_pct" inputMode="numeric" defaultValue={s.terms?.advance_pct ?? ""} error={fieldError(actionError, "advance_pct")} />
              </div>
              <div style={{ flex: "1 1 120px", minWidth: 0 }}>
                <Input label="Remise (%)" name="discount_pct" inputMode="decimal" defaultValue={s.terms?.discount_pct?.replace(".", ",") ?? ""} error={fieldError(actionError, "discount_pct")} />
              </div>
              <div style={{ flex: "1 1 140px", minWidth: 0 }}>
                <Input label="Délai de livraison (jours)" name="delivery_days" inputMode="numeric" defaultValue={s.terms?.delivery_days ?? ""} error={fieldError(actionError, "delivery_days")} />
              </div>
              <div style={{ flex: "1 1 160px", minWidth: 0 }}>
                <Select label="Livraison" name="delivery_included" defaultValue={s.terms?.delivery_included === null || !s.terms ? "" : s.terms.delivery_included ? "yes" : "no"}>
                  <option value="">—</option>
                  <option value="yes">Comprise</option>
                  <option value="no">En sus</option>
                </Select>
              </div>
              <div style={{ flex: "1 1 180px", minWidth: 0 }}>
                <Input label="Minimum de commande (FCFA)" name="minimum_order_amount" inputMode="numeric" defaultValue={s.terms?.minimum_order_amount ?? ""} error={fieldError(actionError, "minimum_order_amount")} />
              </div>
              <div style={{ flex: "1 1 180px", minWidth: 0 }}>
                <Input label="Encours autorisé (FCFA)" name="credit_limit" inputMode="numeric" defaultValue={s.terms?.credit_limit ?? ""} error={fieldError(actionError, "credit_limit")} />
              </div>
              <div style={{ flex: "1 1 100%", minWidth: 0 }}>
                <Textarea label="Précisions" name="notes" defaultValue={s.terms?.notes ?? ""} />
              </div>
              {actionError && !fieldError(actionError, "payment_method") ? (
                <div style={{ flex: "1 1 100%" }}>
                  <ErrorBanner error={actionError} />
                </div>
              ) : null}
              <div style={{ flex: "1 1 100%", display: "flex", gap: 8 }}>
                <Button type="submit" disabled={busy}>
                  Enregistrer une nouvelle version
                </Button>
                <Button variant="ghost" onClick={() => setTermsForm(false)}>
                  Annuler
                </Button>
              </div>
              <p className="hint" style={{ flex: "1 1 100%", margin: 0 }}>
                Les conditions précédentes restent dans l&apos;historique.
              </p>
            </form>
          )}
        </section>
      )}

      {trade && (
        <section className="card wrap" aria-labelledby="h-hist">
          <h2 id="h-hist" className="sec-t" style={{ padding: 16 }}>
            Historique
          </h2>
          {history.loading ? (
            <Loading />
          ) : !h ? (
            <div style={{ padding: 16 }}>
              <ErrorBanner error={history.error} />
            </div>
          ) : (
            <>
              <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(160px, 1fr))", gap: 20, padding: "0 16px 16px" }}>
                <div>
                  <div className="kl">Consultations</div>
                  <div className="kv">{h.consultations.length}</div>
                </div>
                <div>
                  <div className="kl">Achats enregistrés</div>
                  <div className="kv">{h.totals.purchase_count}</div>
                </div>
                <div>
                  <div className="kl">Approuvé (FCFA)</div>
                  <div className="kv">{fcfa(h.totals.approved_amount)}</div>
                </div>
                <div>
                  <div className="kl">Payé (FCFA)</div>
                  <div className="kv">{fcfa(h.totals.paid_amount)}</div>
                </div>
              </div>
              <h3 className="lbl" style={{ padding: "8px 16px", margin: 0 }}>
                Demandes de prix
              </h3>
              {h.consultations.length === 0 ? (
                <Empty>Jamais consulté.</Empty>
              ) : (
                <div style={{ overflowX: "auto" }}>
                  <table className="tbl">
                    <thead>
                      <tr>
                        <th>Demande</th>
                        <th>Statut</th>
                        <th className="hide-sm">Chantier</th>
                        <th className="text-right">Dernière offre</th>
                      </tr>
                    </thead>
                    <tbody>
                      {h.consultations.map((c) => (
                        <tr key={c.rfq_id}>
                          <td>
                            <Link href={`/achats/${c.rfq_id}`} style={{ fontWeight: 600 }}>
                              {c.title}
                            </Link>
                            <div className="muted" style={{ fontSize: 12 }}>
                              {rfqRef(c.number)} · consulté le {date(c.consulted_at)}
                            </div>
                          </td>
                          <td>
                            <Badge tone={RFQ_TONE[c.status]}>{RFQ_STATUS_LABEL[c.status]}</Badge>
                          </td>
                          <td className="hide-sm">{c.project_name}</td>
                          <td className="num text-right">
                            {c.last_offer_total !== null ? `${fcfa(c.last_offer_total)} FCFA` : <span className="muted">Pas de réponse</span>}
                            {c.offer_count > 1 && (
                              <div className="muted" style={{ fontSize: 12 }}>
                                {c.offer_count} versions
                              </div>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <h3 className="lbl" style={{ padding: "16px 16px 8px", margin: 0 }}>
                Bons de commande
              </h3>
              {h.orders.length === 0 ? (
                <Empty>Aucune commande passée à ce fournisseur.</Empty>
              ) : (
                <div style={{ overflowX: "auto" }}>
                  <table className="tbl">
                    <thead>
                      <tr>
                        <th>Commande</th>
                        <th>Statut</th>
                        <th className="hide-sm">Chantier</th>
                        <th className="text-right">Total TTC (FCFA)</th>
                      </tr>
                    </thead>
                    <tbody>
                      {h.orders.map((o) => (
                        <tr key={o.id}>
                          <td>
                            <Link href={`/commandes/${o.id}`} style={{ fontWeight: 600 }}>
                              {poRef(o.number)}
                            </Link>
                            <div className="muted" style={{ fontSize: 12 }}>
                              {date(o.created_at)}
                            </div>
                          </td>
                          <td>
                            <Badge tone={PO_TONE[o.status]}>{PO_STATUS_LABEL[o.status]}</Badge>
                          </td>
                          <td className="hide-sm">{o.project_name}</td>
                          <td className="num text-right">{fcfa(o.total_amount)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <h3 className="lbl" style={{ padding: "16px 16px 8px", margin: 0 }}>
                Achats (dépenses)
              </h3>
              {h.purchases.length === 0 ? (
                <Empty>Aucun achat enregistré auprès de ce fournisseur.</Empty>
              ) : (
                <div style={{ overflowX: "auto" }}>
                  <table className="tbl">
                    <thead>
                      <tr>
                        <th>Dépense</th>
                        <th>Statut</th>
                        <th className="hide-sm">Chantier</th>
                        <th className="text-right">Montant (FCFA)</th>
                      </tr>
                    </thead>
                    <tbody>
                      {h.purchases.map((p) => (
                        <tr key={p.id}>
                          <td>
                            <Link href={`/depenses/${p.id}`} style={{ fontWeight: 600 }}>
                              {p.description}
                            </Link>
                            <div className="muted" style={{ fontSize: 12 }}>
                              {date(p.expense_date)}
                            </div>
                          </td>
                          <td>
                            <Badge tone={EXPENSE_TONE[p.status]}>{EXPENSE_STATUS_LABEL[p.status]}</Badge>
                          </td>
                          <td className="hide-sm">{p.project_name}</td>
                          <td className="num text-right">{fcfa(p.amount)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <p className="hint" style={{ margin: 0, padding: 16 }}>
                Seuls les chantiers auxquels vous avez accès figurent ici.
              </p>
            </>
          )}
        </section>
      )}
    </>
  );
}
