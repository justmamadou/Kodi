DROP TABLE IF EXISTS purchase_order_events;
DROP TABLE IF EXISTS purchase_order_receipt_lines;
DROP TABLE IF EXISTS purchase_order_receipts;
DROP TABLE IF EXISTS purchase_order_lines;
DROP TABLE IF EXISTS purchase_orders;
DROP FUNCTION IF EXISTS apply_purchase_order_receipt_line();
DROP FUNCTION IF EXISTS check_purchase_order_line_change();
DROP FUNCTION IF EXISTS check_purchase_order_change();

ALTER TABLE rfqs
  DROP CONSTRAINT IF EXISTS rfqs_awarded_by_fk,
  DROP CONSTRAINT IF EXISTS rfqs_awarded_offer_fk,
  DROP CONSTRAINT IF EXISTS rfqs_award_closed_chk,
  DROP CONSTRAINT IF EXISTS rfqs_award_chk,
  DROP CONSTRAINT IF EXISTS rfqs_weights_chk,
  DROP COLUMN IF EXISTS award_reason,
  DROP COLUMN IF EXISTS awarded_at,
  DROP COLUMN IF EXISTS awarded_by,
  DROP COLUMN IF EXISTS awarded_offer_id,
  DROP COLUMN IF EXISTS weight_coverage,
  DROP COLUMN IF EXISTS weight_delivery,
  DROP COLUMN IF EXISTS weight_price;

CREATE OR REPLACE FUNCTION check_rfq_change() RETURNS trigger
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
