-- CF-024, CF-025, CF-026 : tâches et rapports journaliers.
CREATE TABLE tasks (
  id              UUID PRIMARY KEY,                -- UUIDv7 client possible (brouillons offline)
  organization_id UUID        NOT NULL,
  project_id      UUID        NOT NULL,
  title           TEXT        NOT NULL CHECK (length(title) BETWEEN 2 AND 200),
  description     TEXT,
  priority        TEXT        NOT NULL DEFAULT 'medium' CHECK (priority IN ('low','medium','high','urgent')),
  status          TEXT        NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','in_progress','blocked','done','cancelled')),
  progress_pct    SMALLINT    NOT NULL DEFAULT 0 CHECK (progress_pct BETWEEN 0 AND 100),
  due_date        DATE,
  completed_at    TIMESTAMPTZ,
  created_by      UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  row_version     INT         NOT NULL DEFAULT 1,
  CONSTRAINT tasks_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT tasks_done_chk CHECK ((status = 'done') = (completed_at IS NOT NULL)),
  CONSTRAINT tasks_done_progress_chk CHECK (status <> 'done' OR progress_pct = 100),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, created_by) REFERENCES users (organization_id, id)
);
CREATE INDEX tasks_project_status_idx ON tasks (organization_id, project_id, status, due_date);
CREATE TRIGGER trg_tasks_updated BEFORE UPDATE ON tasks
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE task_assignments (
  organization_id UUID        NOT NULL,
  task_id         UUID        NOT NULL,
  user_id         UUID        NOT NULL,
  assigned_by     UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (task_id, user_id),
  FOREIGN KEY (organization_id, task_id)     REFERENCES tasks (organization_id, id),
  FOREIGN KEY (organization_id, user_id)     REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, assigned_by) REFERENCES users (organization_id, id)
);
CREATE INDEX task_assignments_user_idx ON task_assignments (organization_id, user_id);

CREATE TABLE task_status_history (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id UUID        NOT NULL,
  task_id         UUID        NOT NULL,
  from_status     TEXT        NOT NULL,
  to_status       TEXT        NOT NULL,
  progress_pct    SMALLINT    NOT NULL,
  actor_id        UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, task_id)  REFERENCES tasks (organization_id, id),
  FOREIGN KEY (organization_id, actor_id) REFERENCES users (organization_id, id)
);
CREATE INDEX task_status_history_idx ON task_status_history (organization_id, task_id, id);
CREATE TRIGGER trg_task_status_history_append_only BEFORE UPDATE OR DELETE ON task_status_history
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE daily_reports (
  id              UUID PRIMARY KEY,                -- UUIDv7 client (offline)
  organization_id UUID        NOT NULL,
  project_id      UUID        NOT NULL,
  report_date     DATE        NOT NULL,
  team_label      TEXT        NOT NULL DEFAULT '', -- '' = rapport unique ; sinon multi-équipes
  work_done       TEXT        NOT NULL CHECK (length(work_done) >= 2),
  workforce_count SMALLINT    CHECK (workforce_count >= 0), -- indicatif, sans lien avec la paie
  issues          TEXT,
  weather         TEXT        CHECK (weather IN ('sunny','cloudy','rain','strong_wind')),
  status          TEXT        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','submitted')),
  submitted_at    TIMESTAMPTZ,
  submitted_by    UUID,
  created_by      UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  row_version     INT         NOT NULL DEFAULT 1,
  CONSTRAINT daily_reports_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT daily_reports_one_per_day_uq UNIQUE (project_id, report_date, team_label),
  CONSTRAINT daily_reports_submitted_chk CHECK ((status = 'submitted') = (submitted_at IS NOT NULL)),
  FOREIGN KEY (organization_id, project_id)   REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, created_by)   REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, submitted_by) REFERENCES users (organization_id, id)
);
CREATE INDEX daily_reports_timeline_idx ON daily_reports (organization_id, project_id, report_date DESC);
CREATE TRIGGER trg_daily_reports_updated BEFORE UPDATE ON daily_reports
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Un rapport soumis est verrouillé : seule la réouverture (action auditée) le fait repasser en brouillon.
CREATE FUNCTION forbid_submitted_report_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status = 'submitted' AND NEW.status = 'submitted' THEN
    RAISE EXCEPTION 'daily report % is locked', OLD.id USING ERRCODE = 'P0001';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_daily_reports_locked BEFORE UPDATE ON daily_reports
  FOR EACH ROW EXECUTE FUNCTION forbid_submitted_report_change();

CREATE TABLE daily_report_tasks (
  organization_id UUID    NOT NULL,
  daily_report_id UUID    NOT NULL,
  task_id         UUID    NOT NULL,
  done_that_day   BOOLEAN NOT NULL DEFAULT false,
  PRIMARY KEY (daily_report_id, task_id),
  FOREIGN KEY (organization_id, daily_report_id) REFERENCES daily_reports (organization_id, id),
  FOREIGN KEY (organization_id, task_id)         REFERENCES tasks (organization_id, id)
);
