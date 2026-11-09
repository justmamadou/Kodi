// Package fieldops gère les tâches et les rapports journaliers.
package fieldops

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
)

// Statuts d'une tâche.
const (
	TaskTodo       = "todo"
	TaskInProgress = "in_progress"
	TaskBlocked    = "blocked"
	TaskDone       = "done"
	TaskCancelled  = "cancelled"
)

var taskTransitions = map[string][]string{
	TaskTodo:       {TaskInProgress, TaskBlocked, TaskDone, TaskCancelled},
	TaskInProgress: {TaskTodo, TaskBlocked, TaskDone, TaskCancelled},
	TaskBlocked:    {TaskTodo, TaskInProgress, TaskCancelled},
	TaskDone:       {TaskInProgress},
	TaskCancelled:  {TaskTodo},
}

// CanTransitionTask applique la machine à états d'une tâche.
func CanTransitionTask(from, to string) bool { return slices.Contains(taskTransitions[from], to) }

var priorities = []string{"low", "medium", "high", "urgent"}

// Service porte les cas d'usage terrain.
type Service struct{ tx *db.TxManager }

// NewService construit le service.
func NewService(tx *db.TxManager) *Service { return &Service{tx: tx} }

// TaskInput décrit une tâche. L'id peut être généré par le client (brouillons hors connexion).
type TaskInput struct {
	ID          *uuid.UUID   `json:"id"`
	Title       string       `json:"title"`
	Description *string      `json:"description"`
	Priority    *string      `json:"priority"`
	DueDate     *civil.Date  `json:"due_date"`
	AssigneeIDs *[]uuid.UUID `json:"assignee_ids"`
}

// TaskDetail est une tâche avec ses responsables.
type TaskDetail struct {
	db.Task
	AssigneeIDs []uuid.UUID `json:"assignee_ids"`
}

func setAssignees(ctx context.Context, q *db.Queries, a auth.Actor, projectID, taskID uuid.UUID, ids []uuid.UUID) error {
	if err := q.ClearTaskAssignments(ctx, db.ClearTaskAssignmentsParams{OrgID: a.OrgID, TaskID: taskID}); err != nil {
		return err
	}
	for _, id := range ids {
		ok, err := q.IsAssignable(ctx, db.IsAssignableParams{OrgID: a.OrgID, ProjectID: projectID, UserID: id})
		if err != nil {
			return err
		}
		if !ok {
			var v apperr.Validator
			v.Check(false, "assignee_ids", "Un responsable doit être actif et avoir accès à ce chantier.")
			return v.Err()
		}
		if err := q.AddTaskAssignment(ctx, db.AddTaskAssignmentParams{OrgID: a.OrgID, TaskID: taskID, UserID: id, AssignedBy: a.UserID}); err != nil {
			return err
		}
	}
	return nil
}

// CreateTask crée une tâche liée à un chantier.
func (s *Service) CreateTask(ctx context.Context, a auth.Actor, projectID uuid.UUID, in TaskInput) (TaskDetail, bool, error) {
	if err := rbac.Require(a, rbac.TaskManage); err != nil {
		return TaskDetail{}, false, err
	}
	var v apperr.Validator
	n := len(strings.TrimSpace(in.Title))
	v.Check(n >= 2 && n <= 200, "title", "Le titre doit contenir entre 2 et 200 caractères.")
	priority := "medium"
	if in.Priority != nil {
		priority = *in.Priority
		v.Check(slices.Contains(priorities, priority), "priority", "Priorité inconnue.")
	}
	if err := v.Err(); err != nil {
		return TaskDetail{}, false, err
	}
	var id uuid.UUID
	if in.ID != nil {
		id = *in.ID
	} else {
		var err error
		if id, err = uuid.NewV7(); err != nil {
			return TaskDetail{}, false, err
		}
	}
	created := false
	var out TaskDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		task, err := q.CreateTask(ctx, db.CreateTaskParams{
			ID: id, OrgID: a.OrgID, ProjectID: projectID, Title: strings.TrimSpace(in.Title),
			Description: in.Description, Priority: priority, DueDate: in.DueDate, CreatedBy: a.UserID,
		})
		if db.IsNoRows(err) {
			task, err = q.GetTask(ctx, db.GetTaskParams{OrgID: a.OrgID, ID: id})
			if db.IsNoRows(err) || (err == nil && task.ProjectID != projectID) {
				return apperr.Conflict("task.id_conflict", "Cet identifiant de tâche n'est pas utilisable.")
			}
			if err != nil {
				return err
			}
		} else {
			if err != nil {
				return err
			}
			created = true
			if in.AssigneeIDs != nil {
				if err := setAssignees(ctx, q, a, projectID, id, *in.AssigneeIDs); err != nil {
					return err
				}
			}
		}
		assignees, err := q.ListTaskAssignees(ctx, db.ListTaskAssigneesParams{OrgID: a.OrgID, TaskID: id})
		out = TaskDetail{Task: task, AssigneeIDs: assignees}
		return err
	})
	return out, created, err
}

