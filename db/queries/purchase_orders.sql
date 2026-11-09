-- name: LockPONumbering :exec
-- Sérialise la numérotation des bons de commande d'une entreprise (BC-0001, BC-0002...).
SELECT pg_advisory_xact_lock(hashtextextended('po-number:' || sqlc.arg(org_id)::uuid, 0));

-- name: CreatePurchaseOrder :one
INSERT INTO purchase_orders (id, organization_id, project_id, supplier_id, rfq_id, offer_id, number,
                             expected_delivery_date, delivery_address, payment_terms, notes, created_by)
VALUES (@id, @org_id, @project_id, @supplier_id, sqlc.narg('rfq_id'), sqlc.narg('offer_id'),
        (SELECT COALESCE(max(number), 0) + 1 FROM purchase_orders WHERE organization_id = @org_id),
        sqlc.narg('expected_delivery_date'), sqlc.narg('delivery_address'), sqlc.narg('payment_terms'), sqlc.narg('notes'),
        @created_by)
RETURNING *;

-- name: UpdatePODraft :one
UPDATE purchase_orders
   SET expected_delivery_date = sqlc.narg('expected_delivery_date'), delivery_address = sqlc.narg('delivery_address'),
       payment_terms = sqlc.narg('payment_terms'), notes = sqlc.narg('notes')
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: RecomputePOTotals :one
-- Totaux tirés des lignes (montants arrondis au franc par la base) : jamais saisis.
UPDATE purchase_orders po
   SET subtotal_amount = t.subtotal, tax_amount = t.tax, total_amount = t.subtotal + t.tax
  FROM (SELECT COALESCE(sum(amount), 0)::bigint AS subtotal, COALESCE(sum(tax_amount), 0)::bigint AS tax
          FROM purchase_order_lines WHERE organization_id = @org_id AND purchase_order_id = @id) t
 WHERE po.organization_id = @org_id AND po.id = @id
RETURNING po.*;

-- name: DeletePOLines :exec
DELETE FROM purchase_order_lines WHERE organization_id = @org_id AND purchase_order_id = @purchase_order_id;

-- name: InsertPOLine :exec
INSERT INTO purchase_order_lines (id, organization_id, purchase_order_id, position, rfq_item_id, material_id, label, unit,
                                  quantity_ordered, unit_price, tax_rate, comment)
VALUES (@id, @org_id, @purchase_order_id, @position, sqlc.narg('rfq_item_id'), sqlc.narg('material_id'), @label, @unit,
        @quantity_ordered, @unit_price, @tax_rate, sqlc.narg('comment'));

-- name: ListPOLines :many
SELECT * FROM purchase_order_lines WHERE organization_id = @org_id AND purchase_order_id = @purchase_order_id ORDER BY position;

-- name: GetPO :one
SELECT po.*, p.name AS project_name, p.address AS project_address, p.city AS project_city,
       s.name AS supplier_name, s.email AS supplier_email, s.phone AS supplier_phone,
       u.full_name AS created_by_name, r.number AS rfq_number
  FROM purchase_orders po
  JOIN projects p  ON p.id = po.project_id
  JOIN suppliers s ON s.id = po.supplier_id
  JOIN users u     ON u.id = po.created_by
  LEFT JOIN rfqs r ON r.id = po.rfq_id
 WHERE po.organization_id = @org_id AND po.id = @id;

-- name: LockPO :one
SELECT * FROM purchase_orders WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: ListPurchaseOrders :many
SELECT po.id, po.number, po.status, po.project_id, po.supplier_id, po.rfq_id, po.total_amount, po.currency,
       po.expected_delivery_date, po.created_at, po.sent_at, p.name AS project_name, s.name AS supplier_name,
       (SELECT count(*) FROM purchase_order_lines l WHERE l.purchase_order_id = po.id)::int AS line_count,
       (SELECT count(*) FROM purchase_order_lines l WHERE l.purchase_order_id = po.id AND l.quantity_remaining = 0)::int AS lines_received
  FROM purchase_orders po
  JOIN projects p  ON p.id = po.project_id
  JOIN suppliers s ON s.id = po.supplier_id
 WHERE po.organization_id = @org_id
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
   AND (sqlc.narg('project_id')::uuid  IS NULL OR po.project_id = sqlc.narg('project_id'))
   AND (sqlc.narg('supplier_id')::uuid IS NULL OR po.supplier_id = sqlc.narg('supplier_id'))
   AND (sqlc.narg('status')::text      IS NULL OR po.status = sqlc.narg('status'))
 ORDER BY po.created_at DESC, po.id DESC
 LIMIT 200;

