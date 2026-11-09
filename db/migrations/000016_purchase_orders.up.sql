-- E3 : comparaison des offres sur critères, sélection, bons de commande, réceptions et entrée en stock.

-- Critères de comparaison d'une demande de prix : poids en pour cent du prix, du délai de livraison et
-- de la couverture (part des lignes chiffrées). Modifiables jusqu'à la sélection.
ALTER TABLE rfqs
  ADD COLUMN weight_price    SMALLINT NOT NULL DEFAULT 60 CHECK (weight_price BETWEEN 0 AND 100),
  ADD COLUMN weight_delivery SMALLINT NOT NULL DEFAULT 25 CHECK (weight_delivery BETWEEN 0 AND 100),
  ADD COLUMN weight_coverage SMALLINT NOT NULL DEFAULT 15 CHECK (weight_coverage BETWEEN 0 AND 100),
  ADD COLUMN awarded_offer_id UUID,
  ADD COLUMN awarded_by       UUID,
  ADD COLUMN awarded_at       TIMESTAMPTZ,
  ADD COLUMN award_reason     TEXT CHECK (length(award_reason) <= 1000),
  ADD CONSTRAINT rfqs_weights_chk CHECK (weight_price + weight_delivery + weight_coverage = 100),
  ADD CONSTRAINT rfqs_award_chk CHECK ((awarded_offer_id IS NULL) = (awarded_at IS NULL AND awarded_by IS NULL)),
  -- On ne retient une offre qu'une fois la consultation close : plus personne ne peut surenchérir.
  ADD CONSTRAINT rfqs_award_closed_chk CHECK (awarded_offer_id IS NULL OR status = 'closed'),
  ADD CONSTRAINT rfqs_awarded_offer_fk FOREIGN KEY (organization_id, awarded_offer_id) REFERENCES rfq_offers (organization_id, id),
  ADD CONSTRAINT rfqs_awarded_by_fk FOREIGN KEY (organization_id, awarded_by) REFERENCES users (organization_id, id);

-- Une demande close reste figée, sauf sa sélection et ses critères ; une demande annulée ne bouge plus.
CREATE OR REPLACE FUNCTION check_rfq_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status = 'cancelled' THEN
    RAISE EXCEPTION 'rfq % is cancelled and immutable', OLD.id USING ERRCODE = 'P0001';
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