// TaskFilter décrit les filtres de la liste de tâches.
type TaskFilter struct {
	Status     *string
	AssigneeID *uuid.UUID
}

// ListTasks liste les tâches d'un chantier.
func (s *Service) ListTasks(ctx context.Context, a auth.Actor, projectID uuid.UUID, f TaskFilter) ([]db.ListTasksRow, error) {
	if err := rbac.Require(a, rbac.TaskView); err != nil {
		return nil, err
	}
	var out []db.ListTasksRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.ListTasks(ctx, db.ListTasksParams{OrgID: a.OrgID, ProjectID: projectID, Status: f.Status, AssigneeID: f.AssigneeID})
		return err
	})
	return out, err
}

func loadTask(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID, forUpdate bool) (db.Task, rbac.ProjectAccess, error) {
	var t db.Task
	var err error
	if forUpdate {
		t, err = q.GetTaskForUpdate(ctx, db.GetTaskForUpdateParams{OrgID: a.OrgID, ID: id})
	} else {
		t, err = q.GetTask(ctx, db.GetTaskParams{OrgID: a.OrgID, ID: id})
	}
	if err != nil {
		if db.IsNoRows(err) {
			return t, rbac.ProjectAccess{}, apperr.NotFound("task")
		}
		return t, rbac.ProjectAccess{}, err
	}
	access, err := rbac.Project(ctx, q, a, t.ProjectID)
	return t, access, err
}

// TaskHistory est le détail d'une tâche et de ses transitions.
type TaskHistory struct {
	TaskDetail
	History []db.ListTaskStatusHistoryRow `json:"history"`
}

// GetTask retourne une tâche, ses responsables et son historique.
func (s *Service) GetTask(ctx context.Context, a auth.Actor, id uuid.UUID) (TaskHistory, error) {
	if err := rbac.Require(a, rbac.TaskView); err != nil {
		return TaskHistory{}, err
	}
	var out TaskHistory
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		t, _, err := loadTask(ctx, q, a, id, false)
		if err != nil {
			return err
		}
		assignees, err := q.ListTaskAssignees(ctx, db.ListTaskAssigneesParams{OrgID: a.OrgID, TaskID: id})
		if err != nil {
			return err
		}
		history, err := q.ListTaskStatusHistory(ctx, db.ListTaskStatusHistoryParams{OrgID: a.OrgID, TaskID: id})
		out = TaskHistory{TaskDetail: TaskDetail{Task: t, AssigneeIDs: assignees}, History: history}
		return err
	})
	return out, err
}

// TaskUpdateInput décrit une modification de tâche.
type TaskUpdateInput struct {
	Title       *string      `json:"title"`
	Description *string      `json:"description"`
	Priority    *string      `json:"priority"`
	DueDate     *civil.Date  `json:"due_date"`
	AssigneeIDs *[]uuid.UUID `json:"assignee_ids"`
}

// UpdateTask modifie une tâche et ses responsables, avec verrou optimiste.
func (s *Service) UpdateTask(ctx context.Context, a auth.Actor, id uuid.UUID, version int32, in TaskUpdateInput) (TaskDetail, error) {
	if err := rbac.Require(a, rbac.TaskManage); err != nil {
		return TaskDetail{}, err
	}
	var v apperr.Validator
	if in.Priority != nil {
		v.Check(slices.Contains(priorities, *in.Priority), "priority", "Priorité inconnue.")
	}
	if in.Title != nil {
		n := len(strings.TrimSpace(*in.Title))
		v.Check(n >= 2 && n <= 200, "title", "Le titre doit contenir entre 2 et 200 caractères.")
	}
	if err := v.Err(); err != nil {
		return TaskDetail{}, err
	}
	var out TaskDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		t, access, err := loadTask(ctx, q, a, id, false)
		if err != nil {
			return err
		}
		if access.Status == "archived" {
			return apperr.Conflict("project.archived", "Ce chantier est archivé : il est en lecture seule.")
		}
		task, err := q.UpdateTask(ctx, db.UpdateTaskParams{
			OrgID: a.OrgID, ID: id, RowVersion: version, Title: in.Title,
			Description: in.Description, Priority: in.Priority, DueDate: in.DueDate,
		})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.StaleVersion()
			}
			return err
		}
		if in.AssigneeIDs != nil {
			if err := setAssignees(ctx, q, a, t.ProjectID, id, *in.AssigneeIDs); err != nil {
				return err
			}
		}
		assignees, err := q.ListTaskAssignees(ctx, db.ListTaskAssigneesParams{OrgID: a.OrgID, TaskID: id})
		out = TaskDetail{Task: task, AssigneeIDs: assignees}
		return err
	})
	return out, err
}

