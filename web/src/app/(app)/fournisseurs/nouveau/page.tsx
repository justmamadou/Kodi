"use client";

import Link from "next/link";
import { SupplierForm } from "../SupplierForm";

export default function NewSupplierPage() {
  return (
    <>
      <div className="wrap">
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/fournisseurs" style={{ fontWeight: 600 }}>
            Fournisseurs
          </Link>
          <span className="muted"> / Nouveau fournisseur</span>
        </nav>
        <h1 className="page-h1" style={{ marginTop: 10 }}>
          Nouveau fournisseur
        </h1>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
          Seul le nom est obligatoire. Les fiches en double sont signalées pendant la saisie.
        </p>
      </div>
      <SupplierForm />
    </>
  );
}
