// Package finance calcule les indicateurs financiers. Ils sont calculés à la lecture depuis les
// sources, via des index couvrants : ils ne peuvent donc pas dériver (voir docs/DECISIONS.md).
package finance

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
)

// Niveaux de santé d'un chantier.
const (
	HealthNoBudget = "no_budget"
	HealthOK       = "ok"
	HealthWarning  = "warning" // un seuil d'alerte est franchi
	HealthOver     = "over"    // dépassement : dépenses approuvées > budget publié
)

// Signaux budget / avancement (libellés des maquettes : Bon rythme, À surveiller, Dépense trop vite).
const (
	SignalNone     = "none"     // pas de budget publié, ou chantier non démarré
	SignalGood     = "good"     // la dépense suit l'avancement
	SignalWatch    = "watch"    // écart à examiner : un signal, jamais une conclusion
	SignalCritical = "critical" // seuil élevé atteint et la dépense devance l'avancement
)

const (
	gapTolerancePoints = 10 // écart toléré entre budget exposé et avancement déclaré
	criticalExposedPct = 90
)

// Signal compare le budget exposé à l'avancement déclaré. Comparaison entière, sans arrondi flottant.
func Signal(budget, approved int64, progressPct int16) string {
	if budget <= 0 {
		if approved > 0 {
			return SignalCritical
		}
		return SignalNone
	}
	ahead := approved*100 > budget*int64(progressPct)
	switch {
	case approved*100 >= budget*criticalExposedPct && ahead:
		return SignalCritical
	case approved*100 > budget*(int64(progressPct)+gapTolerancePoints):
		return SignalWatch
	default:
		return SignalGood
	}
}

// Figures regroupe des montants volontairement distincts : un budget n'est pas une dépense,
// une dépense en attente n'est pas approuvée, et une dépense approuvée n'est pas forcément payée.
type Figures struct {
	Budget   int64 `json:"budget"`
	Approved int64 `json:"approved"`
	Pending  int64 `json:"pending"`
	Paid     int64 `json:"paid"`
	Unpaid   int64 `json:"unpaid"`    // approuvé non encore payé
	Remains  int64 `json:"remaining"` // budget - approuvé ; négatif en cas de dépassement
}

func newFigures(budget, approved, pending, paid int64) Figures {
	return Figures{
		Budget: budget, Approved: approved, Pending: pending, Paid: paid,
		Unpaid: approved - paid, Remains: budget - approved,
	}
}

// ProjectHealth est la santé financière d'un chantier.
type ProjectHealth struct {
	ProjectID       uuid.UUID `json:"project_id"`
	Reference       string    `json:"reference"`
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	ManagerName     string    `json:"manager_name"`
	ProgressPct     int16     `json:"declared_progress_pct"`
	Signal          string    `json:"signal"`
	BudgetVersionNo *int32    `json:"budget_version_no"`
	Figures
	ConsumedPct      *float64 `json:"consumed_pct"`
	Health           string   `json:"health"`
	ThresholdReached *int16   `json:"threshold_reached"`
	Overrun          int64    `json:"overrun"`
}

// Assess détermine la santé d'un chantier. Les dépenses en attente n'entrent jamais dans le calcul.
func Assess(budget, approved int64, thresholds []int16) (health string, consumedPct *float64, reached *int16, overrun int64) {
	if budget <= 0 {
		if approved > 0 {
			return HealthOver, nil, nil, approved
		}
		return HealthNoBudget, nil, nil, 0
	}
	pct := float64(approved) * 100 / float64(budget)
	consumedPct = &pct
	if approved > budget {
		overrun = approved - budget
	}
	sorted := slices.Clone(thresholds)
	slices.Sort(sorted)
	for _, t := range sorted {
		// Comparaison entière : pas d'arrondi flottant sur le franchissement d'un seuil.
		if approved*100 >= budget*int64(t) {
			value := t
			reached = &value
		}
	}
	switch {
	case overrun > 0:
		health = HealthOver
	case reached != nil:
		health = HealthWarning
	default:
		health = HealthOK
	}
	return health, consumedPct, reached, overrun
}

