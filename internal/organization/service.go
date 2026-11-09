// Package organization gère l'entreprise, ses référentiels, ses membres et leurs rôles.
package organization

import (
	"context"
	"log/slog"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/identity"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/mailer"
	"github.com/justmamadou/Kodi/internal/rbac"
)

const invitationTTL = 7 * 24 * time.Hour

// Service porte les cas d'usage de l'organisation.
type Service struct {
	tx         *db.TxManager
	mail       mailer.Mailer
	appBaseURL string
}

// NewService construit le service.
func NewService(tx *db.TxManager, mail mailer.Mailer, appBaseURL string) *Service {
	return &Service{tx: tx, mail: mail, appBaseURL: strings.TrimRight(appBaseURL, "/")}
}

// Get retourne l'organisation de l'acteur.
func (s *Service) Get(ctx context.Context, a auth.Actor) (db.Organization, error) {
	var out db.Organization
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.GetOrganization(ctx, a.OrgID)
		return err
	})
	return out, err
}

// UpdateInput décrit une modification des coordonnées de l'entreprise.
type UpdateInput struct {
	Name         *string `json:"name"`
	Slug         *string `json:"slug"`
	LegalName    *string `json:"legal_name"`
	Phone        *string `json:"phone"`
	Email        *string `json:"email"`
	Address      *string `json:"address"`
	City         *string `json:"city"`
	ActivityType *string `json:"activity_type"`
}

// Update modifie l'entreprise (administrateurs uniquement).
func (s *Service) Update(ctx context.Context, a auth.Actor, in UpdateInput) (db.Organization, error) {
	if err := rbac.Require(a, rbac.OrgManage); err != nil {
		return db.Organization{}, err
	}
	var v apperr.Validator
	if in.Name != nil {
		n := len(strings.TrimSpace(*in.Name))
		v.Check(n >= 2 && n <= 160, "name", "Le nom doit contenir entre 2 et 160 caractères.")
	}
	if in.Slug != nil {
		v.Check(identity.IsValidSlug(*in.Slug), "slug", "Le slug ne peut contenir que des minuscules, chiffres et tirets.")
	}
	if in.ActivityType != nil {
		v.Check(identity.IsActivityType(*in.ActivityType), "activity_type", "Type d'activité inconnu.")
	}
	if in.Email != nil || in.Phone != nil {
		var contact apperr.Validator
		identity.ValidateContact(&contact, in.Email, in.Phone)
		if err := contact.Err(); err != nil {
			return db.Organization{}, err
		}
	}
	if err := v.Err(); err != nil {
		return db.Organization{}, err
	}
	var out db.Organization
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		before, err := q.GetOrganization(ctx, a.OrgID)
		if err != nil {
			return err
		}
		out, err = q.UpdateOrganization(ctx, db.UpdateOrganizationParams{
			OrgID: a.OrgID, Name: in.Name, Slug: in.Slug, LegalName: in.LegalName,
			Phone: in.Phone, Email: in.Email, Address: in.Address, City: in.City, ActivityType: in.ActivityType,
		})
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "organization.updated", EntityType: "organization", EntityID: &a.OrgID, Before: before, After: out,
		})
	})
	if err != nil && apperr.Normalize(err).Code == "resource.duplicate" {
		return db.Organization{}, apperr.Conflict("organization.duplicate", "Ce nom ou ce slug est déjà utilisé.")
	}
	return out, err
}

// GetSettings retourne les paramètres de l'entreprise.
func (s *Service) GetSettings(ctx context.Context, a auth.Actor) (db.OrganizationSetting, error) {
	var out db.OrganizationSetting
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.GetOrganizationSettings(ctx, a.OrgID)
		return err
	})
	return out, err
}

// SettingsInput décrit une modification des paramètres.
type SettingsInput struct {
	BudgetAlertThresholds []int16 `json:"budget_alert_thresholds"`
}

