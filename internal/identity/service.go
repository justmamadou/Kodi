// Package identity gère l'inscription, la connexion, les sessions et l'acceptation d'invitations.
package identity

import (
	"context"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/mailer"
	"github.com/justmamadou/Kodi/internal/rbac"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Durées de session : glissante 30 jours avec « Rester connecté », 12 heures sinon.
const (
	sessionShort   = 12 * time.Hour
	sessionLong    = 30 * 24 * time.Hour
	touchThreshold = 5 * time.Minute
)

// ErrInvalidCredentials ne distingue jamais un email inconnu d'un mauvais mot de passe,
// d'un compte verrouillé ou d'un compte suspendu.
var ErrInvalidCredentials = apperr.New(http.StatusUnauthorized, "auth.invalid_credentials", "Identifiants incorrects.")

var phoneRe = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// Service porte les cas d'usage d'identité.
type Service struct {
	tx         *db.TxManager
	mail       mailer.Mailer
	appBaseURL string
	dummyHash  string
	resetSlots chan struct{}
}

// NewService construit le service ; appBaseURL sert à composer le lien de réinitialisation.
func NewService(tx *db.TxManager, mail mailer.Mailer, appBaseURL string) (*Service, error) {
	// Vérifié quand l'identifiant est inconnu : la réponse prend le même temps que pour un compte existant.
	dummy, err := HashPassword(uuid.NewString())
	if err != nil {
		return nil, err
	}
	return &Service{
		tx: tx, mail: mail, appBaseURL: appBaseURL, dummyHash: dummy,
		resetSlots: make(chan struct{}, resetConcurrency),
	}, nil
}

// SignupInput décrit une inscription : une entreprise et son premier Owner.
type SignupInput struct {
	OrganizationName string  `json:"organization_name"`
	Slug             *string `json:"slug"`
	FullName         string  `json:"full_name"`
	Email            *string `json:"email"`
	Phone            *string `json:"phone"`
	Password         string  `json:"password"`
	Remember         bool    `json:"remember"`
	CompanyPhone     *string `json:"company_phone"`
	ActivityType     *string `json:"activity_type"`
	City             *string `json:"city"`
	Address          *string `json:"address"`
}

// NewSession est retourné après connexion ; le jeton ne quitte le serveur que dans le cookie.
type NewSession struct {
	Token     string
	ExpiresAt time.Time
	User      db.User
}

// Slugify dérive un slug d'URL d'un nom d'entreprise (« Diop Bâtiment » → « diop-batiment »).
func Slugify(name string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	s, _, err := transform.String(t, strings.ToLower(name))
	if err != nil {
		s = strings.ToLower(name)
	}
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteRune('-')
			dash = true
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > 63 {
		out = strings.TrimRight(out[:63], "-")
	}
	return out
}

// ValidateContact vérifie et normalise email et téléphone.
func ValidateContact(v *apperr.Validator, email, phone *string) (*string, *string) {
	var e, p *string
	if email != nil && strings.TrimSpace(*email) != "" {
		trimmed := strings.ToLower(strings.TrimSpace(*email))
		_, err := mail.ParseAddress(trimmed)
		v.Check(err == nil, "email", "Adresse email invalide.")
		e = &trimmed
	}
	if phone != nil && strings.TrimSpace(*phone) != "" {
		trimmed := strings.ReplaceAll(strings.TrimSpace(*phone), " ", "")
		v.Check(phoneRe.MatchString(trimmed), "phone", "Numéro invalide : format international attendu (+221...).")
		p = &trimmed
	}
	v.Check(e != nil || p != nil, "email", "Un email ou un téléphone est obligatoire.")
	return e, p
}

// Signup crée l'entreprise, ses paramètres et catégories par défaut, puis son Owner.
func (s *Service) Signup(ctx context.Context, in SignupInput, meta auth.RequestMeta) (NewSession, error) {
	var v apperr.Validator
	name := strings.TrimSpace(in.OrganizationName)
	fullName := strings.TrimSpace(in.FullName)
	v.Check(len(name) >= 2 && len(name) <= 160, "organization_name", "Le nom de l'entreprise doit contenir entre 2 et 160 caractères.")
	v.Check(len(fullName) >= 2 && len(fullName) <= 160, "full_name", "Le nom complet doit contenir entre 2 et 160 caractères.")
	slug := Slugify(name)
	if in.Slug != nil {
		slug = strings.TrimSpace(*in.Slug)
	}
	v.Check(IsValidSlug(slug), "slug", "Le slug ne peut contenir que des minuscules, chiffres et tirets.")
	email, phone := ValidateContact(&v, in.Email, in.Phone)
	ValidatePassword(&v, in.Password)
	if in.ActivityType != nil {
		v.Check(IsActivityType(*in.ActivityType), "activity_type", "Type d'activité inconnu.")
	}
	if err := v.Err(); err != nil {
		return NewSession{}, err
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return NewSession{}, err
	}
	orgID, err := uuid.NewV7()
	if err != nil {
		return NewSession{}, err
	}
	userID, err := uuid.NewV7()
	if err != nil {
		return NewSession{}, err
	}
	var out NewSession
	err = s.tx.InTenantTx(ctx, orgID, func(q *db.Queries) error {
		if _, err := q.CreateOrganization(ctx, db.CreateOrganizationParams{ID: orgID, Name: name, Slug: slug}); err != nil {
			return err
		}
		if err := q.CreateOrganizationSettings(ctx, orgID); err != nil {
			return err
		}
		if _, err := q.UpdateOrganization(ctx, db.UpdateOrganizationParams{
			OrgID: orgID, Phone: in.CompanyPhone, ActivityType: in.ActivityType, City: in.City, Address: in.Address,
		}); err != nil {
			return err
		}
		user, err := q.CreateUser(ctx, db.CreateUserParams{
			ID: userID, OrgID: orgID, Email: email, Phone: phone, FullName: fullName,
			Role: rbac.RoleOwner, PasswordHash: &hash, Status: "active",
		})
		if err != nil {
			return err
		}
		for i, c := range defaultCategories {
			id, err := uuid.NewV7()
			if err != nil {
				return err
			}
			code := c.code
			if _, err := q.CreateCategory(ctx, db.CreateCategoryParams{
				ID: id, OrgID: orgID, Name: c.name, SystemCode: &code, SortOrder: int16((i + 1) * 10),
			}); err != nil {
				return err
			}
		}
		if err := audit.RecordFor(ctx, q, orgID, userID, audit.Entry{
			Action: "organization.created", EntityType: "organization", EntityID: &orgID,
		}); err != nil {
			return err
		}
		out, err = createSession(ctx, q, user, in.Remember, meta)
		return err
	})
	if err != nil {
		e := apperr.Normalize(err)
		if e.Code == "resource.duplicate" {
			return NewSession{}, apperr.Conflict("signup.conflict", "Ce nom d'entreprise, ce slug ou cet identifiant est déjà utilisé.")
		}
		return NewSession{}, err
	}
	return out, nil
}

// IsActivityType vérifie le type d'activité BTP, avec les valeurs de la contrainte en base.
func IsActivityType(activity string) bool {
	switch activity {
	case "villas", "gros_oeuvre", "second_oeuvre", "renovation", "general":
		return true
	}
	return false
}

var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// IsValidSlug vérifie le format d'un slug.
func IsValidSlug(slug string) bool {
	return len(slug) >= 2 && len(slug) <= 63 && slugRe.MatchString(slug)
}

var defaultCategories = []struct{ code, name string }{
	{"materials", "Matériaux"},
	{"labour", "Main-d'œuvre"},
	{"subcontracting", "Sous-traitance"},
	{"transport", "Transport"},
	{"equipment_rental", "Location d'équipements"},
	{"misc", "Divers"},
}

func createSession(ctx context.Context, q *db.Queries, user db.User, remember bool, meta auth.RequestMeta) (NewSession, error) {
	token, hash, err := NewToken()
	if err != nil {
		return NewSession{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return NewSession{}, err
	}
	expires := time.Now().Add(sessionDuration(remember))
	var ua, ip *string
	if meta.UserAgent != "" {
		ua = &meta.UserAgent
	}
	if meta.IP != "" {
		ip = &meta.IP
	}
	if _, err := q.CreateSession(ctx, db.CreateSessionParams{
		ID: id, OrgID: user.OrganizationID, UserID: user.ID, TokenHash: hash,
		Remember: remember, UserAgent: ua, Ip: ip, ExpiresAt: expires,
	}); err != nil {
		return NewSession{}, err
	}
	return NewSession{Token: token, ExpiresAt: expires, User: user}, nil
}

func sessionDuration(remember bool) time.Duration {
	if remember {
		return sessionLong
	}
	return sessionShort
}

// LoginInput décrit une tentative de connexion.
type LoginInput struct {
	Login    string `json:"login"`
	Password string `json:"password"`
	Remember bool   `json:"remember"`
}

// Login authentifie un utilisateur. Tous les échecs retournent la même erreur.
func (s *Service) Login(ctx context.Context, in LoginInput, meta auth.RequestMeta) (NewSession, error) {
	login := strings.TrimSpace(in.Login)
	u, err := s.tx.FindAuthUser(ctx, login)
	if err != nil {
		if !db.IsNoRows(err) {
			return NewSession{}, err
		}
		VerifyPassword(s.dummyHash, in.Password)
		return NewSession{}, ErrInvalidCredentials
	}

	locked := u.LockedUntil != nil && u.LockedUntil.After(time.Now())
	hash := s.dummyHash
	if u.PasswordHash != nil {
		hash = *u.PasswordHash
	}
	valid := VerifyPassword(hash, in.Password) && u.PasswordHash != nil

	var out NewSession
	err = s.tx.InTenantTx(ctx, u.OrganizationID, func(q *db.Queries) error {
		if !valid {
			if !locked {
				return q.RecordLoginFailure(ctx, db.RecordLoginFailureParams{OrgID: u.OrganizationID, ID: u.UserID})
			}
			return nil
		}
		if locked || u.Status != "active" {
			return nil
		}
		if err := q.RecordLoginSuccess(ctx, db.RecordLoginSuccessParams{OrgID: u.OrganizationID, ID: u.UserID}); err != nil {
			return err
		}
		user, err := q.GetUser(ctx, db.GetUserParams{OrgID: u.OrganizationID, ID: u.UserID})
		if err != nil {
			return err
		}
		out, err = createSession(ctx, q, user, in.Remember, meta)
		return err
	})
	if err != nil {
		return NewSession{}, err
	}
	if out.Token == "" {
		return NewSession{}, ErrInvalidCredentials
	}
	return out, nil
}

// Authenticate résout un jeton de session en acteur ; un compte suspendu ne passe plus.
// renewed est renseigné quand la session vient d'être prolongée : le cookie doit l'être aussi.
func (s *Service) Authenticate(ctx context.Context, token string) (actor auth.Actor, renewed *time.Time, err error) {
	sess, err := s.tx.FindAuthSession(ctx, HashToken(token))
	if err != nil {
		if db.IsNoRows(err) {
			return auth.Actor{}, nil, apperr.Unauthorized()
		}
		return auth.Actor{}, nil, err
	}
	if sess.UserStatus != "active" {
		return auth.Actor{}, nil, apperr.Unauthorized()
	}
	// Durée glissante, écrite au plus toutes les 5 minutes pour ne pas transformer chaque lecture en écriture.
	if time.Since(sess.LastSeenAt) > touchThreshold {
		expires := time.Now().Add(sessionDuration(sess.Remember))
		if err := s.tx.InTenantTx(ctx, sess.OrganizationID, func(q *db.Queries) error {
			return q.TouchSession(ctx, db.TouchSessionParams{OrgID: sess.OrganizationID, ID: sess.SessionID, ExpiresAt: expires})
		}); err != nil {
			return auth.Actor{}, nil, err
		}
		renewed = &expires
	}
	return auth.Actor{UserID: sess.UserID, OrgID: sess.OrganizationID, Role: sess.Role, SessionID: sess.SessionID}, renewed, nil
}

// Logout révoque la session courante.
func (s *Service) Logout(ctx context.Context, a auth.Actor) error {
	return s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		_, err := q.RevokeSession(ctx, db.RevokeSessionParams{OrgID: a.OrgID, ID: a.SessionID, UserID: a.UserID})
		return err
	})
}

