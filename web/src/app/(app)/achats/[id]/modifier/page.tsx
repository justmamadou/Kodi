"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { Banner, ErrorBanner, Loading } from "@/components/ui";
import type { RFQDetail } from "@/lib/api";
import { useResource } from "@/lib/hooks";
import { RFQForm } from "../../RFQForm";

export default function EditRFQPage() {
  const { id } = useParams<{ id: string }>();
  const { data, error, loading } = useResource<RFQDetail>(`/rfqs/${id}`);
  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  return (
    <>
      <div className="wrap">
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/achats" style={{ fontWeight: 600 }}>
            Demandes de prix
          </Link>
          <span className="muted">
            {" / "}
            <Link href={`/achats/${id}`}>{data.reference}</Link> / Modifier
          </span>
        </nav>
        <h1 className="page-h1" style={{ marginTop: 10 }}>
          Modifier {data.reference}
        </h1>
      </div>
      {data.status === "draft" ? (
        <RFQForm rfq={data} />
      ) : (
        <div className="wrap">
          <Banner tone="warn">Cette demande est publiée : son contenu est figé pour que tous les fournisseurs chiffrent la même chose.</Banner>
        </div>
      )}
    </>
  );
}
