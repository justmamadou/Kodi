package project

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
)

// Service porte les cas d'usage des chantiers.
type Service struct{ tx *db.TxManager }

// NewService construit le service.
func NewService(tx *db.TxManager) *Service { return &Service{tx: tx} }

// CreateInput décrit un nouveau chantier.
type CreateInput struct {
	Reference      string      `json:"reference"`
	Name           string      `json:"name"`
	ClientName     string      `json:"client_name"`
	Address        string      `json:"address"`
	City           *string     `json:"city"`
	Description    *string     `json:"description"`
	Status         *string     `json:"status"`
	StartDate      *civil.Date `json:"start_date"`
	PlannedEndDate *civil.Date `json:"planned_end_date"`
	ManagerID      *uuid.UUID  `json:"manager_id"`
}

func validateCore(v *apperr.Validator, reference, name, client, address string) {
	v.Check(len(reference) >= 1 && len(reference) <= 40, "reference", "La référence est obligatoire (40 caractères maximum).")
	v.Check(len(name) >= 2 && len(name) <= 160, "name", "Le nom est obligatoire (2 à 160 caractères).")
	v.Check(len(client) >= 2, "client_name", "Le client est obligatoire.")
	v.Check(len(address) >= 2, "address", "L'adresse est obligatoire.")
}

// Create crée un chantier. Un manager qui crée un chantier pour un autre responsable reste affecté,
// sinon il perdrait aussitôt l'accès à ce qu'il vient de créer.
func (s *Service) Create(ctx context.Context, a auth.Actor, in CreateInput) (db.Project, error) {
	if err := rbac.Require(a, rbac.ProjectCreate); err != nil {
		return db.Project{}, err
	}
	reference := strings.TrimSpace(in.Reference)
	name := strings.TrimSpace(in.Name)
	client := strings.TrimSpace(in.ClientName)
	address := strings.TrimSpace(in.Address)
	var v apperr.Validator
	validateCore(&v, reference, name, client, address)
	v.Check(in.StartDate != nil, "start_date", "La date de démarrage est obligatoire.")
	if in.StartDate != nil && in.PlannedEndDate != nil {
		v.Check(!in.PlannedEndDate.Before(in.StartDate.Time), "planned_end_date", "La date de fin ne peut pas précéder le démarrage.")
	}
	status := StatusDraft
	if in.Status != nil {
		status = *in.Status
		v.Check(status == StatusDraft || status == StatusPlanned || status == StatusInProgress, "status", "Un chantier démarre en brouillon, planifié ou en cours.")
	}
	if err := v.Err(); err != nil {
		return db.Project{}, err
	}
	managerID := a.UserID
	if in.ManagerID != nil {
		managerID = *in.ManagerID
	}
	id, err := uuid.NewV7()
	if err != nil {
		return db.Project{}, err
	}
	var out db.Project
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if err := requireActiveUser(ctx, q, a.OrgID, managerID, "manager_id"); err != nil {
			return err
		}
		var err error
		out, err = q.CreateProject(ctx, db.CreateProjectParams{
			ID: id, OrgID: a.OrgID, Reference: reference, Name: name, ClientName: client, Address: address,
			City: in.City, Description: in.Description, Status: status, StartDate: *in.StartDate,
			PlannedEndDate: in.PlannedEndDate, ManagerID: managerID, CreatedBy: a.UserID,
		})
		if err != nil {
			return duplicateReference(err)
		}
		if err := q.InsertProjectStatusHistory(ctx, db.InsertProjectStatusHistoryParams{
			OrgID: a.OrgID, ProjectID: id, ToStatus: status, ActorID: a.UserID,
		}); err != nil {
			return err
		}
		if !rbac.SeesAllProjects(a) && managerID != a.UserID {
			if _, err := q.UpsertProjectMember(ctx, db.UpsertProjectMemberParams{
				OrgID: a.OrgID, ProjectID: id, UserID: a.UserID, AddedBy: a.UserID,
			}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "project.created", EntityType: "project", EntityID: &id, ProjectID: &id, After: out})
	})
	return out, err
}

