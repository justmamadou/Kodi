-- name: LockRFQNumbering :exec
-- Sérialise la numérotation des demandes de prix d'une entreprise (DP-0001, DP-0002...).
SELECT pg_advisory_xact_lock(hashtextextended('rfq-number:' || sqlc.arg(org_id)::uuid, 0));

-- name: CreateRFQ :one
INSERT INTO rfqs (id, organization_id, project_id, number, title, notes, response_deadline, delivery_date, created_by)
VALUES (@id, @org_id, @project_id,
        (SELECT COALESCE(max(number), 0) + 1 FROM rfqs WHERE organization_id = @org_id),
        @title, sqlc.narg('notes'), sqlc.narg('response_deadline'), sqlc.narg('delivery_date'), @created_by)
RETURNING *;

-- name: UpdateRFQDraft :one
UPDATE rfqs
   SET title = @title, notes = sqlc.narg('notes'),
       response_deadline = sqlc.narg('response_deadline'), delivery_date = sqlc.narg('delivery_date')
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: GetRFQ :one
SELECT r.*, p.name AS project_name, p.address AS project_address, p.city AS project_city,
       u.full_name AS created_by_name
  FROM rfqs r
  JOIN projects p ON p.id = r.project_id
  JOIN users u    ON u.id = r.created_by
 WHERE r.organization_id = @org_id AND r.id = @id;

-- name: LockRFQ :one
-- Les transitions et le dépôt d'offres se sérialisent sur la demande.
SELECT * FROM rfqs WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: LockRFQShared :one
-- Un dépôt d'offre attend une clôture ou une annulation en cours, sans bloquer les autres dépôts.
SELECT * FROM rfqs WHERE organization_id = @org_id AND id = @id FOR SHARE;

-- name: PublishRFQ :one
UPDATE rfqs SET status = 'published', published_at = now(), published_by = @actor_id
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: CloseRFQ :one
UPDATE rfqs SET status = 'closed', closed_at = now(), closed_by = @actor_id
 WHERE organization_id = @org_id AND id = @id AND status = 'published'
RETURNING *;

-- name: CancelRFQ :one
UPDATE rfqs SET status = 'cancelled', cancelled_at = now(), cancelled_by = @actor_id, cancel_reason = @reason
 WHERE organization_id = @org_id AND id = @id AND status IN ('draft','published')
RETURNING *;

-- name: ListRFQs :many
SELECT r.id, r.project_id, r.number, r.title, r.status, r.response_deadline, r.delivery_date,
       r.created_at, r.published_at, p.name AS project_name,
       (SELECT count(*) FROM rfq_items i WHERE i.rfq_id = r.id)::int AS item_count,
       (SELECT count(*) FROM rfq_suppliers s WHERE s.rfq_id = r.id)::int AS supplier_count,
       (SELECT count(DISTINCT o.rfq_supplier_id) FROM rfq_offers o WHERE o.rfq_id = r.id)::int AS responded_count
  FROM rfqs r
  JOIN projects p ON p.id = r.project_id
 WHERE r.organization_id = @org_id
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
   AND (sqlc.narg('project_id')::uuid IS NULL OR r.project_id = sqlc.narg('project_id'))
   AND (sqlc.narg('status')::text     IS NULL OR r.status = sqlc.narg('status'))
 ORDER BY r.created_at DESC, r.id DESC
 LIMIT 200;

-- name: DeleteRFQItems :exec
DELETE FROM rfq_items WHERE organization_id = @org_id AND rfq_id = @rfq_id;

-- name: InsertRFQItem :exec
INSERT INTO rfq_items (id, organization_id, rfq_id, position, kind, material_id, label, quantity, unit, specifications, needed_by)
VALUES (@id, @org_id, @rfq_id, @position, @kind, sqlc.narg('material_id'), @label, @quantity, @unit,
        sqlc.narg('specifications'), sqlc.narg('needed_by'));

-- name: ListRFQItems :many
SELECT * FROM rfq_items WHERE organization_id = @org_id AND rfq_id = @rfq_id ORDER BY position;

-- name: ListRFQSuppliers :many
SELECT rs.id, rs.supplier_id, rs.link_issued_at, rs.created_at,
       s.name AS supplier_name, s.phone AS supplier_phone, s.email AS supplier_email,
       (SELECT count(*) FROM rfq_offers o WHERE o.rfq_supplier_id = rs.id)::int AS offer_count
  FROM rfq_suppliers rs
  JOIN suppliers s ON s.id = rs.supplier_id
 WHERE rs.organization_id = @org_id AND rs.rfq_id = @rfq_id
 ORDER BY s.name, rs.id;

-- name: DeleteRFQSuppliersExcept :exec
DELETE FROM rfq_suppliers
 WHERE organization_id = @org_id AND rfq_id = @rfq_id AND NOT (supplier_id = ANY(@keep::uuid[]));

-- name: AddRFQSupplier :one
INSERT INTO rfq_suppliers (id, organization_id, rfq_id, supplier_id, invited_by)
VALUES (@id, @org_id, @rfq_id, @supplier_id, @invited_by)
ON CONFLICT (rfq_id, supplier_id) DO UPDATE SET rfq_id = rfq_suppliers.rfq_id
RETURNING id, (xmax = 0)::boolean AS inserted;

