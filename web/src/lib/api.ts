import { db } from "./offline/db";

/** Erreur de l'API (RFC 9457) : message affichable et erreurs par champ de formulaire. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly fields: Record<string, string> = {},
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** Le réseau n'a pas répondu : à distinguer d'un refus du serveur. */
export class NetworkError extends Error {
  constructor() {
    super("Pas de connexion. Réessayez dès le retour du réseau.");
    this.name = "NetworkError";
  }
}

export function csrfToken(): string | undefined {
  if (typeof document === "undefined") return undefined;
  return document.cookie
    .split("; ")
    .find((c) => c.startsWith("cf_csrf="))
    ?.split("=")[1];
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  ifMatch?: number;
  idempotencyKey?: string;
}

export async function api<T = unknown>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = {};
  const method = options.method ?? "GET";
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (method !== "GET") {
    const csrf = csrfToken();
    if (csrf) headers["X-CSRF-Token"] = csrf;
    // Une clé par action : un double clic ou un rejeu réseau n'a qu'un seul effet.
    headers["Idempotency-Key"] = options.idempotencyKey ?? crypto.randomUUID();
  }
  if (options.ifMatch !== undefined) headers["If-Match"] = String(options.ifMatch);

  let response: Response;
  try {
    response = await fetch(`/api/v1${path}`, {
      method,
      headers,
      credentials: "same-origin",
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    });
  } catch {
    throw new NetworkError();
  }
  if (response.ok) {
    return (response.status === 204 ? null : await response.json()) as T;
  }
  const problem = (await response.json().catch(() => ({}))) as { code?: string; detail?: string; fields?: Record<string, string> | null };
  throw new ApiError(response.status, problem.code ?? "unknown", problem.detail ?? "Une erreur est survenue.", problem.fields ?? {});
}

/**
 * Lecture avec repli : le réseau d'abord, sinon la dernière réponse connue. C'est ce qui permet de
 * remplir les formulaires de terrain (chantiers, catégories, tâches) sans réseau.
 */
export async function cachedGet<T>(path: string): Promise<{ data: T; stale: boolean }> {
  try {
    const data = await api<T>(path);
    await db.cache.put({ key: path, value: data, at: Date.now() });
    return { data, stale: false };
  } catch (err) {
    if (!(err instanceof NetworkError)) throw err;
    const cached = await db.cache.get(path);
    if (!cached) throw err;
    return { data: cached.value as T, stale: true };
  }
}

/** Toutes les pages d'une liste : les filtres et les compteurs des écrans portent sur l'ensemble. */
export async function fetchAll<T>(path: string): Promise<{ items: T[]; stale: boolean }> {
  const items: T[] = [];
  let cursor: string | null = null;
  let stale = false;
  const separator = path.includes("?") ? "&" : "?";
  do {
    const page: { data: Paged<T>; stale: boolean } = await cachedGet<Paged<T>>(`${path}${separator}limit=200${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`);
    items.push(...page.data.items);
    stale ||= page.stale;
    cursor = page.data.next_cursor;
  } while (cursor);
  return { items, stale };
}

export interface Paged<T> {
  items: T[];
  next_cursor: string | null;
}

export interface Items<T> {
  items: T[];
}

export interface User {
  id: string;
  email: string | null;
  phone: string | null;
  full_name: string;
  role: string;
  status: string;
}

export interface Organization {
  id: string;
  name: string;
  slug: string;
  legal_name: string | null;
  phone: string | null;
  email: string | null;
  address: string | null;
  city: string | null;
  activity_type: string | null;
  currency: string;
  timezone: string;
}

export interface Project {
  id: string;
  reference: string;
  name: string;
  client_name: string;
  address: string;
  city: string | null;
  description: string | null;
  status: string;
  start_date: string;
  planned_end_date: string | null;
  manager_id: string;
  current_budget_version_id: string | null;
  declared_progress_pct: number;
  progress_updated_at?: string | null;
  row_version: number;
  manager_name?: string;
  budget?: number;
  approved?: number;
}

export interface Directory {
  id: string;
  full_name: string;
  role: string;
}

export interface Member extends User {
  project_names: string[];
  last_login_at: string | null;
}

export interface PendingInvitation {
  id: string;
  user_id: string;
  full_name: string;
  email: string | null;
  phone: string | null;
  role: string;
  expires_at: string;
  project_ids?: string[];
}