// ProgressInput décrit une mise à jour d'avancement.
type ProgressInput struct {
	Status      *string `json:"status"`
	ProgressPct *int16  `json:"progress_pct"`
}

// ResolveProgress calcule l'état cible : « done » force 100 %, 100 % sans statut passe à « done ».
func ResolveProgress(current db.Task, in ProgressInput) (status string, pct int16, err error) {
	status = current.Status
	pct = current.ProgressPct
	var v apperr.Validator
	v.Check(in.Status != nil || in.ProgressPct != nil, "status", "Indiquez un statut ou un avancement.")
	if in.ProgressPct != nil {
		v.Check(*in.ProgressPct >= 0 && *in.ProgressPct <= 100, "progress_pct", "L'avancement doit être compris entre 0 et 100.")
		pct = *in.ProgressPct
	}
	if in.Status != nil {
		status = *in.Status
	}
	if err := v.Err(); err != nil {
		return "", 0, err
	}
	if status == TaskDone {
		if in.ProgressPct != nil && pct != 100 {
			v.Check(false, "progress_pct", "Une tâche terminée est à 100 %.")
			return "", 0, v.Err()
		}
		pct = 100
	} else if in.Status == nil && pct == 100 {
		status = TaskDone
	} else if current.Status == TaskDone && pct == 100 && in.ProgressPct == nil {
		// Réouverture d'une tâche terminée sans nouvel avancement : on repart en dessous de 100 %.
		pct = 99
	} else if pct == 100 {
		v.Check(false, "progress_pct", "Un avancement de 100 % correspond au statut terminé.")
		return "", 0, v.Err()
	}
	if status != current.Status && !CanTransitionTask(current.Status, status) {
		return "", 0, apperr.Conflict("task.invalid_transition", "Ce changement de statut n'est pas autorisé.")
	}
	return status, pct, nil
}

// UpdateProgress met à jour le statut et l'avancement. Un manager agit sur toute tâche de ses
// chantiers ; un chef de chantier uniquement sur les tâches dont il est responsable.
func (s *Service) UpdateProgress(ctx context.Context, a auth.Actor, id uuid.UUID, in ProgressInput) (db.Task, error) {
	if err := rbac.Require(a, rbac.TaskProgress); err != nil {
		return db.Task{}, err
	}
	var out db.Task
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		t, access, err := loadTask(ctx, q, a, id, true)
		if err != nil {
			return err
		}
		if access.Status == "archived" {
			return apperr.Conflict("project.archived", "Ce chantier est archivé : il est en lecture seule.")
		}
		if !rbac.Can(a.Role, rbac.TaskManage) {
			assigned, err := q.IsTaskAssignee(ctx, db.IsTaskAssigneeParams{OrgID: a.OrgID, TaskID: id, UserID: a.UserID})
			if err != nil {
				return err
			}
			if !assigned {
				return apperr.Forbidden()
			}
		}
		status, pct, err := ResolveProgress(t, in)
		if err != nil {
			return err
		}
		out, err = q.SetTaskProgress(ctx, db.SetTaskProgressParams{OrgID: a.OrgID, ID: id, Status: status, ProgressPct: pct})
		if err != nil {
			return err
		}
		if status != t.Status {
			return q.InsertTaskStatusHistory(ctx, db.InsertTaskStatusHistoryParams{
				OrgID: a.OrgID, TaskID: id, FromStatus: t.Status, ToStatus: status, ProgressPct: pct, ActorID: a.UserID,
			})
		}
		return nil
	})
	return out, err
}

// ReportInput décrit un rapport journalier.
type ReportInput struct {
	ProjectID      uuid.UUID     `json:"project_id"`
	ReportDate     *civil.Date   `json:"report_date"`
	TeamLabel      string        `json:"team_label"`
	WorkDone       string        `json:"work_done"`
	WorkforceCount *int16        `json:"workforce_count"`
	Issues         *string       `json:"issues"`
	Weather        *string       `json:"weather"`
	Tasks          *[]ReportTask `json:"tasks"`
}