-- name: GetRFQSupplier :one
SELECT rs.*, s.name AS supplier_name, s.phone AS supplier_phone, s.email AS supplier_email
  FROM rfq_suppliers rs
  JOIN suppliers s ON s.id = rs.supplier_id
 WHERE rs.organization_id = @org_id AND rs.rfq_id = @rfq_id AND rs.supplier_id = @supplier_id;

-- name: LockRFQSupplierByID :one
-- Sérialise les versions d'offre d'un même fournisseur.
SELECT * FROM rfq_suppliers WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: IssueRFQLink :exec
UPDATE rfq_suppliers SET token_hash = @token_hash, link_issued_at = now()
 WHERE organization_id = @org_id AND id = @id;

-- name: GetActiveSupplier :one
SELECT * FROM suppliers WHERE organization_id = @org_id AND id = @id AND is_active;

-- name: GetActiveMaterial :one
SELECT * FROM materials WHERE organization_id = @org_id AND id = @id AND archived_at IS NULL;

-- name: RFQIsOpen :one
-- Ouverte aux offres : publiée et date limite non dépassée (incluse, heure de Dakar).
SELECT (status = 'published' AND response_deadline >= (now() AT TIME ZONE 'Africa/Dakar')::date)::boolean
  FROM rfqs WHERE organization_id = @org_id AND id = @id;

-- name: TodayInDakar :one
SELECT (now() AT TIME ZONE 'Africa/Dakar')::date;

-- name: GetOffer :one
SELECT * FROM rfq_offers WHERE organization_id = @org_id AND id = @id;

-- name: NextOfferVersion :one
SELECT (COALESCE(max(version), 0) + 1)::int FROM rfq_offers WHERE organization_id = @org_id AND rfq_supplier_id = @rfq_supplier_id;

-- name: InsertOffer :one
INSERT INTO rfq_offers (id, organization_id, rfq_id, rfq_supplier_id, supplier_id, version, total_amount,
                        valid_until, delivery_days, payment_terms, notes, contact_name, ip)
VALUES (@id, @org_id, @rfq_id, @rfq_supplier_id, @supplier_id, @version, @total_amount,
        sqlc.narg('valid_until'), sqlc.narg('delivery_days'), sqlc.narg('payment_terms'), sqlc.narg('notes'),
        sqlc.narg('contact_name'), sqlc.narg('ip'))
RETURNING *;

-- name: InsertOfferLine :one
INSERT INTO rfq_offer_lines (organization_id, offer_id, rfq_item_id, quantity, unit_price, comment)
VALUES (@org_id, @offer_id, @rfq_item_id, @quantity, @unit_price, sqlc.narg('comment'))
RETURNING amount;

-- name: ListOffers :many
-- Toutes les versions, la plus récente d'abord.
SELECT o.*, s.name AS supplier_name
  FROM rfq_offers o
  JOIN suppliers s ON s.id = o.supplier_id
 WHERE o.organization_id = @org_id AND o.rfq_id = @rfq_id
 ORDER BY o.submitted_at DESC, o.version DESC;

-- name: ListSupplierOffers :many
SELECT * FROM rfq_offers
 WHERE organization_id = @org_id AND rfq_supplier_id = @rfq_supplier_id
 ORDER BY version DESC;

-- name: ListOfferLines :many
SELECT l.* FROM rfq_offer_lines l
  JOIN rfq_offers o ON o.id = l.offer_id
 WHERE l.organization_id = @org_id AND o.rfq_id = @rfq_id;

-- name: GetOrganizationName :one
SELECT name FROM organizations WHERE id = @org_id;

-- name: SetRFQCriteria :one
UPDATE rfqs SET weight_price = @weight_price, weight_delivery = @weight_delivery, weight_coverage = @weight_coverage
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: AwardRFQ :one
UPDATE rfqs SET awarded_offer_id = @offer_id, awarded_by = @actor_id, awarded_at = now(), award_reason = sqlc.narg('reason')
 WHERE organization_id = @org_id AND id = @id AND status = 'closed'
RETURNING *;

-- name: ClearRFQAward :one
UPDATE rfqs SET awarded_offer_id = NULL, awarded_by = NULL, awarded_at = NULL, award_reason = NULL
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: IsLatestOffer :one
-- Seule la dernière version d'un fournisseur peut être retenue.
SELECT NOT EXISTS (SELECT 1 FROM rfq_offers o2 WHERE o2.rfq_supplier_id = o.rfq_supplier_id AND o2.version > o.version)::boolean
  FROM rfq_offers o
 WHERE o.organization_id = @org_id AND o.id = @id AND o.rfq_id = @rfq_id;

-- name: ActiveOrderForRFQ :one
SELECT id, number, status FROM purchase_orders
 WHERE organization_id = @org_id AND rfq_id = @rfq_id AND status <> 'cancelled'
 ORDER BY created_at DESC LIMIT 1;
