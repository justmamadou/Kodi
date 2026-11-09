-- Rôle de l'API : soumis à la RLS (ni propriétaire, ni BYPASSRLS). Développement uniquement ;
-- en production le mot de passe vient du gestionnaire de secrets.
CREATE ROLE cf_app LOGIN PASSWORD 'cf_app' NOSUPERUSER NOBYPASSRLS NOCREATEDB NOCREATEROLE;
