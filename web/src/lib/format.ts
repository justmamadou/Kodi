/** Formats de l'application : fr-SN, FCFA sans décimale, dates JJ/MM/AAAA. */
export function fcfa(amount: number): string {
  return new Intl.NumberFormat("fr-FR", { maximumFractionDigits: 0 }).format(amount).replace(/[  ]/g, " ");
}

/** Montant abrégé des grands indicateurs : « 57,9 M ». */
export function fcfaShort(amount: number): string {
  const abs = Math.abs(amount);
  const sign = amount < 0 ? "-" : "";
  if (abs >= 1_000_000) return `${sign}${(abs / 1_000_000).toFixed(1).replace(".", ",").replace(",0", "")} M`;
  if (abs >= 1_000) return `${sign}${Math.round(abs / 1_000)} k`;
  return `${amount}`;
}

/** Quantité de stock : « 1 400 », « 2,5 ». */
export function qty(value: string | number): string {
  return new Intl.NumberFormat("fr-FR", { maximumFractionDigits: 3 }).format(Number(value)).replace(/[\u202f\u00a0]/g, " ");
}

/** Part entière d'un montant dans un autre, en pour cent. */
export function pct(part: number, total: number): number {
  return total > 0 ? Math.round((part * 100) / total) : 0;
}

export function date(value?: string | null): string {
  if (!value) return "—";
  const [y, m, d] = value.slice(0, 10).split("-");
  return y && m && d ? `${d}/${m}/${y}` : value;
}

/** Date courte « 03/11 ». */
export function dayMonth(value?: string | null): string {
  if (!value) return "—";
  const [, m, d] = value.slice(0, 10).split("-");
  return m && d ? `${d}/${m}` : value;
}

/** Date courte à deux chiffres d'année « 03/03/26 ». */
export function dateShort(value?: string | null): string {
  if (!value) return "—";
  const [y, m, d] = value.slice(0, 10).split("-");
  return y && m && d ? `${d}/${m}/${y.slice(2)}` : value;
}

export function dateTime(value?: string | null): string {
  if (!value) return "—";
  return new Date(value).toLocaleString("fr-FR", { timeZone: "Africa/Dakar", dateStyle: "short", timeStyle: "short" });
}

