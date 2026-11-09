DROP FUNCTION IF EXISTS auth_find_rfq_link(BYTEA);
DROP TABLE IF EXISTS rfq_offer_lines;
DROP TABLE IF EXISTS rfq_offers;
DROP TABLE IF EXISTS rfq_suppliers;
DROP TABLE IF EXISTS rfq_items;
DROP TABLE IF EXISTS rfqs;
DROP FUNCTION IF EXISTS check_rfq_children_change();
DROP FUNCTION IF EXISTS check_rfq_change();
ALTER TABLE suppliers DROP COLUMN IF EXISTS email;
