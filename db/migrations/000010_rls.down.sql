DROP FUNCTION IF EXISTS auth_find_invitation(BYTEA);
DROP FUNCTION IF EXISTS auth_find_session(BYTEA);
DROP FUNCTION IF EXISTS auth_find_user(TEXT);

REVOKE ALL ON ALL TABLES IN SCHEMA public FROM cf_app;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM cf_app;
REVOKE USAGE ON SCHEMA public FROM cf_app;

DROP POLICY IF EXISTS tenant_isolation ON organizations;
ALTER TABLE organizations NO FORCE ROW LEVEL SECURITY;
ALTER TABLE organizations DISABLE ROW LEVEL SECURITY;

DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'organization_settings','users','sessions','invitations',
    'projects','project_status_history','project_members','project_phases',
    'expense_categories','suppliers','materials',
    'budget_versions','budget_lines','budget_revisions',
    'expenses','expense_approvals','payments',
    'tasks','task_assignments','task_status_history','daily_reports','daily_report_tasks',
    'stock_movements','stock_movement_items','project_material_stock',
    'attachments','idempotency_keys','audit_logs']
  LOOP
    EXECUTE format('DROP POLICY IF EXISTS tenant_isolation ON %I', t);
    EXECUTE format('ALTER TABLE %I NO FORCE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', t);
  END LOOP;
END $$;