func duplicateReference(err error) error {
	if apperr.Normalize(err).Code == "resource.duplicate" {
		return apperr.Conflict("project.duplicate_reference", "Cette référence de chantier est déjà utilisée.")
	}
	return err
}

// requireActiveUser refuse un utilisateur absent de l'organisation (y compris d'un autre tenant) ou inactif.
func requireActiveUser(ctx context.Context, q *db.Queries, orgID, userID uuid.UUID, field string) error {
	u, err := q.GetUser(ctx, db.GetUserParams{OrgID: orgID, ID: userID})
	var v apperr.Validator
	if err != nil {
		if !db.IsNoRows(err) {
			return err
		}
		v.Check(false, field, "Cet utilisateur n'appartient pas à l'entreprise.")
		return v.Err()
	}
	v.Check(u.Status == "active", field, "Seul un utilisateur actif peut être affecté.")
	return v.Err()
}

// Get retourne un chantier accessible à l'acteur.
func (s *Service) Get(ctx context.Context, a auth.Actor, id uuid.UUID) (Detail, error) {
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, id); err != nil {
			return err
		}
		var err error
		if out.Project, err = q.GetProject(ctx, db.GetProjectParams{OrgID: a.OrgID, ID: id}); err != nil {
			return err
		}
		manager, err := q.GetUser(ctx, db.GetUserParams{OrgID: a.OrgID, ID: out.ManagerID})
		out.ManagerName = manager.FullName
		return err
	})
	return out, err
}

// Detail est un chantier avec le nom de son responsable.
type Detail struct {
	db.Project
	ManagerName string `json:"manager_name"`
}

// ListFilter décrit les filtres de la liste des chantiers.
type ListFilter struct {
	Status    *string
	ManagerID *uuid.UUID
	Q         *string
	Cursor    *uuid.UUID
	Limit     int32
}

// List liste les chantiers visibles par l'acteur, du plus récent au plus ancien.
func (s *Service) List(ctx context.Context, a auth.Actor, f ListFilter) ([]db.ListProjectsRow, error) {
	var out []db.ListProjectsRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListProjects(ctx, db.ListProjectsParams{
			OrgID: a.OrgID, AllProjects: rbac.SeesAllProjects(a), UserID: a.UserID,
			Status: f.Status, ManagerID: f.ManagerID, Q: f.Q, Cursor: f.Cursor, PageSize: f.Limit,
		})
		return err
	})
	// Les montants suivent la même règle que le reste de la finance : le terrain ne les voit pas.
	if !rbac.Can(a.Role, rbac.FinanceView) {
		for i := range out {
			out[i].Budget, out[i].Approved = 0, 0
		}
	}
	return out, err
}

// UpdateInput décrit une modification de chantier.
type UpdateInput struct {
	Reference      *string     `json:"reference"`
	Name           *string     `json:"name"`
	ClientName     *string     `json:"client_name"`
	Address        *string     `json:"address"`
	City           *string     `json:"city"`
	Description    *string     `json:"description"`
	StartDate      *civil.Date `json:"start_date"`
	PlannedEndDate *civil.Date `json:"planned_end_date"`
	ManagerID      *uuid.UUID  `json:"manager_id"`
}

// Update modifie un chantier, avec verrou optimiste.
func (s *Service) Update(ctx context.Context, a auth.Actor, id uuid.UUID, version int32, in UpdateInput) (db.Project, error) {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return db.Project{}, err
	}
	var out db.Project
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, id); err != nil {
			return err
		}
		before, err := q.GetProject(ctx, db.GetProjectParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			return err
		}
		if in.ManagerID != nil {
			if err := requireActiveUser(ctx, q, a.OrgID, *in.ManagerID, "manager_id"); err != nil {
				return err
			}
		}
		out, err = q.UpdateProject(ctx, db.UpdateProjectParams{
			OrgID: a.OrgID, ID: id, RowVersion: version, Reference: in.Reference, Name: in.Name, ClientName: in.ClientName,
			Address: in.Address, City: in.City, Description: in.Description, StartDate: in.StartDate,
			PlannedEndDate: in.PlannedEndDate, ManagerID: in.ManagerID,
		})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.StaleVersion()
			}
			return duplicateReference(err)
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "project.updated", EntityType: "project", EntityID: &id, ProjectID: &id, Before: before, After: out})
	})
	return out, err
}

