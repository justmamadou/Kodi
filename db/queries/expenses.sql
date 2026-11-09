-- name: ListCategories :many
SELECT * FROM expense_categories WHERE organization_id = @org_id ORDER BY sort_order, name;

-- name: CreateCategory :one
INSERT INTO expense_categories (id, organization_id, name, system_code, sort_order)
VALUES (@id, @org_id, @name, sqlc.narg('system_code'), @sort_order)
RETURNING *;

-- name: UpdateCategory :one
UPDATE expense_categories
   SET name = COALESCE(sqlc.narg('name'), name),
       is_active = COALESCE(sqlc.narg('is_active'), is_active),
       sort_order = COALESCE(sqlc.narg('sort_order'), sort_order)
 WHERE organization_id = @org_id AND id = @id
RETURNING *;

-- name: GetCategory :one
SELECT * FROM expense_categories WHERE organization_id = @org_id AND id = @id;

-- name: CreateExpense :one
-- Idempotent : l'id est généré par le client ; un renvoi offline ne crée pas de doublon.
INSERT INTO expenses (id, organization_id, project_id, category_id, supplier_id, description, amount,
                      currency, expense_date, comment, declared_paid_on, declared_payment_method, created_by)
VALUES (@id, @org_id, @project_id, @category_id, sqlc.narg('supplier_id'), @description, @amount,
        @currency, @expense_date, sqlc.narg('comment'), sqlc.narg('declared_paid_on'),
        sqlc.narg('declared_payment_method'), @created_by)
ON CONFLICT (id) DO NOTHING
RETURNING *;

-- name: GetExpense :one
SELECT * FROM expenses WHERE organization_id = @org_id AND id = @id;

-- name: GetExpenseLabels :one
SELECT p.name AS project_name, c.name AS category_name, s.name AS supplier_name, u.full_name AS created_by_name
  FROM expenses e
  JOIN projects p           ON p.id = e.project_id
  JOIN expense_categories c ON c.id = e.category_id
  LEFT JOIN suppliers s     ON s.id = e.supplier_id
  JOIN users u              ON u.id = e.created_by
 WHERE e.organization_id = @org_id AND e.id = @id;

-- name: ListExpenseDuplicates :many
-- Même chantier, même montant, même fournisseur, à ± 2 jours : un avertissement, la décision reste humaine.
SELECT d.id, d.description, d.amount, d.expense_date, d.status, u.full_name AS created_by_name
  FROM expenses e
  JOIN expenses d ON d.organization_id = e.organization_id AND d.project_id = e.project_id AND d.id <> e.id
                 AND d.amount = e.amount AND d.status <> 'cancelled'
                 AND d.supplier_id IS NOT DISTINCT FROM e.supplier_id
                 AND d.expense_date BETWEEN e.expense_date - 2 AND e.expense_date + 2
  JOIN users u ON u.id = d.created_by
 WHERE e.organization_id = @org_id AND e.id = @id
 ORDER BY d.expense_date DESC, d.id
 LIMIT 5;

-- name: CountExpenses :many
-- Compteurs des onglets de la file de dépenses, sur les chantiers visibles.
SELECT e.status, e.payment_status, count(*)::bigint AS total, COALESCE(SUM(e.amount), 0)::bigint AS amount
  FROM expenses e
  JOIN projects p ON p.id = e.project_id
 WHERE e.organization_id = @org_id
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
   AND (sqlc.narg('project_id')::uuid  IS NULL OR e.project_id = sqlc.narg('project_id'))
   AND (sqlc.narg('category_id')::uuid IS NULL OR e.category_id = sqlc.narg('category_id'))
   AND (sqlc.narg('date_from')::date   IS NULL OR e.expense_date >= sqlc.narg('date_from'))
   AND (sqlc.narg('date_to')::date     IS NULL OR e.expense_date <= sqlc.narg('date_to'))
 GROUP BY e.status, e.payment_status;

-- name: GetExpenseForUpdate :one
SELECT * FROM expenses WHERE organization_id = @org_id AND id = @id FOR UPDATE;

-- name: UpdateDraftExpense :one
UPDATE expenses
   SET category_id  = COALESCE(sqlc.narg('category_id'), category_id),
       supplier_id  = COALESCE(sqlc.narg('supplier_id'), supplier_id),
       description  = COALESCE(sqlc.narg('description'), description),
       amount       = COALESCE(sqlc.narg('amount'), amount),
       expense_date = COALESCE(sqlc.narg('expense_date'), expense_date),
       row_version  = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status = 'draft' AND row_version = @row_version
RETURNING *;

-- name: SubmitExpense :one
UPDATE expenses
   SET status = 'pending', submitted_at = now(), row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status = 'draft'
RETURNING *;

-- name: ApproveExpense :one
-- La garde sur le statut fait perdre proprement une course entre deux approbateurs.
UPDATE expenses e
   SET status = 'approved', decided_by = @actor_id, decided_at = now(), decision_comment = sqlc.narg('comment'),
       budget_version_id = (SELECT p.current_budget_version_id FROM projects p WHERE p.id = e.project_id),
       row_version = e.row_version + 1
 WHERE e.organization_id = @org_id AND e.id = @id AND e.status = 'pending'