// UpdateSettings modifie les paramètres (administrateurs uniquement).
func (s *Service) UpdateSettings(ctx context.Context, a auth.Actor, in SettingsInput) (db.OrganizationSetting, error) {
	if err := rbac.Require(a, rbac.OrgManage); err != nil {
		return db.OrganizationSetting{}, err
	}
	var v apperr.Validator
	v.Check(len(in.BudgetAlertThresholds) > 0, "budget_alert_thresholds", "Au moins un seuil est requis.")
	for _, t := range in.BudgetAlertThresholds {
		v.Check(t >= 1 && t <= 200, "budget_alert_thresholds", "Les seuils doivent être compris entre 1 et 200 %.")
	}
	if err := v.Err(); err != nil {
		return db.OrganizationSetting{}, err
	}
	thresholds := slices.Clone(in.BudgetAlertThresholds)
	slices.Sort(thresholds)
	var out db.OrganizationSetting
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.UpdateOrganizationSettings(ctx, db.UpdateOrganizationSettingsParams{OrgID: a.OrgID, BudgetAlertThresholds: thresholds})
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "organization.settings_updated", EntityType: "organization", EntityID: &a.OrgID, After: out})
	})
	return out, err
}

// ListCategories liste les catégories de dépenses.
func (s *Service) ListCategories(ctx context.Context, a auth.Actor) ([]db.ExpenseCategory, error) {
	var out []db.ExpenseCategory
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListCategories(ctx, a.OrgID)
		return err
	})
	return out, err
}

// CategoryInput décrit une catégorie.
type CategoryInput struct {
	Name      *string `json:"name"`
	IsActive  *bool   `json:"is_active"`
	SortOrder *int16  `json:"sort_order"`
}

// CreateCategory ajoute une catégorie.
func (s *Service) CreateCategory(ctx context.Context, a auth.Actor, in CategoryInput) (db.ExpenseCategory, error) {
	if err := rbac.Require(a, rbac.CategoryManage); err != nil {
		return db.ExpenseCategory{}, err
	}
	var v apperr.Validator
	v.Check(in.Name != nil && len(strings.TrimSpace(*in.Name)) >= 2, "name", "Le nom est obligatoire.")
	if err := v.Err(); err != nil {
		return db.ExpenseCategory{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return db.ExpenseCategory{}, err
	}
	order := int16(100)
	if in.SortOrder != nil {
		order = *in.SortOrder
	}
	var out db.ExpenseCategory
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.CreateCategory(ctx, db.CreateCategoryParams{ID: id, OrgID: a.OrgID, Name: strings.TrimSpace(*in.Name), SortOrder: order})
		return err
	})
	return out, err
}

// UpdateCategory modifie une catégorie.
func (s *Service) UpdateCategory(ctx context.Context, a auth.Actor, id uuid.UUID, in CategoryInput) (db.ExpenseCategory, error) {
	if err := rbac.Require(a, rbac.CategoryManage); err != nil {
		return db.ExpenseCategory{}, err
	}
	var out db.ExpenseCategory
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.UpdateCategory(ctx, db.UpdateCategoryParams{OrgID: a.OrgID, ID: id, Name: in.Name, IsActive: in.IsActive, SortOrder: in.SortOrder})
		return err
	})
	return out, err
}

// ListMembers liste les utilisateurs de l'entreprise, avec leurs chantiers.
func (s *Service) ListMembers(ctx context.Context, a auth.Actor) ([]db.ListMembersRow, error) {
	if err := rbac.Require(a, rbac.MembersManage); err != nil {
		return nil, err
	}
	var out []db.ListMembersRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListMembers(ctx, a.OrgID)
		return err
	})
	return out, err
}

// Directory est l'annuaire réduit qui permet de désigner un responsable de chantier.
func (s *Service) Directory(ctx context.Context, a auth.Actor) ([]db.ListDirectoryRow, error) {
	if err := rbac.Require(a, rbac.ProjectManage); err != nil {
		return nil, err
	}
	var out []db.ListDirectoryRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListDirectory(ctx, a.OrgID)
		return err
	})
	return out, err
}

// InviteInput décrit une invitation.
type InviteInput struct {
	FullName string  `json:"full_name"`
	Email    *string `json:"email"`
	Phone    *string `json:"phone"`
	Role     string  `json:"role"`
	// ProjectIDs : chantiers accessibles, appliqués quand l'invitation est acceptée.
	ProjectIDs []uuid.UUID `json:"project_ids"`
}

// Invitation est la réponse à une invitation : le lien n'est disponible qu'à ce moment-là,
// seul le hash du jeton est conservé.
type Invitation struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	InviteURL string    `json:"invite_url"`
}

