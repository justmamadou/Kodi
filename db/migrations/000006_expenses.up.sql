-- CF-015 à CF-018 : dépenses, événements d'approbation, paiements partiels.
-- Deux axes indépendants : status (validation) et payment_status (paiement).
CREATE TABLE expenses (
  id                UUID PRIMARY KEY,              -- UUIDv7 généré par le client (idempotence offline)
  organization_id   UUID        NOT NULL,
  project_id        UUID        NOT NULL,
  category_id       UUID        NOT NULL,
  supplier_id       UUID,
  description       TEXT        NOT NULL CHECK (length(description) BETWEEN 2 AND 500),
  amount            BIGINT      NOT NULL CHECK (amount > 0),
  currency          CHAR(3)     NOT NULL,
  expense_date      DATE        NOT NULL,
  status            TEXT        NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','pending','approved','rejected','cancelled')),
  payment_status    TEXT        NOT NULL DEFAULT 'unpaid'
                    CHECK (payment_status IN ('unpaid','partially_paid','paid')),
  amount_paid       BIGINT      NOT NULL DEFAULT 0 CHECK (amount_paid >= 0),
  budget_version_id UUID,                          -- version en vigueur à l'approbation, jamais réaffectée
  submitted_at      TIMESTAMPTZ,
  decided_by        UUID,
  decided_at        TIMESTAMPTZ,
  decision_comment  TEXT,
  has_receipt       BOOLEAN     NOT NULL DEFAULT false,
  created_by        UUID        NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  row_version       INT         NOT NULL DEFAULT 1,
  CONSTRAINT expenses_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT expenses_paid_le_amount_chk CHECK (amount_paid <= amount),
  CONSTRAINT expenses_payment_consistency_chk CHECK (
       (payment_status = 'unpaid'         AND amount_paid = 0)
    OR (payment_status = 'partially_paid' AND amount_paid > 0 AND amount_paid < amount)
    OR (payment_status = 'paid'           AND amount_paid = amount)),
  CONSTRAINT expenses_paid_only_approved_chk CHECK (amount_paid = 0 OR status = 'approved'),
  CONSTRAINT expenses_decision_chk CHECK ((status IN ('approved','rejected')) = (decided_at IS NOT NULL)),
  CONSTRAINT expenses_reject_comment_chk CHECK (status <> 'rejected' OR length(btrim(decision_comment)) > 0),
  -- La séparation des tâches est aussi garantie par la base.
  CONSTRAINT expenses_no_self_approval_chk CHECK (status <> 'approved' OR decided_by <> created_by),
  FOREIGN KEY (organization_id, project_id)        REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, category_id)       REFERENCES expense_categories (organization_id, id),
  FOREIGN KEY (organization_id, supplier_id)       REFERENCES suppliers (organization_id, id),
  FOREIGN KEY (organization_id, budget_version_id) REFERENCES budget_versions (organization_id, id),
  FOREIGN KEY (organization_id, created_by)        REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, decided_by)        REFERENCES users (organization_id, id)
);
CREATE INDEX expenses_project_date_idx ON expenses (organization_id, project_id, expense_date DESC, id DESC);
CREATE INDEX expenses_org_status_idx   ON expenses (organization_id, status, expense_date DESC, id DESC);
CREATE INDEX expenses_pending_idx      ON expenses (organization_id, submitted_at, id) WHERE status = 'pending';
-- Index couvrant des agrégations du dashboard (CF-019, CF-020).
CREATE INDEX expenses_financials_idx   ON expenses (organization_id, project_id, status)
  INCLUDE (amount, amount_paid) WHERE status IN ('pending','approved');
CREATE TRIGGER trg_expenses_updated BEFORE UPDATE ON expenses
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Garde-fou SQL de la machine à états (les règles vivent dans internal/expense/domain.go).
CREATE FUNCTION check_expense_transition() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status <> NEW.status AND NOT (
       (OLD.status = 'draft'    AND NEW.status IN ('pending','cancelled'))
    OR (OLD.status = 'pending'  AND NEW.status IN ('approved','rejected','draft'))
    OR (OLD.status = 'rejected' AND NEW.status = 'draft')
    OR (OLD.status = 'approved' AND NEW.status = 'cancelled')
  ) THEN
    RAISE EXCEPTION 'invalid expense transition % -> %', OLD.status, NEW.status USING ERRCODE = 'P0001';
  END IF;
  IF OLD.status IN ('pending','approved','rejected','cancelled') AND (
       NEW.amount       IS DISTINCT FROM OLD.amount
    OR NEW.project_id   IS DISTINCT FROM OLD.project_id
    OR NEW.category_id  IS DISTINCT FROM OLD.category_id) THEN
    RAISE EXCEPTION 'expense % financial fields are locked in status %', OLD.id, OLD.status USING ERRCODE = 'P0001';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_expenses_transition BEFORE UPDATE ON expenses
  FOR EACH ROW EXECUTE FUNCTION check_expense_transition();
CREATE TRIGGER trg_expenses_no_delete BEFORE DELETE ON expenses
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

-- Événements du workflow, écrits dans la même transaction que le changement de statut.
CREATE TABLE expense_approvals (
  id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id UUID        NOT NULL,
  expense_id      UUID        NOT NULL,
  event           TEXT        NOT NULL CHECK (event IN ('submitted','approved','rejected','withdrawn','reopened','cancelled')),
  from_status     TEXT        NOT NULL,
  to_status       TEXT        NOT NULL,
  actor_id        UUID        NOT NULL,
  comment         TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, expense_id) REFERENCES expenses (organization_id, id),
  FOREIGN KEY (organization_id, actor_id)   REFERENCES users (organization_id, id)
);
CREATE INDEX expense_approvals_expense_idx ON expense_approvals (organization_id, expense_id, id);
CREATE TRIGGER trg_expense_approvals_append_only BEFORE UPDATE OR DELETE ON expense_approvals
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

CREATE TABLE payments (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL,
  expense_id      UUID        NOT NULL,
  amount          BIGINT      NOT NULL CHECK (amount > 0),
  paid_on         DATE        NOT NULL,
  method          TEXT        NOT NULL CHECK (method IN ('cash','wave','orange_money','free_money','bank_transfer','cheque','other')),
  reference       TEXT,
  recorded_by     UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, expense_id)  REFERENCES expenses (organization_id, id),
  FOREIGN KEY (organization_id, recorded_by) REFERENCES users (organization_id, id)
);
CREATE INDEX payments_expense_idx ON payments (organization_id, expense_id, created_at);
CREATE TRIGGER trg_payments_append_only BEFORE UPDATE OR DELETE ON payments
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