-- name: SubmitPO :one
UPDATE purchase_orders SET status = 'submitted', submitted_by = @actor_id, submitted_at = now()
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: ReturnPO :one
UPDATE purchase_orders SET status = 'draft'
 WHERE organization_id = @org_id AND id = @id AND status = 'submitted'
RETURNING *;

-- name: ApprovePO :one
UPDATE purchase_orders SET status = 'approved', approved_by = @actor_id, approved_at = now()
 WHERE organization_id = @org_id AND id = @id AND status = 'submitted'
RETURNING *;

-- name: SendPO :one
UPDATE purchase_orders SET status = 'sent', sent_by = @actor_id, sent_at = now()
 WHERE organization_id = @org_id AND id = @id AND status = 'approved'
RETURNING *;

-- name: CancelPO :one
UPDATE purchase_orders SET status = 'cancelled', cancelled_by = @actor_id, cancelled_at = now(), cancel_reason = @reason
 WHERE organization_id = @org_id AND id = @id AND status IN ('draft','submitted','approved','sent')
RETURNING *;

-- name: SetPOReceptionStatus :one
UPDATE purchase_orders SET status = @status
 WHERE organization_id = @org_id AND id = @id AND status IN ('sent','partially_received')
RETURNING *;

-- name: POFullyReceived :one
SELECT COALESCE(bool_and(quantity_remaining = 0), false)::boolean
  FROM purchase_order_lines WHERE organization_id = @org_id AND purchase_order_id = @purchase_order_id;

-- name: InsertPOEvent :exec
INSERT INTO purchase_order_events (organization_id, purchase_order_id, event, from_status, to_status, actor_id, comment, changes)
VALUES (@org_id, @purchase_order_id, @event, sqlc.narg('from_status'), @to_status, @actor_id, sqlc.narg('comment'), sqlc.narg('changes'));

-- name: ListPOEvents :many
SELECT e.*, u.full_name AS actor_name
  FROM purchase_order_events e
  JOIN users u ON u.id = e.actor_id
 WHERE e.organization_id = @org_id AND e.purchase_order_id = @purchase_order_id
 ORDER BY e.id;

-- name: GetPOReceipt :one
SELECT * FROM purchase_order_receipts WHERE organization_id = @org_id AND id = @id;

-- name: InsertPOReceipt :one
INSERT INTO purchase_order_receipts (id, organization_id, purchase_order_id, received_on, delivery_note_ref, notes,
                                     stock_movement_id, received_by)
VALUES (@id, @org_id, @purchase_order_id, @received_on, sqlc.narg('delivery_note_ref'), sqlc.narg('notes'),
        sqlc.narg('stock_movement_id'), @received_by)
RETURNING *;

-- name: InsertPOReceiptLine :exec
INSERT INTO purchase_order_receipt_lines (organization_id, receipt_id, line_id, quantity)
VALUES (@org_id, @receipt_id, @line_id, @quantity);

-- name: ListPOReceipts :many
SELECT r.*, u.full_name AS received_by_name
  FROM purchase_order_receipts r
  JOIN users u ON u.id = r.received_by
 WHERE r.organization_id = @org_id AND r.purchase_order_id = @purchase_order_id
 ORDER BY r.created_at, r.id;

-- name: ListPOReceiptLines :many
SELECT rl.* FROM purchase_order_receipt_lines rl
  JOIN purchase_order_receipts r ON r.id = rl.receipt_id
 WHERE rl.organization_id = @org_id AND r.purchase_order_id = @purchase_order_id;

-- name: ReceiptNoteExists :one
SELECT EXISTS (SELECT 1 FROM purchase_order_receipts
                WHERE organization_id = @org_id AND purchase_order_id = @purchase_order_id
                  AND upper(regexp_replace(delivery_note_ref, '\s', '', 'g')) = upper(regexp_replace(@ref::text, '\s', '', 'g')))::boolean;
