-- E2 : gestion des fournisseurs. Fiche légale et commerciale, contacts, conditions commerciales
-- historisées, archivage, contrôle des doublons.
CREATE EXTENSION IF NOT EXISTS unaccent;

-- unaccent() n'est que STABLE : cette enveloppe, liée à un dictionnaire précis, peut servir dans
-- une colonne générée et un index.
CREATE FUNCTION f_unaccent(text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT AS $$ SELECT public.unaccent('public.unaccent'::regdictionary, $1) $$;

-- Clé de doublon stricte : sans casse, accents, espaces ni ponctuation (« Quincaillerie  N'Diaye »
-- = « quincaillerie ndiaye »).
CREATE FUNCTION supplier_name_key(text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT AS $$
  SELECT regexp_replace(lower(f_unaccent($1)), '[^a-z0-9]+', '', 'g')
$$;

-- Clé large, pour signaler un doublon probable : sans la forme juridique ni « Ets »
-- (« Ets Ndiaye SARL » ~ « Ndiaye »).
CREATE FUNCTION supplier_loose_key(text) RETURNS text
LANGUAGE sql IMMUTABLE PARALLEL SAFE STRICT AS $$
  SELECT regexp_replace(
           regexp_replace(lower(f_unaccent($1)),
             '\m(sarl|suarl|sa|sas|sasu|gie|ets|etablissements?|ste|societe|entreprise|group|groupe)\M', ' ', 'g'),
           '[^a-z0-9]+', '', 'g')
$$;

ALTER TABLE suppliers
  ADD COLUMN legal_name   TEXT CHECK (length(legal_name) BETWEEN 2 AND 200),
  ADD COLUMN legal_form   TEXT CHECK (legal_form IN ('ei','suarl','sarl','sa','sas','gie','other')),
  ADD COLUMN ninea        TEXT CHECK (ninea ~ '^[0-9]{7,9}[0-9A-Z]{0,3}$'),           -- identifiant fiscal, sans espace
  ADD COLUMN rccm         TEXT CHECK (rccm ~ '^[A-Z0-9][A-Z0-9./ -]{4,39}$'),        -- registre du commerce
  ADD COLUMN address      TEXT CHECK (length(address) <= 300),
  ADD COLUMN city         TEXT CHECK (length(city) <= 100),
  ADD COLUMN country_code CHAR(2) NOT NULL DEFAULT 'SN',
  ADD COLUMN activities   TEXT[] NOT NULL DEFAULT '{}'
    CHECK (activities <@ ARRAY['materials','equipment_rental','subcontracting','transport','services','other']),
  ADD COLUMN notes        TEXT CHECK (length(notes) <= 2000),
  ADD COLUMN archived_at  TIMESTAMPTZ,
  ADD COLUMN archived_by  UUID,
  ADD COLUMN updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  ADD COLUMN name_key     TEXT GENERATED ALWAYS AS (supplier_name_key(name)) STORED;
-- Les fournisseurs désactivés jusqu'ici deviennent des fournisseurs archivés.
UPDATE suppliers SET archived_at = created_at WHERE NOT is_active;
ALTER TABLE suppliers
  ADD CONSTRAINT suppliers_archived_chk CHECK ((archived_at IS NULL) = is_active),
  ADD CONSTRAINT suppliers_archived_by_fk FOREIGN KEY (organization_id, archived_by) REFERENCES users (organization_id, id);
CREATE TRIGGER trg_suppliers_updated BEFORE UPDATE ON suppliers
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Doublons bloquants, par entreprise uniquement : deux entreprises gardent chacune leur fiche.
DROP INDEX suppliers_name_uq;
CREATE UNIQUE INDEX suppliers_name_key_uq ON suppliers (organization_id, name_key);
CREATE UNIQUE INDEX suppliers_ninea_uq ON suppliers (organization_id, ninea) WHERE ninea IS NOT NULL;
CREATE UNIQUE INDEX suppliers_rccm_uq ON suppliers (organization_id, rccm) WHERE rccm IS NOT NULL;
-- Doublons probables : même téléphone, même email, nom très proche.
CREATE INDEX suppliers_phone_idx ON suppliers (organization_id, phone) WHERE phone IS NOT NULL;
CREATE INDEX suppliers_email_idx ON suppliers (organization_id, email) WHERE email IS NOT NULL;
CREATE INDEX suppliers_name_trgm_idx ON suppliers USING gin (supplier_loose_key(name) gin_trgm_ops);

CREATE TABLE supplier_contacts (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL,
  supplier_id     UUID        NOT NULL,
  full_name       TEXT        NOT NULL CHECK (length(full_name) BETWEEN 2 AND 160),
  job_title       TEXT        CHECK (length(job_title) <= 100),
  phone           TEXT        CHECK (phone ~ '^\+[1-9][0-9]{6,14}$'),
  email           CITEXT,
  is_primary      BOOLEAN     NOT NULL DEFAULT false,
  notes           TEXT        CHECK (length(notes) <= 500),
  created_by      UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT supplier_contacts_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT supplier_contacts_reach_chk CHECK (phone IS NOT NULL OR email IS NOT NULL),
  FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id),
  FOREIGN KEY (organization_id, created_by)  REFERENCES users (organization_id, id)
);
CREATE INDEX supplier_contacts_supplier_idx ON supplier_contacts (organization_id, supplier_id);
-- Un seul contact principal par fournisseur.
CREATE UNIQUE INDEX supplier_contacts_primary_uq ON supplier_contacts (supplier_id) WHERE is_primary;
CREATE TRIGGER trg_supplier_contacts_updated BEFORE UPDATE ON supplier_contacts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Conditions commerciales : chaque changement est une nouvelle version, jamais modifiée ni supprimée.
CREATE TABLE supplier_terms (
  id                   UUID PRIMARY KEY,
  organization_id      UUID         NOT NULL,
  supplier_id          UUID         NOT NULL,
  version              INT          NOT NULL CHECK (version > 0),
  effective_from       DATE         NOT NULL,
  payment_method       TEXT         CHECK (payment_method IN ('cash','wave','orange_money','free_money','bank_transfer','cheque','other')),
  payment_delay_days   SMALLINT     CHECK (payment_delay_days BETWEEN 0 AND 365),
  advance_pct          SMALLINT     CHECK (advance_pct BETWEEN 0 AND 100),
  discount_pct         NUMERIC(5,2) CHECK (discount_pct BETWEEN 0 AND 100),
  delivery_days        SMALLINT     CHECK (delivery_days BETWEEN 0 AND 365),
  delivery_included    BOOLEAN,
  minimum_order_amount BIGINT       CHECK (minimum_order_amount >= 0),
  credit_limit         BIGINT       CHECK (credit_limit >= 0),
  notes                TEXT         CHECK (length(notes) <= 2000),
  created_by           UUID         NOT NULL,
  created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
  CONSTRAINT supplier_terms_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT supplier_terms_version_uq UNIQUE (supplier_id, version),
  FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id),
  FOREIGN KEY (organization_id, created_by)  REFERENCES users (organization_id, id)
);
CREATE TRIGGER trg_supplier_terms_append_only BEFORE UPDATE OR DELETE ON supplier_terms
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY['supplier_contacts','supplier_terms'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I USING (organization_id = current_org()) '
      'WITH CHECK (organization_id = current_org())', t);
  END LOOP;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON supplier_contacts TO cf_app;
GRANT SELECT, INSERT ON supplier_terms TO cf_app;
GRANT EXECUTE ON FUNCTION f_unaccent(text), supplier_name_key(text), supplier_loose_key(text) TO cf_app;