CREATE TABLE purchase_orders (
  id                     UUID PRIMARY KEY,
  organization_id        UUID        NOT NULL,
  project_id             UUID        NOT NULL,
  supplier_id            UUID        NOT NULL,
  rfq_id                 UUID,
  offer_id               UUID,
  number                 INT         NOT NULL CHECK (number > 0),          -- affiché BC-0001, propre à l'entreprise
  status                 TEXT        NOT NULL DEFAULT 'draft' CHECK (status IN
                           ('draft','submitted','approved','sent','partially_received','received','cancelled')),
  currency               CHAR(3)     NOT NULL DEFAULT 'XOF',
  expected_delivery_date DATE,
  delivery_address       TEXT        CHECK (length(delivery_address) <= 300),
  payment_terms          TEXT        CHECK (length(payment_terms) <= 500),
  notes                  TEXT        CHECK (length(notes) <= 2000),
  subtotal_amount        BIGINT      NOT NULL DEFAULT 0 CHECK (subtotal_amount >= 0),  -- HT, recalculé à chaque écriture des lignes
  tax_amount             BIGINT      NOT NULL DEFAULT 0 CHECK (tax_amount >= 0),
  total_amount           BIGINT      NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
  created_by             UUID        NOT NULL,
  created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
  submitted_by           UUID,
  submitted_at           TIMESTAMPTZ,
  approved_by            UUID,
  approved_at            TIMESTAMPTZ,
  sent_by                UUID,
  sent_at                TIMESTAMPTZ,
  cancelled_by           UUID,
  cancelled_at           TIMESTAMPTZ,
  cancel_reason          TEXT,
  CONSTRAINT purchase_orders_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT purchase_orders_number_uq UNIQUE (organization_id, number),
  CONSTRAINT purchase_orders_total_chk CHECK (total_amount = subtotal_amount + tax_amount),
  -- Séparation des tâches, garantie aussi par la base : on n'approuve pas sa propre commande.
  CONSTRAINT purchase_orders_no_self_approval_chk CHECK (approved_by IS NULL OR approved_by <> created_by),
  CONSTRAINT purchase_orders_approved_chk CHECK (status NOT IN ('approved','sent','partially_received','received')
    OR (approved_at IS NOT NULL AND approved_by IS NOT NULL)),
  CONSTRAINT purchase_orders_sent_chk CHECK (status NOT IN ('sent','partially_received','received')
    OR (sent_at IS NOT NULL AND sent_by IS NOT NULL)),
  CONSTRAINT purchase_orders_cancelled_chk CHECK ((status = 'cancelled') =
    (cancelled_at IS NOT NULL AND cancelled_by IS NOT NULL AND length(btrim(cancel_reason)) > 0)),
  FOREIGN KEY (organization_id, project_id)   REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, supplier_id)  REFERENCES suppliers (organization_id, id),
  FOREIGN KEY (organization_id, rfq_id)       REFERENCES rfqs (organization_id, id),
  FOREIGN KEY (organization_id, offer_id)     REFERENCES rfq_offers (organization_id, id),
  FOREIGN KEY (organization_id, created_by)   REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, submitted_by) REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, approved_by)  REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, sent_by)      REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, cancelled_by) REFERENCES users (organization_id, id)
);
CREATE INDEX purchase_orders_project_idx ON purchase_orders (organization_id, project_id, created_at DESC);
CREATE INDEX purchase_orders_supplier_idx ON purchase_orders (organization_id, supplier_id, created_at DESC);
-- Une offre retenue ne donne qu'une commande active.
CREATE UNIQUE INDEX purchase_orders_offer_uq ON purchase_orders (offer_id) WHERE offer_id IS NOT NULL AND status <> 'cancelled';
CREATE TRIGGER trg_purchase_orders_updated BEFORE UPDATE ON purchase_orders
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Cycle de vie : draft -> submitted -> approved -> sent -> partially_received -> received ;
-- submitted -> draft (renvoyée pour correction) ; annulation possible tant que rien n'est reçu.
-- Le contenu ne change qu'en brouillon.
CREATE FUNCTION check_purchase_order_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.status IN ('received','cancelled') THEN
    RAISE EXCEPTION 'purchase order % is % and immutable', OLD.id, OLD.status USING ERRCODE = 'P0001';
  END IF;
  IF NEW.status <> OLD.status AND NOT (
       (OLD.status = 'draft' AND NEW.status IN ('submitted','cancelled'))
    OR (OLD.status = 'submitted' AND NEW.status IN ('approved','draft','cancelled'))
    OR (OLD.status = 'approved' AND NEW.status IN ('sent','cancelled'))
    OR (OLD.status = 'sent' AND NEW.status IN ('partially_received','received','cancelled'))
    OR (OLD.status = 'partially_received' AND NEW.status = 'received')) THEN
    RAISE EXCEPTION 'purchase order transition % -> % refused', OLD.status, NEW.status USING ERRCODE = 'P0001';
  END IF;
  IF OLD.status <> 'draft' AND (NEW.project_id, NEW.supplier_id, NEW.number, NEW.expected_delivery_date, NEW.delivery_address,
       NEW.payment_terms, NEW.notes, NEW.subtotal_amount, NEW.tax_amount, NEW.total_amount)
       IS DISTINCT FROM (OLD.project_id, OLD.supplier_id, OLD.number, OLD.expected_delivery_date, OLD.delivery_address,
       OLD.payment_terms, OLD.notes, OLD.subtotal_amount, OLD.tax_amount, OLD.total_amount) THEN
    RAISE EXCEPTION 'purchase order % is no longer a draft: content is frozen', OLD.id USING ERRCODE = 'P0001';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_purchase_orders_change BEFORE UPDATE ON purchase_orders
  FOR EACH ROW EXECUTE FUNCTION check_purchase_order_change();