// Me retourne l'utilisateur courant et son organisation.
func (s *Service) Me(ctx context.Context, a auth.Actor) (db.User, db.Organization, error) {
	var user db.User
	var org db.Organization
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		if user, err = q.GetUser(ctx, db.GetUserParams{OrgID: a.OrgID, ID: a.UserID}); err != nil {
			return err
		}
		org, err = q.GetOrganization(ctx, a.OrgID)
		return err
	})
	return user, org, err
}

// ListSessions liste les sessions actives de l'utilisateur.
func (s *Service) ListSessions(ctx context.Context, a auth.Actor) ([]db.Session, error) {
	var out []db.Session
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListUserSessions(ctx, db.ListUserSessionsParams{OrgID: a.OrgID, UserID: a.UserID})
		return err
	})
	return out, err
}

// RevokeSession révoque l'une des sessions de l'utilisateur (« Déconnecter cet appareil »).
func (s *Service) RevokeSession(ctx context.Context, a auth.Actor, id uuid.UUID) error {
	return s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		n, err := q.RevokeSession(ctx, db.RevokeSessionParams{OrgID: a.OrgID, ID: id, UserID: a.UserID})
		if err != nil {
			return err
		}
		if n == 0 {
			return apperr.NotFound("session")
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "session.revoked", EntityType: "session", EntityID: &id})
	})
}

