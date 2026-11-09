// Package project gère les chantiers, leurs membres, leurs phases et leur cycle de vie.
package project

import "slices"

// Statuts d'un chantier (libellés des maquettes : Brouillon, Planifié, En cours, Terminé, Archivé).
const (
	StatusDraft      = "draft"
	StatusPlanned    = "planned"
	StatusInProgress = "in_progress"
	StatusCompleted  = "completed"
	StatusArchived   = "archived"
)

var transitions = map[string][]string{
	StatusDraft:      {StatusPlanned, StatusInProgress, StatusArchived},
	StatusPlanned:    {StatusDraft, StatusInProgress, StatusArchived},
	StatusInProgress: {StatusCompleted},
	StatusCompleted:  {StatusInProgress, StatusArchived},
	StatusArchived:   {},
}

// IsStatus indique si le statut existe.
func IsStatus(s string) bool {
	_, ok := transitions[s]
	return ok
}

// CanTransition applique la machine à états du chantier. Le trigger SQL check_project_transition
// impose les mêmes règles à tout ce qui écrit en base.
func CanTransition(from, to string) bool { return slices.Contains(transitions[from], to) }
