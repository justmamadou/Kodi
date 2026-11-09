-- name: ListSuppliers :many
SELECT * FROM suppliers
 WHERE organization_id = @org_id
   AND (@include_archived::boolean OR is_active)
   AND (sqlc.narg('activity')::text IS NULL OR sqlc.narg('activity')::text = ANY(activities))
   AND (sqlc.narg('q')::text IS NULL
        OR name ILIKE '%' || sqlc.narg('q') || '%'
        OR legal_name ILIKE '%' || sqlc.narg('q') || '%'
        OR ninea ILIKE '%' || sqlc.narg('q') || '%'
        OR phone ILIKE '%' || sqlc.narg('q') || '%'
        OR email ILIKE '%' || sqlc.narg('q') || '%')
 ORDER BY is_active DESC, name, id
 LIMIT 500;

-- name: GetSupplier :one
SELECT * FROM suppliers WHERE organization_id = @org_id AND id = @id;

-- name: LockSupplier :one
SELECT * FROM suppliers WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: CreateSupplier :one
INSERT INTO suppliers (id, organization_id, name, phone, email, legal_name, legal_form, ninea, rccm,
                       address, city, activities, notes, created_by)
VALUES (@id, @org_id, @name, sqlc.narg('phone'), sqlc.narg('email'), sqlc.narg('legal_name'), sqlc.narg('legal_form'),
        sqlc.narg('ninea'), sqlc.narg('rccm'), sqlc.narg('address'), sqlc.narg('city'), @activities, sqlc.narg('notes'),
        @created_by)
RETURNING *;

-- name: UpdateSupplier :one
-- Remplace la fiche : le service fusionne d'abord la modification avec l'existant.
UPDATE suppliers
   SET name = @name, phone = sqlc.narg('phone'), email = sqlc.narg('email'),
       legal_name = sqlc.narg('legal_name'), legal_form = sqlc.narg('legal_form'),
       ninea = sqlc.narg('ninea'), rccm = sqlc.narg('rccm'),
       address = sqlc.narg('address'), city = sqlc.narg('city'), activities = @activities, notes = sqlc.narg('notes')
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: UpsertSupplierByName :one
-- Saisie terrain, y compris hors ligne : un nom suffit, et un nom déjà connu (casse, accents,
-- espaces et ponctuation mis à part) réutilise la fiche existante, même archivée, sans la réactiver.
INSERT INTO suppliers (id, organization_id, name, created_by)
VALUES (@id, @org_id, @name, @created_by)
ON CONFLICT (organization_id, name_key) DO UPDATE SET name = suppliers.name
RETURNING *;

-- name: ArchiveSupplier :one
UPDATE suppliers SET is_active = false, archived_at = now(), archived_by = @actor_id
 WHERE organization_id = @org_id AND id = @id AND is_active
RETURNING *;

-- name: RestoreSupplier :one
UPDATE suppliers SET is_active = true, archived_at = NULL, archived_by = NULL
 WHERE organization_id = @org_id AND id = @id AND NOT is_active
RETURNING *;

-- name: FindSupplierDuplicates :many
-- Règles de doublon. Bloquantes : même nom (sans casse, accents, espaces ni ponctuation), même NINEA,
-- même RCCM. Probables : même téléphone ou email (fiche ou contact), nom très proche une fois la forme
-- juridique retirée.
SELECT s.id, s.name, s.is_active,
       (s.name_key = supplier_name_key(@name::text))::boolean                                        AS same_name,
       COALESCE(s.ninea = sqlc.narg('ninea')::text, false)::boolean                                    AS same_ninea,
       COALESCE(s.rccm = sqlc.narg('rccm')::text, false)::boolean                                      AS same_rccm,
       COALESCE(s.phone = sqlc.narg('phone')::text
          OR EXISTS (SELECT 1 FROM supplier_contacts c WHERE c.supplier_id = s.id AND c.phone = sqlc.narg('phone')::text), false)::boolean AS same_phone,
       COALESCE(s.email = sqlc.narg('email')::citext
          OR EXISTS (SELECT 1 FROM supplier_contacts c WHERE c.supplier_id = s.id AND c.email = sqlc.narg('email')::citext), false)::boolean AS same_email,
       (supplier_loose_key(s.name) = supplier_loose_key(@name::text)
          OR similarity(supplier_loose_key(s.name), supplier_loose_key(@name::text)) >= 0.75)::boolean AS similar_name
  FROM suppliers s
 WHERE s.organization_id = @org_id
   AND (sqlc.narg('exclude_id')::uuid IS NULL OR s.id <> sqlc.narg('exclude_id')::uuid)
   AND (s.name_key = supplier_name_key(@name::text)
        OR (sqlc.narg('ninea')::text IS NOT NULL AND s.ninea = sqlc.narg('ninea')::text)
        OR (sqlc.narg('rccm')::text IS NOT NULL AND s.rccm = sqlc.narg('rccm')::text)
        OR (sqlc.narg('phone')::text IS NOT NULL AND (s.phone = sqlc.narg('phone')::text
            OR EXISTS (SELECT 1 FROM supplier_contacts c WHERE c.supplier_id = s.id AND c.phone = sqlc.narg('phone')::text)))
        OR (sqlc.narg('email')::text IS NOT NULL AND (s.email = sqlc.narg('email')::citext
            OR EXISTS (SELECT 1 FROM supplier_contacts c WHERE c.supplier_id = s.id AND c.email = sqlc.narg('email')::citext)))
        OR supplier_loose_key(s.name) = supplier_loose_key(@name::text)
        OR similarity(supplier_loose_key(s.name), supplier_loose_key(@name::text)) >= 0.75)
 ORDER BY s.is_active DESC, s.name
 LIMIT 10;

