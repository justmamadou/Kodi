-- name: CreateMaterial :one
INSERT INTO materials (id, organization_id, name, unit, sku, low_stock_threshold)
VALUES (@id, @org_id, @name, @unit, sqlc.narg('sku'), sqlc.narg('low_stock_threshold'))
RETURNING *;

-- name: GetMaterial :one
SELECT * FROM materials WHERE organization_id = @org_id AND id = @id;

-- name: UpdateMaterial :one
UPDATE materials
   SET name = COALESCE(sqlc.narg('name'), name),
       unit = COALESCE(sqlc.narg('unit'), unit),
       sku  = COALESCE(sqlc.narg('sku'), sku),
       low_stock_threshold = COALESCE(sqlc.narg('low_stock_threshold'), low_stock_threshold)
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: ArchiveMaterial :one
UPDATE materials SET archived_at = now()
 WHERE organization_id = @org_id AND id = @id AND archived_at IS NULL
RETURNING *;

-- name: ListMaterials :many
SELECT * FROM materials
 WHERE organization_id = @org_id
   AND (@include_archived::boolean OR archived_at IS NULL)
   AND (sqlc.narg('q')::text IS NULL OR name ILIKE '%' || sqlc.narg('q') || '%' OR sku ILIKE '%' || sqlc.narg('q') || '%')
 ORDER BY name, id
 LIMIT 500;

-- name: CreateStockMovement :one
INSERT INTO stock_movements (id, organization_id, project_id, movement_type, occurred_on, reason, created_by)
VALUES (@id, @org_id, @project_id, @movement_type, @occurred_on, sqlc.narg('reason'), @created_by)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetStockMovement :one
SELECT * FROM stock_movements WHERE organization_id = @org_id AND id = @id;

-- name: GetStockMovementForUpdate :one
SELECT * FROM stock_movements WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: InsertStockMovementItem :exec
INSERT INTO stock_movement_items (organization_id, movement_id, material_id, quantity)
VALUES (@org_id, @movement_id, @material_id, @quantity);

-- name: ListStockMovementItems :many
-- Trié par article : un ordre stable des verrous évite les interblocages au posting.
SELECT i.material_id, i.quantity, m.name AS material_name, m.unit, m.archived_at
  FROM stock_movement_items i
  JOIN materials m ON m.id = i.material_id
 WHERE i.organization_id = @org_id AND i.movement_id = @movement_id
 ORDER BY i.material_id;

-- name: PostStockMovement :one
UPDATE stock_movements SET status = 'posted', posted_by = @actor_id, posted_at = now()
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: EnsureStockRow :exec
-- Étape séparée : un CHECK est évalué avant ON CONFLICT, un delta négatif échouerait à tort dans un upsert.
INSERT INTO project_material_stock (organization_id, project_id, material_id)
VALUES (@org_id, @project_id, @material_id)
ON CONFLICT (project_id, material_id) DO NOTHING;

-- name: ApplyStockDelta :one
-- Le verrou de ligne sérialise les mouvements concurrents ; CHECK (on_hand >= 0) refuse le négatif.
UPDATE project_material_stock
   SET on_hand = on_hand + @delta::numeric,
       last_movement_on = GREATEST(last_movement_on, @occurred_on::date),
       updated_at = now()
 WHERE organization_id = @org_id AND project_id = @project_id AND material_id = @material_id
RETURNING on_hand;

-- name: ListProjectStock :many
SELECT s.material_id, m.name, m.unit, m.sku, m.archived_at, s.on_hand, s.last_movement_on, m.low_stock_threshold,
       (m.low_stock_threshold IS NOT NULL AND s.on_hand <= m.low_stock_threshold)::boolean AS is_low,
       COALESCE(j.qty_in, 0)::numeric     AS qty_in,
       COALESCE(j.qty_out, 0)::numeric    AS qty_out,
       COALESCE(j.qty_adjust, 0)::numeric AS qty_adjust,
       j.last_type::text                  AS last_movement_type
  FROM project_material_stock s
  JOIN materials m ON m.id = s.material_id
  LEFT JOIN LATERAL (
    SELECT SUM(i.quantity) FILTER (WHERE sm.movement_type = 'in')         AS qty_in,
           SUM(i.quantity) FILTER (WHERE sm.movement_type = 'out')        AS qty_out,
           SUM(i.quantity) FILTER (WHERE sm.movement_type = 'adjustment') AS qty_adjust,
           (array_agg(sm.movement_type ORDER BY sm.occurred_on DESC, sm.id DESC))[1] AS last_type
      FROM stock_movement_items i
      JOIN stock_movements sm ON sm.id = i.movement_id
     WHERE sm.project_id = s.project_id AND sm.status = 'posted' AND i.material_id = s.material_id
  ) j ON true
 WHERE s.organization_id = @org_id AND s.project_id = @project_id
   AND (sqlc.narg('q')::text IS NULL OR m.name ILIKE '%' || sqlc.narg('q') || '%')
 ORDER BY m.name, s.material_id;

-- name: CountPostedMovementsSince :one
SELECT count(*)::bigint AS total FROM stock_movements
 WHERE organization_id = @org_id AND project_id = @project_id AND status = 'posted' AND occurred_on >= @since::date;

-- name: ListStockHistory :many
SELECT sm.id AS movement_id, sm.movement_type, sm.occurred_on, sm.reason, sm.posted_at,
       i.material_id, i.quantity, m.name AS material_name, m.unit,
       sm.posted_by AS author_id, u.full_name AS author_name
  FROM stock_movements sm
  JOIN stock_movement_items i ON i.movement_id = sm.id
  JOIN materials m ON m.id = i.material_id
  JOIN users u ON u.id = sm.posted_by
 WHERE sm.organization_id = @org_id AND sm.project_id = @project_id AND sm.status = 'posted'
   AND (sqlc.narg('material_id')::uuid   IS NULL OR i.material_id = sqlc.narg('material_id'))
   AND (sqlc.narg('movement_type')::text IS NULL OR sm.movement_type = sqlc.narg('movement_type'))
   AND (sqlc.narg('date_from')::date     IS NULL OR sm.occurred_on >= sqlc.narg('date_from'))
   AND (sqlc.narg('date_to')::date       IS NULL OR sm.occurred_on <= sqlc.narg('date_to'))
 ORDER BY sm.occurred_on DESC, sm.id DESC, i.material_id
 LIMIT @page_size OFFSET @page_offset;

-- name: ReconcileProjectStock :many
-- Rapprochement : solde projeté contre recalcul depuis le journal des mouvements postés.
SELECT s.material_id, s.on_hand,
       COALESCE((SELECT SUM(CASE sm.movement_type WHEN 'out' THEN -i.quantity ELSE i.quantity END)
                   FROM stock_movement_items i
                   JOIN stock_movements sm ON sm.id = i.movement_id
                  WHERE sm.project_id = s.project_id AND sm.status = 'posted' AND i.material_id = s.material_id),
                0)::numeric AS journal_total
  FROM project_material_stock s
 WHERE s.organization_id = @org_id AND s.project_id = @project_id;