// Invite crée un compte en attente et un lien à usage unique valable 7 jours. Réinviter une personne
// déjà invitée remplace son lien (action « Renvoyer » des maquettes) ; une personne déjà membre,
// ici ou ailleurs (un utilisateur = une organisation), est refusée.
func (s *Service) Invite(ctx context.Context, a auth.Actor, in InviteInput) (Invitation, error) {
	if err := rbac.Require(a, rbac.MembersManage); err != nil {
		return Invitation{}, err
	}
	var v apperr.Validator
	name := strings.TrimSpace(in.FullName)
	v.Check(len(name) >= 2 && len(name) <= 160, "full_name", "Le nom doit contenir entre 2 et 160 caractères.")
	v.Check(rbac.IsRole(in.Role), "role", "Rôle inconnu.")
	email, phone := identity.ValidateContact(&v, in.Email, in.Phone)
	if err := v.Err(); err != nil {
		return Invitation{}, err
	}
	token, hash, err := identity.NewToken()
	if err != nil {
		return Invitation{}, err
	}
	invID, err := uuid.NewV7()
	if err != nil {
		return Invitation{}, err
	}
	expires := time.Now().Add(invitationTTL)
	userID := uuid.Nil
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		existing, err := q.FindUserByContact(ctx, db.FindUserByContactParams{OrgID: a.OrgID, Email: email, Phone: phone})
		switch {
		case err == nil:
			if existing.Status != "invited" {
				return apperr.Conflict("member.already_exists", "Cette personne est déjà membre de l'entreprise.")
			}
			userID = existing.ID
			if err := q.RevokePendingInvitations(ctx, db.RevokePendingInvitationsParams{OrgID: a.OrgID, UserID: userID}); err != nil {
				return err
			}
			if _, err := q.UpdateUserRole(ctx, db.UpdateUserRoleParams{OrgID: a.OrgID, ID: userID, Role: in.Role}); err != nil {
				return err
			}
		case db.IsNoRows(err):
			if userID, err = uuid.NewV7(); err != nil {
				return err
			}
			if _, err := q.CreateUser(ctx, db.CreateUserParams{
				ID: userID, OrgID: a.OrgID, Email: email, Phone: phone, FullName: name, Role: in.Role, Status: "invited",
			}); err != nil {
				return err
			}
		default:
			return err
		}
		for _, projectID := range in.ProjectIDs {
			if _, err := q.GetProject(ctx, db.GetProjectParams{OrgID: a.OrgID, ID: projectID}); err != nil {
				if db.IsNoRows(err) {
					var fields apperr.Validator
					fields.Check(false, "project_ids", "Un chantier sélectionné est introuvable.")
					return fields.Err()
				}
				return err
			}
		}
		projectIDs := in.ProjectIDs
		if projectIDs == nil {
			projectIDs = []uuid.UUID{}
		}
		if _, err := q.CreateInvitation(ctx, db.CreateInvitationParams{
			ID: invID, OrgID: a.OrgID, UserID: userID, TokenHash: hash, InvitedBy: a.UserID, ExpiresAt: expires, ProjectIds: projectIDs,
		}); err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "member.invited", EntityType: "user", EntityID: &userID,
			After: map[string]any{"role": in.Role, "full_name": name},
		})
	})
	if err != nil {
		if apperr.Normalize(err).Code == "resource.duplicate" {
			return Invitation{}, apperr.Conflict("member.identifier_taken", "Cet email ou ce téléphone est déjà associé à un compte.")
		}
		return Invitation{}, err
	}
	inviteURL := s.appBaseURL + "/invitation/" + url.PathEscape(token)
	if email != nil {
		body := "Vous êtes invité(e) à rejoindre Kodi.\n\nActivez votre compte (lien valable 7 jours, à usage unique) :\n" + inviteURL + "\n"
		// Hors transaction : l'envoi n'est pas fiable, et le lien reste de toute façon retourné à l'administrateur.
		if err := s.mail.Send(ctx, *email, "Invitation à rejoindre Kodi", body); err != nil {
			slog.ErrorContext(ctx, "envoi de l'invitation en échec", "error", err)
		}
	}
	return Invitation{ID: invID, UserID: userID, ExpiresAt: expires, InviteURL: inviteURL}, nil
}

// ListInvitations liste les invitations en attente.
func (s *Service) ListInvitations(ctx context.Context, a auth.Actor) ([]db.ListPendingInvitationsRow, error) {
	if err := rbac.Require(a, rbac.MembersManage); err != nil {
		return nil, err
	}
	var out []db.ListPendingInvitationsRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListPendingInvitations(ctx, a.OrgID)
		return err
	})
	return out, err
}

