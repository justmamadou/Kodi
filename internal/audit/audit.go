// Package audit écrit le journal immuable des actions sensibles, dans la transaction métier :
// une action sans trace d'audit est impossible, et une trace sans action aussi.
package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
)

// Entry décrit une action auditée. Before et After ne doivent jamais contenir de secret :
// les modèles sqlc masquent déjà les hashs et jetons via leurs tags JSON.
type Entry struct {
	Action     string
	EntityType string
	EntityID   *uuid.UUID
	ProjectID  *uuid.UUID
	Before     any
	After      any
}

// Record écrit une entrée pour l'acteur courant.
func Record(ctx context.Context, q *db.Queries, a auth.Actor, e Entry) error {
	actorID := a.UserID
	return write(ctx, q, a.OrgID, &actorID, e)
}

// RecordFor écrit une entrée pour une action sans session (inscription, acceptation d'invitation).
func RecordFor(ctx context.Context, q *db.Queries, orgID, actorID uuid.UUID, e Entry) error {
	return write(ctx, q, orgID, &actorID, e)
}

// RecordExternal écrit une entrée pour une action venue de l'extérieur, sans utilisateur de
// l'entreprise derrière (offre déposée par un fournisseur) : l'acteur reste vide.
func RecordExternal(ctx context.Context, q *db.Queries, orgID uuid.UUID, e Entry) error {
	return write(ctx, q, orgID, nil, e)
}

func write(ctx context.Context, q *db.Queries, orgID uuid.UUID, actorID *uuid.UUID, e Entry) error {
	before, err := marshal(e.Before)
	if err != nil {
		return err
	}
	after, err := marshal(e.After)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	meta := auth.MetaFrom(ctx)
	return q.InsertAuditLog(ctx, db.InsertAuditLogParams{
		ID:         id,
		OrgID:      orgID,
		ActorID:    actorID,
		Action:     e.Action,
		EntityType: e.EntityType,
		EntityID:   e.EntityID,
		ProjectID:  e.ProjectID,
		Before:     before,
		After:      after,
		Ip:         nonEmpty(meta.IP),
		UserAgent:  nonEmpty(meta.UserAgent),
		RequestID:  nonEmpty(meta.RequestID),
	})
}

func marshal(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
