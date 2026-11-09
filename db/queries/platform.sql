-- name: CreateAttachment :one
INSERT INTO attachments (id, organization_id, project_id, expense_id, daily_report_id, storage_key, kind,
                         mime_type, size_bytes, original_name, uploaded_by)
VALUES (@id, @org_id, @project_id, sqlc.narg('expense_id'), sqlc.narg('daily_report_id'), @storage_key, @kind,
        @mime_type, @size_bytes, sqlc.narg('original_name'), @uploaded_by)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetAttachment :one
SELECT * FROM attachments WHERE organization_id = @org_id AND id = @id;

-- name: CompleteAttachment :one
UPDATE attachments SET status = 'uploaded', uploaded_at = now(), size_bytes = @size_bytes
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: ListExpenseAttachments :many
SELECT * FROM attachments
 WHERE organization_id = @org_id AND expense_id = @expense_id AND status = 'uploaded' ORDER BY id;

-- name: ListDailyReportAttachments :many
SELECT * FROM attachments
 WHERE organization_id = @org_id AND daily_report_id = @daily_report_id AND status = 'uploaded' ORDER BY id;

-- name: InsertAuditLog :exec
INSERT INTO audit_logs (id, organization_id, actor_id, action, entity_type, entity_id, project_id,
                        before, after, ip, user_agent, request_id)
VALUES (@id, @org_id, sqlc.narg('actor_id'), @action, @entity_type, sqlc.narg('entity_id'), sqlc.narg('project_id'),
        sqlc.narg('before'), sqlc.narg('after'), sqlc.narg('ip'), sqlc.narg('user_agent'), sqlc.narg('request_id'));

-- name: ListAuditLogs :many
SELECT a.id, a.actor_id, a.action, a.entity_type, a.entity_id, a.project_id, a.before, a.after, a.created_at,
       u.full_name AS actor_name
  FROM audit_logs a
  LEFT JOIN users u ON u.id = a.actor_id
 WHERE a.organization_id = @org_id
   AND (sqlc.narg('action')::text IS NULL OR a.action = sqlc.narg('action'))
   AND (sqlc.narg('entity_id')::uuid IS NULL OR a.entity_id = sqlc.narg('entity_id'))
 ORDER BY a.created_at DESC, a.id DESC
 LIMIT @page_size;

-- name: ClaimIdempotencyKey :one
INSERT INTO idempotency_keys (organization_id, key, user_id, method, path, request_hash)
VALUES (@org_id, @key, @user_id, @method, @path, @request_hash)
ON CONFLICT (organization_id, key) DO NOTHING
RETURNING key;

-- name: GetIdempotencyKey :one
SELECT * FROM idempotency_keys WHERE organization_id = @org_id AND key = @key;

-- name: SaveIdempotencyResponse :exec
UPDATE idempotency_keys SET response_status = @response_status, response_body = @response_body
 WHERE organization_id = @org_id AND key = @key;

-- name: DeleteIdempotencyKey :exec
DELETE FROM idempotency_keys WHERE organization_id = @org_id AND key = @key;

-- name: ListProjectAttachments :many
SELECT a.*, u.full_name AS uploaded_by_name
  FROM attachments a
  JOIN users u ON u.id = a.uploaded_by
 WHERE a.organization_id = @org_id AND a.project_id = @project_id AND a.status = 'uploaded'
 ORDER BY a.id DESC
 LIMIT 200;
