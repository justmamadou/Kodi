-- CF-003 : Row Level Security sur toutes les tables, droits du rôle applicatif,
-- et seules fonctions autorisées à lire hors contexte tenant.
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
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I USING (organization_id = current_org()) '
      'WITH CHECK (organization_id = current_org())', t);
  END LOOP;
END $$;

ALTER TABLE organizations ENABLE ROW LEVEL SECURITY;
ALTER TABLE organizations FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON organizations
  USING (id = current_org()) WITH CHECK (id = current_org());

GRANT USAGE ON SCHEMA public TO cf_app;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA public TO cf_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO cf_app;
-- Le journal d'audit et les historiques ne sont ni modifiables ni supprimables par l'API.
REVOKE UPDATE ON audit_logs, project_status_history, budget_revisions,
                 expense_approvals, payments, task_status_history FROM cf_app;
-- La RLS ne s'applique qu'à travers la table parente : l'API ne doit jamais toucher une partition directement.
DO $$
DECLARE
  p REGCLASS;
BEGIN
  FOR p IN SELECT inhrelid::regclass FROM pg_inherits WHERE inhparent = 'audit_logs'::regclass LOOP
    EXECUTE format('REVOKE ALL ON %s FROM cf_app', p);
  END LOOP;
END $$;
-- Suppressions physiques limitées aux liens et aux données de travail.
GRANT DELETE ON project_members, project_phases, budget_lines, task_assignments,
                daily_report_tasks, stock_movement_items, idempotency_keys TO cf_app;

-- Fonctions SECURITY DEFINER : leur propriétaire (rôle de migration, BYPASSRLS) les exécute
-- avant que le tenant ne soit connu. Elles exposent le strict nécessaire.
CREATE FUNCTION auth_find_user(p_login TEXT)
RETURNS TABLE (user_id UUID, organization_id UUID, role TEXT, password_hash TEXT, status TEXT,
               locked_until TIMESTAMPTZ, failed_login_count SMALLINT)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS $$
  SELECT u.id, u.organization_id, u.role, u.password_hash, u.status, u.locked_until, u.failed_login_count
    FROM users u
   WHERE u.email = p_login::citext OR u.phone = p_login
   LIMIT 1
$$;

CREATE FUNCTION auth_find_session(p_token_hash BYTEA)
RETURNS TABLE (session_id UUID, user_id UUID, organization_id UUID, role TEXT, user_status TEXT,
               remember BOOLEAN, last_seen_at TIMESTAMPTZ, expires_at TIMESTAMPTZ)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS $$
  SELECT s.id, u.id, u.organization_id, u.role, u.status, s.remember, s.last_seen_at, s.expires_at
    FROM sessions s
    JOIN users u ON u.id = s.user_id
   WHERE s.token_hash = p_token_hash AND s.revoked_at IS NULL AND s.expires_at > now()
$$;

CREATE FUNCTION auth_find_invitation(p_token_hash BYTEA)
RETURNS TABLE (invitation_id UUID, user_id UUID, organization_id UUID)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS $$
  SELECT i.id, i.user_id, i.organization_id
    FROM invitations i
   WHERE i.token_hash = p_token_hash
     AND i.accepted_at IS NULL AND i.revoked_at IS NULL AND i.expires_at > now()
$$;

REVOKE ALL ON FUNCTION auth_find_user(TEXT), auth_find_session(BYTEA), auth_find_invitation(BYTEA) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_find_user(TEXT), auth_find_session(BYTEA), auth_find_invitation(BYTEA) TO cf_app;