// MonthPaid est un point de la courbe des paiements mensuels.
type MonthPaid struct {
	Month string `json:"month"` // AAAA-MM
	Paid  int64  `json:"paid"`
}

// ToProcess regroupe ce qui attend une action : la carte « À traiter ».
type ToProcess struct {
	PendingCount   int64                             `json:"pending_count"`
	PendingAmount  int64                             `json:"pending_amount"`
	DuplicateCount int64                             `json:"duplicate_count"`
	MissingReports []db.ListProjectsMissingReportRow `json:"missing_reports"`
	MissingFor     civil.Date                        `json:"missing_for"`
}

// Dashboard est la vue globale : totaux et détail par chantier, sur le même périmètre.
type Dashboard struct {
	Totals       Figures         `json:"totals"`
	ProjectCount int             `json:"project_count"`
	OverCount    int             `json:"over_budget_count"`
	Projects     []ProjectHealth `json:"projects"`
	// Period : week, month ou all ; seul PeriodPaid en dépend, un budget n'a pas de période.
	Period      string      `json:"period"`
	PeriodPaid  int64       `json:"period_paid"`
	PaidByMonth []MonthPaid `json:"paid_by_month"`
	ToProcess   ToProcess   `json:"to_process"`
}

// PeriodStart donne le début de la période : lundi de la semaine, 1er du mois, ou rien.
func PeriodStart(period string, now time.Time) *civil.Date {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch period {
	case "week":
		offset := (int(today.Weekday()) + 6) % 7
		d := civil.Date{Time: today.AddDate(0, 0, -offset)}
		return &d
	case "month":
		d := civil.Date{Time: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)}
		return &d
	}
	return nil
}

// Service porte les cas d'usage financiers.
type Service struct{ tx *db.TxManager }

// NewService construit le service.
func NewService(tx *db.TxManager) *Service { return &Service{tx: tx} }

// Dashboard calcule la vue globale ; un manager ne voit que ses chantiers, jamais un autre tenant.
func (s *Service) Dashboard(ctx context.Context, a auth.Actor, period string) (Dashboard, error) {
	if err := rbac.Require(a, rbac.FinanceView); err != nil {
		return Dashboard{}, err
	}
	if period != "week" && period != "month" {
		period = "all"
	}
	out := Dashboard{Projects: []ProjectHealth{}, Period: period, PaidByMonth: []MonthPaid{}}
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		settings, err := q.GetOrganizationSettings(ctx, a.OrgID)
		if err != nil {
			return err
		}
		rows, err := q.ListProjectFinancials(ctx, db.ListProjectFinancialsParams{
			OrgID: a.OrgID, AllProjects: rbac.SeesAllProjects(a), UserID: a.UserID,
		})
		if err != nil {
			return err
		}
		var budget, approved, pending, paid int64
		for _, r := range rows {
			p := toHealth(r.ID, r.Reference, r.Name, r.Status, r.ManagerName, r.DeclaredProgressPct, r.BudgetVersionNo, r.Budget, r.Approved, r.Pending, r.Paid, settings.BudgetAlertThresholds)
			out.Projects = append(out.Projects, p)
			budget += r.Budget
			approved += r.Approved
			pending += r.Pending
			paid += r.Paid
			if p.Health == HealthOver {
				out.OverCount++
			}
		}
		out.ProjectCount = len(rows)
		out.Totals = newFigures(budget, approved, pending, paid)
		out.PeriodPaid = paid
		now := time.Now().UTC()
		all := rbac.SeesAllProjects(a)
		if start := PeriodStart(period, now); start != nil {
			if out.PeriodPaid, err = q.SumPaidSince(ctx, db.SumPaidSinceParams{OrgID: a.OrgID, Since: *start, AllProjects: all, UserID: a.UserID}); err != nil {
				return err
			}
		}
		// Les cinq derniers mois, y compris ceux sans paiement : la courbe ne doit pas mentir sur le rythme.
		firstMonth := time.Date(now.Year(), now.Month()-4, 1, 0, 0, 0, 0, time.UTC)
		months, err := q.ListPaidByMonth(ctx, db.ListPaidByMonthParams{OrgID: a.OrgID, Since: civil.Date{Time: firstMonth}, AllProjects: all, UserID: a.UserID})
		if err != nil {
			return err
		}
		byMonth := make(map[string]int64, len(months))
		for _, m := range months {
			byMonth[m.Month] = m.Paid
		}
		for i := range 5 {
			key := firstMonth.AddDate(0, i, 0).Format("2006-01")
			out.PaidByMonth = append(out.PaidByMonth, MonthPaid{Month: key, Paid: byMonth[key]})
		}
		pendingSummary, err := q.GetPendingSummary(ctx, db.GetPendingSummaryParams{OrgID: a.OrgID, AllProjects: all, UserID: a.UserID})
		if err != nil {
			return err
		}
		yesterday := civil.Date{Time: time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, time.UTC)}
		missing, err := q.ListProjectsMissingReport(ctx, db.ListProjectsMissingReportParams{OrgID: a.OrgID, ReportDate: yesterday, AllProjects: all, UserID: a.UserID})
		if err != nil {
			return err
		}
		out.ToProcess = ToProcess{
			PendingCount: pendingSummary.Total, PendingAmount: pendingSummary.Amount, DuplicateCount: pendingSummary.Duplicates,
			MissingReports: missing, MissingFor: yesterday,
		}
		return nil
	})
	return out, err
}