// TransitionInput décrit un changement de statut.
type TransitionInput struct {
	Status  string  `json:"status"`
	Comment *string `json:"comment"`
}

// Transition change le statut d'un chantier ; l'acteur et la date sont conservés dans l'historique.
func (s *Service) Transition(ctx context.Context, a auth.Actor, id uuid.UUID, in TransitionInput) (db.Project, error) {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return db.Project{}, err
	}
	var v apperr.Validator
	v.Check(IsStatus(in.Status), "status", "Statut inconnu.")
	if err := v.Err(); err != nil {
		return db.Project{}, err
	}
	var out db.Project
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		access, err := rbac.Project(ctx, q, a, id)
		if err != nil {
			return err
		}
		if !CanTransition(access.Status, in.Status) {
			return apperr.Conflict("project.invalid_transition", "Ce changement de statut n'est pas autorisé.")
		}
		out, err = q.SetProjectStatus(ctx, db.SetProjectStatusParams{OrgID: a.OrgID, ID: id, Status: in.Status, FromStatus: access.Status})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.StaleVersion()
			}
			return err
		}
		from := access.Status
		if err := q.InsertProjectStatusHistory(ctx, db.InsertProjectStatusHistoryParams{
			OrgID: a.OrgID, ProjectID: id, FromStatus: &from, ToStatus: in.Status, ActorID: a.UserID, Comment: in.Comment,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "project.status_changed", EntityType: "project", EntityID: &id, ProjectID: &id,
			Before: map[string]string{"status": from}, After: map[string]string{"status": in.Status},
		})
	})
	return out, err
}

// ProgressInput décrit une mise à jour de l'avancement déclaré.
type ProgressInput struct {
	ProgressPct int16 `json:"progress_pct"`
}

// SetProgress enregistre l'avancement déclaré. C'est une déclaration, comparée au budget exposé pour
// signaler un écart : jamais une conclusion.
func (s *Service) SetProgress(ctx context.Context, a auth.Actor, id uuid.UUID, in ProgressInput) (db.Project, error) {
	if err := rbac.Require(a, rbac.ReportWrite); err != nil {
		return db.Project{}, err
	}
	var v apperr.Validator
	v.Check(in.ProgressPct >= 0 && in.ProgressPct <= 100, "progress_pct", "L'avancement doit être compris entre 0 et 100.")
	if err := v.Err(); err != nil {
		return db.Project{}, err
	}
	var out db.Project
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, id); err != nil {
			return err
		}
		before, err := q.GetProject(ctx, db.GetProjectParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			return err
		}
		out, err = q.SetProjectProgress(ctx, db.SetProjectProgressParams{OrgID: a.OrgID, ID: id, ProgressPct: in.ProgressPct})
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "project.progress_updated", EntityType: "project", EntityID: &id, ProjectID: &id,
			Before: map[string]int16{"progress_pct": before.DeclaredProgressPct},
			After:  map[string]int16{"progress_pct": in.ProgressPct},
		})
	})
	return out, err
}

// Activity retourne le fil d'activité du chantier, réservé à ceux qui voient ses finances.
func (s *Service) Activity(ctx context.Context, a auth.Actor, id uuid.UUID, limit int32) ([]db.ListProjectHistoryRow, error) {
	if err := rbac.Require(a, rbac.FinanceView); err != nil {
		return nil, err
	}
	var out []db.ListProjectHistoryRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, id); err != nil {
			return err
		}
		var err error
		out, err = q.ListProjectHistory(ctx, db.ListProjectHistoryParams{OrgID: a.OrgID, ProjectID: &id, PageSize: limit})
		return err
	})
	return out, err
}

