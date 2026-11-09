"use client";

import { useState } from "react";
import { IconDoc, IconImage } from "@/components/icons";
import { Card, Empty, ErrorBanner, Loading } from "@/components/ui";
import type { Attachment, Items } from "@/lib/api";
import { api } from "@/lib/api";
import { date } from "@/lib/format";
import { useResource } from "@/lib/hooks";

const KIND_LABEL: Record<string, string> = { receipt: "Justificatif", site_photo: "Photo de chantier", report_photo: "Photo de rapport", document: "Document" };

function size(bytes: number): string {
  return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} Ko` : `${(bytes / (1024 * 1024)).toFixed(1).replace(".", ",")} Mo`;
}

export function DocumentsTab({ projectId }: { projectId: string }) {
  const { data, error, loading } = useResource<Items<Attachment>>(`/projects/${projectId}/attachments`);
  const [openError, setOpenError] = useState<unknown>(null);

  async function open(id: string) {
    setOpenError(null);
    try {
      const { url } = await api<{ url: string }>(`/attachments/${id}/url`);
      window.open(url, "_blank", "noopener");
    } catch (err) {
      setOpenError(err);
    }
  }

  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  return (
    <Card style={{ padding: "18px 20px" }}>
      <h2 className="sec-t">Documents</h2>
      <ErrorBanner error={openError} />
      {data.items.length === 0 ? (
        <Empty>Aucun document. Les justificatifs et les photos des rapports apparaîtront ici.</Empty>
      ) : (
        data.items.map((d) => (
          <div key={d.id} className="li">
            <span style={{ display: "flex", gap: 12, alignItems: "center", minWidth: 0 }}>
              <span className="mi mut">{d.mime_type.startsWith("image/") ? <IconImage size={18} /> : <IconDoc size={18} />}</span>
              <span style={{ minWidth: 0 }}>
                <span style={{ display: "block", overflow: "hidden", textOverflow: "ellipsis" }}>{d.original_name ?? KIND_LABEL[d.kind] ?? "Document"}</span>
                <span className="muted" style={{ display: "block", fontSize: 12 }}>
                  {KIND_LABEL[d.kind] ?? d.kind} · {size(d.size_bytes)} · {date(d.created_at)}
                  {d.uploaded_by_name ? ` · ${d.uploaded_by_name}` : ""}
                </span>
              </span>
            </span>
            <button type="button" className="btn btn-s" style={{ height: 36 }} onClick={() => open(d.id)}>
              Ouvrir
            </button>
          </div>
        ))
      )}
    </Card>
  );
}
