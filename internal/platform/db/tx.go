package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect ouvre le pool de connexions et vérifie que la base répond.
func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("configuration PostgreSQL invalide : %w", err)
	}
	cfg.MaxConnLifetime = 30 * time.Minute
	// Sans fournisseur de traces configuré, ce traceur ne coûte rien. Les paramètres ne sont pas tracés.
	cfg.ConnConfig.Tracer = otelpgx.NewTracer(otelpgx.WithDisableSQLStatementInAttributes())
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("PostgreSQL injoignable : %w", err)
	}
	return pool, nil
}

// TxManager exécute le code métier dans une transaction liée à un tenant.
type TxManager struct{ pool *pgxpool.Pool }

// NewTxManager construit un TxManager.
func NewTxManager(pool *pgxpool.Pool) *TxManager { return &TxManager{pool: pool} }

// Ping vérifie la connexion à la base (readiness).
func (m *TxManager) Ping(ctx context.Context) error { return m.pool.Ping(ctx) }

// InTenantTx ouvre une transaction et y positionne le tenant courant, seule condition pour que
// la RLS laisse voir des lignes. set_config(..., true) équivaut à SET LOCAL : la valeur disparaît
// à la fin de la transaction et ne peut pas fuiter via le pool.
func (m *TxManager) InTenantTx(ctx context.Context, orgID uuid.UUID, fn func(q *Queries) error) error {
	return pgx.BeginTxFunc(ctx, m.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.current_org', $1, true)", orgID.String()); err != nil {
			return err
		}
		return fn(New(tx))
	})
}

// AuthUser est le résultat de auth_find_user : seule lecture de users hors contexte tenant.
type AuthUser struct {
	UserID           uuid.UUID
	OrganizationID   uuid.UUID
	Role             string
	PasswordHash     *string
	Status           string
	LockedUntil      *time.Time
	FailedLoginCount int16
}

// FindAuthUser cherche un utilisateur par email ou téléphone, sans tenant.
func (m *TxManager) FindAuthUser(ctx context.Context, login string) (AuthUser, error) {
	var u AuthUser
	err := m.pool.QueryRow(ctx,
		`SELECT user_id, organization_id, role, password_hash, status, locked_until, failed_login_count
		   FROM auth_find_user($1)`, login).
		Scan(&u.UserID, &u.OrganizationID, &u.Role, &u.PasswordHash, &u.Status, &u.LockedUntil, &u.FailedLoginCount)
	return u, err
}

// AuthSession est le résultat de auth_find_session.
type AuthSession struct {
	SessionID      uuid.UUID
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	Role           string
	UserStatus     string
	Remember       bool
	LastSeenAt     time.Time
	ExpiresAt      time.Time
}

// FindAuthSession résout un jeton de session valide (ni révoqué ni expiré), sans tenant.
func (m *TxManager) FindAuthSession(ctx context.Context, tokenHash []byte) (AuthSession, error) {
	var s AuthSession
	err := m.pool.QueryRow(ctx,
		`SELECT session_id, user_id, organization_id, role, user_status, remember, last_seen_at, expires_at
		   FROM auth_find_session($1)`, tokenHash).
		Scan(&s.SessionID, &s.UserID, &s.OrganizationID, &s.Role, &s.UserStatus, &s.Remember, &s.LastSeenAt, &s.ExpiresAt)
	return s, err
}

// AuthInvitation est le résultat de auth_find_invitation.
type AuthInvitation struct {
	InvitationID   uuid.UUID
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

// FindAuthInvitation résout un jeton d'invitation encore utilisable, sans tenant.
func (m *TxManager) FindAuthInvitation(ctx context.Context, tokenHash []byte) (AuthInvitation, error) {
	var i AuthInvitation
	err := m.pool.QueryRow(ctx,
		`SELECT invitation_id, user_id, organization_id FROM auth_find_invitation($1)`, tokenHash).
		Scan(&i.InvitationID, &i.UserID, &i.OrganizationID)
	return i, err
}

// AuthPasswordReset est le résultat de auth_find_password_reset.
type AuthPasswordReset struct {
	ResetID        uuid.UUID
	UserID         uuid.UUID
	OrganizationID uuid.UUID
}

// FindAuthPasswordReset résout un jeton de réinitialisation encore utilisable, sans tenant.
func (m *TxManager) FindAuthPasswordReset(ctx context.Context, tokenHash []byte) (AuthPasswordReset, error) {
	var r AuthPasswordReset
	err := m.pool.QueryRow(ctx,
		`SELECT reset_id, user_id, organization_id FROM auth_find_password_reset($1)`, tokenHash).
		Scan(&r.ResetID, &r.UserID, &r.OrganizationID)
	return r, err
}

// RFQLink est le résultat de auth_find_rfq_link : le fournisseur et la demande d'un lien de consultation.
type RFQLink struct {
	RFQSupplierID  uuid.UUID
	RFQID          uuid.UUID
	SupplierID     uuid.UUID
	OrganizationID uuid.UUID
}

// FindRFQLink résout le lien d'un fournisseur consulté, sans tenant.
func (m *TxManager) FindRFQLink(ctx context.Context, tokenHash []byte) (RFQLink, error) {
	var l RFQLink
	err := m.pool.QueryRow(ctx,
		`SELECT rfq_supplier_id, rfq_id, supplier_id, organization_id FROM auth_find_rfq_link($1)`, tokenHash).
		Scan(&l.RFQSupplierID, &l.RFQID, &l.SupplierID, &l.OrganizationID)
	return l, err
}

// PurgeExpired retire les clés d'idempotence et les sessions terminées anciennes.
func (m *TxManager) PurgeExpired(ctx context.Context) (keys, sessions int64, err error) {
	err = m.pool.QueryRow(ctx, "SELECT idempotency_keys, sessions FROM purge_expired()").Scan(&keys, &sessions)
	return keys, sessions, err
}

// IsNoRows indique qu'une requête :one n'a retourné aucune ligne.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
