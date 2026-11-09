package expense

import (
	"context"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
)

type decisionFunc func(context.Context, auth.Actor, uuid.UUID, DecisionInput) (db.Expense, error)