// History retourne l'historique des statuts.
func (s *Service) History(ctx context.Context, a auth.Actor, id uuid.UUID) ([]db.ListProjectStatusHistoryRow, error) {
	var out []db.ListProjectStatusHistoryRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, id); err != nil {
			return err
		}
		var err error
		out, err = q.ListProjectStatusHistory(ctx, db.ListProjectStatusHistoryParams{OrgID: a.OrgID, ProjectID: id})
		return err
	})
	return out, err
}

// MemberInput décrit l'affectation d'un membre et son éventuelle délégation d'approbation.
type MemberInput struct {
	CanApprove    bool   `json:"can_approve"`
	ApprovalLimit *int64 `json:"approval_limit"`
}

// AssignMember affecte un utilisateur actif de l'organisation à un chantier.
func (s *Service) AssignMember(ctx context.Context, a auth.Actor, projectID, userID uuid.UUID, in MemberInput) (db.ProjectMember, error) {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return db.ProjectMember{}, err
	}
	var v apperr.Validator
	v.Check(in.ApprovalLimit == nil || *in.ApprovalLimit >= 0, "approval_limit", "Le plafond ne peut pas être négatif.")
	if err := v.Err(); err != nil {
		return db.ProjectMember{}, err
	}
	// Déléguer une approbation est une décision financière : réservée à l'administrateur.
	if in.CanApprove && a.Role != rbac.RoleOwner {
		return db.ProjectMember{}, apperr.Forbidden()
	}
	var out db.ProjectMember
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		if err := requireActiveUser(ctx, q, a.OrgID, userID, "user_id"); err != nil {
			return err
		}
		var err error
		out, err = q.UpsertProjectMember(ctx, db.UpsertProjectMemberParams{
			OrgID: a.OrgID, ProjectID: projectID, UserID: userID, CanApprove: in.CanApprove,
			ApprovalLimit: in.ApprovalLimit, AddedBy: a.UserID,
		})
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "project.member_assigned", EntityType: "user", EntityID: &userID, ProjectID: &projectID, After: out})
	})
	return out, err
}

// RemoveMember retire un utilisateur d'un chantier.
func (s *Service) RemoveMember(ctx context.Context, a auth.Actor, projectID, userID uuid.UUID) error {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return err
	}
	return s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		n, err := q.RemoveProjectMember(ctx, db.RemoveProjectMemberParams{OrgID: a.OrgID, ProjectID: projectID, UserID: userID})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("member")
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "project.member_removed", EntityType: "user", EntityID: &userID, ProjectID: &projectID})
	})
}

// ListMembers liste l'équipe d'un chantier.
func (s *Service) ListMembers(ctx context.Context, a auth.Actor, projectID uuid.UUID) ([]db.ListProjectMembersRow, error) {
	var out []db.ListProjectMembersRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.ListProjectMembers(ctx, db.ListProjectMembersParams{OrgID: a.OrgID, ProjectID: projectID})
		return err
	})
	return out, err
}

// PhaseInput décrit une phase.
type PhaseInput struct {
	Name         *string     `json:"name"`
	Status       *string     `json:"status"`
	PlannedStart *civil.Date `json:"planned_start"`
	PlannedEnd   *civil.Date `json:"planned_end"`
}

