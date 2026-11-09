"use client";

import { useLiveQuery } from "dexie-react-hooks";
import { useState } from "react";
import { Badge, Button, Card, Empty, PageTitle, type Tone } from "@/components/ui";
import { dateTime } from "@/lib/format";
import { useOnline } from "@/lib/hooks";
import { db, type OutboxState } from "@/lib/offline/db";
import { discard, keepMine, retry } from "@/lib/offline/sync";
import { syncNow } from "@/lib/offline/useSync";

const STATE: Record<OutboxState, { label: string; tone: Tone }> = {
  pending: { label: "En attente d'envoi", tone: "orange" },
  failed: { label: "Échec", tone: "red" },
  conflict: { label: "Conflit", tone: "red" },
  synced: { label: "Synchronisé", tone: "green" },
};

export default function SyncPage() {
  const online = useOnline();
  const [message, setMessage] = useState<string | null>(null);
  const entries = useLiveQuery(() => db.outbox.orderBy("seq").reverse().toArray(), [], []);

  async function sync() {
    const result = await syncNow();
    if (result.needsLogin) setMessage("Votre session a expiré : reconnectez-vous, vos saisies sont conservées.");
    else if (result.interrupted) setMessage("Le serveur ne répond pas pour le moment. Nouvel essai automatique bientôt.");
    else setMessage(result.sent > 0 ? `${result.sent} élément(s) envoyé(s).` : "Rien de plus à envoyer.");
  }

  return (
    <>
      <PageTitle
        title="Envois"
        subtitle={online ? "L'envoi est automatique dès que le serveur répond." : "Hors connexion : l'envoi reprendra au retour du réseau."}
        action={
          <Button variant="secondary" onClick={sync} disabled={!online}>
            Envoyer maintenant
          </Button>
        }
      />
      {message && (
        <p role="status" className="mb-3 text-sm text-muted">
          {message}
        </p>
      )}
      <Card className="p-6 wrap">
        {entries.length === 0 ? (
          <Empty>Tout est à jour. Vos saisies hors connexion apparaîtront ici.</Empty>
        ) : (
          <ul className="divide-y divide-line">
            {entries.map((e) => (
              <li key={e.seq} className="py-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="min-w-0 font-medium">{e.label}</span>
                  <Badge tone={STATE[e.state].tone}>{STATE[e.state].label}</Badge>
                </div>
                <p className="mt-0.5 text-xs text-muted">Saisi le {dateTime(new Date(e.createdAt).toISOString())}</p>
                {e.error && (
                  <p role="alert" className="mt-1 text-sm text-bad-ink">
                    {e.error}
                  </p>
                )}
                {e.state === "failed" && (
                  <div className="mt-2 flex flex-wrap gap-2">
                    <Button variant="secondary" onClick={() => retry(e.seq!).then(sync)}>
                      Réessayer
                    </Button>
                    <Button variant="danger" onClick={() => discard(e.seq!)}>
                      Abandonner
                    </Button>
                  </div>
                )}
                {e.state === "conflict" && (
                  <>
                    <p className="mt-1 text-sm">Quelqu&apos;un a modifié ce rapport pendant que vous étiez hors connexion. Rien n&apos;a été écrasé : à vous de choisir.</p>
                    <div className="mt-2 flex flex-wrap gap-2">
                      <Button variant="secondary" onClick={() => keepMine(e.seq!).then(sync)}>
                        Garder ma version
                      </Button>
                      <Button variant="danger" onClick={() => discard(e.seq!)}>
                        Garder la version du serveur
                      </Button>
                    </div>
                  </>
                )}
              </li>
            ))}
          </ul>
        )}
      </Card>
    </>
  );
}
