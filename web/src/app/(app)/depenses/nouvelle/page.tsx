"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { type ChangeEvent, Suspense, useEffect, useRef, useState } from "react";
import { IconBack, IconCamera, IconImage, IconInfo, IconOffline, IconWarn } from "@/components/icons";
import { ErrorBanner, Loading } from "@/components/ui";
import type { Category, Expense, Items, Paged, Project, Supplier } from "@/lib/api";
import { useCurrentProject } from "@/lib/current-project";
import { date, fcfa, METHOD_LABEL, today } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { db } from "@/lib/offline/db";
import { compressPhoto } from "@/lib/offline/photos";
import { enqueue } from "@/lib/offline/sync";
import { syncNow } from "@/lib/offline/useSync";

export default function NewExpensePage() {
  return (
    <Suspense fallback={<Loading />}>
      <ExpenseForm />
    </Suspense>
  );
}

function ExpenseForm() {
  const params = useSearchParams();
  const router = useRouter();
  const online = useOnline();
  const [stored] = useCurrentProject();
  const projects = useResource<Paged<Project>>("/projects?limit=100");
  const categories = useResource<Items<Category>>("/categories");
  const suppliers = useResource<Items<Supplier>>("/suppliers");
  const list = projects.data?.items.filter((p) => p.status !== "archived") ?? [];
  const [picked, setPicked] = useState(params.get("project") ?? "");
  const projectId = picked || list.find((p) => p.id === stored)?.id || list[0]?.id || "";
  const known = useResource<Paged<Expense>>(projectId ? `/expenses?project_id=${projectId}&limit=100` : null);

  const [amount, setAmount] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [description, setDescription] = useState("");
  const [supplier, setSupplier] = useState("");
  const [expenseDate, setExpenseDate] = useState(today());
  const [paid, setPaid] = useState(false);
  const [paidOn, setPaidOn] = useState(today());
  const [method, setMethod] = useState("cash");
  const [comment, setComment] = useState("");
  const [receipt, setReceipt] = useState<{ blob: Blob; name: string; url: string } | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [fields, setFields] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const camera = useRef<HTMLInputElement>(null);
  const file = useRef<HTMLInputElement>(null);
  const receiptUrl = receipt?.url;

  useEffect(
    () => () => {
      if (receiptUrl) URL.revokeObjectURL(receiptUrl);
    },
    [receiptUrl],
  );

  const amountValue = Number(amount.replace(/\D/g, ""));
  const name = supplier.trim().toLowerCase();
  // Avertissement seulement : la décision reste humaine et se prend à la validation.
  const similar = amountValue > 0 ? known.data?.items.find((e) => e.amount === amountValue && e.status !== "rejected" && e.status !== "cancelled" && (!name || e.supplier_name?.toLowerCase() === name)) : undefined;
  const recent = suppliers.data?.items.filter((s) => s.is_active).slice(0, 2) ?? [];

  async function pickReceipt(event: ChangeEvent<HTMLInputElement>) {
    const picked = event.target.files?.[0];
    event.target.value = "";
    if (!picked) return;
    try {
      const blob = await compressPhoto(picked);
      setReceipt({ blob, name: picked.name, url: URL.createObjectURL(blob) });
    } catch (err) {
      setError(err);
    }
  }

  /** La dépense reçoit son identifiant ici : elle existe sur le téléphone avant d'exister sur le serveur. */
  async function save(submit: boolean) {
    const text = description.trim();
    const problems: Record<string, string> = {};
    if (!projectId) problems.project = "Choisissez un chantier.";
    if (amountValue <= 0) problems.amount = "Indiquez un montant en FCFA entiers, supérieur à zéro.";
    if (!categoryId) problems.category = "Choisissez une catégorie.";
    if (text.length < 2) problems.description = "Décrivez la dépense.";
    if (!expenseDate) problems.date = "La date est obligatoire.";
    if (paid && !paidOn) problems.paidOn = "Indiquez la date de paiement.";
    setFields(problems);
    if (Object.keys(problems).length > 0) return;

    setBusy(true);
    setError(null);
    try {
      const id = crypto.randomUUID();
      await enqueue({
        kind: "expense.create",
        label: `Dépense · ${text}`,
        entityId: id,
        method: "POST",
        path: `/api/v1/projects/${projectId}/expenses`,
        body: {
          id,
          category_id: categoryId,
          description: text,
          amount: amountValue,
          expense_date: expenseDate,
          supplier_name: supplier.trim() || null,
          comment: comment.trim() || null,
          declared_paid_on: paid ? paidOn : null,
          declared_payment_method: paid ? method : null,
        },
      });
      if (receipt) {
        const blobId = crypto.randomUUID();
        await db.blobs.put({ id: blobId, blob: receipt.blob, mime: "image/jpeg", name: receipt.name });
        await enqueue({
          kind: "photo.upload",
          label: `Reçu · ${text}`,
          entityId: id,
          method: "POST",
          path: "/api/v1/attachments/presign",
          blobId,
          body: { id: crypto.randomUUID(), expense_id: id, kind: "receipt", mime_type: "image/jpeg", size_bytes: receipt.blob.size, original_name: receipt.name },
        });
      }
      if (submit) {
        await enqueue({ kind: "expense.submit", label: `Soumission · ${text}`, entityId: id, method: "POST", path: `/api/v1/expenses/${id}/submit`, body: {} });
      }
      if (navigator.onLine) await syncNow();
      const waiting = await db.outbox.where("state").anyOf("pending", "failed", "conflict").count();
      router.push(waiting > 0 ? "/ajouter" : `/depenses/${id}`);
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

  const err = (key: string) =>
    fields[key] ? (
      <span role="alert" className="field-err">
        {fields[key]}
      </span>
    ) : null;

  return (
    <main style={{ maxWidth: 560, margin: "0 auto", minHeight: "100dvh", display: "flex", flexDirection: "column" }}>
      <header style={{ display: "flex", alignItems: "center", gap: 8, padding: "12px 8px", background: "#FFFFFF", borderBottom: "1px solid #E5E5EA" }}>
        <Link href="/" aria-label="Retour" style={{ width: 44, height: 44, display: "flex", alignItems: "center", justifyContent: "center", color: "#1D1D1F" }}>
          <IconBack />
        </Link>
        <h1 style={{ margin: 0, fontSize: 18, fontWeight: 700, flex: 1 }}>Nouvelle dépense</h1>
        {!online && (
          <span style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "4px 10px", marginRight: 8, borderRadius: 999, background: "#F2F2F7", fontSize: 12, fontWeight: 600, color: "#48484A" }}>
            <IconOffline size={14} />
            Hors ligne
          </span>
        )}
      </header>
      {!online && (
        <div role="status" className="info" style={{ margin: "12px 16px 0", padding: "12px 14px", borderRadius: 12, fontSize: 14, display: "flex", gap: 10 }}>
          <IconInfo size={20} />
          <span>Pas de réseau : la dépense est gardée sur le téléphone et envoyée automatiquement dès le retour de la connexion.</span>
        </div>
      )}
      <form
        className="m-form"
        noValidate
        onSubmit={(e) => {
          e.preventDefault();
          void save(true);
        }}
        style={{ flex: 1, padding: 16, display: "flex", flexDirection: "column", gap: 20 }}
      >
        <div className="f">
          <label htmlFor="ch">
            Chantier <span className="req">*</span>
          </label>
          <select id="ch" className="inp" value={projectId} onChange={(e) => setPicked(e.target.value)} aria-invalid={fields.project ? true : undefined}>
            {list.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          {err("project")}
        </div>
        <div className="f">
          <label htmlFor="mt">
            Montant <span className="req">*</span>
          </label>
          <div style={{ position: "relative" }}>
            <input id="mt" inputMode="numeric" className="inp" value={amount} onChange={(e) => setAmount(fcfaInput(e.target.value))} aria-invalid={fields.amount ? true : undefined} style={{ height: 60, fontSize: 26, fontWeight: 700, paddingRight: 72, fontVariantNumeric: "tabular-nums" }} />
            <span style={{ position: "absolute", right: 16, top: 19, fontSize: 15, fontWeight: 700, color: "#6E6E73" }}>FCFA</span>
          </div>
          {err("amount")}
        </div>
        <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
          <legend className="lg" style={{ marginBottom: 8 }}>
            Catégorie <span className="req">*</span>
          </legend>
          <div className="chips">
            {categories.data?.items
              .filter((c) => c.is_active)
              .map((c) => (
                <button key={c.id} type="button" className={`chip ${categoryId === c.id ? "on" : ""}`} aria-pressed={categoryId === c.id} onClick={() => setCategoryId(c.id)}>
                  {c.name}
                </button>
              ))}
          </div>
          {err("category")}
        </fieldset>
        <div className="f">
          <label htmlFor="ds">
            Description <span className="req">*</span>
          </label>
          <input id="ds" className="inp" value={description} onChange={(e) => setDescription(e.target.value)} aria-invalid={fields.description ? true : undefined} placeholder="Ciment 50 sacs" />
          {err("description")}
        </div>
        <div className="f">
          <label htmlFor="fo">Fournisseur</label>
          <input id="fo" className="inp" list="fournisseurs" value={supplier} onChange={(e) => setSupplier(e.target.value)} autoComplete="off" />
          <datalist id="fournisseurs">
            {suppliers.data?.items.map((s) => (
              <option key={s.id} value={s.name} />
            ))}
          </datalist>
          {recent.length > 0 && (
            <span style={{ fontSize: 13, color: "#6E6E73" }}>
              Fournisseurs récents :{" "}
              {recent.map((s, i) => (
                <span key={s.id}>
                  {i > 0 && " · "}
                  <button type="button" className="link-btn" onClick={() => setSupplier(s.name)}>
                    {s.name}
                  </button>
                </span>
              ))}
            </span>
          )}
        </div>
        <div className="f">
          <label htmlFor="dt">
            Date de la dépense <span className="req">*</span>
          </label>
          <input id="dt" type="date" className="inp" value={expenseDate} max={today()} onChange={(e) => setExpenseDate(e.target.value)} aria-invalid={fields.date ? true : undefined} />
          {err("date")}
        </div>
        <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
          <legend className="lg" style={{ marginBottom: 8 }}>
            Déjà payée ?
          </legend>
          <div style={{ display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 8 }}>
            <button type="button" className={`chip ${paid ? "on" : ""}`} aria-pressed={paid} onClick={() => setPaid(true)}>
              Oui, payée
            </button>
            <button type="button" className={`chip ${!paid ? "on" : ""}`} aria-pressed={!paid} onClick={() => setPaid(false)}>
              Non, à payer
            </button>
          </div>
        </fieldset>
        {paid && (
          <>
            <div className="f">
              <label htmlFor="dp">
                Date de paiement <span className="req">*</span>
              </label>
              <input id="dp" type="date" className="inp" value={paidOn} max={today()} onChange={(e) => setPaidOn(e.target.value)} />
              {err("paidOn")}
            </div>
            <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
              <legend className="lg" style={{ marginBottom: 8 }}>
                Mode de paiement
              </legend>
              <div className="chips">
                {Object.entries(METHOD_LABEL).map(([value, label]) => (
                  <button key={value} type="button" className={`chip ${method === value ? "on" : ""}`} aria-pressed={method === value} onClick={() => setMethod(value)}>
                    {label}
                  </button>
                ))}
              </div>
            </fieldset>
          </>
        )}
        <div>
          <div className="lg" style={{ marginBottom: 8 }}>
            Photo du reçu ou de la facture
          </div>
          <div style={{ display: "flex", gap: 12, alignItems: "stretch" }}>
            <div role="img" aria-label="Aperçu du reçu photographié" style={{ width: 110, height: 132, borderRadius: 12, background: "#E5E5EA", display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", gap: 6, color: "#48484A", fontSize: 12, fontWeight: 600, flex: "none", overflow: "hidden", textAlign: "center" }}>
              {receipt ? (
                // eslint-disable-next-line @next/next/no-img-element -- aperçu local d'un Blob, sans passer par le serveur
                <img src={receipt.url} alt="" style={{ width: "100%", height: "100%", objectFit: "cover" }} />
              ) : (
                <>
                  <IconImage size={22} />
                  Aucun reçu
                </>
              )}
            </div>
            <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", gap: 8 }}>
              <button type="button" className="btn" onClick={() => camera.current?.click()} style={{ height: 62, border: "1.5px dashed #2B59E6", background: "#F2F5FE", color: "#2B59E6", gap: 8, fontSize: 15 }}>
                <IconCamera size={20} />
                {receipt ? "Reprendre la photo" : "Prendre la photo"}
              </button>
              <button type="button" className="btn" onClick={() => file.current?.click()} style={{ height: 62, borderColor: "#D2D2D7", background: "#FFFFFF", color: "#1D1D1F", fontSize: 15 }}>
                Choisir un fichier
              </button>
              {receipt && <span className="hint">Compressé · {Math.round(receipt.blob.size / 1024)} Ko</span>}
            </div>
          </div>
          <input ref={camera} type="file" accept="image/*" capture="environment" onChange={pickReceipt} aria-label="Prendre le reçu en photo" style={{ display: "none" }} />
          <input ref={file} type="file" accept="image/*" onChange={pickReceipt} aria-label="Photo du reçu ou de la facture" style={{ display: "none" }} />
        </div>
        {similar && (
          <div className="warn" style={{ display: "flex", gap: 10, padding: "12px 14px", borderRadius: 12, fontSize: 14 }}>
            <IconWarn size={20} />
            <span style={{ color: "#1D1D1F" }}>
              Une dépense similaire existe déjà : {fcfa(similar.amount)} FCFA{similar.supplier_name ? `, ${similar.supplier_name}` : ""}, {date(similar.expense_date)}. Vérifiez qu&apos;il ne s&apos;agit pas d&apos;un doublon.
            </span>
          </div>
        )}
        <div className="f">
          <label htmlFor="cm">Commentaire</label>
          <textarea id="cm" className="inp" placeholder="Facultatif" value={comment} onChange={(e) => setComment(e.target.value)} />
        </div>
        <ErrorBanner error={error} />
        <div style={{ marginTop: "auto", display: "grid", gridTemplateColumns: "repeat(2, minmax(0, 1fr))", gap: 10, paddingTop: 8 }}>
          <button type="button" className="mbtn" disabled={busy} onClick={() => save(false)} style={{ borderColor: "#D2D2D7", background: "#FFFFFF", color: "#1D1D1F" }}>
            Brouillon
          </button>
          <button type="submit" className="mbtn" disabled={busy} style={{ background: "#2B59E6", color: "#FFFFFF" }}>
            Soumettre
          </button>
        </div>
      </form>
    </main>
  );
}

function fcfaInput(text: string): string {
  const digits = text.replace(/\D/g, "");
  return digits ? fcfa(Number(digits)) : "";
}
