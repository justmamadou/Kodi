-- name: ListProjectFinancials :many
-- Budget de référence = version publiée en vigueur. Les archivés sont exclus du portefeuille.
SELECT p.id, p.reference, p.name, p.status, p.manager_id, u.full_name AS manager_name, p.declared_progress_pct,
       COALESCE(v.total_amount, 0)::bigint AS budget,
       v.version_no                        AS budget_version_no,
       COALESCE(SUM(e.amount)      FILTER (WHERE e.status = 'approved'), 0)::bigint AS approved,
       COALESCE(SUM(e.amount)      FILTER (WHERE e.status = 'pending'),  0)::bigint AS pending,
       COALESCE(SUM(e.amount_paid) FILTER (WHERE e.status = 'approved'), 0)::bigint AS paid
  FROM projects p
  JOIN users u ON u.id = p.manager_id
  LEFT JOIN budget_versions v ON v.id = p.current_budget_version_id
  LEFT JOIN expenses e ON e.organization_id = p.organization_id AND e.project_id = p.id
                      AND e.status IN ('pending','approved')
 WHERE p.organization_id = @org_id
   AND p.status <> 'archived'
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
 GROUP BY p.id, u.full_name, v.total_amount, v.version_no
 ORDER BY p.name, p.id;

-- name: GetProjectFinancials :one
SELECT p.id, p.reference, p.name, p.status, p.manager_id, u.full_name AS manager_name, p.declared_progress_pct,
       COALESCE(v.total_amount, 0)::bigint AS budget,
       v.version_no                        AS budget_version_no,
       COALESCE(SUM(e.amount)      FILTER (WHERE e.status = 'approved'), 0)::bigint AS approved,
       COALESCE(SUM(e.amount)      FILTER (WHERE e.status = 'pending'),  0)::bigint AS pending,
       COALESCE(SUM(e.amount_paid) FILTER (WHERE e.status = 'approved'), 0)::bigint AS paid
  FROM projects p
  JOIN users u ON u.id = p.manager_id
  LEFT JOIN budget_versions v ON v.id = p.current_budget_version_id
  LEFT JOIN expenses e ON e.organization_id = p.organization_id AND e.project_id = p.id
                      AND e.status IN ('pending','approved')
 WHERE p.organization_id = @org_id AND p.id = @project_id
 GROUP BY p.id, u.full_name, v.total_amount, v.version_no;

-- name: ListProjectCategoryFinancials :many
WITH planned AS (
  SELECT l.category_id, SUM(l.amount)::bigint AS budget
    FROM projects p
    JOIN budget_lines l ON l.budget_version_id = p.current_budget_version_id
   WHERE p.organization_id = @org_id AND p.id = @project_id
   GROUP BY l.category_id
), spent AS (
  SELECT e.category_id,
         COALESCE(SUM(e.amount)      FILTER (WHERE e.status = 'approved'), 0)::bigint AS approved,
         COALESCE(SUM(e.amount)      FILTER (WHERE e.status = 'pending'),  0)::bigint AS pending,
         COALESCE(SUM(e.amount_paid) FILTER (WHERE e.status = 'approved'), 0)::bigint AS paid
    FROM expenses e
   WHERE e.organization_id = @org_id AND e.project_id = @project_id AND e.status IN ('pending','approved')
   GROUP BY e.category_id
)
SELECT c.id AS category_id, c.name,
       COALESCE(planned.budget, 0)::bigint AS budget,
       COALESCE(spent.approved, 0)::bigint AS approved,
       COALESCE(spent.pending, 0)::bigint  AS pending,
       COALESCE(spent.paid, 0)::bigint     AS paid
  FROM expense_categories c
  LEFT JOIN planned ON planned.category_id = c.id
  LEFT JOIN spent   ON spent.category_id = c.id
 WHERE c.organization_id = @org_id AND (planned.category_id IS NOT NULL OR spent.category_id IS NOT NULL)
 ORDER BY c.sort_order, c.name;

-- name: ListPaidByMonth :many
SELECT to_char(pay.paid_on, 'YYYY-MM') AS month, SUM(pay.amount)::bigint AS paid
  FROM payments pay
  JOIN expenses e ON e.id = pay.expense_id
  JOIN projects p ON p.id = e.project_id
 WHERE pay.organization_id = @org_id AND pay.paid_on >= @since::date
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
 GROUP BY 1
 ORDER BY 1;