export interface ExpenseDetail extends Expense {
  approvals: { id: number; event: string; actor_name: string; comment: string | null; created_at: string }[];
  payments: { id: string; amount: number; paid_on: string; method: string; reference: string | null; recorded_by_name: string }[];
  attachments: { id: string; original_name: string | null; kind: string; mime_type: string; status: string }[];
  duplicates: { id: string; description: string; amount: number; expense_date: string; status: string; created_by_name: string }[];
}

export interface ExpenseCounts {
  pending: number;
  pending_amount: number;
  approved: number;
  paid: number;
  rejected: number;
  draft: number;
  all: number;
}

export interface Supplier {
  id: string;
  name: string;
  phone: string | null;
  email: string | null;
  is_active: boolean;
  legal_name?: string | null;
  legal_form?: string | null;
  ninea?: string | null;
  rccm?: string | null;
  address?: string | null;
  city?: string | null;
  activities?: string[];
  notes?: string | null;
  archived_at?: string | null;
  created_at?: string;
  updated_at?: string;
}

export interface SupplierContact {
  id: string;
  supplier_id: string;
  full_name: string;
  job_title: string | null;
  phone: string | null;
  email: string | null;
  is_primary: boolean;
  notes: string | null;
}

export interface SupplierTerms {
  id: string;
  version: number;
  effective_from: string;
  payment_method: string | null;
  payment_delay_days: number | null;
  advance_pct: number | null;
  discount_pct: string | null;
  delivery_days: number | null;
  delivery_included: boolean | null;
  minimum_order_amount: number | null;
  credit_limit: number | null;
  notes: string | null;
  created_at: string;
  created_by_name?: string;
}

export interface SupplierDetail extends Supplier {
  contacts: SupplierContact[];
  terms: SupplierTerms | null;
}

export interface SupplierDuplicate {
  id: string;
  name: string;
  archived: boolean;
  blocking: boolean;
  reasons: string[];
}

export interface SupplierHistory {
  consultations: {
    rfq_id: string;
    number: number;
    title: string;
    status: string;
    project_id: string;
    project_name: string;
    consulted_at: string;
    offer_count: number;
    last_offer_total: number | null;
    last_offer_at: string | null;
  }[];
  purchases: {
    id: string;
    description: string;
    amount: number;
    amount_paid: number;
    expense_date: string;
    status: string;
    payment_status: string;
    project_id: string;
    project_name: string;
  }[];
  totals: { purchase_count: number; approved_amount: number; paid_amount: number };
  orders: { id: string; number: number; status: string; total_amount: number; created_at: string; expected_delivery_date: string | null; project_id: string; project_name: string }[];
}

/** Demandes de prix (E1). */
export type RFQStatus = "draft" | "published" | "closed" | "cancelled";

export interface RFQSummary {
  id: string;
  project_id: string;
  project_name: string;
  number: number;
  title: string;
  status: RFQStatus;
  response_deadline: string | null;
  delivery_date: string | null;
  created_at: string;
  published_at: string | null;
  item_count: number;
  supplier_count: number;
  responded_count: number;
}

export interface RFQItem {
  id: string;
  position: number;
  kind: "material" | "service";
  material_id: string | null;
  label: string;
  quantity: string;
  unit: string;
  specifications: string | null;
  needed_by: string | null;
}

export interface RFQSupplier {
  id: string;
  supplier_id: string;
  supplier_name: string;
  supplier_phone: string | null;
  supplier_email: string | null;
  link_issued_at: string | null;
  offer_count: number;
}

export interface OfferLine {
  rfq_item_id: string;
  quantity: string;
  unit_price: number;
  amount: number;
  comment: string | null;
}

export interface Offer {
  id: string;
  rfq_supplier_id: string;
  supplier_id: string;
  supplier_name?: string;
  version: number;
  total_amount: number;
  valid_until: string | null;
  delivery_days: number | null;
  payment_terms: string | null;
  notes: string | null;
  contact_name: string | null;
  submitted_at: string;
  lines: OfferLine[];
}

export interface RFQDetail {
  id: string;
  project_id: string;
  project_name: string;
  project_address: string;
  project_city: string | null;
  number: number;
  reference: string;
  title: string;
  notes: string | null;
  response_deadline: string | null;
  delivery_date: string | null;
  status: RFQStatus;
  open: boolean;
  cancel_reason: string | null;
  created_by_name: string;
  created_at: string;
  published_at: string | null;
  closed_at: string | null;
  cancelled_at: string | null;
  items: RFQItem[];
  suppliers: RFQSupplier[];
  offers: Offer[];
  weight_price: number;
  weight_delivery: number;
  weight_coverage: number;
  awarded_offer_id: string | null;
  awarded_at: string | null;
  award_reason: string | null;
  evaluation?: Evaluation;
  order?: { id: string; reference: string; status: string };
}

