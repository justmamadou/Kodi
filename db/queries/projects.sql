-- name: GetProjectAccess :one
SELECT p.id, p.status, p.manager_id,
       (m.user_id IS NOT NULL)::boolean        AS is_member,
       COALESCE(m.can_approve, false)::boolean AS can_approve,
       m.approval_limit
  FROM projects p
  LEFT JOIN project_members m ON m.project_id = p.id AND m.user_id = @user_id
 WHERE p.organization_id = @org_id AND p.id = @project_id;

-- name: CreateProject :one
INSERT INTO projects (id, organization_id, reference, name, client_name, address, city, description,
                      status, start_date, planned_end_date, manager_id, created_by)
VALUES (@id, @org_id, @reference, @name, @client_name, @address, sqlc.narg('city'), sqlc.narg('description'),
        @status, @start_date, sqlc.narg('planned_end_date'), @manager_id, @created_by)
RETURNING *;

-- name: GetProject :one
SELECT * FROM projects WHERE organization_id = @org_id AND id = @id;

-- name: LockProject :one
-- Sérialise les opérations de versionnage de budget d'un même chantier.
SELECT id, status, current_budget_version_id FROM projects
 WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: ListProjects :many
SELECT p.*, u.full_name AS manager_name,
       COALESCE(v.total_amount, 0)::bigint AS budget,
       COALESCE(f.approved, 0)::bigint AS approved
  FROM projects p
  JOIN users u ON u.id = p.manager_id
  LEFT JOIN budget_versions v ON v.id = p.current_budget_version_id
  LEFT JOIN LATERAL (SELECT SUM(e.amount) AS approved FROM expenses e
                      WHERE e.organization_id = p.organization_id AND e.project_id = p.id
                        AND e.status = 'approved') f ON true
 WHERE p.organization_id = @org_id
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
   AND (sqlc.narg('status')::text IS NULL OR p.status = sqlc.narg('status'))
   AND (sqlc.narg('manager_id')::uuid IS NULL OR p.manager_id = sqlc.narg('manager_id'))
   AND (sqlc.narg('q')::text IS NULL OR p.name ILIKE '%' || sqlc.narg('q') || '%' OR p.reference ILIKE '%' || sqlc.narg('q') || '%')
   AND (sqlc.narg('cursor')::uuid IS NULL OR p.id < sqlc.narg('cursor'))
 ORDER BY p.id DESC
 LIMIT @page_size;

-- name: UpdateProject :one
-- Verrou optimiste : aucune ligne si row_version a changé.
UPDATE projects
   SET reference        = COALESCE(sqlc.narg('reference'), reference),
       name             = COALESCE(sqlc.narg('name'), name),
       client_name      = COALESCE(sqlc.narg('client_name'), client_name),
       address          = COALESCE(sqlc.narg('address'), address),
       city             = COALESCE(sqlc.narg('city'), city),
       description      = COALESCE(sqlc.narg('description'), description),
       start_date       = COALESCE(sqlc.narg('start_date'), start_date),
       planned_end_date = COALESCE(sqlc.narg('planned_end_date'), planned_end_date),
       manager_id       = COALESCE(sqlc.narg('manager_id'), manager_id),
       row_version      = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND row_version = @row_version
RETURNING *;

-- name: SetProjectProgress :one
UPDATE projects
   SET declared_progress_pct = @progress_pct, progress_updated_at = now(), row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: SetProjectStatus :one
UPDATE projects
   SET status = @status,
       archived_at = CASE WHEN @status::text = 'archived' THEN now() END,
       row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status = @from_status
RETURNING *;

-- name: InsertProjectStatusHistory :exec
INSERT INTO project_status_history (organization_id, project_id, from_status, to_status, actor_id, comment)
VALUES (@org_id, @project_id, sqlc.narg('from_status'), @to_status, @actor_id, sqlc.narg('comment'));

-- name: ListProjectStatusHistory :many
SELECT h.id, h.from_status, h.to_status, h.actor_id, h.comment, h.created_at, u.full_name AS actor_name
  FROM project_status_history h
  JOIN users u ON u.id = h.actor_id
 WHERE h.organization_id = @org_id AND h.project_id = @project_id
 ORDER BY h.id;

-- name: UpsertProjectMember :one
INSERT INTO project_members (organization_id, project_id, user_id, can_approve, approval_limit, added_by)
VALUES (@org_id, @project_id, @user_id, @can_approve, sqlc.narg('approval_limit'), @added_by)
ON CONFLICT (project_id, user_id) DO UPDATE
   SET can_approve = EXCLUDED.can_approve, approval_limit = EXCLUDED.approval_limit
RETURNING *;

-- name: RemoveProjectMember :execrows
DELETE FROM project_members WHERE organization_id = @org_id AND project_id = @project_id AND user_id = @user_id;

-- name: ListProjectMembers :many
SELECT m.user_id, m.can_approve, m.approval_limit, m.created_at, u.full_name, u.role, u.status
  FROM project_members m
  JOIN users u ON u.id = m.user_id
 WHERE m.organization_id = @org_id AND m.project_id = @project_id
 ORDER BY u.full_name;

-- name: CreateProjectPhase :one
INSERT INTO project_phases (id, organization_id, project_id, name, position, planned_start, planned_end)
SELECT @id, @org_id, @project_id, @name,
       COALESCE((SELECT MAX(position) FROM project_phases x WHERE x.project_id = @project_id), 0) + 1,
       sqlc.narg('planned_start'), sqlc.narg('planned_end')
RETURNING *;

-- name: ListProjectPhases :many
SELECT * FROM project_phases WHERE organization_id = @org_id AND project_id = @project_id ORDER BY position;

-- name: UpdateProjectPhase :one
UPDATE project_phases
   SET name          = COALESCE(sqlc.narg('name'), name),
       status        = COALESCE(sqlc.narg('status'), status),
       planned_start = COALESCE(sqlc.narg('planned_start'), planned_start),
       planned_end   = COALESCE(sqlc.narg('planned_end'), planned_end)
 WHERE organization_id = @org_id AND project_id = @project_id AND id = @id
RETURNING *;

-- name: DeleteProjectPhase :execrows
DELETE FROM project_phases WHERE organization_id = @org_id AND project_id = @project_id AND id = @id;

-- name: ReorderProjectPhases :execrows
UPDATE project_phases p
   SET position = o.ord
  FROM unnest(@ids::uuid[]) WITH ORDINALITY AS o(id, ord)
 WHERE p.organization_id = @org_id AND p.project_id = @project_id AND p.id = o.id;

-- name: CountProjectPhases :one
SELECT count(*) FROM project_phases WHERE organization_id = @org_id AND project_id = @project_id;

-- name: IsAssignable :one
-- Un responsable doit être actif et avoir accès au chantier.
SELECT EXISTS (
  SELECT 1 FROM users u
    JOIN projects p ON p.organization_id = u.organization_id AND p.id = @project_id
   WHERE u.organization_id = @org_id AND u.id = @user_id AND u.status = 'active'
     AND (u.role IN ('owner','accountant') OR p.manager_id = u.id
          OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = u.id))
)::boolean AS assignable;

-- name: ListProjectHistory :many
-- Fil d'activité du chantier. Le journal d'audit reste la source : rien n'est saisi deux fois.
SELECT a.id, a.action, a.entity_type, a.entity_id, a.before, a.after, a.created_at, u.full_name AS actor_name
  FROM audit_logs a
  LEFT JOIN users u ON u.id = a.actor_id
 WHERE a.organization_id = @org_id AND a.project_id = @project_id
 ORDER BY a.created_at DESC, a.id DESC
 LIMIT @page_size;
