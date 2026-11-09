DROP TABLE IF EXISTS supplier_terms;
DROP TABLE IF EXISTS supplier_contacts;
DROP INDEX IF EXISTS suppliers_name_trgm_idx;
DROP INDEX IF EXISTS suppliers_email_idx;
DROP INDEX IF EXISTS suppliers_phone_idx;
DROP INDEX IF EXISTS suppliers_rccm_uq;
DROP INDEX IF EXISTS suppliers_ninea_uq;
DROP INDEX IF EXISTS suppliers_name_key_uq;
DROP TRIGGER IF EXISTS trg_suppliers_updated ON suppliers;
ALTER TABLE suppliers
  DROP CONSTRAINT IF EXISTS suppliers_archived_by_fk,
  DROP CONSTRAINT IF EXISTS suppliers_archived_chk,
  DROP COLUMN IF EXISTS name_key,
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS archived_by,
  DROP COLUMN IF EXISTS archived_at,
  DROP COLUMN IF EXISTS notes,
  DROP COLUMN IF EXISTS activities,
  DROP COLUMN IF EXISTS country_code,
  DROP COLUMN IF EXISTS city,
  DROP COLUMN IF EXISTS address,
  DROP COLUMN IF EXISTS rccm,
  DROP COLUMN IF EXISTS ninea,
  DROP COLUMN IF EXISTS legal_form,
  DROP COLUMN IF EXISTS legal_name;
CREATE UNIQUE INDEX suppliers_name_uq ON suppliers (organization_id, lower(name));
DROP FUNCTION IF EXISTS supplier_loose_key(text);
DROP FUNCTION IF EXISTS supplier_name_key(text);
DROP FUNCTION IF EXISTS f_unaccent(text);
-- L'extension unaccent reste : d'autres objets peuvent en dépendre.
