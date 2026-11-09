-- CF-022, CF-023 : mouvements de stock (en-tête + lignes) et projection de solde par chantier.
CREATE TABLE stock_movements (
  id              UUID PRIMARY KEY,                -- UUIDv7 client possible
  organization_id UUID        NOT NULL,
  project_id      UUID        NOT NULL,
  movement_type   TEXT        NOT NULL CHECK (movement_type IN ('in','out','adjustment')),
  status          TEXT        NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','posted')),
  occurred_on     DATE        NOT NULL,
  reason          TEXT,
  created_by      UUID        NOT NULL,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  posted_by       UUID,
  posted_at       TIMESTAMPTZ,
  CONSTRAINT stock_movements_org_id_uq UNIQUE (organization_id, id),
  CONSTRAINT stock_movements_posted_chk CHECK ((status = 'posted') = (posted_at IS NOT NULL AND posted_by IS NOT NULL)),
  CONSTRAINT stock_movements_adjust_reason_chk CHECK (movement_type <> 'adjustment' OR length(btrim(reason)) > 0),
  FOREIGN KEY (organization_id, project_id) REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, created_by) REFERENCES users (organization_id, id),
  FOREIGN KEY (organization_id, posted_by)  REFERENCES users (organization_id, id)
);
CREATE INDEX stock_movements_project_idx ON stock_movements (organization_id, project_id, occurred_on DESC, id DESC);

CREATE TABLE stock_movement_items (
  organization_id UUID          NOT NULL,
  movement_id     UUID          NOT NULL,
  material_id     UUID          NOT NULL,
  quantity        NUMERIC(14,3) NOT NULL CHECK (quantity <> 0),  -- signée pour un ajustement uniquement
  PRIMARY KEY (movement_id, material_id),
  FOREIGN KEY (organization_id, movement_id) REFERENCES stock_movements (organization_id, id),
  FOREIGN KEY (organization_id, material_id) REFERENCES materials (organization_id, id)
);
CREATE INDEX stock_movement_items_material_idx ON stock_movement_items (organization_id, material_id);

-- Un mouvement posté est immuable (en-tête et lignes).
CREATE FUNCTION forbid_posted_movement_change() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
  v_status TEXT;
BEGIN
  IF TG_TABLE_NAME = 'stock_movements' THEN
    IF OLD.status = 'posted' THEN
      RAISE EXCEPTION 'stock movement % is posted and immutable', OLD.id USING ERRCODE = 'P0001';
    END IF;
    IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
    RETURN NEW;
  END IF;
  IF TG_OP = 'DELETE' THEN
    SELECT status INTO v_status FROM stock_movements WHERE id = OLD.movement_id;
  ELSE
    SELECT status INTO v_status FROM stock_movements WHERE id = NEW.movement_id;
  END IF;
  IF v_status = 'posted' THEN
    RAISE EXCEPTION 'items of a posted stock movement are immutable' USING ERRCODE = 'P0001';
  END IF;
  IF TG_OP = 'DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER trg_stock_movements_immutable BEFORE UPDATE OR DELETE ON stock_movements
  FOR EACH ROW EXECUTE FUNCTION forbid_posted_movement_change();
CREATE TRIGGER trg_stock_items_immutable BEFORE INSERT OR UPDATE OR DELETE ON stock_movement_items
  FOR EACH ROW EXECUTE FUNCTION forbid_posted_movement_change();

-- Projection : le CHECK garantit, même en cas de course, qu'une sortie ne rend jamais le stock négatif.
CREATE TABLE project_material_stock (
  organization_id  UUID          NOT NULL,
  project_id       UUID          NOT NULL,
  material_id      UUID          NOT NULL,
  on_hand          NUMERIC(14,3) NOT NULL DEFAULT 0 CHECK (on_hand >= 0),
  last_movement_on DATE,
  updated_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, material_id),
  FOREIGN KEY (organization_id, project_id)  REFERENCES projects (organization_id, id),
  FOREIGN KEY (organization_id, material_id) REFERENCES materials (organization_id, id)
);
CREATE INDEX project_material_stock_org_idx ON project_material_stock (organization_id, project_id);