-- name: SumPaidSince :one
SELECT COALESCE(SUM(pay.amount), 0)::bigint AS paid
  FROM payments pay
  JOIN expenses e ON e.id = pay.expense_id
  JOIN projects p ON p.id = e.project_id
 WHERE pay.organization_id = @org_id AND pay.paid_on >= @since::date AND p.status <> 'archived'
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id));

-- name: GetPendingSummary :one
SELECT count(*)::bigint AS total, COALESCE(SUM(e.amount), 0)::bigint AS amount,
       count(*) FILTER (WHERE EXISTS (
         SELECT 1 FROM expenses d
          WHERE d.organization_id = e.organization_id AND d.project_id = e.project_id AND d.id <> e.id
            AND d.amount = e.amount AND d.status <> 'cancelled'
            AND d.supplier_id IS NOT DISTINCT FROM e.supplier_id
            AND d.expense_date BETWEEN e.expense_date - 2 AND e.expense_date + 2))::bigint AS duplicates
  FROM expenses e
  JOIN projects p ON p.id = e.project_id
 WHERE e.organization_id = @org_id AND e.status = 'pending'
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id));

-- name: ListProjectsMissingReport :many
SELECT p.id, p.name
  FROM projects p
 WHERE p.organization_id = @org_id AND p.status = 'in_progress' AND p.start_date <= @report_date::date
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
   AND NOT EXISTS (SELECT 1 FROM daily_reports d
                    WHERE d.project_id = p.id AND d.report_date = @report_date::date AND d.status = 'submitted')
 ORDER BY p.name;

-- name: ListPaidByCategory :many
SELECT c.name, SUM(e.amount_paid)::bigint AS paid
  FROM expenses e
  JOIN expense_categories c ON c.id = e.category_id
  JOIN projects p ON p.id = e.project_id
 WHERE e.organization_id = @org_id AND e.status = 'approved' AND e.amount_paid > 0
   AND (sqlc.narg('project_id')::uuid IS NULL OR e.project_id = sqlc.narg('project_id'))
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
 GROUP BY c.name, c.sort_order
 ORDER BY paid DESC;

-- name: ListSubmittedReportDates :many
SELECT d.project_id, d.report_date
  FROM daily_reports d
 WHERE d.organization_id = @org_id AND d.status = 'submitted'
   AND d.report_date BETWEEN @date_from::date AND @date_to::date;

-- name: ListLatestReports :many
SELECT d.id, d.project_id, d.report_date, d.work_done, d.workforce_count, d.issues, d.status,
       p.name AS project_name, u.full_name AS author_name,
       (SELECT count(*) FROM attachments a WHERE a.daily_report_id = d.id AND a.status = 'uploaded')::bigint AS photo_count
  FROM daily_reports d
  JOIN projects p ON p.id = d.project_id
  JOIN users u ON u.id = d.created_by
 WHERE d.organization_id = @org_id AND d.status = 'submitted'
   AND (sqlc.narg('project_id')::uuid IS NULL OR d.project_id = sqlc.narg('project_id'))
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m WHERE m.project_id = p.id AND m.user_id = @user_id))
 ORDER BY d.report_date DESC, d.id DESC
 LIMIT @page_size;

-- name: ListStockOverview :many
SELECT s.project_id, p.name AS project_name, s.material_id, m.name, m.unit, s.on_hand,
       (m.low_stock_threshold IS NOT NULL AND s.on_hand <= m.low_stock_threshold)::boolean AS is_low
  FROM project_material_stock s
  JOIN materials m ON m.id = s.material_id
  JOIN projects p ON p.id = s.project_id
 WHERE s.organization_id = @org_id AND p.status <> 'archived'
   AND (sqlc.narg('project_id')::uuid IS NULL OR s.project_id = sqlc.narg('project_id'))
   AND (@all_projects::boolean
        OR p.manager_id = @user_id
        OR EXISTS (SELECT 1 FROM project_members m2 WHERE m2.project_id = p.id AND m2.user_id = @user_id))
 ORDER BY p.name, m.name;