// RevokeInvitation rend un lien d'invitation inutilisable.
func (s *Service) RevokeInvitation(ctx context.Context, a auth.Actor, id uuid.UUID) error {
	if err := rbac.Require(a, rbac.MembersManage); err != nil {
		return err
	}
	return s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		inv, err := q.RevokeInvitation(ctx, db.RevokeInvitationParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("invitation")
			}
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "member.invitation_revoked", EntityType: "user", EntityID: &inv.UserID})
	})
}

// UpdateMemberInput décrit le changement de rôle et/ou de statut d'un membre.
type UpdateMemberInput struct {
	Role   *string `json:"role"`
	Status *string `json:"status"` // active | suspended
}

// UpdateMember attribue un rôle ou suspend/réactive un compte. Chaque changement est audité.
// L'entreprise garde toujours au moins un Owner actif : personne ne peut s'en exclure par erreur.
func (s *Service) UpdateMember(ctx context.Context, a auth.Actor, id uuid.UUID, in UpdateMemberInput) (db.User, error) {
	if err := rbac.Require(a, rbac.MembersManage); err != nil {
		return db.User{}, err
	}
	var v apperr.Validator
	v.Check(in.Role != nil || in.Status != nil, "role", "Indiquez un rôle ou un statut.")
	if in.Role != nil {
		v.Check(rbac.IsRole(*in.Role), "role", "Rôle inconnu.")
	}
	if in.Status != nil {
		v.Check(*in.Status == "active" || *in.Status == "suspended", "status", "Statut inconnu.")
	}
	if err := v.Err(); err != nil {
		return db.User{}, err
	}
	var out db.User
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		before, err := q.GetUser(ctx, db.GetUserParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("member")
			}
			return err
		}
		out = before
		removesOwner := before.Role == rbac.RoleOwner && before.Status == "active" &&
			((in.Role != nil && *in.Role != rbac.RoleOwner) || (in.Status != nil && *in.Status == "suspended"))
		if removesOwner {
			owners, err := q.CountActiveOwners(ctx, a.OrgID)
			if err != nil {
				return err
			}
			if owners <= 1 {
				return apperr.Conflict("member.last_owner", "L'entreprise doit conserver au moins un Owner actif.")
			}
		}
		if in.Role != nil && *in.Role != before.Role {
			if out, err = q.UpdateUserRole(ctx, db.UpdateUserRoleParams{OrgID: a.OrgID, ID: id, Role: *in.Role}); err != nil {
				return err
			}
			if err := audit.Record(ctx, q, a, audit.Entry{
				Action: "member.role_changed", EntityType: "user", EntityID: &id,
				Before: map[string]string{"role": before.Role}, After: map[string]string{"role": *in.Role},
			}); err != nil {
				return err
			}
		}
		if in.Status != nil && *in.Status != before.Status {
			switch *in.Status {
			case "suspended":
				if out, err = q.SuspendUser(ctx, db.SuspendUserParams{OrgID: a.OrgID, ID: id}); err != nil {
					if db.IsNoRows(err) {
						return apperr.Conflict("member.invalid_status", "Seul un compte actif peut être suspendu.")
					}
					return err
				}
				// Une suspension prend effet immédiatement, sur tous les appareils.
				if err := q.RevokeAllUserSessions(ctx, db.RevokeAllUserSessionsParams{OrgID: a.OrgID, UserID: id}); err != nil {
					return err
				}
			case "active":
				if out, err = q.ReactivateUser(ctx, db.ReactivateUserParams{OrgID: a.OrgID, ID: id}); err != nil {
					if db.IsNoRows(err) {
						return apperr.Conflict("member.invalid_status", "Seul un compte suspendu peut être réactivé.")
					}
					return err
				}
			}
			if err := audit.Record(ctx, q, a, audit.Entry{
				Action: "member.status_changed", EntityType: "user", EntityID: &id,
				Before: map[string]string{"status": before.Status}, After: map[string]string{"status": *in.Status},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// ListAudit retourne le journal d'audit.
func (s *Service) ListAudit(ctx context.Context, a auth.Actor, action *string, entityID *uuid.UUID, limit int32) ([]db.ListAuditLogsRow, error) {
	if err := rbac.Require(a, rbac.AuditRead); err != nil {
		return nil, err
	}
	var out []db.ListAuditLogsRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListAuditLogs(ctx, db.ListAuditLogsParams{OrgID: a.OrgID, Action: action, EntityID: entityID, PageSize: limit})
		return err
	})
	return out, err
}
