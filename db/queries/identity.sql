-- name: CreateOrganization :one
INSERT INTO organizations (id, name, slug) VALUES (@id, @name, @slug) RETURNING *;

-- name: CreateOrganizationSettings :exec
INSERT INTO organization_settings (organization_id) VALUES (@org_id);

-- name: GetOrganization :one
SELECT * FROM organizations WHERE id = @org_id;

-- name: UpdateOrganization :one
UPDATE organizations
   SET name       = COALESCE(sqlc.narg('name'), name),
       slug       = COALESCE(sqlc.narg('slug'), slug),
       legal_name = COALESCE(sqlc.narg('legal_name'), legal_name),
       phone      = COALESCE(sqlc.narg('phone'), phone),
       email      = COALESCE(sqlc.narg('email'), email),
       address    = COALESCE(sqlc.narg('address'), address),
       city       = COALESCE(sqlc.narg('city'), city),
       activity_type = COALESCE(sqlc.narg('activity_type'), activity_type),
       row_version = row_version + 1
 WHERE id = @org_id
RETURNING *;

-- name: GetOrganizationSettings :one
SELECT * FROM organization_settings WHERE organization_id = @org_id;

-- name: UpdateOrganizationSettings :one
UPDATE organization_settings
   SET budget_alert_thresholds = @budget_alert_thresholds
 WHERE organization_id = @org_id
RETURNING *;

-- name: CreateUser :one
INSERT INTO users (id, organization_id, email, phone, full_name, role, password_hash, status)
VALUES (@id, @org_id, sqlc.narg('email'), sqlc.narg('phone'), @full_name, @role, sqlc.narg('password_hash'), @status)
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE organization_id = @org_id AND id = @id;

-- name: ListUsers :many
SELECT * FROM users WHERE organization_id = @org_id ORDER BY full_name, id;

-- name: UpdateUserRole :one
UPDATE users SET role = @role WHERE organization_id = @org_id AND id = @id RETURNING *;

-- name: SuspendUser :one
UPDATE users SET status = 'suspended', suspended_at = now()
 WHERE organization_id = @org_id AND id = @id AND status = 'active'
RETURNING *;

-- name: ReactivateUser :one
UPDATE users SET status = 'active', suspended_at = NULL, failed_login_count = 0, locked_until = NULL
 WHERE organization_id = @org_id AND id = @id AND status = 'suspended'
RETURNING *;

-- name: ActivateInvitedUser :one
UPDATE users SET status = 'active', password_hash = @password_hash, full_name = COALESCE(sqlc.narg('full_name'), full_name)
 WHERE organization_id = @org_id AND id = @id AND status = 'invited'
RETURNING *;

-- name: FindUserByContact :one
SELECT * FROM users
 WHERE organization_id = @org_id
   AND ((sqlc.narg('email')::citext IS NOT NULL AND email = sqlc.narg('email')::citext)
     OR (sqlc.narg('phone')::text IS NOT NULL AND phone = sqlc.narg('phone')::text))
 LIMIT 1;

-- name: RevokePendingInvitations :exec
UPDATE invitations SET revoked_at = now()
 WHERE organization_id = @org_id AND user_id = @user_id AND accepted_at IS NULL AND revoked_at IS NULL;

-- name: CountActiveOwners :one
SELECT count(*) FROM users WHERE organization_id = @org_id AND role = 'owner' AND status = 'active';

-- name: RecordLoginFailure :exec
-- Verrouillage de 15 minutes après 5 échecs ; un verrou expiré repart de zéro.
UPDATE users
   SET failed_login_count = CASE WHEN locked_until IS NOT NULL AND locked_until <= now() THEN 1 ELSE failed_login_count + 1 END,
       locked_until = CASE
         WHEN locked_until IS NOT NULL AND locked_until <= now() THEN NULL
         WHEN failed_login_count + 1 >= 5 THEN now() + interval '15 minutes'
         ELSE locked_until END
 WHERE organization_id = @org_id AND id = @id;

-- name: RecordLoginSuccess :exec
UPDATE users SET failed_login_count = 0, locked_until = NULL, last_login_at = now()
 WHERE organization_id = @org_id AND id = @id;

-- name: CreateSession :one
INSERT INTO sessions (id, organization_id, user_id, token_hash, remember, user_agent, ip, expires_at)
VALUES (@id, @org_id, @user_id, @token_hash, @remember, sqlc.narg('user_agent'), sqlc.narg('ip'), @expires_at)
RETURNING *;

