"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { Banner, ErrorBanner, Loading } from "@/components/ui";
import type { SupplierDetail } from "@/lib/api";
import { useResource } from "@/lib/hooks";
import { SupplierForm } from "../../SupplierForm";

export default function EditSupplierPage() {
  const { id } = useParams<{ id: string }>();
  const { data, error, loading } = useResource<SupplierDetail>(`/suppliers/${id}`);
  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  return (
    <>
      <div className="wrap">
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/fournisseurs" style={{ fontWeight: 600 }}>
            Fournisseurs
          </Link>
          <span className="muted">
            {" / "}
            <Link href={`/fournisseurs/${id}`}>{data.name}</Link> / Modifier
          </span>
        </nav>
        <h1 className="page-h1" style={{ marginTop: 10 }}>
          Modifier {data.name}
        </h1>
      </div>
      {data.is_active ? (
        <SupplierForm supplier={data} />
      ) : (
        <div className="wrap">
          <Banner tone="warn">Ce fournisseur est archivé : restaurez-le depuis sa fiche pour le modifier.</Banner>
        </div>
      )}
    </>
  );
}