// ReportTask lie une tâche à un rapport.
type ReportTask struct {
	TaskID      uuid.UUID `json:"task_id"`
	DoneThatDay bool      `json:"done_that_day"`
}

// ReportDetail est un rapport avec ses tâches et photos.
type ReportDetail struct {
	db.DailyReport
	Tasks       []db.ListDailyReportTasksRow `json:"tasks"`
	Attachments []db.Attachment              `json:"attachments"`
}

var weathers = []string{"sunny", "cloudy", "rain", "strong_wind"}

// SaveReport crée ou met à jour un brouillon. L'id vient du client : un rejeu ne crée pas de doublon.
// baseVersion, s'il est fourni, protège des écrasements silencieux (conflit hors connexion).
func (s *Service) SaveReport(ctx context.Context, a auth.Actor, id uuid.UUID, baseVersion *int32, in ReportInput) (ReportDetail, error) {
	if err := rbac.Require(a, rbac.ReportWrite); err != nil {
		return ReportDetail{}, err
	}
	var v apperr.Validator
	v.Check(in.ProjectID != uuid.Nil, "project_id", "Le chantier est obligatoire.")
	v.Check(in.ReportDate != nil, "report_date", "La date est obligatoire.")
	if in.ReportDate != nil {
		v.Check(!in.ReportDate.After(time.Now().AddDate(0, 0, 1)), "report_date", "La date ne peut pas être dans le futur.")
	}
	v.Check(len(strings.TrimSpace(in.WorkDone)) >= 2, "work_done", "Décrivez les travaux réalisés.")
	if in.WorkforceCount != nil {
		v.Check(*in.WorkforceCount >= 0, "workforce_count", "Le nombre d'ouvriers ne peut pas être négatif.")
	}
	if in.Weather != nil {
		v.Check(slices.Contains(weathers, *in.Weather), "weather", "Météo inconnue.")
	}
	if err := v.Err(); err != nil {
		return ReportDetail{}, err
	}
	var out ReportDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, in.ProjectID); err != nil {
			return err
		}
		report, err := q.UpsertDailyReport(ctx, db.UpsertDailyReportParams{
			ID: id, OrgID: a.OrgID, ProjectID: in.ProjectID, ReportDate: *in.ReportDate,
			TeamLabel: strings.TrimSpace(in.TeamLabel), WorkDone: strings.TrimSpace(in.WorkDone),
			WorkforceCount: in.WorkforceCount, Issues: in.Issues, Weather: in.Weather,
			CreatedBy: a.UserID, BaseRowVersion: baseVersion,
		})
		if err != nil {
			if db.IsNoRows(err) {
				existing, getErr := q.GetDailyReport(ctx, db.GetDailyReportParams{OrgID: a.OrgID, ID: id})
				if getErr == nil && existing.Status == "submitted" {
					return apperr.Conflict("report.locked", "Ce rapport est soumis : il est verrouillé.")
				}
				return apperr.StaleVersion()
			}
			if apperr.Normalize(err).Code == "resource.duplicate" {
				return apperr.Conflict("report.duplicate", "Un rapport existe déjà pour ce chantier, cette date et cette équipe.")
			}
			return err
		}
		if in.Tasks != nil {
			if err := q.ClearDailyReportTasks(ctx, db.ClearDailyReportTasksParams{OrgID: a.OrgID, DailyReportID: id}); err != nil {
				return err
			}
			for _, t := range *in.Tasks {
				task, err := q.GetTask(ctx, db.GetTaskParams{OrgID: a.OrgID, ID: t.TaskID})
				if err != nil || task.ProjectID != in.ProjectID {
					if err != nil && !db.IsNoRows(err) {
						return err
					}
					var fields apperr.Validator
					fields.Check(false, "tasks", "Une tâche citée n'appartient pas à ce chantier.")
					return fields.Err()
				}
				if err := q.AddDailyReportTask(ctx, db.AddDailyReportTaskParams{
					OrgID: a.OrgID, DailyReportID: id, TaskID: t.TaskID, DoneThatDay: t.DoneThatDay,
				}); err != nil {
					return err
				}
			}
		}
		out, err = reportDetail(ctx, q, a.OrgID, report)
		return err
	})
	return out, err
}