-- name: TouchSession :exec
UPDATE sessions SET last_seen_at = now(), expires_at = @expires_at
 WHERE organization_id = @org_id AND id = @id AND revoked_at IS NULL;

-- name: RevokeSession :execrows
UPDATE sessions SET revoked_at = now()
 WHERE organization_id = @org_id AND id = @id AND user_id = @user_id AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
UPDATE sessions SET revoked_at = now()
 WHERE organization_id = @org_id AND user_id = @user_id AND revoked_at IS NULL;

-- name: ListUserSessions :many
SELECT * FROM sessions
 WHERE organization_id = @org_id AND user_id = @user_id AND revoked_at IS NULL AND expires_at > now()
 ORDER BY last_seen_at DESC;

-- name: CreateInvitation :one
INSERT INTO invitations (id, organization_id, user_id, token_hash, invited_by, expires_at, project_ids)
VALUES (@id, @org_id, @user_id, @token_hash, @invited_by, @expires_at, @project_ids)
RETURNING *;

-- name: AcceptInvitation :one
-- Usage unique : la garde ne laisse passer qu'une seule transaction, même en concurrence.
UPDATE invitations SET accepted_at = now()
 WHERE organization_id = @org_id AND id = @id
   AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()
RETURNING *;

-- name: RevokeInvitation :one
UPDATE invitations SET revoked_at = now()
 WHERE organization_id = @org_id AND id = @id AND accepted_at IS NULL AND revoked_at IS NULL
RETURNING *;

-- name: ListPendingInvitations :many
SELECT i.id, i.user_id, i.invited_by, i.expires_at, i.created_at, i.project_ids, u.full_name, u.email, u.phone, u.role
  FROM invitations i
  JOIN users u ON u.id = i.user_id
 WHERE i.organization_id = @org_id AND i.accepted_at IS NULL AND i.revoked_at IS NULL AND i.expires_at > now()
 ORDER BY i.created_at DESC;

-- name: ListMembers :many
-- Vue de l'écran Équipe : les chantiers de chaque membre, jamais les secrets.
SELECT u.id, u.email, u.phone, u.full_name, u.role, u.status, u.last_login_at, u.suspended_at,
       COALESCE((SELECT array_agg(t.name ORDER BY t.name)
                   FROM (SELECT p.name FROM project_members m JOIN projects p ON p.id = m.project_id
                          WHERE m.user_id = u.id AND p.status <> 'archived'
                         UNION
                         SELECT p.name FROM projects p
                          WHERE p.manager_id = u.id AND p.status <> 'archived') t), '{}')::text[] AS project_names
  FROM users u
 WHERE u.organization_id = @org_id
 ORDER BY u.full_name, u.id;

-- name: ListDirectory :many
-- Annuaire réduit : de quoi désigner un responsable, sans coordonnées.
SELECT id, full_name, role FROM users
 WHERE organization_id = @org_id AND status = 'active'
 ORDER BY full_name, id;

-- name: LockUser :one
-- Sérialise les demandes de réinitialisation d'un même compte : le plafond ne se contourne pas en rafale.
SELECT * FROM users WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: CountRecentPasswordResets :one
-- Plafond par compte : au-delà, la demande est ignorée sans que la réponse change.
SELECT count(*) FROM password_resets
 WHERE organization_id = @org_id AND user_id = @user_id AND created_at > now() - interval '1 hour';

-- name: RevokePendingPasswordResets :exec
UPDATE password_resets SET revoked_at = now()
 WHERE organization_id = @org_id AND user_id = @user_id AND used_at IS NULL AND revoked_at IS NULL;

-- name: CreatePasswordReset :exec
INSERT INTO password_resets (id, organization_id, user_id, token_hash, expires_at, ip)
VALUES (@id, @org_id, @user_id, @token_hash, @expires_at, sqlc.narg('ip'));

-- name: UsePasswordReset :one
-- Usage unique : de deux confirmations concurrentes, une seule passe.
UPDATE password_resets SET used_at = now()
 WHERE organization_id = @org_id AND id = @id
   AND used_at IS NULL AND revoked_at IS NULL AND expires_at > now()
RETURNING user_id;

-- name: SetUserPassword :execrows
-- Un nouveau mot de passe lève aussi le verrouillage dû aux essais manqués.
UPDATE users SET password_hash = @password_hash, failed_login_count = 0, locked_until = NULL
 WHERE organization_id = @org_id AND id = @id AND status = 'active';
