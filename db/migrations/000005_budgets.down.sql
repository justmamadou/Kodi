ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_current_budget_fk;
DROP TABLE IF EXISTS budget_revisions;
DROP TABLE IF EXISTS budget_lines;
DROP TABLE IF EXISTS budget_versions;
DROP FUNCTION IF EXISTS forbid_non_draft_budget_line_change();
DROP FUNCTION IF EXISTS forbid_published_budget_change();