// AcceptInput décrit l'acceptation d'une invitation.
type AcceptInput struct {
	Password string  `json:"password"`
	FullName *string `json:"full_name"`
}

// AcceptInvitation active le compte invité. L'invitation n'est utilisable qu'une fois et avant expiration.
func (s *Service) AcceptInvitation(ctx context.Context, token string, in AcceptInput, meta auth.RequestMeta) (NewSession, error) {
	var v apperr.Validator
	ValidatePassword(&v, in.Password)
	if err := v.Err(); err != nil {
		return NewSession{}, err
	}
	invalid := apperr.New(http.StatusGone, "invitation.invalid", "Ce lien d'invitation n'est plus valable.")
	inv, err := s.tx.FindAuthInvitation(ctx, HashToken(token))
	if err != nil {
		if db.IsNoRows(err) {
			return NewSession{}, invalid
		}
		return NewSession{}, err
	}
	hash, err := HashPassword(in.Password)
	if err != nil {
		return NewSession{}, err
	}
	var out NewSession
	err = s.tx.InTenantTx(ctx, inv.OrganizationID, func(q *db.Queries) error {
		// Garde atomique : de deux acceptations concurrentes, une seule passe.
		accepted, err := q.AcceptInvitation(ctx, db.AcceptInvitationParams{OrgID: inv.OrganizationID, ID: inv.InvitationID})
		if err != nil {
			if db.IsNoRows(err) {
				return invalid
			}
			return err
		}
		user, err := q.ActivateInvitedUser(ctx, db.ActivateInvitedUserParams{
			OrgID: inv.OrganizationID, ID: inv.UserID, PasswordHash: &hash, FullName: in.FullName,
		})
		if err != nil {
			if db.IsNoRows(err) {
				return invalid
			}
			return err
		}
		// Le compte est actif : les chantiers choisis à l'invitation peuvent s'appliquer.
		for _, projectID := range accepted.ProjectIds {
			if _, err := q.UpsertProjectMember(ctx, db.UpsertProjectMemberParams{
				OrgID: inv.OrganizationID, ProjectID: projectID, UserID: inv.UserID, AddedBy: accepted.InvitedBy,
			}); err != nil {
				return err
			}
		}
		if err := audit.RecordFor(ctx, q, inv.OrganizationID, inv.UserID, audit.Entry{
			Action: "invitation.accepted", EntityType: "user", EntityID: &inv.UserID,
		}); err != nil {
			return err
		}
		out, err = createSession(ctx, q, user, false, meta)
		return err
	})
	return out, err
}
