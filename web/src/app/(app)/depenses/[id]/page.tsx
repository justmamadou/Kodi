"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { type FormEvent, useState } from "react";
import { Badge, Button, Card, ErrorBanner, EXPENSE_TONE, fieldError, Input, Loading, Select } from "@/components/ui";
import type { ExpenseDetail } from "@/lib/api";
import { api } from "@/lib/api";
import { date, dateTime, EXPENSE_STATUS_LABEL, fcfa, METHOD_LABEL, PAYMENT_STATUS_LABEL, today } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

const EVENT_LABEL: Record<string, string> = {
  submitted: "Soumise",
  approved: "Approuvée",
  rejected: "Rejetée",
  withdrawn: "Retirée",
  reopened: "Reprise pour correction",
  cancelled: "Annulée",
};

export default function ExpenseDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { user } = useSession();
  const online = useOnline();
  const { data: e, error, loading, reload } = useResource<ExpenseDetail>(`/expenses/${id}`);
  const [actionError, setActionError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  if (loading) return <Loading />;
  if (!e) return <ErrorBanner error={error} />;

  async function act(action: string, body?: unknown) {
    setBusy(true);
    setActionError(null);
    try {
      await api(`/expenses/${id}/${action}`, { method: "POST", body });
      reload();
    } catch (err) {
      setActionError(err);
    } finally {
      setBusy(false);
    }
  }

  async function pay(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formEl = event.currentTarget;
    const form = new FormData(formEl);
    setBusy(true);
    setActionError(null);
    try {
      await api(`/expenses/${id}/payments`, {
        method: "POST",
        body: { amount: Number(form.get("amount")), paid_on: form.get("paid_on"), method: form.get("method"), reference: form.get("reference") || null },
      });
      formEl.reset();
      reload();
    } catch (err) {
      setActionError(err);
    } finally {
      setBusy(false);
    }
  }

  async function openAttachment(attachmentId: string) {
    try {
      const { url } = await api<{ url: string }>(`/attachments/${attachmentId}/url`);
      window.open(url, "_blank", "noopener");
    } catch (err) {
      setActionError(err);
    }
  }

  const remaining = e.amount - e.amount_paid;
  const mine = e.created_by === user.id;
  return (
    <>
      <div className="wrap">
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/depenses" style={{ fontWeight: 600 }}>
            Dépenses
          </Link>
          <span className="muted"> / {e.description}</span>
        </nav>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap", marginTop: 10 }}>
          <h1 className="page-h1">{e.description}</h1>
          <Badge tone={EXPENSE_TONE[e.status]}>{EXPENSE_STATUS_LABEL[e.status]}</Badge>
        </div>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 14 }}>
          {[e.project_name, e.category_name, e.supplier_name, `Dépense du ${date(e.expense_date)}`, e.created_by_name].filter(Boolean).join(" · ")}
        </p>
      </div>
      <Card className="wrap p-6">
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(170px, 1fr))", gap: 20 }}>
          <div>
            <div className="kl">Montant (FCFA)</div>
            <div className="kv">{fcfa(e.amount)}</div>
          </div>
          <div>
            <div className="kl">Payé</div>
            <div className="kv">{fcfa(e.amount_paid)}</div>
          </div>
          <div>
            <div className="kl">Reste à payer</div>
            <div className="kv" style={remaining > 0 && e.status === "approved" ? { color: "#9A5200" } : undefined}>
              {fcfa(remaining)}
            </div>
          </div>
        </div>
        <p className="mt-3 text-sm text-muted">{PAYMENT_STATUS_LABEL[e.payment_status]}</p>
        {e.comment && <p className="mt-2 text-sm">Commentaire : {e.comment}</p>}
        {e.decision_comment && <p className="mt-2 text-sm">Motif : {e.decision_comment}</p>}
        <div className="mt-3 flex flex-wrap gap-2">
          {e.status === "draft" && mine && (
            <Button disabled={busy} onClick={() => act("submit")}>
              Soumettre
            </Button>
          )}
          {(e.status === "rejected" || e.status === "pending") && mine && (
            <Button variant="secondary" disabled={busy} onClick={() => act("back-to-draft")}>
              {e.status === "rejected" ? "Corriger" : "Retirer"}
            </Button>
          )}
          {e.status === "approved" && e.amount_paid === 0 && can.pay(user.role) && (
            <Button variant="danger" disabled={busy || !online} onClick={() => act("cancel", {})}>
              Annuler la dépense
            </Button>
          )}
        </div>
        {actionError ? (
          <div className="mt-3">
            <ErrorBanner error={actionError} />
          </div>
        ) : null}
      </Card>

      {e.status === "approved" && remaining > 0 && can.pay(user.role) && (
        <Card className="wrap p-6">
          <h2 className="mb-3 text-lg font-bold">Enregistrer un paiement</h2>
          {online ? (
            <form onSubmit={pay} noValidate className="grid gap-3 sm:grid-cols-2">
              <Input label="Montant (FCFA)" name="amount" inputMode="numeric" required defaultValue={remaining} hint={`Reste à payer : ${fcfa(remaining)}`} error={fieldError(actionError, "amount")} />
              <Input label="Date" name="paid_on" type="date" required defaultValue={today()} max={today()} error={fieldError(actionError, "paid_on")} />
              <Select label="Moyen de paiement" name="method" required defaultValue="cash">
                {Object.entries(METHOD_LABEL).map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </Select>
              <Input label="Référence" name="reference" hint="N° de transaction Wave ou OM, n° de chèque." />
              <div className="sm:col-span-2">
                <Button type="submit" disabled={busy}>
                  Enregistrer
                </Button>
              </div>
            </form>
          ) : (
            <p className="text-sm text-warn-ink">Connexion requise pour enregistrer un paiement.</p>
          )}
        </Card>
      )}

      {e.payments.length > 0 && (
        <Card className="wrap p-6">
          <h2 className="mb-2 text-lg font-bold">Paiements</h2>
          <ul className="divide-y divide-line text-sm">
            {e.payments.map((p) => (
              <li key={p.id} className="flex justify-between gap-3 py-2">
                <span>
                  {date(p.paid_on)} · {METHOD_LABEL[p.method]}
                  {p.reference ? ` · ${p.reference}` : ""} · {p.recorded_by_name}
                </span>
                <span className="tabular font-semibold">{fcfa(p.amount)}</span>
              </li>
            ))}
          </ul>
        </Card>
      )}

      {e.attachments.length > 0 && (
        <Card className="wrap p-6">
          <h2 className="mb-2 text-lg font-bold">Justificatifs</h2>
          <ul className="flex flex-col gap-1 text-sm">
            {e.attachments.map((a) => (
              <li key={a.id}>
                <button type="button" className="min-h-10 font-semibold text-action-hover" onClick={() => openAttachment(a.id)}>
                  {a.original_name ?? "Justificatif"}
                </button>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <Card className="wrap p-6">
        <h2 className="mb-2 text-lg font-bold">Historique</h2>
        <ol className="flex flex-col gap-2 text-sm">
          {e.approvals.length === 0 && <li className="text-muted">Brouillon, pas encore soumis.</li>}
          {e.approvals.map((a) => (
            <li key={a.id}>
              <span className="font-semibold">{EVENT_LABEL[a.event] ?? a.event}</span> · {a.actor_name} · {dateTime(a.created_at)}
              {a.comment && <span className="block text-muted">{a.comment}</span>}
            </li>
          ))}
        </ol>
      </Card>
    </>
  );
}
