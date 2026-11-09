package identity

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/httpx"
	"github.com/justmamadou/Kodi/internal/platform/db"
)

// SessionCookie est le cookie HttpOnly portant le jeton opaque.
const SessionCookie = "cf_session"

// Handler expose l'authentification.
type Handler struct {
	svc          *Service
	cookieSecure bool
}

// NewHandler construit le handler.
func NewHandler(svc *Service, cookieSecure bool) *Handler {
	return &Handler{svc: svc, cookieSecure: cookieSecure}
}

// PublicRoutes monte les routes sans session (limitées en débit par l'appelant).
func (h *Handler) PublicRoutes(r chi.Router) {
	r.Post("/auth/signup", httpx.H(h.signup))
	r.Post("/auth/login", httpx.H(h.login))
	r.Post("/auth/invitations/{token}/accept", httpx.H(h.accept))
	r.Post("/auth/password-reset", httpx.H(h.requestReset))
	r.Post("/auth/password-reset/confirm", httpx.H(h.confirmReset))
}

// Routes monte les routes de la session courante.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/auth/me", httpx.H(h.me))
	r.Post("/auth/logout", httpx.H(h.logout))
	r.Get("/auth/sessions", httpx.H(h.listSessions))
	r.Delete("/auth/sessions/{id}", httpx.H(h.revokeSession))
}

// RequireSession authentifie la requête via le cookie de session. L'organisation vient de là, et de là seulement.
func (h *Handler) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookie)
		if err != nil || cookie.Value == "" {
			httpx.WriteError(w, r, apperr.Unauthorized())
			return
		}
		actor, renewed, err := h.svc.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if renewed != nil {
			h.extendCookies(w, r, cookie.Value, *renewed)
		}
		next.ServeHTTP(w, r.WithContext(auth.WithActor(r.Context(), actor)))
	})
}

func (h *Handler) setCookies(w http.ResponseWriter, s NewSession) error {
	// Secure vient de la configuration, qui l'impose en production (voir config.load).
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124 : Secure configurable, forcé en production
		Name: SessionCookie, Value: s.Token, Path: "/", Expires: s.ExpiresAt,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	// Volontairement lisible par la PWA : double envoi anti-CSRF.
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124 : doit rester lisible par la PWA
		Name: httpx.CSRFCookie, Value: base64.RawURLEncoding.EncodeToString(b), Path: "/", Expires: s.ExpiresAt,
		HttpOnly: false, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// extendCookies repousse l'expiration sans changer les valeurs : la PWA reste connectée tant qu'elle sert.
func (h *Handler) extendCookies(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124 : Secure configurable, forcé en production
		Name: SessionCookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
	})
	if csrf, err := r.Cookie(httpx.CSRFCookie); err == nil {
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124 : doit rester lisible par la PWA
			Name: httpx.CSRFCookie, Value: csrf.Value, Path: "/", Expires: expires,
			HttpOnly: false, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
		})
	}
}

func (h *Handler) clearCookies(w http.ResponseWriter) {
	for _, name := range []string{SessionCookie, httpx.CSRFCookie} {
		http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124 : effacement de cookie
			Name: name, Value: "", Path: "/", Expires: time.Unix(0, 0), MaxAge: -1,
			HttpOnly: name == SessionCookie, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode,
		})
	}
}

func (h *Handler) signup(w http.ResponseWriter, r *http.Request) error {
	in, err := httpx.Decode[SignupInput](r)
	if err != nil {
		return err
	}
	s, err := h.svc.Signup(r.Context(), in, auth.MetaFrom(r.Context()))
	if err != nil {
		return err
	}
	if err := h.setCookies(w, s); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, map[string]any{"user": s.User})
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) error {
	in, err := httpx.Decode[LoginInput](r)
	if err != nil {
		return err
	}
	s, err := h.svc.Login(r.Context(), in, auth.MetaFrom(r.Context()))
	if err != nil {
		return err
	}
	if err := h.setCookies(w, s); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"user": s.User})
}

func (h *Handler) accept(w http.ResponseWriter, r *http.Request) error {
	in, err := httpx.Decode[AcceptInput](r)
	if err != nil {
		return err
	}
	s, err := h.svc.AcceptInvitation(r.Context(), chi.URLParam(r, "token"), in, auth.MetaFrom(r.Context()))
	if err != nil {
		return err
	}
	if err := h.setCookies(w, s); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"user": s.User})
}

func (h *Handler) requestReset(w http.ResponseWriter, r *http.Request) error {
	in, err := httpx.Decode[ResetRequestInput](r)
	if err != nil {
		return err
	}
	if err := h.svc.RequestPasswordReset(r.Context(), in); err != nil {
		return err
	}
	// Toujours la même réponse : elle ne dit pas si le compte existe.
	return httpx.JSON(w, http.StatusAccepted, map[string]string{
		"detail": "Si un compte correspond à cet identifiant, un lien de réinitialisation vient de lui être envoyé.",
	})
}

func (h *Handler) confirmReset(w http.ResponseWriter, r *http.Request) error {
	in, err := httpx.Decode[ResetConfirmInput](r)
	if err != nil {
		return err
	}
	if err := h.svc.ConfirmPasswordReset(r.Context(), in); err != nil {
		return err
	}
	// Toutes les sessions du compte viennent d'être révoquées, celle de ce navigateur comprise.
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	user, org, err := h.svc.Me(r.Context(), httpx.Actor(r))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"user": user, "organization": org})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	if err := h.svc.Logout(r.Context(), httpx.Actor(r)); err != nil {
		return err
	}
	h.clearCookies(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) listSessions(w http.ResponseWriter, r *http.Request) error {
	sessions, err := h.svc.ListSessions(r.Context(), httpx.Actor(r))
	if err != nil {
		return err
	}
	if sessions == nil {
		sessions = []db.Session{}
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": sessions})
}

func (h *Handler) revokeSession(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	if err := h.svc.RevokeSession(r.Context(), httpx.Actor(r), id); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
