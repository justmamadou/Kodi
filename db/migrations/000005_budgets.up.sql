-- CF-012, CF-013, CF-014 : budgets versionnés, immuables après publication.
CREATE TABLE budget_versions (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL,
  project_id      UUID        NOT NULL,
  version_no      INT         NOT NULL CHECK (version_no >= 1),
  status          TEXT        NOT NULL DEFAULT 'draft'
                  CHECK (status IN ('draft','published','superseded','discarded')),
  total_amount    BIGINT      NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
  created_by      UUID        NOT NULL,
  published_by    UUID,
  published_at    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT budget_versions_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT budget_versions_no_uq UNIQUE (project_id, version_no),
  CONSTRAINT budget_versions_publication_chk CHECK (
    (status IN ('published','superseded')) = (published_at IS NOT NULL AND published_by IS NOT NULL)),
  FOREIGN KEY (organization_id, project_id)   REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, created_by)   REFERENCES users    (organization_id, id),
  FOREIGN KEY (organization_id, published_by) REFERENCES users    (organization_id, id)
);
-- Au plus UNE version publiée et UN brouillon par chantier.
CREATE UNIQUE INDEX budget_versions_one_published_uq ON budget_versions (project_id) WHERE status = 'published';
CREATE UNIQUE INDEX budget_versions_one_draft_uq ON budget_versions (project_id) WHERE status = 'draft';

ALTER TABLE projects ADD CONSTRAINT projects_current_budget_fk
  FOREIGN KEY (organization_id, current_budget_version_id) REFERENCES budget_versions (organization_id, id);

CREATE TABLE budget_lines (
  id                UUID PRIMARY KEY,
  organization_id   UUID          NOT NULL,
  budget_version_id UUID          NOT NULL,
  line_key          UUID          NOT NULL,        -- stable entre versions : clé du diff
  category_id       UUID          NOT NULL,
  label             TEXT          NOT NULL CHECK (length(label) BETWEEN 2 AND 200),
  quantity          NUMERIC(14,3) NOT NULL CHECK (quantity > 0),
  unit              TEXT          NOT NULL CHECK (length(unit) BETWEEN 1 AND 20),
  unit_price        BIGINT        NOT NULL CHECK (unit_price >= 0),
  -- Montant calculé par la base : le client ne peut pas le fournir.
  amount            BIGINT        GENERATED ALWAYS AS (round(quantity * unit_price)::bigint) STORED NOT NULL,
  position          INT           NOT NULL DEFAULT 0,
  CONSTRAINT budget_lines_key_uq UNIQUE (budget_version_id, line_key),
  FOREIGN KEY (organization_id, budget_version_id) REFERENCES budget_versions (organization_id, id),
  FOREIGN KEY (organization_id, category_id)       REFERENCES expense_categories (organization_id, id)
);
CREATE INDEX budget_lines_category_idx ON budget_lines (organization_id, budget_version_id, category_id);

CREATE TABLE budget_revisions (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL,
  project_id      UUID        NOT NULL,
  from_version_id UUID        NOT NULL,
  to_version_id   UUID        NOT NULL UNIQUE,
  reason          TEXT        NOT NULL CHECK (length(btrim(reason)) >= 3),
  created_by      UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, project_id)      REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, from_version_id) REFERENCES budget_versions (organization_id, id),
  FOREIGN KEY (organization_id, to_version_id)   REFERENCES budget_versions (organization_id, id),
  FOREIGN KEY (organization_id, created_by)      REFERENCES users (organization_id, id)
);
CREATE TRIGGER trg_budget_revisions_append_only BEFORE UPDATE OR DELETE ON budget_revisions
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Une version publiée n'accepte plus qu'une seule évolution : published -> superseded.
CREATE FUNCTION forbid_published_budget_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF OLD.status <> 'draft' THEN
      RAISE EXCEPTION 'budget version % cannot be deleted', OLD.id USING ERRCODE = 'P0001';
    END IF;
    RETURN OLD;
  END IF;
  IF OLD.status IN ('superseded','discarded') OR (OLD.status = 'published' AND (
       NEW.status <> 'superseded'
    OR NEW.total_amount IS DISTINCT FROM OLD.total_amount
    OR NEW.version_no   IS DISTINCT FROM OLD.version_no
    OR NEW.published_by IS DISTINCT FROM OLD.published_by
    OR NEW.published_at IS DISTINCT FROM OLD.published_at)) THEN
    RAISE EXCEPTION 'budget version % is immutable', OLD.id USING ERRCODE = 'P0001';
  END IF;
  IF OLD.status = 'draft' AND NEW.status = 'superseded' THEN
    RAISE EXCEPTION 'a draft budget version cannot be superseded' USING ERRCODE = 'P0001';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_budget_versions_immutable BEFORE UPDATE OR DELETE ON budget_versions
  FOR EACH ROW EXECUTE FUNCTION forbid_published_budget_change();

CREATE FUNCTION forbid_non_draft_budget_line_change() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  v_version UUID;
  v_status  TEXT;
BEGIN
  IF TG_OP = 'DELETE' THEN v_version := OLD.budget_version_id; ELSE v_version := NEW.budget_version_id; END IF;
  SELECT status INTO v_status FROM budget_versions WHERE id = v_version FOR SHARE;
  IF v_status <> 'draft' THEN
    RAISE EXCEPTION 'budget lines of version % are immutable', v_version USING ERRCODE = 'P0001';
  END IF;
  IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_budget_lines_immutable BEFORE INSERT OR UPDATE OR DELETE ON budget_lines
  FOR EACH ROW EXECUTE FUNCTION forbid_non_draft_budget_line_change();