func reportDetail(ctx context.Context, q *db.Queries, orgID uuid.UUID, r db.DailyReport) (ReportDetail, error) {
	tasks, err := q.ListDailyReportTasks(ctx, db.ListDailyReportTasksParams{OrgID: orgID, DailyReportID: r.ID})
	if err != nil {
		return ReportDetail{}, err
	}
	files, err := q.ListDailyReportAttachments(ctx, db.ListDailyReportAttachmentsParams{OrgID: orgID, DailyReportID: &r.ID})
	if err != nil {
		return ReportDetail{}, err
	}
	return ReportDetail{DailyReport: r, Tasks: tasks, Attachments: files}, nil
}

func loadReport(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID) (db.DailyReport, rbac.ProjectAccess, error) {
	r, err := q.GetDailyReport(ctx, db.GetDailyReportParams{OrgID: a.OrgID, ID: id})
	if err != nil {
		if db.IsNoRows(err) {
			return r, rbac.ProjectAccess{}, apperr.NotFound("report")
		}
		return r, rbac.ProjectAccess{}, err
	}
	access, err := rbac.Project(ctx, q, a, r.ProjectID)
	return r, access, err
}

// GetReport retourne un rapport, ses tâches et ses photos.
func (s *Service) GetReport(ctx context.Context, a auth.Actor, id uuid.UUID) (ReportDetail, error) {
	if err := rbac.Require(a, rbac.ReportView); err != nil {
		return ReportDetail{}, err
	}
	var out ReportDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		r, _, err := loadReport(ctx, q, a, id)
		if err != nil {
			return err
		}
		out, err = reportDetail(ctx, q, a.OrgID, r)
		return err
	})
	return out, err
}

// ListReports liste les rapports d'un chantier.
func (s *Service) ListReports(ctx context.Context, a auth.Actor, projectID uuid.UUID, from, to *civil.Date) ([]db.DailyReport, error) {
	if err := rbac.Require(a, rbac.ReportView); err != nil {
		return nil, err
	}
	var out []db.DailyReport
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.ListDailyReports(ctx, db.ListDailyReportsParams{OrgID: a.OrgID, ProjectID: projectID, DateFrom: from, DateTo: to})
		return err
	})
	return out, err
}

// SubmitReport soumet un rapport ; il est ensuite verrouillé.
func (s *Service) SubmitReport(ctx context.Context, a auth.Actor, id uuid.UUID) (db.DailyReport, error) {
	if err := rbac.Require(a, rbac.ReportWrite); err != nil {
		return db.DailyReport{}, err
	}
	var out db.DailyReport
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		r, access, err := loadReport(ctx, q, a, id)
		if err != nil {
			return err
		}
		if access.Status == "archived" {
			return apperr.Conflict("project.archived", "Ce chantier est archivé : il est en lecture seule.")
		}
		out, err = q.SubmitDailyReport(ctx, db.SubmitDailyReportParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.Conflict("report.already_submitted", "Ce rapport est déjà soumis.")
			}
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "report.submitted", EntityType: "daily_report", EntityID: &id, ProjectID: &r.ProjectID})
	})
	return out, err
}

// ReopenInput motive la réouverture d'un rapport soumis.
type ReopenInput struct {
	Reason string `json:"reason"`
}

// ReopenReport déverrouille un rapport soumis : c'est la seule voie de correction, et elle est auditée.
func (s *Service) ReopenReport(ctx context.Context, a auth.Actor, id uuid.UUID, in ReopenInput) (db.DailyReport, error) {
	if err := rbac.Require(a, rbac.ReportReopen); err != nil {
		return db.DailyReport{}, err
	}
	var v apperr.Validator
	v.Check(len(strings.TrimSpace(in.Reason)) >= 3, "reason", "Le motif de réouverture est obligatoire.")
	if err := v.Err(); err != nil {
		return db.DailyReport{}, err
	}
	var out db.DailyReport
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		r, access, err := loadReport(ctx, q, a, id)
		if err != nil {
			return err
		}
		if access.Status == "archived" {
			return apperr.Conflict("project.archived", "Ce chantier est archivé : il est en lecture seule.")
		}
		out, err = q.ReopenDailyReport(ctx, db.ReopenDailyReportParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.Conflict("report.not_submitted", "Seul un rapport soumis peut être rouvert.")
			}
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "report.reopened", EntityType: "daily_report", EntityID: &id, ProjectID: &r.ProjectID,
			Before: r, After: map[string]string{"reason": strings.TrimSpace(in.Reason)},
		})
	})
	return out, err
}