-- name: ListSupplierContacts :many
SELECT * FROM supplier_contacts
 WHERE organization_id = @org_id AND supplier_id = @supplier_id
 ORDER BY is_primary DESC, full_name, id;

-- name: GetSupplierContact :one
SELECT * FROM supplier_contacts WHERE organization_id = @org_id AND id = @id;

-- name: ClearPrimaryContact :exec
UPDATE supplier_contacts SET is_primary = false
 WHERE organization_id = @org_id AND supplier_id = @supplier_id AND is_primary;

-- name: CreateSupplierContact :one
INSERT INTO supplier_contacts (id, organization_id, supplier_id, full_name, job_title, phone, email, is_primary, notes, created_by)
VALUES (@id, @org_id, @supplier_id, @full_name, sqlc.narg('job_title'), sqlc.narg('phone'), sqlc.narg('email'),
        @is_primary, sqlc.narg('notes'), @created_by)
RETURNING *;

-- name: UpdateSupplierContact :one
UPDATE supplier_contacts
   SET full_name = @full_name, job_title = sqlc.narg('job_title'), phone = sqlc.narg('phone'),
       email = sqlc.narg('email'), is_primary = @is_primary, notes = sqlc.narg('notes')
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: DeleteSupplierContact :execrows
DELETE FROM supplier_contacts WHERE organization_id = @org_id AND id = @id;

-- name: ListSupplierTerms :many
-- Toutes les versions, la plus récente d'abord.
SELECT t.*, u.full_name AS created_by_name
  FROM supplier_terms t
  JOIN users u ON u.id = t.created_by
 WHERE t.organization_id = @org_id AND t.supplier_id = @supplier_id
 ORDER BY t.version DESC;

-- name: NextSupplierTermsVersion :one
SELECT (COALESCE(max(version), 0) + 1)::int FROM supplier_terms WHERE organization_id = @org_id AND supplier_id = @supplier_id;

-- name: InsertSupplierTerms :one
INSERT INTO supplier_terms (id, organization_id, supplier_id, version, effective_from, payment_method, payment_delay_days,
                            advance_pct, discount_pct, delivery_days, delivery_included, minimum_order_amount,
                            credit_limit, notes, created_by)
VALUES (@id, @org_id, @supplier_id, @version, @effective_from, sqlc.narg('payment_method'), sqlc.narg('payment_delay_days'),
        sqlc.narg('advance_pct'), sqlc.narg('discount_pct'), sqlc.narg('delivery_days'), sqlc.narg('delivery_included'),
        sqlc.narg('minimum_order_amount'), sqlc.narg('credit_limit'), sqlc.narg('notes'), @created_by)
RETURNING *;

-- name: SupplierConsultations :many
-- Demandes de prix où le fournisseur a été consulté, limitées aux chantiers visibles par l'acteur.
SELECT r.id AS rfq_id, r.number, r.title, r.status, r.response_deadline, r.published_at,
       p.id AS project_id, p.name AS project_name, rs.created_at AS consulted_at,
       (SELECT count(*) FROM rfq_offers o WHERE o.rfq_supplier_id = rs.id)::int AS offer_count,
       last.total_amount AS last_offer_total, last.submitted_at AS last_offer_at
  FROM rfq_suppliers rs
  JOIN rfqs r     ON r.id = rs.rfq_id
  JOIN projects p ON p.id = r.project_id
  LEFT JOIN rfq_offers last ON last.rfq_supplier_id = rs.id
   AND last.version = (SELECT max(x.version) FROM rfq_offers x WHERE x.rfq_supplier_id = rs.id)
 WHERE rs.organization_id = @org_id AND rs.supplier_id = @supplier_id
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
 ORDER BY rs.created_at DESC
 LIMIT 100;

-- name: SupplierPurchases :many
-- Achats enregistrés auprès du fournisseur (dépenses), limités aux chantiers visibles par l'acteur.
SELECT e.id, e.description, e.amount, e.amount_paid, e.expense_date, e.status, e.payment_status,
       p.id AS project_id, p.name AS project_name
  FROM expenses e
  JOIN projects p ON p.id = e.project_id
 WHERE e.organization_id = @org_id AND e.supplier_id = @supplier_id AND e.status <> 'cancelled'
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
 ORDER BY e.expense_date DESC, e.id DESC
 LIMIT 100;

-- name: SupplierPurchaseTotals :one
SELECT count(*)::int AS purchase_count,
       COALESCE(sum(e.amount) FILTER (WHERE e.status = 'approved'), 0)::bigint AS approved_amount,
       COALESCE(sum(e.amount_paid), 0)::bigint AS paid_amount
  FROM expenses e
  JOIN projects p ON p.id = e.project_id
 WHERE e.organization_id = @org_id AND e.supplier_id = @supplier_id AND e.status <> 'cancelled'
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id));

-- name: SupplierOrders :many
-- Bons de commande du fournisseur, limités aux chantiers visibles par l'acteur.
SELECT po.id, po.number, po.status, po.total_amount, po.created_at, po.expected_delivery_date,
       p.id AS project_id, p.name AS project_name
  FROM purchase_orders po
  JOIN projects p ON p.id = po.project_id
 WHERE po.organization_id = @org_id AND po.supplier_id = @supplier_id
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
 ORDER BY po.created_at DESC
 LIMIT 100;
