// Package apperr définit les erreurs métier exposées par l'API (RFC 9457).
package apperr

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Error est une erreur affichable : un code stable pour le client, un message en français.
type Error struct {
	Status  int               `json:"status"`
	Code    string            `json:"code"`
	Message string            `json:"detail"`
	Fields  map[string]string `json:"fields,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// New construit une erreur métier.
func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// NotFound : ressource absente ou appartenant à une autre organisation (indiscernables).
func NotFound(what string) *Error {
	return New(http.StatusNotFound, what+".not_found", "Ressource introuvable.")
}

// Forbidden : authentifié mais non autorisé.
func Forbidden() *Error {
	return New(http.StatusForbidden, "auth.forbidden", "Vous n'avez pas les droits nécessaires pour cette action.")
}

// Unauthorized : session absente ou invalide.
func Unauthorized() *Error {
	return New(http.StatusUnauthorized, "auth.unauthenticated", "Veuillez vous connecter.")
}

// Conflict : état incompatible avec l'action demandée.
func Conflict(code, message string) *Error { return New(http.StatusConflict, code, message) }

// StaleVersion : la ressource a changé depuis sa lecture (verrou optimiste).
func StaleVersion() *Error {
	return Conflict("resource.stale", "Cette donnée a été modifiée entre-temps. Rechargez-la avant de réessayer.")
}

// Validator accumule les erreurs de formulaire pour les afficher champ par champ.
type Validator struct{ fields map[string]string }

// Check enregistre message pour field si la condition est fausse.
func (v *Validator) Check(ok bool, field, message string) {
	if ok {
		return
	}
	if v.fields == nil {
		v.fields = map[string]string{}
	}
	if _, exists := v.fields[field]; !exists {
		v.fields[field] = message
	}
}

// Err retourne une erreur 422 si au moins un champ est invalide.
func (v *Validator) Err() error {
	if len(v.fields) == 0 {
		return nil
	}
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation.failed",
		Message: "Certains champs sont invalides.",
		Fields:  v.fields,
	}
}

// Normalize traduit les erreurs PostgreSQL en erreurs métier. Les contraintes SQL sont le dernier
// rempart : si le service a laissé passer un cas, le client reçoit quand même une réponse propre.
func Normalize(err error) *Error {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return NotFound("resource")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return Conflict("resource.duplicate", "Cette valeur existe déjà.")
		case "23503": // foreign_key_violation : y compris une référence vers un autre tenant
			return New(http.StatusUnprocessableEntity, "resource.invalid_reference", "Une référence fournie est introuvable.")
		case "23514", "23502", "22003", "22001", "22P02": // check, not null, plage, longueur, format
			return New(http.StatusUnprocessableEntity, "validation.constraint", "Une valeur fournie est invalide.")
		case "P0001": // RAISE EXCEPTION des triggers d'intégrité
			return Conflict("resource.immutable", "Cette opération n'est pas autorisée dans l'état actuel de la ressource.")
		case "42501": // RLS : une ligne d'un autre tenant ne doit pas se distinguer d'une ligne absente
			return NotFound("resource")
		}
	}
	return New(http.StatusInternalServerError, "internal.error", "Une erreur interne est survenue.")
}