export interface EvaluatedOffer {
  offer_id: string;
  supplier_id: string;
  supplier_name: string;
  version: number;
  total_amount: number;
  quoted_lines: number;
  total_lines: number;
  delivery_days: number | null;
  valid_until: string | null;
  expired: boolean;
  payment_terms: string | null;
  price_score: number;
  delivery_score: number;
  coverage_score: number;
  score: number;
  rank: number;
  selected: boolean;
}

export interface Evaluation {
  weight_price: number;
  weight_delivery: number;
  weight_coverage: number;
  offers: EvaluatedOffer[];
}

export type POStatus = "draft" | "submitted" | "approved" | "sent" | "partially_received" | "received" | "cancelled";

export interface POLine {
  id: string;
  position: number;
  rfq_item_id: string | null;
  material_id: string | null;
  label: string;
  unit: string;
  quantity_ordered: string;
  quantity_received: string;
  quantity_remaining: string;
  unit_price: number | null;
  tax_rate: string | null;
  amount: number | null;
  tax_amount: number | null;
  comment: string | null;
}

export interface POReceipt {
  id: string;
  received_on: string;
  delivery_note_ref: string | null;
  notes: string | null;
  stock_movement_id: string | null;
  received_by_name: string;
  created_at: string;
  lines: { line_id: string; quantity: string }[];
}

export interface POEvent {
  id: number;
  event: string;
  from_status: string | null;
  to_status: string;
  actor_name: string;
  comment: string | null;
  changes: unknown;
  created_at: string;
}

export interface PurchaseOrder {
  id: string;
  number: number;
  reference: string;
  status: POStatus;
  project_id: string;
  project_name: string;
  supplier_id: string;
  supplier_name: string;
  supplier_email: string | null;
  supplier_phone: string | null;
  rfq_id: string | null;
  rfq_reference: string | null;
  expected_delivery_date: string | null;
  delivery_address: string | null;
  payment_terms: string | null;
  notes: string | null;
  subtotal_amount: number | null;
  tax_amount: number | null;
  total_amount: number | null;
  created_by: string;
  created_by_name: string;
  created_at: string;
  approved_at: string | null;
  sent_at: string | null;
  cancel_reason: string | null;
  lines: POLine[];
  receipts: POReceipt[];
  events: POEvent[];
  prices_visible: boolean;
}

export interface POSummary {
  id: string;
  number: number;
  reference: string;
  status: POStatus;
  project_id: string;
  project_name: string;
  supplier_id: string;
  supplier_name: string;
  rfq_id: string | null;
  total_amount: number | null;
  expected_delivery_date: string | null;
  created_at: string;
  line_count: number;
  lines_received: number;
}

/** Lien de consultation d'un fournisseur : visible au moment où il est émis, seulement. */
export interface SupplierLink {
  supplier_id: string;
  supplier_name: string;
  phone: string | null;
  email: string | null;
  url: string;
  emailed: boolean;
}

export interface WithLinks {
  rfq: RFQDetail;
  links: SupplierLink[];
}

/** Ce que voit le fournisseur avec son lien. */
export interface SupplierPortal {
  organization_name: string;
  supplier_name: string;
  reference: string;
  title: string;
  notes: string | null;
  status: RFQStatus;
  open: boolean;
  cancel_reason: string | null;
  response_deadline: string | null;
  delivery_date: string | null;
  delivery_place: string;
  items: Omit<RFQItem, "material_id">[];
  offers: Offer[];
}

export interface Attachment {
  id: string;
  kind: string;
  mime_type: string;
  size_bytes: number;
  original_name: string | null;
  created_at: string;
  uploaded_by_name?: string;
}

export interface HistoryEntry {
  id: string;
  action: string;
  entity_type: string;
  before: Record<string, unknown> | null;
  after: Record<string, unknown> | null;
  created_at: string;
  actor_name: string | null;
}