/** « Jeudi 5 novembre » */
export function longDate(now = new Date()): string {
  const text = now.toLocaleDateString("fr-FR", { timeZone: "Africa/Dakar", weekday: "long", day: "numeric", month: "long" });
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** « Jeudi 5 nov. » */
export function mediumDate(now = new Date()): string {
  const text = now.toLocaleDateString("fr-FR", { timeZone: "Africa/Dakar", weekday: "long", day: "numeric", month: "short" });
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** Mois « Juil. » depuis « 2026-07 ». */
export function monthShort(month: string): string {
  const text = new Date(`${month}-15T00:00:00Z`).toLocaleDateString("fr-FR", { timeZone: "UTC", month: "short" });
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** Mois long « Octobre » depuis « 2026-10 ». */
export function monthLong(month: string): string {
  const text = new Date(`${month}-15T00:00:00Z`).toLocaleDateString("fr-FR", { timeZone: "UTC", month: "long" });
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** « Il y a 20 min », « Hier, 18:12 ». */
export function ago(value?: string | null, now = Date.now()): string {
  if (!value) return "Jamais connecté";
  const minutes = Math.floor((now - new Date(value).getTime()) / 60_000);
  if (minutes < 2) return "Maintenant";
  if (minutes < 60) return `Il y a ${minutes} min`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `Il y a ${hours} h`;
  if (hours < 48) return `Hier, ${new Date(value).toLocaleTimeString("fr-FR", { timeZone: "Africa/Dakar", hour: "2-digit", minute: "2-digit" })}`;
  return `Le ${date(value)}`;
}

/** Date du jour dans le fuseau de l'entreprise, au format AAAA-MM-JJ. */
export function today(): string {
  return new Date().toLocaleDateString("en-CA", { timeZone: "Africa/Dakar" });
}

/** Dernier jour du mois « 2026-11 ». */
export function monthEnd(month: string): string {
  const [y, m] = month.split("-").map(Number);
  return new Date(Date.UTC(y ?? 2026, m ?? 1, 0)).toISOString().slice(0, 10);
}

/** « Novembre 2026 » depuis « 2026-11 ». */
export function monthYear(month: string): string {
  const text = new Date(`${month}-15T00:00:00Z`).toLocaleDateString("fr-FR", { timeZone: "UTC", month: "long", year: "numeric" });
  return text.charAt(0).toUpperCase() + text.slice(1);
}

/** Abrège un nom : « Ahmed Diallo » → « A. Diallo ». */
export function shortName(name?: string | null): string {
  if (!name) return "—";
  const parts = name.trim().split(/\s+/);
  const first = parts[0];
  return parts.length > 1 && first ? `${first[0]}. ${parts.slice(1).join(" ")}` : name;
}

export const ROLE_LABEL: Record<string, string> = {
  owner: "Owner / Admin",
  manager: "Conducteur",
  accountant: "Finance",
  site_worker: "Chef de chantier",
  viewer: "Lecture seule",
};

export const PROJECT_STATUS_LABEL: Record<string, string> = {
  draft: "Brouillon",
  planned: "Planifié",
  in_progress: "En cours",
  completed: "Terminé",
  archived: "Archivé",
};

export const EXPENSE_STATUS_LABEL: Record<string, string> = {
  draft: "Brouillon",
  pending: "À valider",
  approved: "Approuvée",
  rejected: "Rejetée",
  cancelled: "Annulée",
};

export const PAYMENT_STATUS_LABEL: Record<string, string> = {
  unpaid: "non payée",
  partially_paid: "paiement partiel",
  paid: "payée",
};

export const TASK_STATUS_LABEL: Record<string, string> = {
  todo: "À faire",
  in_progress: "En cours",
  blocked: "Bloquée",
  done: "Terminée",
  cancelled: "Annulée",
};

export const PRIORITY_LABEL: Record<string, string> = { low: "Basse", medium: "Normale", high: "Haute", urgent: "Urgente" };

export const WEATHER_LABEL: Record<string, string> = { sunny: "Ensoleillé", cloudy: "Nuageux", rain: "Pluie", strong_wind: "Vent fort" };

/** Moyens de paiement proposés à la saisie, dans l'ordre de la maquette. */
export const METHOD_LABEL: Record<string, string> = {
  cash: "Espèces",
  wave: "Wave",
  orange_money: "Orange Money",
  bank_transfer: "Virement",
  cheque: "Chèque",
  free_money: "Free Money",
  other: "Autre",
};

export const MOVEMENT_LABEL: Record<string, string> = { in: "Entrée", out: "Sortie", adjustment: "Ajustement" };

export const UNIT_LABEL: Record<string, string> = {
  sac: "sac",
  barre: "barre",
  m3: "m³",
  m2: "m²",
  ml: "ml",
  unite: "unité",
  kg: "kg",
  tonne: "tonne",
  litre: "litre",
  rouleau: "rouleau",
  boite: "boîte",
};

/** Unités propres aux prestations d'une demande de prix. */
export const SERVICE_UNIT_LABEL: Record<string, string> = { forfait: "forfait", jour: "jour", heure: "heure" };

export function unitLabel(unit: string): string {
  return UNIT_LABEL[unit] ?? SERVICE_UNIT_LABEL[unit] ?? unit;
}

export const PO_STATUS_LABEL: Record<string, string> = {
  draft: "Brouillon",
  submitted: "À approuver",
  approved: "Approuvée",
  sent: "Envoyée",
  partially_received: "Reçue en partie",
  received: "Reçue",
  cancelled: "Annulée",
};

export const PO_EVENT_LABEL: Record<string, string> = {
  created: "Créée depuis l'offre retenue",
  updated: "Modifiée",
  submitted: "Soumise à l'approbation",
  returned: "Renvoyée en brouillon",
  approved: "Approuvée",
  sent: "Envoyée au fournisseur",
  received: "Réception enregistrée",
  cancelled: "Annulée",
};

/** Numéro affiché d'un bon de commande : BC-0007. */
export function poRef(number: number): string {
  return `BC-${String(number).padStart(4, "0")}`;
}

export const RFQ_STATUS_LABEL: Record<string, string> = {
  draft: "Brouillon",
  published: "Publiée",
  closed: "Clôturée",
  cancelled: "Annulée",
};

/** Numéro affiché d'une demande de prix : DP-0007. */
export function rfqRef(number: number): string {
  return `DP-${String(number).padStart(4, "0")}`;
}

export const LEGAL_FORM_LABEL: Record<string, string> = {
  ei: "Entreprise individuelle",
  suarl: "SUARL",
  sarl: "SARL",
  sa: "SA",
  sas: "SAS",
  gie: "GIE",
  other: "Autre",
};

/** Ce que fournit un fournisseur. */
export const SUPPLIER_ACTIVITY_LABEL: Record<string, string> = {
  materials: "Matériaux",
  equipment_rental: "Location de matériel",
  subcontracting: "Sous-traitance",
  transport: "Transport",
  services: "Services",
  other: "Autre",
};

export const DUPLICATE_REASON_LABEL: Record<string, string> = {
  same_name: "même nom",
  same_ninea: "même NINEA",
  same_rccm: "même RCCM",
  same_phone: "même téléphone",
  same_email: "même email",
  similar_name: "nom très proche",
};

export const ACTIVITY_LABEL: Record<string, string> = {
  villas: "Construction de villas et maisons",
  gros_oeuvre: "Gros œuvre",
  second_oeuvre: "Second œuvre",
  renovation: "Rénovation et aménagement",
  general: "Tous corps d'état",
};

export const CITIES = ["Dakar", "Rufisque", "Thiès", "Mbour / Saly", "Autre"];