func toHealth(id uuid.UUID, ref, name, status, manager string, progress int16, versionNo *int32, budget, approved, pending, paid int64, thresholds []int16) ProjectHealth {
	health, pct, reached, overrun := Assess(budget, approved, thresholds)
	return ProjectHealth{
		ProjectID: id, Reference: ref, Name: name, Status: status, ManagerName: manager, ProgressPct: progress,
		Signal: Signal(budget, approved, progress), BudgetVersionNo: versionNo,
		Figures: newFigures(budget, approved, pending, paid), ConsumedPct: pct,
		Health: health, ThresholdReached: reached, Overrun: overrun,
	}
}

// CategoryFigures sont les montants d'une catégorie.
type CategoryFigures struct {
	CategoryID uuid.UUID `json:"category_id"`
	Name       string    `json:"name"`
	Figures
}

// ProjectDetail est la santé d'un chantier, détaillée par catégorie.
type ProjectDetail struct {
	ProjectHealth
	Categories []CategoryFigures `json:"categories"`
}

// Project calcule la santé financière d'un chantier.
func (s *Service) Project(ctx context.Context, a auth.Actor, projectID uuid.UUID) (ProjectDetail, error) {
	if err := rbac.Require(a, rbac.FinanceView); err != nil {
		return ProjectDetail{}, err
	}
	var out ProjectDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		settings, err := q.GetOrganizationSettings(ctx, a.OrgID)
		if err != nil {
			return err
		}
		r, err := q.GetProjectFinancials(ctx, db.GetProjectFinancialsParams{OrgID: a.OrgID, ProjectID: projectID})
		if err != nil {
			return err
		}
		out.ProjectHealth = toHealth(r.ID, r.Reference, r.Name, r.Status, r.ManagerName, r.DeclaredProgressPct, r.BudgetVersionNo, r.Budget, r.Approved, r.Pending, r.Paid, settings.BudgetAlertThresholds)
		rows, err := q.ListProjectCategoryFinancials(ctx, db.ListProjectCategoryFinancialsParams{OrgID: a.OrgID, ProjectID: projectID})
		if err != nil {
			return err
		}
		out.Categories = make([]CategoryFigures, len(rows))
		for i, c := range rows {
			out.Categories[i] = CategoryFigures{CategoryID: c.CategoryID, Name: c.Name, Figures: newFigures(c.Budget, c.Approved, c.Pending, c.Paid)}
		}
		return nil
	})
	return out, err
}

// WeekProject est la ligne d'un chantier dans le suivi hebdomadaire des rapports.
type WeekProject struct {
	ProjectID uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	Reported  []string  `json:"reported"` // dates des rapports reçus
}

// CategoryPaid est le montant payé d'une catégorie, tous chantiers visibles confondus.
type CategoryPaid struct {
	Name string `json:"name"`
	Paid int64  `json:"paid"`
}

