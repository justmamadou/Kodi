-- E1 : demandes de prix (RFQ), consultation des fournisseurs, réception des offres.

-- Le lien de consultation part par email quand le fournisseur en a un.
ALTER TABLE suppliers ADD COLUMN email CITEXT;

CREATE TABLE rfqs (
  id                UUID PRIMARY KEY,
  organization_id   UUID        NOT NULL,
  project_id        UUID        NOT NULL,
  number            INT         NOT NULL CHECK (number > 0),          -- affiché DP-0001, propre à l'entreprise
  title             TEXT        NOT NULL CHECK (length(title) BETWEEN 2 AND 160),
  notes             TEXT        CHECK (length(notes) <= 4000),
  response_deadline DATE,                                             -- incluse, heure de Dakar ; obligatoire à la publication
  delivery_date     DATE,                                             -- livraison souhaitée
  status            TEXT        NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','published','closed','cancelled')),
  cancel_reason     TEXT,
  created_by        UUID        NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at      TIMESTAMPTZ,
  published_by      UUID,
  closed_at         TIMESTAMPTZ,
  closed_by         UUID,
  cancelled_at      TIMESTAMPTZ,
  cancelled_by      UUID,
  CONSTRAINT rfqs_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT rfqs_number_uq UNIQUE (organization_id, number),
  CONSTRAINT rfqs_published_chk CHECK (status NOT IN ('published','closed')
    OR (published_at IS NOT NULL AND published_by IS NOT NULL AND response_deadline IS NOT NULL)),
  CONSTRAINT rfqs_closed_chk CHECK ((status = 'closed') = (closed_at IS NOT NULL AND closed_by IS NOT NULL)),
  CONSTRAINT rfqs_cancelled_chk CHECK ((status = 'cancelled') =
    (cancelled_at IS NOT NULL AND cancelled_by IS NOT NULL AND length(btrim(cancel_reason)) > 0)),
  FOREIGN KEY (organization_id, project_id)   REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, created_by)   REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, published_by) REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, closed_by)    REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, cancelled_by) REFERENCES users (organization_id, id)
);
CREATE INDEX rfqs_project_idx ON rfqs (organization_id, project_id, created_at DESC);
CREATE INDEX rfqs_status_idx ON rfqs (organization_id, status, created_at DESC);
CREATE TRIGGER trg_rfqs_updated BEFORE UPDATE ON rfqs
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Cycle de vie : draft -> published -> closed, draft|published -> cancelled. Le contenu est figé dès
-- la publication : les fournisseurs chiffrent tous la même demande.
CREATE FUNCTION check_rfq_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status IN ('closed','cancelled') THEN
    RAISE EXCEPTION 'rfq % is % and immutable', OLD.id, OLD.status USING ERRCODE = 'P0001';
  END IF;
  IF NEW.status <> OLD.status AND NOT (
       (OLD.status = 'draft' AND NEW.status IN ('published','cancelled'))
    OR (OLD.status = 'published' AND NEW.status IN ('closed','cancelled'))) THEN
    RAISE EXCEPTION 'rfq transition % -> % refused', OLD.status, NEW.status USING ERRCODE = 'P0001';
  END IF;
  IF OLD.status <> 'draft' AND (NEW.project_id, NEW.number, NEW.title, NEW.notes, NEW.response_deadline, NEW.delivery_date)
       IS DISTINCT FROM (OLD.project_id, OLD.number, OLD.title, OLD.notes, OLD.response_deadline, OLD.delivery_date) THEN
    RAISE EXCEPTION 'rfq % is published: content is frozen', OLD.id USING ERRCODE = 'P0001';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_rfqs_change BEFORE UPDATE ON rfqs
  FOR EACH ROW EXECUTE FUNCTION check_rfq_change();

