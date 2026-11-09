-- CF-009, CF-010, CF-011 : chantiers, membres, phases, historique des statuts.
CREATE TABLE projects (
  id                    UUID PRIMARY KEY,
  organization_id       UUID        NOT NULL REFERENCES organizations(id),
  reference             TEXT        NOT NULL CHECK (length(reference) BETWEEN 1 AND 40),
  name                  TEXT        NOT NULL CHECK (length(name) BETWEEN 2 AND 160),
  client_name           TEXT        NOT NULL CHECK (length(client_name) >= 2),
  address               TEXT        NOT NULL CHECK (length(address) >= 2),
  city                  TEXT,
  description           TEXT,
  status                TEXT        NOT NULL DEFAULT 'draft'
                        CHECK (status IN ('draft','planned','in_progress','completed','archived')),
  start_date            DATE        NOT NULL,
  planned_end_date      DATE,
  manager_id            UUID        NOT NULL,      -- conducteur responsable
  current_budget_version_id UUID,                  -- FK ajoutée après budget_versions
  created_by            UUID        NOT NULL,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  archived_at           TIMESTAMPTZ,
  row_version           INT         NOT NULL DEFAULT 1,
  CONSTRAINT projects_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT projects_dates_chk CHECK (planned_end_date IS NULL OR planned_end_date >= start_date),
  CONSTRAINT projects_archived_chk CHECK ((status = 'archived') = (archived_at IS NOT NULL)),
  FOREIGN KEY (organization_id, manager_id) REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, created_by) REFERENCES users (organization_id, id)
);
CREATE UNIQUE INDEX projects_org_reference_uq ON projects (organization_id, lower(reference));
CREATE INDEX projects_org_status_idx ON projects (organization_id, status, id);
CREATE INDEX projects_name_trgm ON projects USING gin (name gin_trgm_ops);
CREATE TRIGGER trg_projects_updated BEFORE UPDATE ON projects
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Garde-fou SQL de la machine à états (les règles vivent dans internal/project/domain.go).
CREATE FUNCTION check_project_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status = NEW.status THEN RETURN NEW; END IF;
  IF NOT (
       (OLD.status = 'draft'       AND NEW.status IN ('planned','in_progress','archived'))
    OR (OLD.status = 'planned'     AND NEW.status IN ('draft','in_progress','archived'))
    OR (OLD.status = 'in_progress' AND NEW.status = 'completed')
    OR (OLD.status = 'completed'   AND NEW.status IN ('in_progress','archived'))
  ) THEN
    RAISE EXCEPTION 'invalid project transition % -> %', OLD.status, NEW.status USING ERRCODE = 'P0001';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_projects_transition BEFORE UPDATE OF status ON projects
  FOR EACH ROW EXECUTE FUNCTION check_project_transition();

CREATE TABLE project_status_history (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id UUID        NOT NULL,
  project_id      UUID        NOT NULL,
  from_status     TEXT,
  to_status       TEXT        NOT NULL,
  actor_id        UUID        NOT NULL,
  comment         TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, actor_id)   REFERENCES users (organization_id, id)
);
CREATE INDEX project_status_history_idx ON project_status_history (organization_id, project_id, created_at);
CREATE TRIGGER trg_project_status_history_append_only BEFORE UPDATE OR DELETE ON project_status_history
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE project_members (
  organization_id UUID        NOT NULL,
  project_id      UUID        NOT NULL,
  user_id         UUID        NOT NULL,
  can_approve     BOOLEAN     NOT NULL DEFAULT false,      -- délégation d'approbation
  approval_limit  BIGINT      CHECK (approval_limit >= 0), -- NULL = pas de plafond
  added_by        UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, user_id),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, user_id)    REFERENCES users    (organization_id, id),
  FOREIGN KEY (organization_id, added_by)   REFERENCES users    (organization_id, id)
);
CREATE INDEX project_members_user_idx ON project_members (organization_id, user_id);

CREATE TABLE project_phases (
  id               UUID PRIMARY KEY,
  organization_id  UUID        NOT NULL,
  project_id       UUID        NOT NULL,
  name             TEXT        NOT NULL CHECK (length(name) BETWEEN 2 AND 160),
  position         INT         NOT NULL CHECK (position >= 1),
  status           TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','in_progress','done')),
  planned_start    DATE,
  planned_end      DATE,
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT project_phases_org_id_uq UNIQUE (organization_id, id),
  -- Différée : un réordonnancement permute les positions dans une seule transaction.
  CONSTRAINT project_phases_position_uq UNIQUE (project_id, position) DEFERRABLE INITIALLY DEFERRED,
  CONSTRAINT project_phases_dates_chk CHECK (planned_end IS NULL OR planned_start IS NULL OR planned_end >= planned_start),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects (organization_id, id)
);
CREATE TRIGGER trg_project_phases_updated BEFORE UPDATE ON project_phases
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
