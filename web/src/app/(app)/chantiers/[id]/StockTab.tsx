"use client";

import { type FormEvent, useState } from "react";
import { Badge, Button, Card, Empty, ErrorBanner, fieldError, Input, Loading, Select, type Tone } from "@/components/ui";
import type { Items, Material, Project, StockHistoryRow, StockView } from "@/lib/api";
import { api } from "@/lib/api";
import { date, MOVEMENT_LABEL, today, UNIT_LABEL } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

const TONE: Record<string, Tone> = { in: "green", out: "orange", adjustment: "blue" };

export function StockTab({ project }: { project: Project }) {
  const { user } = useSession();
  const online = useOnline();
  const stock = useResource<StockView>(`/projects/${project.id}/stock`);
  const [material, setMaterial] = useState("");
  const [type, setType] = useState("");
  const query = new URLSearchParams({ limit: "100" });
  if (material) query.set("material_id", material);
  if (type) query.set("movement_type", type);
  const history = useResource<Items<StockHistoryRow>>(`/projects/${project.id}/stock/history?${query}`);
  const materials = useResource<Items<Material>>("/materials");

  function refresh() {
    stock.reload();
    history.reload();
  }

  return (
    <div className="flex flex-col gap-4">
      <Card className="p-6">
        <h2 className="mb-1 text-lg font-bold">Stock théorique</h2>
        <p className="mb-2 text-xs text-muted">Calculé à partir des mouvements enregistrés : ce n&apos;est pas un inventaire physique.</p>
        {stock.loading ? (
          <Loading />
        ) : stock.error ? (
          <ErrorBanner error={stock.error} />
        ) : stock.data?.items.length === 0 ? (
          <Empty>Aucun mouvement sur ce chantier.</Empty>
        ) : (
          <ul className="divide-y divide-line">
            {stock.data?.items.map((s) => (
              <li key={s.material_id} className="flex items-center justify-between gap-3 py-2">
                <span className="min-w-0">
                  <span className="block truncate font-medium">{s.name}</span>
                  {s.last_movement_on && <span className="block text-xs text-muted">Dernier mouvement le {date(s.last_movement_on)}</span>}
                </span>
                <span className="tabular shrink-0 font-bold">
                  {s.on_hand} {UNIT_LABEL[s.unit] ?? s.unit}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {can.moveStock(user.role) && project.status !== "archived" && (
        <Card className="p-6">
          <h2 className="mb-3 text-lg font-bold">Nouveau mouvement</h2>
          {online ? <MovementForm projectId={project.id} materials={materials.data?.items ?? []} onDone={refresh} /> : <p className="text-sm text-warn-ink">Connexion requise : un mouvement posté vérifie le stock disponible.</p>}
        </Card>
      )}

      <Card className="p-6">
        <h2 className="mb-3 text-lg font-bold">Historique</h2>
        <div className="mb-3 grid gap-3 sm:grid-cols-2">
          <Select label="Article" value={material} onChange={(e) => setMaterial(e.target.value)}>
            <option value="">Tous</option>
            {materials.data?.items.map((m) => (
              <option key={m.id} value={m.id}>
                {m.name}
              </option>
            ))}
          </Select>
          <Select label="Type" value={type} onChange={(e) => setType(e.target.value)}>
            <option value="">Tous</option>
            {Object.entries(MOVEMENT_LABEL).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </Select>
        </div>
        {history.data?.items.length === 0 ? (
          <Empty>Aucun mouvement.</Empty>
        ) : (
          <ul className="divide-y divide-line text-sm">
            {history.data?.items.map((h) => (
              <li key={`${h.movement_id}-${h.material_name}`} className="py-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="min-w-0 font-medium">
                    {h.quantity} {UNIT_LABEL[h.unit] ?? h.unit} · {h.material_name}
                  </span>
                  <Badge tone={TONE[h.movement_type]}>{MOVEMENT_LABEL[h.movement_type]}</Badge>
                </div>
                <p className="text-xs text-muted">
                  {date(h.occurred_on)} · {h.author_name}
                  {h.reason ? ` · ${h.reason}` : ""}
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

function MovementForm({ projectId, materials, onDone }: { projectId: string; materials: Material[]; onDone: () => void }) {
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [type, setType] = useState("in");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const formEl = event.currentTarget;
    const form = new FormData(formEl);
    setBusy(true);
    setError(null);
    try {
      const movement = await api<{ id: string }>(`/projects/${projectId}/stock-movements`, {
        method: "POST",
        body: {
          movement_type: type,
          occurred_on: form.get("occurred_on"),
          reason: form.get("reason") || null,
          items: [{ material_id: form.get("material_id"), quantity: String(form.get("quantity")).replace(",", ".") }],
        },
      });
      await api(`/stock-movements/${movement.id}/post`, { method: "POST" });
      formEl.reset();
      onDone();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="grid gap-3 sm:grid-cols-2">
      <Select label="Type" value={type} onChange={(e) => setType(e.target.value)}>
        {Object.entries(MOVEMENT_LABEL).map(([value, label]) => (
          <option key={value} value={value}>
            {label}
          </option>
        ))}
      </Select>
      <Select label="Article" name="material_id" required defaultValue="">
        <option value="">Choisir…</option>
        {materials.map((m) => (
          <option key={m.id} value={m.id}>
            {m.name} ({UNIT_LABEL[m.unit] ?? m.unit})
          </option>
        ))}
      </Select>
      <Input label="Quantité" name="quantity" inputMode="decimal" required hint={type === "adjustment" ? "Négative pour retirer du stock." : undefined} error={fieldError(error, "items")} />
      <Input label="Date" name="occurred_on" type="date" required defaultValue={today()} max={today()} error={fieldError(error, "occurred_on")} />
      <div className="sm:col-span-2">
        <Input label="Motif" name="reason" required={type === "adjustment"} hint={type === "adjustment" ? "Obligatoire pour un ajustement." : undefined} error={fieldError(error, "reason")} />
      </div>
      <div className="flex flex-col gap-2 sm:col-span-2">
        <ErrorBanner error={error} />
        <Button type="submit" disabled={busy}>
          Enregistrer le mouvement
        </Button>
      </div>
    </form>
  );
}
