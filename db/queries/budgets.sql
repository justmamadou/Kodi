-- name: NextBudgetVersionNo :one
SELECT (COALESCE(MAX(version_no), 0) + 1)::int AS next_no
  FROM budget_versions WHERE organization_id = @org_id AND project_id = @project_id;

-- name: CreateBudgetVersion :one
INSERT INTO budget_versions (id, organization_id, project_id, version_no, created_by)
VALUES (@id, @org_id, @project_id, @version_no, @created_by)
RETURNING *;

-- name: GetBudgetVersion :one
SELECT * FROM budget_versions WHERE organization_id = @org_id AND id = @id;

-- name: ListBudgetVersions :many
SELECT v.*, r.reason AS revision_reason, u.full_name AS published_by_name
  FROM budget_versions v
  LEFT JOIN budget_revisions r ON r.to_version_id = v.id
  LEFT JOIN users u ON u.id = v.published_by
 WHERE v.organization_id = @org_id AND v.project_id = @project_id
 ORDER BY v.version_no DESC;

-- name: CreateBudgetLine :one
INSERT INTO budget_lines (id, organization_id, budget_version_id, line_key, category_id, label, quantity, unit, unit_price, position)
VALUES (@id, @org_id, @budget_version_id, @line_key, @category_id, @label, @quantity, @unit, @unit_price, @position)
RETURNING *;

-- name: GetBudgetLine :one
SELECT * FROM budget_lines WHERE organization_id = @org_id AND id = @id;

-- name: UpdateBudgetLine :one
UPDATE budget_lines
   SET category_id = COALESCE(sqlc.narg('category_id'), category_id),
       label       = COALESCE(sqlc.narg('label'), label),
       quantity    = COALESCE(sqlc.narg('quantity'), quantity),
       unit        = COALESCE(sqlc.narg('unit'), unit),
       unit_price  = COALESCE(sqlc.narg('unit_price'), unit_price),
       position    = COALESCE(sqlc.narg('position'), position)
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: DeleteBudgetLine :execrows
DELETE FROM budget_lines WHERE organization_id = @org_id AND id = @id;

-- name: ListBudgetLines :many
SELECT l.*, c.name AS category_name
  FROM budget_lines l
  JOIN expense_categories c ON c.id = l.category_id
 WHERE l.organization_id = @org_id AND l.budget_version_id = @budget_version_id
 ORDER BY l.position, l.id;

-- name: CountBudgetLines :one
SELECT count(*) FROM budget_lines WHERE organization_id = @org_id AND budget_version_id = @budget_version_id;

-- name: RecalcBudgetTotal :one
-- Le total n'est jamais fourni par le client : toujours recalculé depuis les lignes.
UPDATE budget_versions v
   SET total_amount = (SELECT COALESCE(SUM(l.amount), 0)::bigint FROM budget_lines l WHERE l.budget_version_id = v.id)
 WHERE v.organization_id = @org_id AND v.id = @id AND v.status = 'draft'
RETURNING *;

-- name: SupersedePublishedBudget :execrows
UPDATE budget_versions SET status = 'superseded'
 WHERE organization_id = @org_id AND project_id = @project_id AND status = 'published';

-- name: PublishBudgetVersion :one
UPDATE budget_versions
   SET status = 'published', published_by = @actor_id, published_at = now()
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: SetProjectCurrentBudget :exec
UPDATE projects SET current_budget_version_id = @budget_version_id
 WHERE organization_id = @org_id AND id = @project_id;

-- name: DiscardBudgetVersion :one
UPDATE budget_versions SET status = 'discarded'
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: CopyBudgetLines :exec
-- Conserve line_key : c'est lui qui permet de comparer deux versions ligne à ligne.
INSERT INTO budget_lines (id, organization_id, budget_version_id, line_key, category_id, label, quantity, unit, unit_price, position)
SELECT gen_random_uuid(), l.organization_id, @to_version_id, l.line_key, l.category_id, l.label, l.quantity, l.unit, l.unit_price, l.position
  FROM budget_lines l
 WHERE l.organization_id = @org_id AND l.budget_version_id = @from_version_id;

-- name: CreateBudgetRevision :one
INSERT INTO budget_revisions (id, organization_id, project_id, from_version_id, to_version_id, reason, created_by)
VALUES (@id, @org_id, @project_id, @from_version_id, @to_version_id, @reason, @created_by)
RETURNING *;