-- Matériaux (éventuellement liés au catalogue) ou prestations demandés.
CREATE TABLE rfq_items (
  id              UUID PRIMARY KEY,
  organization_id UUID          NOT NULL,
  rfq_id          UUID          NOT NULL,
  position        SMALLINT      NOT NULL CHECK (position BETWEEN 1 AND 200),
  kind            TEXT          NOT NULL CHECK (kind IN ('material','service')),
  material_id     UUID,
  label           TEXT          NOT NULL CHECK (length(label) BETWEEN 2 AND 200),
  quantity        NUMERIC(14,3) NOT NULL CHECK (quantity > 0),
  unit            TEXT          NOT NULL CHECK (unit IN ('sac','barre','m3','m2','ml','unite','kg','tonne','litre',
                                                         'rouleau','boite','forfait','jour','heure')),
  specifications  TEXT          CHECK (length(specifications) <= 2000),
  needed_by       DATE,
  CONSTRAINT rfq_items_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT rfq_items_position_uq UNIQUE (rfq_id, position),
  CONSTRAINT rfq_items_material_chk CHECK (kind = 'material' OR material_id IS NULL),
  FOREIGN KEY (organization_id, rfq_id)      REFERENCES rfqs (organization_id, id),
  FOREIGN KEY (organization_id, material_id) REFERENCES materials (organization_id, id)
);

-- Fournisseurs consultés. Le jeton de leur lien n'existe qu'à partir de la publication, et seul son
-- hash est conservé.
CREATE TABLE rfq_suppliers (
  id              UUID PRIMARY KEY,
  organization_id UUID        NOT NULL,
  rfq_id          UUID        NOT NULL,
  supplier_id     UUID        NOT NULL,
  token_hash      BYTEA       UNIQUE,
  link_issued_at  TIMESTAMPTZ,
  invited_by      UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT rfq_suppliers_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT rfq_suppliers_uq UNIQUE (rfq_id, supplier_id),
  CONSTRAINT rfq_suppliers_link_chk CHECK ((token_hash IS NULL) = (link_issued_at IS NULL)),
  FOREIGN KEY (organization_id, rfq_id)      REFERENCES rfqs (organization_id, id),
  FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers (organization_id, id),
  FOREIGN KEY (organization_id, invited_by)  REFERENCES users (organization_id, id)
);

-- Lignes et fournisseurs ne bougent qu'en brouillon ; une fois publiée, on peut encore inviter un
-- fournisseur ou renouveler son lien, tant que la demande est ouverte.
CREATE FUNCTION check_rfq_children_change() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  v_status TEXT;
BEGIN
  SELECT status INTO v_status FROM rfqs WHERE id = COALESCE(NEW.rfq_id, OLD.rfq_id);
  IF TG_TABLE_NAME = 'rfq_suppliers' AND TG_OP <> 'DELETE' AND v_status = 'published' THEN
    IF TG_OP = 'UPDATE' AND (NEW.rfq_id, NEW.supplier_id) IS DISTINCT FROM (OLD.rfq_id, OLD.supplier_id) THEN
      RAISE EXCEPTION 'rfq supplier % cannot be reassigned', OLD.id USING ERRCODE = 'P0001';
    END IF;
    RETURN NEW;
  END IF;
  IF v_status IS DISTINCT FROM 'draft' THEN
    RAISE EXCEPTION '% of rfq % cannot change once %', TG_TABLE_NAME, COALESCE(NEW.rfq_id, OLD.rfq_id), v_status
      USING ERRCODE = 'P0001';
  END IF;
  RETURN COALESCE(NEW, OLD);
END $$;
CREATE TRIGGER trg_rfq_items_change BEFORE INSERT OR UPDATE OR DELETE ON rfq_items
  FOR EACH ROW EXECUTE FUNCTION check_rfq_children_change();
CREATE TRIGGER trg_rfq_suppliers_change BEFORE INSERT OR UPDATE OR DELETE ON rfq_suppliers
  FOR EACH ROW EXECUTE FUNCTION check_rfq_children_change();

