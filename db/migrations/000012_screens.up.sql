-- Données affichées par les maquettes : avancement déclaré, activité, déclaration de paiement à la
-- saisie, seuil de stock bas, chantiers accessibles choisis à l'invitation.
ALTER TABLE projects
  ADD COLUMN declared_progress_pct SMALLINT NOT NULL DEFAULT 0 CHECK (declared_progress_pct BETWEEN 0 AND 100),
  ADD COLUMN progress_updated_at   TIMESTAMPTZ;

ALTER TABLE organizations
  ADD COLUMN activity_type TEXT CHECK (activity_type IN ('villas','gros_oeuvre','second_oeuvre','renovation','general'));

-- Simple déclaration du saisisseur : le paiement n'existe qu'une fois enregistré par Finance (CF-018).
ALTER TABLE expenses
  ADD COLUMN comment TEXT,
  ADD COLUMN declared_paid_on DATE,
  ADD COLUMN declared_payment_method TEXT
    CHECK (declared_payment_method IN ('cash','wave','orange_money','free_money','bank_transfer','cheque','other'));

ALTER TABLE materials
  ADD COLUMN low_stock_threshold NUMERIC(14,3) CHECK (low_stock_threshold >= 0);

-- Appliqués à l'acceptation : un compte invité n'est pas encore actif, donc pas affectable (CF-010).
ALTER TABLE invitations
  ADD COLUMN project_ids UUID[] NOT NULL DEFAULT '{}';

-- Repères de consultation : doublons (même chantier, montant, fournisseur, ± 2 jours) et documents.
CREATE INDEX attachments_project_idx ON attachments (organization_id, project_id, id DESC) WHERE status = 'uploaded';
CREATE INDEX payments_paid_on_idx ON payments (organization_id, paid_on);
CREATE INDEX audit_logs_project_idx ON audit_logs (organization_id, project_id, created_at DESC);
