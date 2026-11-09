-- CF-027 : pièces jointes (métadonnées ; les octets vivent dans l'object storage privé).
CREATE TABLE attachments (
  id              UUID PRIMARY KEY,                -- UUIDv7 client
  organization_id UUID        NOT NULL REFERENCES organizations(id),
  project_id      UUID        NOT NULL,
  expense_id      UUID,
  daily_report_id UUID,
  storage_key     TEXT        NOT NULL UNIQUE,     -- org/{org}/proj/{proj}/{yyyy}/{mm}/{id}.{ext}
  kind            TEXT        NOT NULL CHECK (kind IN ('receipt','site_photo','document')),
  mime_type       TEXT        NOT NULL CHECK (mime_type IN
                    ('image/jpeg','image/png','image/webp','application/pdf')),
  size_bytes      BIGINT      NOT NULL CHECK (size_bytes > 0 AND size_bytes <= 26214400), -- 25 Mo max
  original_name   TEXT        CHECK (length(original_name) <= 255),
  status          TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','uploaded')),
  uploaded_by     UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  uploaded_at     TIMESTAMPTZ,
  CONSTRAINT attachments_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT attachments_owner_chk CHECK (num_nonnulls(expense_id, daily_report_id) <= 1),
  FOREIGN KEY (organization_id, project_id)      REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, expense_id)      REFERENCES expenses (organization_id, id),
  FOREIGN KEY (organization_id, daily_report_id) REFERENCES daily_reports (organization_id, id),
  FOREIGN KEY (organization_id, uploaded_by)     REFERENCES users (organization_id, id)
);
CREATE INDEX attachments_expense_idx ON attachments (organization_id, expense_id) WHERE expense_id IS NOT NULL;
CREATE INDEX attachments_report_idx  ON attachments (organization_id, daily_report_id) WHERE daily_report_id IS NOT NULL;

-- CF-016, CF-030 : idempotence des mutations (rejeu d'une même clé = réponse mémorisée).
CREATE TABLE idempotency_keys (
  organization_id UUID        NOT NULL,
  key             UUID        NOT NULL,
  user_id         UUID        NOT NULL,
  method          TEXT        NOT NULL,
  path            TEXT        NOT NULL,
  request_hash    BYTEA       NOT NULL,
  response_status SMALLINT,                        -- NULL = requête en cours
  response_body   BYTEA,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (organization_id, key)
);
CREATE INDEX idempotency_keys_gc_idx ON idempotency_keys (created_at);

-- CF-005 : journal d'audit append-only, partitionné par mois.
CREATE TABLE audit_logs (
  id              UUID        NOT NULL,
  organization_id UUID        NOT NULL,
  actor_id        UUID,                            -- NULL = système
  action          TEXT        NOT NULL,            -- expense.approved, member.role_changed...
  entity_type     TEXT        NOT NULL,
  entity_id       UUID,
  project_id      UUID,
  before          JSONB,
  after           JSONB,
  ip              TEXT,
  user_agent      TEXT,
  request_id      TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
CREATE INDEX audit_logs_org_idx    ON audit_logs (organization_id, created_at DESC);
CREATE INDEX audit_logs_entity_idx ON audit_logs (organization_id, entity_type, entity_id, created_at DESC);

CREATE TABLE audit_logs_default PARTITION OF audit_logs DEFAULT;
-- Un trigger sur la table parente s'applique à toutes ses partitions, présentes et futures.
CREATE TRIGGER trg_audit_append_only BEFORE UPDATE OR DELETE ON audit_logs
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Partitions de 2026-11 à 2028-12 ; ensuite la partition par défaut prend le relais
-- (voir docs/RUNBOOK.md pour en créer de nouvelles).
DO $$
DECLARE
  m DATE := DATE '2026-11-01';
BEGIN
  WHILE m < DATE '2029-01-01' LOOP
    EXECUTE format('CREATE TABLE %I PARTITION OF audit_logs FOR VALUES FROM (%L) TO (%L)',
                   'audit_logs_' || to_char(m, 'YYYY_MM'), m, (m + INTERVAL '1 month')::date);
    m := (m + INTERVAL '1 month')::date;
  END LOOP;
END $$;
