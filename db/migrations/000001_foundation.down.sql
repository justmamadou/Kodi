DROP FUNCTION IF EXISTS forbid_mutation();
DROP FUNCTION IF EXISTS current_org();
DROP FUNCTION IF EXISTS set_updated_at();
-- Le rôle cf_app est global au cluster : il n'est pas supprimé ici.
DROP EXTENSION IF EXISTS pg_trgm;
DROP EXTENSION IF EXISTS citext;
