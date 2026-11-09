import Link from "next/link";

export const metadata = { title: "Hors connexion — Kodi" };

export default function OfflinePage() {
  return (
    <main className="mx-auto flex min-h-dvh max-w-md flex-col justify-center gap-3 px-4 text-center">
      <h1 className="text-2xl font-bold">Cette page n&apos;est pas disponible hors connexion</h1>
      <p className="text-sm text-muted">Vous pouvez continuer à saisir vos rapports et dépenses : ils seront envoyés au retour du réseau.</p>
      <p className="flex flex-col gap-2 pt-3 text-sm font-semibold text-action-hover">
        <Link href="/rapports/nouveau">Saisir un rapport</Link>
        <Link href="/depenses/nouvelle">Saisir une dépense</Link>
        <Link href="/synchro">Voir mes envois en attente</Link>
      </p>
    </main>
  );
}