// Overview alimente l'écran Rapports : tout y est calculé sur les chantiers visibles par l'acteur.
type Overview struct {
	Projects       []ProjectHealth           `json:"projects"`
	Totals         Figures                   `json:"totals"`
	PaidByCategory []CategoryPaid            `json:"paid_by_category"`
	WeekDays       []string                  `json:"week_days"` // lundi à samedi
	WeekProjects   []WeekProject             `json:"week_projects"`
	LatestReports  []db.ListLatestReportsRow `json:"latest_reports"`
	Stock          []db.ListStockOverviewRow `json:"stock"`
}

// WeekDays retourne les six jours ouvrés de la semaine de now.
func WeekDays(now time.Time) []civil.Date {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	days := make([]civil.Date, 6)
	for i := range days {
		days[i] = civil.Date{Time: monday.AddDate(0, 0, i)}
	}
	return days
}

// Overview calcule la synthèse de l'écran Rapports, éventuellement limitée à un chantier.
func (s *Service) Overview(ctx context.Context, a auth.Actor, projectID *uuid.UUID) (Overview, error) {
	if err := rbac.Require(a, rbac.FinanceView); err != nil {
		return Overview{}, err
	}
	out := Overview{Projects: []ProjectHealth{}, PaidByCategory: []CategoryPaid{}, WeekProjects: []WeekProject{}}
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if projectID != nil {
			if _, err := rbac.Project(ctx, q, a, *projectID); err != nil {
				return err
			}
		}
		all := rbac.SeesAllProjects(a)
		settings, err := q.GetOrganizationSettings(ctx, a.OrgID)
		if err != nil {
			return err
		}
		rows, err := q.ListProjectFinancials(ctx, db.ListProjectFinancialsParams{OrgID: a.OrgID, AllProjects: all, UserID: a.UserID})
		if err != nil {
			return err
		}
		var budget, approved, pending, paid int64
		for _, r := range rows {
			if projectID != nil && r.ID != *projectID {
				continue
			}
			out.Projects = append(out.Projects, toHealth(r.ID, r.Reference, r.Name, r.Status, r.ManagerName, r.DeclaredProgressPct, r.BudgetVersionNo, r.Budget, r.Approved, r.Pending, r.Paid, settings.BudgetAlertThresholds))
			budget += r.Budget
			approved += r.Approved
			pending += r.Pending
			paid += r.Paid
		}
		out.Totals = newFigures(budget, approved, pending, paid)

		categories, err := q.ListPaidByCategory(ctx, db.ListPaidByCategoryParams{OrgID: a.OrgID, ProjectID: projectID, AllProjects: all, UserID: a.UserID})
		if err != nil {
			return err
		}
		for _, c := range categories {
			out.PaidByCategory = append(out.PaidByCategory, CategoryPaid{Name: c.Name, Paid: c.Paid})
		}

		days := WeekDays(time.Now().UTC())
		for _, d := range days {
			out.WeekDays = append(out.WeekDays, d.String())
		}
		dates, err := q.ListSubmittedReportDates(ctx, db.ListSubmittedReportDatesParams{OrgID: a.OrgID, DateFrom: days[0], DateTo: days[len(days)-1]})
		if err != nil {
			return err
		}
		reported := map[uuid.UUID][]string{}
		for _, d := range dates {
			reported[d.ProjectID] = append(reported[d.ProjectID], d.ReportDate.String())
		}
		for _, p := range out.Projects {
			if p.Status == "in_progress" {
				list := reported[p.ProjectID]
				if list == nil {
					list = []string{}
				}
				out.WeekProjects = append(out.WeekProjects, WeekProject{ProjectID: p.ProjectID, Name: p.Name, Reported: list})
			}
		}
		if out.LatestReports, err = q.ListLatestReports(ctx, db.ListLatestReportsParams{OrgID: a.OrgID, ProjectID: projectID, AllProjects: all, UserID: a.UserID, PageSize: 20}); err != nil {
			return err
		}
		out.Stock, err = q.ListStockOverview(ctx, db.ListStockOverviewParams{OrgID: a.OrgID, ProjectID: projectID, AllProjects: all, UserID: a.UserID})
		return err
	})
	return out, err
}
