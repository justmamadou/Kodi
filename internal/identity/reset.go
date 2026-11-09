package identity

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
)

// Mot de passe oublié : lien à usage unique, valable 30 minutes, 3 demandes par compte et par heure
// (en plus de la limite par adresse IP des routes publiques).
const (
	resetTTL         = 30 * time.Minute
	resetMaxPerHour  = 3
	resetConcurrency = 16
)

// ResetLinkPath est la page de la PWA qui reçoit le jeton. Il passe dans le fragment (#), que le
// navigateur n'envoie jamais au serveur : il n'apparaît dans aucun journal d'accès.
const ResetLinkPath = "/nouveau-mot-de-passe#"

// ErrResetInvalid couvre un jeton inconnu, expiré, déjà utilisé ou remplacé, sans les distinguer.
var ErrResetInvalid = apperr.New(http.StatusGone, "password_reset.invalid",
	"Ce lien de réinitialisation n'est plus valable. Demandez-en un nouveau.")

// ResetRequestInput décrit une demande de réinitialisation.
type ResetRequestInput struct {
	Login string `json:"login"`
}

// ResetConfirmInput décrit le choix du nouveau mot de passe.
type ResetConfirmInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// RequestPasswordReset accepte la demande et la traite en arrière-plan : la réponse est la même, et
// prend le même temps, que le compte existe ou non, qu'il ait un email ou non, qu'il soit plafonné ou non.
func (s *Service) RequestPasswordReset(ctx context.Context, in ResetRequestInput) error {
	login := strings.TrimSpace(in.Login)
	if !strings.Contains(login, "@") {
		login = strings.ReplaceAll(login, " ", "")
	}
	var v apperr.Validator
	v.Check(login != "", "login", "Indiquez votre email ou votre téléphone.")
	v.Check(len(login) <= 254, "login", "Identifiant trop long.")
	if err := v.Err(); err != nil {
		return err
	}
	// Borne le travail en cours face à un afflux venant de nombreuses adresses.
	select {
	case s.resetSlots <- struct{}{}:
	default:
		slog.WarnContext(ctx, "trop de demandes de réinitialisation en cours : demande ignorée")
		return nil
	}
	// Garde l'identifiant de requête et l'adresse IP pour l'audit, sans dépendre de la fin de la requête.
	bg := context.WithoutCancel(ctx)
	go func() {
		defer func() { <-s.resetSlots }()
		ctx, cancel := context.WithTimeout(bg, time.Minute)
		defer cancel()
		if err := s.processPasswordReset(ctx, login); err != nil {
			slog.ErrorContext(ctx, "demande de réinitialisation en échec", "error", err)
		}
	}()
	return nil
}

func (s *Service) processPasswordReset(ctx context.Context, login string) error {
	u, err := s.tx.FindAuthUser(ctx, login)
	if err != nil {
		if db.IsNoRows(err) {
			return nil
		}
		return err
	}
	// Un compte invité active son compte par son invitation ; un compte suspendu reste bloqué.
	if u.Status != "active" {
		return nil
	}
	token, hash, err := NewToken()
	if err != nil {
		return err
	}
	resetID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	var email *string
	err = s.tx.InTenantTx(ctx, u.OrganizationID, func(q *db.Queries) error {
		user, err := q.LockUser(ctx, db.LockUserParams{OrgID: u.OrganizationID, ID: u.UserID})
		if err != nil {
			return err
		}
		recent, err := q.CountRecentPasswordResets(ctx, db.CountRecentPasswordResetsParams{OrgID: u.OrganizationID, UserID: u.UserID})
		if err != nil {
			return err
		}
		if recent >= resetMaxPerHour {
			return nil
		}
		if user.Email == nil {
			// Aucun prestataire SMS : la demande est tracée pour que l'administrateur puisse aider.
			return audit.RecordFor(ctx, q, u.OrganizationID, u.UserID, audit.Entry{
				Action: "password_reset.requested", EntityType: "user", EntityID: &u.UserID,
				After: map[string]any{"channel": "none"},
			})
		}
		// Seul le dernier lien envoyé reste valable.
		if err := q.RevokePendingPasswordResets(ctx, db.RevokePendingPasswordResetsParams{OrgID: u.OrganizationID, UserID: u.UserID}); err != nil {
			return err
		}
		var ip *string
		if meta := auth.MetaFrom(ctx); meta.IP != "" {
			ip = &meta.IP
		}
		if err := q.CreatePasswordReset(ctx, db.CreatePasswordResetParams{
			ID: resetID, OrgID: u.OrganizationID, UserID: u.UserID, TokenHash: hash,
			ExpiresAt: time.Now().Add(resetTTL), Ip: ip,
		}); err != nil {
			return err
		}
		email = user.Email
		return audit.RecordFor(ctx, q, u.OrganizationID, u.UserID, audit.Entry{
			Action: "password_reset.requested", EntityType: "user", EntityID: &u.UserID,
			After: map[string]any{"channel": "email"},
		})
	})
	if err != nil || email == nil {
		return err
	}
	body := "Bonjour,\n\nUne réinitialisation du mot de passe de votre compte Kodi a été demandée.\n\n" +
		"Choisissez un nouveau mot de passe (lien valable 30 minutes, à usage unique) :\n" +
		s.appBaseURL + ResetLinkPath + token + "\n\n" +
		"Si vous n'êtes pas à l'origine de cette demande, ignorez ce message : votre mot de passe reste inchangé.\n"
	return s.mail.Send(ctx, *email, "Réinitialisation de votre mot de passe Kodi", body)
}

// ConfirmPasswordReset change le mot de passe, consomme le jeton et déconnecte toutes les sessions du compte.
func (s *Service) ConfirmPasswordReset(ctx context.Context, in ResetConfirmInput) error {
	var v apperr.Validator
	ValidatePassword(&v, in.Password)
	if err := v.Err(); err != nil {
		return err
	}
	if in.Token == "" || len(in.Token) > 128 {
		return ErrResetInvalid
	}
	reset, err := s.tx.FindAuthPasswordReset(ctx, HashToken(in.Token))
	if err != nil {
		if db.IsNoRows(err) {
			return ErrResetInvalid
		}
		return err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return err
	}
	orgID := reset.OrganizationID
	return s.tx.InTenantTx(ctx, orgID, func(q *db.Queries) error {
		// Garde atomique : de deux confirmations concurrentes, une seule passe.
		userID, err := q.UsePasswordReset(ctx, db.UsePasswordResetParams{OrgID: orgID, ID: reset.ResetID})
		if err != nil {
			if db.IsNoRows(err) {
				return ErrResetInvalid
			}
			return err
		}
		n, err := q.SetUserPassword(ctx, db.SetUserPasswordParams{OrgID: orgID, ID: userID, PasswordHash: &hash})
		if err != nil {
			return err
		}
		if n == 0 { // suspendu entre la demande et le changement
			return ErrResetInvalid
		}
		if err := q.RevokePendingPasswordResets(ctx, db.RevokePendingPasswordResetsParams{OrgID: orgID, UserID: userID}); err != nil {
			return err
		}
		if err := q.RevokeAllUserSessions(ctx, db.RevokeAllUserSessionsParams{OrgID: orgID, UserID: userID}); err != nil {
			return err
		}
		return audit.RecordFor(ctx, q, orgID, userID, audit.Entry{
			Action: "password.reset", EntityType: "user", EntityID: &userID,
		})
	})
}
