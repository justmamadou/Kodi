-- Purge des données techniques expirées. Sans tenant : c'est une maintenance globale, qui ne retourne
-- qu'un décompte. SECURITY DEFINER car cf_app n'a ni la vue sur tous les tenants ni le droit de supprimer ces lignes.
CREATE FUNCTION purge_expired() RETURNS TABLE (idempotency_keys BIGINT, sessions BIGINT)
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
  RETURN QUERY SELECT keys_deleted, sessions_deleted;
END $$;

REVOKE ALL ON FUNCTION purge_expired() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION purge_expired() TO cf_app;