export interface Overview {
  projects: ProjectHealth[];
  totals: Figures;
  paid_by_category: { name: string; paid: number }[];
  week_days: string[];
  week_projects: { project_id: string; name: string; reported: string[] }[];
  latest_reports: { id: string; project_id: string; report_date: string; work_done: string; workforce_count: number | null; issues: string | null; project_name: string; author_name: string; photo_count: number }[];
  stock: { project_id: string; project_name: string; material_id: string; name: string; unit: string; on_hand: string; is_low: boolean }[];
}

export interface Category {
  id: string;
  name: string;
  is_active: boolean;
  system_code: string | null;
}

export interface Expense {
  id: string;
  project_id: string;
  category_id: string;
  description: string;
  amount: number;
  amount_paid: number;
  expense_date: string;
  status: string;
  payment_status: string;
  has_receipt: boolean;
  created_by: string;
  decision_comment: string | null;
  row_version: number;
  project_name?: string;
  category_name?: string;
  supplier_name?: string | null;
  created_by_name?: string;
  submitted_at?: string | null;
  possible_duplicate?: boolean;
  comment?: string | null;
  declared_paid_on?: string | null;
  declared_payment_method?: string | null;
}

export interface ProjectDetail extends ProjectHealth {
  categories: (Figures & { category_id: string; name: string })[];
}

export interface Figures {
  budget: number;
  approved: number;
  pending: number;
  paid: number;
  unpaid: number;
  remaining: number;
}

export interface ProjectHealth extends Figures {
  project_id: string;
  reference: string;
  name: string;
  status: string;
  manager_name: string;
  declared_progress_pct: number;
  signal: "none" | "good" | "watch" | "critical";
  budget_version_no: number | null;
  consumed_pct: number | null;
  health: "no_budget" | "ok" | "warning" | "over";
  threshold_reached: number | null;
  overrun: number;
}

export interface Dashboard {
  totals: Figures;
  project_count: number;
  over_budget_count: number;
  projects: ProjectHealth[];
  period: string;
  period_paid: number;
  paid_by_month: { month: string; paid: number }[];
  to_process: {
    pending_count: number;
    pending_amount: number;
    duplicate_count: number;
    missing_reports: { id: string; name: string }[];
    missing_for: string;
  };
}

export interface BudgetVersion {
  id: string;
  version_no: number;
  status: string;
  total_amount: number;
  published_at: string | null;
  published_by_name?: string | null;
  revision_reason?: string | null;
}

export interface BudgetLine {
  id: string;
  category_id: string;
  category_name: string;
  label: string;
  quantity: string;
  unit: string;
  unit_price: number;
  amount: number;
}

export interface BudgetDetail extends BudgetVersion {
  lines: BudgetLine[];
}

export interface Task {
  id: string;
  title: string;
  priority: string;
  status: string;
  progress_pct: number;
  due_date: string | null;
  assignee_ids: string[];
}

export interface DailyReport {
  id: string;
  project_id: string;
  report_date: string;
  team_label: string;
  work_done: string;
  workforce_count: number | null;
  issues: string | null;
  weather: string | null;
  status: string;
  row_version: number;
}

export interface Material {
  id: string;
  name: string;
  unit: string;
  sku: string | null;
  archived_at: string | null;
  low_stock_threshold: string | null;
}

export interface StockRow {
  material_id: string;
  name: string;
  unit: string;
  sku?: string | null;
  on_hand: string;
  last_movement_on: string | null;
  low_stock_threshold: string | null;
  is_low: boolean;
  qty_in: string;
  qty_out: string;
  qty_adjust: string;
  last_movement_type: string | null;
}

export interface StockView {
  items: StockRow[];
  low_count: number;
  movements_this_month: number;
}

export interface StockHistoryRow {
  movement_id: string;
  movement_type: string;
  occurred_on: string;
  reason: string | null;
  material_name: string;
  unit: string;
  quantity: string;
  author_name: string;
}

export interface ProjectMember {
  user_id: string;
  full_name: string;
  role: string;
  status?: string;
  can_approve: boolean;
  approval_limit: number | null;
}

export interface Phase {
  id: string;
  name: string;
  position: number;
  status: string;
}

export interface StatusHistory {
  id: number;
  from_status: string | null;
  to_status: string;
  actor_name: string;
  comment: string | null;
  created_at: string;
}

export interface Comparison {
  from_version_no: number;
  to_version_no: number;
  from_total: number;
  to_total: number;
  delta: number;
  lines: { line_key: string; label: string; category: string; change: string; from_amount: number; to_amount: number; delta: number }[];
}
