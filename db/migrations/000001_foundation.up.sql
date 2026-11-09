-- Fondations : extensions, fonctions utilitaires, rôle applicatif.
-- Conventions : voir docs/architecture (§7.1) et docs/DECISIONS.md.
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- cf_app : rôle de l'API, ni propriétaire ni BYPASSRLS. Créé NOLOGIN s'il manque ;
-- l'infra lui donne LOGIN et son mot de passe (deploy/initdb/01-roles.sql).
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'cf_app') THEN
    CREATE ROLE cf_app NOLOGIN NOSUPERUSER NOBYPASSRLS;
  END IF;
END $$;

CREATE FUNCTION set_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END $$;

-- Tenant courant, positionné par l'API à chaque transaction.
-- Absent => NULL => aucune ligne visible (fail closed).
CREATE FUNCTION current_org() RETURNS uuid
LANGUAGE sql STABLE AS $$
  SELECT nullif(current_setting('app.current_org', true), '')::uuid
$$;

CREATE FUNCTION forbid_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION '% is append-only', TG_TABLE_NAME USING ERRCODE = 'P0001';
END $$;
