DROP INDEX IF EXISTS audit_logs_project_idx;
DROP INDEX IF EXISTS payments_paid_on_idx;
DROP INDEX IF EXISTS attachments_project_idx;
ALTER TABLE invitations DROP COLUMN IF EXISTS project_ids;
ALTER TABLE materials DROP COLUMN IF EXISTS low_stock_threshold;
ALTER TABLE expenses
  DROP COLUMN IF EXISTS declared_payment_method,
  DROP COLUMN IF EXISTS declared_paid_on,
  DROP COLUMN IF EXISTS comment;
ALTER TABLE organizations DROP COLUMN IF EXISTS activity_type;
ALTER TABLE projects DROP COLUMN IF EXISTS progress_updated_at, DROP COLUMN IF EXISTS declared_progress_pct;
