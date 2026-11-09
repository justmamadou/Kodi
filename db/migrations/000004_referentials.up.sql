-- CF-015, CF-021 : catégories, fournisseurs, catalogue de matériaux.
CREATE TABLE expense_categories (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL REFERENCES organizations(id),
  name            TEXT        NOT NULL CHECK (length(name) BETWEEN 2 AND 80),
  system_code     TEXT        CHECK (system_code IN ('materials','labour','subcontracting','transport','equipment_rental','misc')),
  is_active       BOOLEAN     NOT NULL DEFAULT true,
  sort_order      SMALLINT    NOT NULL DEFAULT 100,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT expense_categories_org_id_uq UNIQUE (organization_id, id)
);
CREATE UNIQUE INDEX expense_categories_name_uq ON expense_categories (organization_id, lower(name));

CREATE TABLE suppliers (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL REFERENCES organizations(id),
  name            TEXT        NOT NULL CHECK (length(name) BETWEEN 2 AND 160),
  phone           TEXT,
  is_active       BOOLEAN     NOT NULL DEFAULT true,
  created_by      UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT suppliers_org_id_uq UNIQUE (organization_id, id),
  FOREIGN KEY (organization_id, created_by) REFERENCES users (organization_id, id)
);
CREATE UNIQUE INDEX suppliers_name_uq ON suppliers (organization_id, lower(name));

CREATE TABLE materials (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL REFERENCES organizations(id),
  name            TEXT        NOT NULL CHECK (length(name) BETWEEN 2 AND 160),
  unit            TEXT        NOT NULL CHECK (unit IN ('sac','barre','m3','m2','ml','unite','kg','tonne','litre','rouleau','boite')),
  sku             TEXT        CHECK (sku IS NULL OR sku ~ '^[A-Za-z0-9._-]{1,40}$'),
  archived_at     TIMESTAMPTZ,                     -- archivé : masqué du catalogue, conservé dans l'historique
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT materials_org_id_uq UNIQUE (organization_id, id)
);
CREATE UNIQUE INDEX materials_sku_uq ON materials (organization_id, lower(sku)) WHERE sku IS NOT NULL;
CREATE UNIQUE INDEX materials_name_unit_uq ON materials (organization_id, lower(name), unit);
CREATE TRIGGER trg_materials_updated BEFORE UPDATE ON materials
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