-- Offres : chaque envoi du fournisseur est une nouvelle version, jamais modifiée ni supprimée.
CREATE TABLE rfq_offers (
  id              UUID PRIMARY KEY,                -- UUID client possible : un double envoi n'a qu'un effet
  organization_id UUID        NOT NULL,
  rfq_id          UUID        NOT NULL,
  rfq_supplier_id UUID        NOT NULL,
  supplier_id     UUID        NOT NULL,
  version         INT         NOT NULL CHECK (version > 0),
  total_amount    BIGINT      NOT NULL CHECK (total_amount >= 0),
  currency        CHAR(3)     NOT NULL DEFAULT 'XOF',
  valid_until     DATE,
  delivery_days   SMALLINT    CHECK (delivery_days BETWEEN 0 AND 365),
  payment_terms   TEXT        CHECK (length(payment_terms) <= 500),
  notes           TEXT        CHECK (length(notes) <= 2000),
  contact_name    TEXT        CHECK (length(contact_name) <= 160),
  ip              TEXT,
  submitted_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT rfq_offers_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT rfq_offers_version_uq UNIQUE (rfq_supplier_id, version),
  FOREIGN KEY (organization_id, rfq_id)          REFERENCES rfqs (organization_id, id),
  FOREIGN KEY (organization_id, rfq_supplier_id) REFERENCES rfq_suppliers (organization_id, id),
  FOREIGN KEY (organization_id, supplier_id)     REFERENCES suppliers (organization_id, id)
);
CREATE INDEX rfq_offers_rfq_idx ON rfq_offers (organization_id, rfq_id, submitted_at DESC);

CREATE TABLE rfq_offer_lines (
  organization_id UUID          NOT NULL,
  offer_id        UUID          NOT NULL,
  rfq_item_id     UUID          NOT NULL,
  quantity        NUMERIC(14,3) NOT NULL CHECK (quantity > 0),   -- copiée de la ligne demandée
  unit_price      BIGINT        NOT NULL CHECK (unit_price >= 0),
  amount          BIGINT        GENERATED ALWAYS AS (round(quantity * unit_price)::bigint) STORED,
  comment         TEXT          CHECK (length(comment) <= 500),
  PRIMARY KEY (offer_id, rfq_item_id),
  FOREIGN KEY (organization_id, offer_id)    REFERENCES rfq_offers (organization_id, id),
  FOREIGN KEY (organization_id, rfq_item_id) REFERENCES rfq_items (organization_id, id)
);

CREATE TRIGGER trg_rfq_offers_append_only BEFORE UPDATE OR DELETE ON rfq_offers
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER trg_rfq_offer_lines_append_only BEFORE UPDATE OR DELETE ON rfq_offer_lines
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY['rfqs','rfq_items','rfq_suppliers','rfq_offers','rfq_offer_lines'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I USING (organization_id = current_org()) '
      'WITH CHECK (organization_id = current_org())', t);
  END LOOP;
END $$;

GRANT SELECT, INSERT, UPDATE ON rfqs, rfq_items, rfq_suppliers TO cf_app;
GRANT DELETE ON rfq_items, rfq_suppliers TO cf_app;
GRANT SELECT, INSERT ON rfq_offers, rfq_offer_lines TO cf_app;

-- Résout le lien d'un fournisseur avant que le tenant ne soit connu, comme auth_find_invitation.
CREATE FUNCTION auth_find_rfq_link(p_token_hash BYTEA)
RETURNS TABLE (rfq_supplier_id UUID, rfq_id UUID, supplier_id UUID, organization_id UUID)
LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public AS $$
  SELECT s.id, s.rfq_id, s.supplier_id, s.organization_id
    FROM rfq_suppliers s
   WHERE s.token_hash = p_token_hash
$$;
REVOKE ALL ON FUNCTION auth_find_rfq_link(BYTEA) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION auth_find_rfq_link(BYTEA) TO cf_app;