RETURNING e.*;

-- name: RejectExpense :one
UPDATE expenses
   SET status = 'rejected', decided_by = @actor_id, decided_at = now(), decision_comment = @comment,
       row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status = 'pending'
RETURNING *;

-- name: ReturnExpenseToDraft :one
-- Retrait par l'auteur (pending) ou correction après rejet (rejected).
UPDATE expenses
   SET status = 'draft', submitted_at = NULL, decided_by = NULL, decided_at = NULL, decision_comment = NULL,
       row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status = @from_status
RETURNING *;

-- name: CancelExpense :one
UPDATE expenses
   SET status = 'cancelled', decided_by = NULL, decided_at = NULL, budget_version_id = NULL,
       decision_comment = sqlc.narg('comment'), row_version = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status IN ('draft','approved') AND amount_paid = 0
RETURNING *;

-- name: InsertExpenseApproval :exec
INSERT INTO expense_approvals (organization_id, expense_id, event, from_status, to_status, actor_id, comment)
VALUES (@org_id, @expense_id, @event, @from_status, @to_status, @actor_id, sqlc.narg('comment'));

-- name: ListExpenseApprovals :many
SELECT a.id, a.event, a.from_status, a.to_status, a.actor_id, a.comment, a.created_at, u.full_name AS actor_name
  FROM expense_approvals a
  JOIN users u ON u.id = a.actor_id
 WHERE a.organization_id = @org_id AND a.expense_id = @expense_id
 ORDER BY a.id;

-- name: ListExpenses :many
SELECT e.*, p.name AS project_name, c.name AS category_name, s.name AS supplier_name, u.full_name AS created_by_name,
       EXISTS (SELECT 1 FROM expenses d
                WHERE d.organization_id = e.organization_id AND d.project_id = e.project_id AND d.id <> e.id
                  AND d.amount = e.amount AND d.status <> 'cancelled'
                  AND d.supplier_id IS NOT DISTINCT FROM e.supplier_id
                  AND d.expense_date BETWEEN e.expense_date - 2 AND e.expense_date + 2)::boolean AS possible_duplicate
  FROM expenses e
  JOIN projects p           ON p.id = e.project_id
  JOIN expense_categories c ON c.id = e.category_id
  LEFT JOIN suppliers s     ON s.id = e.supplier_id
  JOIN users u              ON u.id = e.created_by
 WHERE e.organization_id = @org_id
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
   AND (sqlc.narg('project_id')::uuid  IS NULL OR e.project_id = sqlc.narg('project_id'))
   AND (sqlc.narg('status')::text      IS NULL OR e.status = sqlc.narg('status'))
   AND (sqlc.narg('payment_status')::text IS NULL OR e.payment_status = sqlc.narg('payment_status'))
   AND (NOT @unpaid_only::boolean OR e.payment_status <> 'paid')
   AND (sqlc.narg('category_id')::uuid IS NULL OR e.category_id = sqlc.narg('category_id'))
   AND (sqlc.narg('date_from')::date   IS NULL OR e.expense_date >= sqlc.narg('date_from'))
   AND (sqlc.narg('date_to')::date     IS NULL OR e.expense_date <= sqlc.narg('date_to'))
   AND (sqlc.narg('q')::text           IS NULL OR e.description ILIKE '%' || sqlc.narg('q') || '%')
   AND (sqlc.narg('cursor_date')::date IS NULL
        OR (e.expense_date, e.id) < (sqlc.narg('cursor_date')::date, sqlc.narg('cursor_id')::uuid))
 ORDER BY e.expense_date DESC, e.id DESC
 LIMIT @page_size;

-- name: SetExpenseHasReceipt :exec
UPDATE expenses SET has_receipt = true WHERE organization_id = @org_id AND id = @id;

-- name: InsertPayment :one
INSERT INTO payments (id, organization_id, expense_id, amount, paid_on, method, reference, recorded_by)
VALUES (@id, @org_id, @expense_id, @amount, @paid_on, @method, sqlc.narg('reference'), @recorded_by)
RETURNING *;

-- name: ApplyPaymentToExpense :one
UPDATE expenses
   SET amount_paid    = amount_paid + @delta::bigint,
       payment_status = CASE WHEN amount_paid + @delta::bigint = amount THEN 'paid' ELSE 'partially_paid' END,
       row_version    = row_version + 1
 WHERE organization_id = @org_id AND id = @id AND status = 'approved'
RETURNING *;

-- name: ListPayments :many
SELECT pay.*, u.full_name AS recorded_by_name
  FROM payments pay
  JOIN users u ON u.id = pay.recorded_by
 WHERE pay.organization_id = @org_id AND pay.expense_id = @expense_id
 ORDER BY pay.created_at, pay.id;
