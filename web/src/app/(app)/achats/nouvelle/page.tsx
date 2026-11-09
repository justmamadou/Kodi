"use client";

import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense } from "react";
import { Loading } from "@/components/ui";
import { RFQForm } from "../RFQForm";

function NewRFQ() {
  const project = useSearchParams().get("project") ?? undefined;
  return <RFQForm projectId={project} />;
}

export default function NewRFQPage() {
  return (
    <>
      <div className="wrap">
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/achats" style={{ fontWeight: 600 }}>
            Demandes de prix
          </Link>
          <span className="muted"> / Nouvelle demande</span>
        </nav>
        <h1 className="page-h1" style={{ marginTop: 10 }}>
          Nouvelle demande de prix
        </h1>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
          Enregistrée en brouillon : vous la relisez, puis vous la publiez aux fournisseurs.
        </p>
      </div>
      <Suspense fallback={<Loading />}>
        <NewRFQ />
      </Suspense>
    </>
  );
}