CREATE TABLE purchase_order_lines (
  id                 UUID PRIMARY KEY,
  organization_id    UUID          NOT NULL,
  purchase_order_id  UUID          NOT NULL,
  position           SMALLINT      NOT NULL CHECK (position BETWEEN 1 AND 200),
  rfq_item_id        UUID,
  material_id        UUID,                                             -- seul un article du catalogue entre en stock
  label              TEXT          NOT NULL CHECK (length(label) BETWEEN 2 AND 200),
  unit               TEXT          NOT NULL CHECK (unit IN ('sac','barre','m3','m2','ml','unite','kg','tonne','litre',
                                                            'rouleau','boite','forfait','jour','heure')),
  quantity_ordered   NUMERIC(14,3) NOT NULL CHECK (quantity_ordered > 0),
  unit_price         BIGINT        NOT NULL CHECK (unit_price >= 0),
  tax_rate           NUMERIC(5,2)  NOT NULL DEFAULT 0 CHECK (tax_rate BETWEEN 0 AND 100),
  amount             BIGINT        GENERATED ALWAYS AS (round(quantity_ordered * unit_price)::bigint) STORED,
  tax_amount         BIGINT        GENERATED ALWAYS AS (round(round(quantity_ordered * unit_price) * tax_rate / 100)::bigint) STORED,
  -- Tenue par les réceptions (trigger) : jamais négative, jamais au-delà de la quantité commandée.
  quantity_received  NUMERIC(14,3) NOT NULL DEFAULT 0,
  quantity_remaining NUMERIC(14,3) GENERATED ALWAYS AS (quantity_ordered - quantity_received) STORED,
  comment            TEXT          CHECK (length(comment) <= 500),
  CONSTRAINT purchase_order_lines_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT purchase_order_lines_position_uq UNIQUE (purchase_order_id, position),
  CONSTRAINT purchase_order_lines_received_chk CHECK (quantity_received >= 0 AND quantity_received <= quantity_ordered),
  FOREIGN KEY (organization_id, purchase_order_id) REFERENCES purchase_orders (organization_id, id),
  FOREIGN KEY (organization_id, rfq_item_id)       REFERENCES rfq_items (organization_id, id),
  FOREIGN KEY (organization_id, material_id)       REFERENCES materials (organization_id, id)
);

-- Les lignes ne changent qu'en brouillon ; seule la quantité reçue évolue ensuite, par les réceptions.
CREATE FUNCTION check_purchase_order_line_change() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  v_status TEXT;
BEGIN
  SELECT status INTO v_status FROM purchase_orders WHERE id = COALESCE(NEW.purchase_order_id, OLD.purchase_order_id);
  IF v_status = 'draft' THEN
    IF TG_OP = 'UPDATE' AND NEW.quantity_received <> OLD.quantity_received THEN
      RAISE EXCEPTION 'a draft purchase order cannot be received' USING ERRCODE = 'P0001';
    END IF;
    RETURN COALESCE(NEW, OLD);
  END IF;
  IF TG_OP = 'UPDATE' AND v_status IN ('sent','partially_received')
     AND (NEW.purchase_order_id, NEW.position, NEW.rfq_item_id, NEW.material_id, NEW.label, NEW.unit,
          NEW.quantity_ordered, NEW.unit_price, NEW.tax_rate, NEW.comment)
         IS NOT DISTINCT FROM (OLD.purchase_order_id, OLD.position, OLD.rfq_item_id, OLD.material_id, OLD.label, OLD.unit,
          OLD.quantity_ordered, OLD.unit_price, OLD.tax_rate, OLD.comment) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'lines of purchase order % cannot change once %', COALESCE(NEW.purchase_order_id, OLD.purchase_order_id), v_status
    USING ERRCODE = 'P0001';
END $$;
CREATE TRIGGER trg_purchase_order_lines_change BEFORE INSERT OR UPDATE OR DELETE ON purchase_order_lines
  FOR EACH ROW EXECUTE FUNCTION check_purchase_order_line_change();

