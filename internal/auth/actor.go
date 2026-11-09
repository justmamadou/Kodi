// Package auth transporte l'identité authentifiée et le contexte de requête.
package auth

import (
	"context"

	"github.com/google/uuid"
)

// Actor est l'utilisateur authentifié. L'organisation vient UNIQUEMENT de la session,
// jamais d'un paramètre d'URL ou du corps de la requête.
type Actor struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	Role      string
	SessionID uuid.UUID
}

// RequestMeta décrit la requête en cours, pour l'audit et les logs.
type RequestMeta struct {
	RequestID string
	IP        string
	UserAgent string
}

type (
	actorKey     struct{}
	metaKey      struct{}
	actorSlotKey struct{}
)

// WithActorSlot prépare un emplacement partagé : un middleware externe (les logs) y retrouve l'acteur
// authentifié plus bas dans la chaîne, alors qu'un contexte enfant ne remonte jamais vers son parent.
func WithActorSlot(ctx context.Context) (context.Context, *Actor) {
	slot := &Actor{}
	return context.WithValue(ctx, actorSlotKey{}, slot), slot
}

// WithActor attache l'acteur au contexte.
func WithActor(ctx context.Context, a Actor) context.Context {
	if slot, ok := ctx.Value(actorSlotKey{}).(*Actor); ok {
		*slot = a
	}
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom lit l'acteur du contexte.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// WithMeta attache les métadonnées de requête au contexte.
func WithMeta(ctx context.Context, m RequestMeta) context.Context {
	return context.WithValue(ctx, metaKey{}, m)
}

// MetaFrom lit les métadonnées de requête du contexte.
func MetaFrom(ctx context.Context) RequestMeta {
	m, _ := ctx.Value(metaKey{}).(RequestMeta)
	return m
}
