// Package httpx regroupe les helpers et middlewares HTTP transverses.
package httpx

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/civil"
)

// MaxBodyBytes limite le corps JSON : les fichiers passent directement par l'object storage.
const MaxBodyBytes = 1 << 20

// HandlerFunc est un handler qui retourne son erreur au lieu de l'écrire.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// H adapte un HandlerFunc à net/http et traduit les erreurs au format RFC 9457.
func H(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}

// WriteError écrit une erreur application/problem+json. Une erreur 500 n'expose jamais sa cause.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	e := apperr.Normalize(err)
	if e.Status >= 500 {
		slog.ErrorContext(r.Context(), "requête en échec", "error", err, "request_id", auth.MetaFrom(r.Context()).RequestID)
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type":       "about:blank",
		"title":      http.StatusText(e.Status),
		"status":     e.Status,
		"code":       e.Code,
		"detail":     e.Message,
		"fields":     e.Fields,
		"request_id": auth.MetaFrom(r.Context()).RequestID,
	})
}

// JSON écrit une réponse JSON.
func JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// Decode lit un corps JSON strict : un champ inconnu est refusé, ce qui évite qu'un client croie
// fixer un champ calculé par le serveur (total, statut, organisation...).
func Decode[T any](r *http.Request) (T, error) {
	var v T
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, MaxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		if errors.Is(err, io.EOF) {
			return v, apperr.New(http.StatusBadRequest, "request.empty_body", "Le corps de la requête est vide.")
		}
		return v, &apperr.Error{Status: http.StatusBadRequest, Code: "request.invalid_json",
			Message: "Le corps de la requête est invalide : " + err.Error()}
	}
	return v, nil
}

// DecodeOptional lit un corps JSON facultatif (ex. commentaire d'approbation).
func DecodeOptional[T any](r *http.Request) (T, error) {
	var zero T
	v, err := Decode[T](r)
	var e *apperr.Error
	if errors.As(err, &e) && e.Code == "request.empty_body" {
		return zero, nil
	}
	return v, err
}

// Actor retourne l'acteur authentifié, posé par le middleware de session.
func Actor(r *http.Request) auth.Actor {
	a, ok := auth.ActorFrom(r.Context())
	if !ok {
		panic("httpx.Actor appelé sur une route sans session")
	}
	return a
}

// PathUUID lit un identifiant d'URL.
func PathUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, apperr.New(http.StatusBadRequest, "request.invalid_id", "Identifiant invalide.")
	}
	return id, nil
}

// QueryUUID lit un filtre UUID facultatif.
func QueryUUID(r *http.Request, name string) (*uuid.UUID, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return nil, apperr.New(http.StatusBadRequest, "request.invalid_filter", "Filtre invalide : "+name+".")
	}
	return &id, nil
}

// QueryString lit un filtre texte facultatif.
func QueryString(r *http.Request, name string) *string {
	v := strings.TrimSpace(r.URL.Query().Get(name))
	if v == "" {
		return nil
	}
	return &v
}

// QueryDate lit un filtre date facultatif.
func QueryDate(r *http.Request, name string) (*civil.Date, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return nil, nil
	}
	d, err := civil.Parse(v)
	if err != nil {
		return nil, apperr.New(http.StatusBadRequest, "request.invalid_filter", "Filtre invalide : "+name+".")
	}
	return &d, nil
}

// Limit lit le paramètre limit (1..200, défaut 50).
func Limit(r *http.Request) int32 {
	n, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	if err != nil || n < 1 {
		return 50
	}
	return int32(min(n, 200))
}

// IfMatch lit la version attendue d'une ressource (verrou optimiste). Obligatoire pour les mises à jour.
func IfMatch(r *http.Request) (int32, error) {
	v := strings.Trim(r.Header.Get("If-Match"), `"`)
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil || n < 1 {
		return 0, apperr.New(http.StatusPreconditionRequired, "request.if_match_required",
			"L'en-tête If-Match est requis : fournissez la row_version lue.")
	}
	return int32(n), nil
}

// Page est la réponse paginée par curseur.
type Page[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

// EncodeCursor rend un curseur opaque pour le client.
func EncodeCursor(parts ...string) *string {
	s := base64.RawURLEncoding.EncodeToString([]byte(strings.Join(parts, "|")))
	return &s
}

// DecodeCursor relit un curseur ; n parties attendues.
func DecodeCursor(raw string, n int) ([]string, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err == nil {
		if parts := strings.Split(string(b), "|"); len(parts) == n {
			return parts, nil
		}
	}
	return nil, apperr.New(http.StatusBadRequest, "request.invalid_cursor", "Curseur de pagination invalide.")
}