-- Réceptions : l'identifiant vient de l'appareil (un renvoi n'a qu'un effet) et un même bon de
-- livraison ne s'enregistre qu'une fois par commande. Jamais modifiées ni supprimées.
CREATE TABLE purchase_order_receipts (
  id                UUID PRIMARY KEY,
  organization_id   UUID        NOT NULL,
  purchase_order_id UUID        NOT NULL,
  received_on       DATE        NOT NULL,
  delivery_note_ref TEXT        CHECK (length(btrim(delivery_note_ref)) BETWEEN 1 AND 60),
  notes             TEXT        CHECK (length(notes) <= 1000),
  stock_movement_id UUID,
  received_by       UUID        NOT NULL,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT purchase_order_receipts_org_id_uq UNIQUE (organization_id, id),
  FOREIGN KEY (organization_id, purchase_order_id) REFERENCES purchase_orders (organization_id, id),
  FOREIGN KEY (organization_id, stock_movement_id) REFERENCES stock_movements (organization_id, id),
  FOREIGN KEY (organization_id, received_by)       REFERENCES users (organization_id, id)
);
CREATE UNIQUE INDEX purchase_order_receipts_note_uq ON purchase_order_receipts
  (purchase_order_id, upper(regexp_replace(delivery_note_ref, '\s', '', 'g'))) WHERE delivery_note_ref IS NOT NULL;
CREATE INDEX purchase_order_receipts_po_idx ON purchase_order_receipts (organization_id, purchase_order_id, created_at);

CREATE TABLE purchase_order_receipt_lines (
  organization_id UUID          NOT NULL,
  receipt_id      UUID          NOT NULL,
  line_id         UUID          NOT NULL,
  quantity        NUMERIC(14,3) NOT NULL CHECK (quantity > 0),
  PRIMARY KEY (receipt_id, line_id),
  FOREIGN KEY (organization_id, receipt_id) REFERENCES purchase_order_receipts (organization_id, id),
  FOREIGN KEY (organization_id, line_id)    REFERENCES purchase_order_lines (organization_id, id)
);

-- Quantité reçue = somme des réceptions, par construction : chaque ligne de réception l'incrémente
-- dans la même transaction, et le CHECK de la ligne de commande refuse tout dépassement.
CREATE FUNCTION apply_purchase_order_receipt_line() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  UPDATE purchase_order_lines SET quantity_received = quantity_received + NEW.quantity
   WHERE id = NEW.line_id AND organization_id = NEW.organization_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'purchase order line % not found', NEW.line_id USING ERRCODE = 'P0001';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_purchase_order_receipt_lines_apply AFTER INSERT ON purchase_order_receipt_lines
  FOR EACH ROW EXECUTE FUNCTION apply_purchase_order_receipt_line();

-- Historique des événements d'une commande : transitions, modifications, réceptions.
CREATE TABLE purchase_order_events (
  id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  organization_id   UUID        NOT NULL,
  purchase_order_id UUID        NOT NULL,
  event             TEXT        NOT NULL CHECK (event IN ('created','updated','submitted','returned','approved','sent','received','cancelled')),
  from_status       TEXT,
  to_status         TEXT        NOT NULL,
  actor_id          UUID        NOT NULL,
  comment           TEXT        CHECK (length(comment) <= 1000),
  changes           JSONB,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  FOREIGN KEY (organization_id, purchase_order_id) REFERENCES purchase_orders (organization_id, id),
  FOREIGN KEY (organization_id, actor_id)          REFERENCES users (organization_id, id)
);
CREATE INDEX purchase_order_events_po_idx ON purchase_order_events (organization_id, purchase_order_id, id);

CREATE TRIGGER trg_purchase_order_receipts_append_only BEFORE UPDATE OR DELETE ON purchase_order_receipts
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER trg_purchase_order_receipt_lines_append_only BEFORE UPDATE OR DELETE ON purchase_order_receipt_lines
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER trg_purchase_order_events_append_only BEFORE UPDATE OR DELETE ON purchase_order_events
  FOR EACH ROW EXECUTE FUNCTION forbid_mutation();

DO $$
DECLARE
  t TEXT;
BEGIN
  FOREACH t IN ARRAY ARRAY['purchase_orders','purchase_order_lines','purchase_order_receipts',
                           'purchase_order_receipt_lines','purchase_order_events'] LOOP
    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
    EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
    EXECUTE format(
      'CREATE POLICY tenant_isolation ON %I USING (organization_id = current_org()) '
      'WITH CHECK (organization_id = current_org())', t);
  END LOOP;
END $$;

GRANT SELECT, INSERT, UPDATE ON purchase_orders, purchase_order_lines TO cf_app;
GRANT DELETE ON purchase_order_lines TO cf_app;
GRANT SELECT, INSERT ON purchase_order_receipts, purchase_order_receipt_lines, purchase_order_events TO cf_app;
GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO cf_app;