// CreatePhase ajoute une phase à la fin du chantier.
func (s *Service) CreatePhase(ctx context.Context, a auth.Actor, projectID uuid.UUID, in PhaseInput) (db.ProjectPhase, error) {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return db.ProjectPhase{}, err
	}
	var v apperr.Validator
	v.Check(in.Name != nil && len(strings.TrimSpace(*in.Name)) >= 2, "name", "Le nom de la phase est obligatoire.")
	if err := v.Err(); err != nil {
		return db.ProjectPhase{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return db.ProjectPhase{}, err
	}
	var out db.ProjectPhase
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		// Sérialise le calcul de la position suivante.
		if _, err := q.LockProject(ctx, db.LockProjectParams{OrgID: a.OrgID, ID: projectID}); err != nil {
			return err
		}
		var err error
		out, err = q.CreateProjectPhase(ctx, db.CreateProjectPhaseParams{
			ID: id, OrgID: a.OrgID, ProjectID: projectID, Name: strings.TrimSpace(*in.Name),
			PlannedStart: in.PlannedStart, PlannedEnd: in.PlannedEnd,
		})
		return err
	})
	return out, err
}

// ListPhases retourne les phases dans l'ordre.
func (s *Service) ListPhases(ctx context.Context, a auth.Actor, projectID uuid.UUID) ([]db.ProjectPhase, error) {
	var out []db.ProjectPhase
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.ListProjectPhases(ctx, db.ListProjectPhasesParams{OrgID: a.OrgID, ProjectID: projectID})
		return err
	})
	return out, err
}

// UpdatePhase modifie une phase.
func (s *Service) UpdatePhase(ctx context.Context, a auth.Actor, projectID, id uuid.UUID, in PhaseInput) (db.ProjectPhase, error) {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return db.ProjectPhase{}, err
	}
	var v apperr.Validator
	if in.Status != nil {
		v.Check(*in.Status == "pending" || *in.Status == "in_progress" || *in.Status == "done", "status", "Statut de phase inconnu.")
	}
	if err := v.Err(); err != nil {
		return db.ProjectPhase{}, err
	}
	var out db.ProjectPhase
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.UpdateProjectPhase(ctx, db.UpdateProjectPhaseParams{
			OrgID: a.OrgID, ProjectID: projectID, ID: id, Name: in.Name, Status: in.Status,
			PlannedStart: in.PlannedStart, PlannedEnd: in.PlannedEnd,
		})
		if db.IsNoRows(err) {
			return apperr.NotFound("phase")
		}
		return err
	})
	return out, err
}

// DeletePhase retire une phase et referme les trous de numérotation.
func (s *Service) DeletePhase(ctx context.Context, a auth.Actor, projectID, id uuid.UUID) error {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return err
	}
	return s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		n, err := q.DeleteProjectPhase(ctx, db.DeleteProjectPhaseParams{OrgID: a.OrgID, ProjectID: projectID, ID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("phase")
		}
		rest, err := q.ListProjectPhases(ctx, db.ListProjectPhasesParams{OrgID: a.OrgID, ProjectID: projectID})
		if err != nil {
			return err
		}
		ids := make([]uuid.UUID, len(rest))
		for i, p := range rest {
			ids[i] = p.ID
		}
		_, err = q.ReorderProjectPhases(ctx, db.ReorderProjectPhasesParams{OrgID: a.OrgID, ProjectID: projectID, Ids: ids})
		return err
	})
}

// ReorderPhases applique un nouvel ordre ; il doit citer toutes les phases, une seule fois.
func (s *Service) ReorderPhases(ctx context.Context, a auth.Actor, projectID uuid.UUID, ids []uuid.UUID) ([]db.ProjectPhase, error) {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return nil, err
	}
	var out []db.ProjectPhase
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		total, err := q.CountProjectPhases(ctx, db.CountProjectPhasesParams{OrgID: a.OrgID, ProjectID: projectID})
		if err != nil {
			return err
		}
		n, err := q.ReorderProjectPhases(ctx, db.ReorderProjectPhasesParams{OrgID: a.OrgID, ProjectID: projectID, Ids: ids})
		if err != nil {
			return err
		}
		if n != total || int64(len(ids)) != total {
			var v apperr.Validator
			v.Check(false, "phase_ids", "L'ordre doit citer chaque phase du chantier une seule fois.")
			return v.Err()
		}
		out, err = q.ListProjectPhases(ctx, db.ListProjectPhasesParams{OrgID: a.OrgID, ProjectID: projectID})
		return err
	})
	return out, err
}
