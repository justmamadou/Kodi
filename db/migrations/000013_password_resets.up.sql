-- Mot de passe oublié : jetons à usage unique, stockés hachés, valables 30 minutes.
CREATE TABLE password_resets (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL,
  user_id         UUID        NOT NULL,
  token_hash      BYTEA       NOT NULL UNIQUE,     -- SHA-256 du jeton envoyé par email
  expires_at      TIMESTAMPTZ NOT NULL,
  used_at         TIMESTAMPTZ,
  revoked_at      TIMESTAMPTZ,                     -- remplacé par une demande plus récente ou par le changement
  ip              TEXT,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, user_id) REFERENCES users (organization_id, id) ON DELETE CASCADE
);
-- Sert aussi au plafond de demandes par compte et par heure.
CREATE INDEX password_resets_user_idx ON password_resets (organization_id, user_id, created_at DESC);

ALTER TABLE password_resets ENABLE ROW LEVEL SECURITY;
ALTER TABLE password_resets FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON password_resets
  USING (organization_id = current_org()) WITH CHECK (organization_id = current_org());
GRANT SELECT, INSERT, UPDATE ON password_resets TO cf_app;

-- Résout un jeton encore utilisable avant que le tenant ne soit connu, comme auth_find_invitation.
CREATE FUNCTION auth_find_password_reset(p_token_hash BYTEA)
RETURNS TABLE (reset_id UUID, user_id UUID, organization_id UUID)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS $$
  SELECT r.id, r.user_id, r.organization_id
    FROM password_resets r
   WHERE r.token_hash = p_token_hash
     AND r.used_at IS NULL AND r.revoked_at IS NULL AND r.expires_at > now()
$$;
REVOKE ALL ON FUNCTION auth_find_password_reset(BYTEA) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_find_password_reset(BYTEA) TO cf_app;

-- La purge retire aussi les jetons terminés depuis plus de 30 jours ; la signature ne change pas.
CREATE OR REPLACE FUNCTION purge_expired() RETURNS TABLE (idempotency_keys BIGINT, sessions BIGINT)
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
DECLARE
  keys_deleted BIGINT;
  sessions_deleted BIGINT;
BEGIN
  DELETE FROM idempotency_keys WHERE created_at < now() - interval '7 days';
  GET DIAGNOSTICS keys_deleted = ROW_COUNT;
  -- Les sessions terminées restent 30 jours : elles servent à enquêter sur un incident.
  DELETE FROM sessions WHERE expires_at < now() - interval '30 days' OR revoked_at < now() - interval '30 days';
  GET DIAGNOSTICS sessions_deleted = ROW_COUNT;
  DELETE FROM password_resets WHERE expires_at < now() - interval '30 days';
  RETURN QUERY SELECT keys_deleted, sessions_deleted;
END $$;
