-- name: CreateTask :one
INSERT INTO tasks (id, organization_id, project_id, title, description, priority, due_date, created_by)
VALUES (@id, @org_id, @project_id, @title, sqlc.narg('description'), @priority, sqlc.narg('due_date'), @created_by)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetTask :one
SELECT * FROM tasks WHERE organization_id = @org_id AND id = @id;

-- name: GetTaskForUpdate :one
SELECT * FROM tasks WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: UpdateTask :one
UPDATE tasks
   SET title       = COALESCE(sqlc.narg('title'), title),
       description = COALESCE(sqlc.narg('description'), description),
       priority    = COALESCE(sqlc.narg('priority'), priority),
       due_date    = COALESCE(sqlc.narg('due_date'), due_date),
       row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND row_version = @row_version
RETURNING *;

-- name: SetTaskProgress :one
UPDATE tasks
   SET status = @status, progress_pct = @progress_pct,
       completed_at = CASE WHEN @status::text = 'done' THEN COALESCE(completed_at, now()) END,
       row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: InsertTaskStatusHistory :exec
INSERT INTO task_status_history (organization_id, task_id, from_status, to_status, progress_pct, actor_id)
VALUES (@org_id, @task_id, @from_status, @to_status, @progress_pct, @actor_id);

-- name: ListTaskStatusHistory :many
SELECT h.id, h.from_status, h.to_status, h.progress_pct, h.actor_id, h.created_at, u.full_name AS actor_name
  FROM task_status_history h
  JOIN users u ON u.id = h.actor_id
 WHERE h.organization_id = @org_id AND h.task_id = @task_id
 ORDER BY h.id;

-- name: AddTaskAssignment :exec
INSERT INTO task_assignments (organization_id, task_id, user_id, assigned_by)
VALUES (@org_id, @task_id, @user_id, @assigned_by)
ON CONFLICT (task_id, user_id) DO NOTHING;

-- name: ClearTaskAssignments :exec
DELETE FROM task_assignments WHERE organization_id = @org_id AND task_id = @task_id;

-- name: ListTaskAssignees :many
SELECT user_id FROM task_assignments WHERE organization_id = @org_id AND task_id = @task_id ORDER BY user_id;

-- name: IsTaskAssignee :one
SELECT EXISTS (SELECT 1 FROM task_assignments
                WHERE organization_id = @org_id AND task_id = @task_id AND user_id = @user_id)::boolean AS assigned;

-- name: ListTasks :many
SELECT t.*, COALESCE((SELECT array_agg(a.user_id ORDER BY a.user_id) FROM task_assignments a WHERE a.task_id = t.id),
                     '{}')::uuid[] AS assignee_ids
  FROM tasks t
 WHERE t.organization_id = @org_id AND t.project_id = @project_id
   AND (sqlc.narg('status')::text IS NULL OR t.status = sqlc.narg('status'))
   AND (sqlc.narg('assignee_id')::uuid IS NULL OR EXISTS
         (SELECT 1 FROM task_assignments a WHERE a.task_id = t.id AND a.user_id = sqlc.narg('assignee_id')))
 ORDER BY t.due_date NULLS LAST, t.id
 LIMIT 500;

-- name: UpsertDailyReport :one
-- Idempotent + verrou optimiste : aucune ligne si le rapport est soumis ou a changé => conflit côté API.
INSERT INTO daily_reports AS d (id, organization_id, project_id, report_date, team_label, work_done,
                                workforce_count, issues, weather, created_by)
VALUES (@id, @org_id, @project_id, @report_date, @team_label, @work_done,
        sqlc.narg('workforce_count'), sqlc.narg('issues'), sqlc.narg('weather'), @created_by)
ON CONFLICT (id) DO UPDATE SET
   report_date = EXCLUDED.report_date,
   team_label = EXCLUDED.team_label,
   work_done = EXCLUDED.work_done,
   workforce_count = EXCLUDED.workforce_count,
   issues = EXCLUDED.issues,
   weather = EXCLUDED.weather,
   row_version = d.row_version + 1
 WHERE d.status = 'draft' AND d.project_id = EXCLUDED.project_id
   AND (sqlc.narg('base_row_version')::int IS NULL OR d.row_version = sqlc.narg('base_row_version')::int)
RETURNING *;

-- name: GetDailyReport :one
SELECT * FROM daily_reports WHERE organization_id = @org_id AND id = @id;

-- name: ListDailyReports :many
SELECT * FROM daily_reports
 WHERE organization_id = @org_id AND project_id = @project_id
   AND (sqlc.narg('date_from')::date IS NULL OR report_date >= sqlc.narg('date_from'))
   AND (sqlc.narg('date_to')::date   IS NULL OR report_date <= sqlc.narg('date_to'))
 ORDER BY report_date DESC, id DESC
 LIMIT 200;

-- name: SubmitDailyReport :one
UPDATE daily_reports
   SET status = 'submitted', submitted_at = now(), submitted_by = @actor_id, row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: ReopenDailyReport :one
UPDATE daily_reports
   SET status = 'draft', submitted_at = NULL, submitted_by = NULL, row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status = 'submitted'
RETURNING *;

-- name: ClearDailyReportTasks :exec
DELETE FROM daily_report_tasks WHERE organization_id = @org_id AND daily_report_id = @daily_report_id;

-- name: AddDailyReportTask :exec
INSERT INTO daily_report_tasks (organization_id, daily_report_id, task_id, done_that_day)
VALUES (@org_id, @daily_report_id, @task_id, @done_that_day);

-- name: ListDailyReportTasks :many
SELECT l.task_id, l.done_that_day, t.title
  FROM daily_report_tasks l
  JOIN tasks t ON t.id = l.task_id
 WHERE l.organization_id = @org_id AND l.daily_report_id = @daily_report_id
 ORDER BY t.title;
