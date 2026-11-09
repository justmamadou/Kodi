"use client";

import Link from "next/link";
import type { ButtonHTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from "react";
import { useId } from "react";
import { ApiError } from "@/lib/api";
import { IconAlertCircle, IconSearch } from "./icons";

export function Card({ children, className = "", style, ...rest }: { children: ReactNode; className?: string; style?: React.CSSProperties; "aria-labelledby"?: string; "aria-label"?: string; id?: string }) {
  return (
    <section className={`card ${className}`} style={style} {...rest}>
      {children}
    </section>
  );
}

export function PageTitle({ title, subtitle, action }: { title: string; subtitle?: ReactNode; action?: ReactNode }) {
  return (
    <div className="wrap" style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
      <div style={{ minWidth: 0 }}>
        <h1 className="page-h1">{title}</h1>
        {subtitle && (
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
            {subtitle}
          </p>
        )}
      </div>
      {action}
    </div>
  );
}

type Variant = "primary" | "secondary" | "danger" | "ghost";
const VARIANTS: Record<Variant, string> = { primary: "btn-p", secondary: "btn-s", danger: "btn-d", ghost: "btn-g" };

export function Button({ variant = "primary", className = "", ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant }) {
  return <button type="button" {...props} className={`btn ${VARIANTS[variant]} ${className}`} />;
}

export function LinkButton({ href, variant = "primary", children, download }: { href: string; variant?: Variant; children: ReactNode; download?: boolean }) {
  return download ? (
    <a href={href} className={`btn ${VARIANTS[variant]}`}>
      {children}
    </a>
  ) : (
    <Link href={href} className={`btn ${VARIANTS[variant]}`}>
      {children}
    </Link>
  );
}

interface FieldShell {
  label: ReactNode;
  error?: string;
  hint?: ReactNode;
  required?: boolean;
  /** Affiche le label sous forme accessible seulement (filtres de barre d'outils). */
  hiddenLabel?: boolean;
}

function Shell({ label, error, hint, required, id, children, hiddenLabel }: FieldShell & { id: string; children: ReactNode }) {
  return (
    <div className="f">
      <label htmlFor={id} className={hiddenLabel ? "sr-only" : undefined}>
        {label}
        {required && <span className="req"> *</span>}
      </label>
      {children}
      {hint && !error && <span className="hint">{hint}</span>}
      {error && (
        <span id={`${id}-error`} role="alert" className="field-err">
          {error}
        </span>
      )}
    </div>
  );
}

export function Input({ label, error, hint, required, hiddenLabel, className = "", ...props }: FieldShell & InputHTMLAttributes<HTMLInputElement>) {
  const id = useId();
  return (
    <Shell id={id} label={label} error={error} hint={hint} required={required} hiddenLabel={hiddenLabel}>
      <input id={id} aria-required={required} aria-invalid={error ? true : undefined} aria-describedby={error ? `${id}-error` : undefined} {...props} className={`inp ${className}`} />
    </Shell>
  );
}

export function Textarea({ label, error, hint, required, hiddenLabel, ...props }: FieldShell & TextareaHTMLAttributes<HTMLTextAreaElement>) {
  const id = useId();
  return (
    <Shell id={id} label={label} error={error} hint={hint} required={required} hiddenLabel={hiddenLabel}>
      <textarea id={id} aria-required={required} aria-invalid={error ? true : undefined} aria-describedby={error ? `${id}-error` : undefined} {...props} className="inp" />
    </Shell>
  );
}

export function Select({ label, error, hint, required, hiddenLabel, children, ...props }: FieldShell & SelectHTMLAttributes<HTMLSelectElement>) {
  const id = useId();
  return (
    <Shell id={id} label={label} error={error} hint={hint} required={required} hiddenLabel={hiddenLabel}>
      <select id={id} aria-required={required} aria-invalid={error ? true : undefined} aria-describedby={error ? `${id}-error` : undefined} {...props} className="inp">
        {children}
      </select>
    </Shell>
  );
}

export function SearchBox({ label, placeholder, value, onChange, width }: { label: string; placeholder: string; value: string; onChange: (value: string) => void; width?: number }) {
  return (
    <label className="search" style={width ? { width } : undefined}>
      <IconSearch />
      <input type="search" aria-label={label} placeholder={placeholder} value={value} onChange={(e) => onChange(e.target.value)} />
    </label>
  );
}

export type Tone = "neutral" | "blue" | "green" | "orange" | "red";
const TONES: Record<Tone, string> = { neutral: "mut", blue: "info", green: "ok", orange: "warn", red: "err" };

export function Badge({ tone = "neutral", children }: { tone?: Tone; children: ReactNode }) {
  return <span className={`bdg ${TONES[tone]}`}>{children}</span>;
}

/** Fonction prévue par les maquettes mais pas encore livrée : affichée telle quelle, jamais simulée. */
export function Soon() {
  return <span className="bdg mut">Bientôt</span>;
}

export const EXPENSE_TONE: Record<string, Tone> = { draft: "neutral", pending: "orange", approved: "green", rejected: "red", cancelled: "neutral" };
export const PO_TONE: Record<string, Tone> = { draft: "neutral", submitted: "orange", approved: "blue", sent: "blue", partially_received: "orange", received: "green", cancelled: "red" };
export const RFQ_TONE: Record<string, Tone> = { draft: "neutral", published: "blue", closed: "green", cancelled: "red" };
export const PROJECT_TONE: Record<string, Tone> = { draft: "neutral", planned: "neutral", in_progress: "blue", completed: "green", archived: "neutral" };

/** Message d'erreur d'une action. Les erreurs de champ s'affichent sous chaque champ. */
export function ErrorBanner({ error }: { error: unknown }) {
  if (!error) return null;
  const message = error instanceof Error ? error.message : "Une erreur est survenue.";
  return (
    <div role="alert" style={{ display: "flex", gap: 10, padding: "12px 14px", borderRadius: 10, background: "#FDECEA", color: "#B3261E", fontSize: 14 }}>
      <IconAlertCircle size={18} />
      <span>{message}</span>
    </div>
  );
}

export function fieldError(error: unknown, field: string): string | undefined {
  return error instanceof ApiError ? error.fields[field] : undefined;
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <p className="muted" style={{ padding: "32px 16px", textAlign: "center", fontSize: 14, margin: 0 }}>
      {children}
    </p>
  );
}

export function Loading() {
  return (
    <p role="status" className="muted" style={{ padding: "32px 16px", textAlign: "center", fontSize: 14, margin: 0 }}>
      Chargement…
    </p>
  );
}

/** Anneau d'avancement des maquettes 03 et 08. */
export function Ring({ pct, color, size = 52, stroke = 6, track = "#F1F1F4", text, fontSize = 12 }: { pct: number; color: string; size?: number; stroke?: number; track?: string; text?: string; fontSize?: number }) {
  const r = (size - stroke - 4) / 2;
  const c = 2 * Math.PI * r;
  const center = size / 2;
  return (
    <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} role="img" aria-label={`Avancement ${pct} %`} style={{ flex: "none" }}>
      <circle cx={center} cy={center} r={r} fill="none" stroke={track} strokeWidth={stroke} />
      {pct > 0 && <circle cx={center} cy={center} r={r} fill="none" stroke={color} strokeWidth={stroke} strokeLinecap="round" strokeDasharray={`${(Math.min(100, pct) / 100) * c} ${c}`} transform={`rotate(-90 ${center} ${center})`} />}
      <text x={center} y={center + fontSize / 3} textAnchor="middle" fontSize={fontSize} fontWeight={700} fill="#1D1D1F">
        {text ?? pct}
      </text>
    </svg>
  );
}

/** Pastille aux initiales. */
export function Avatar({ name, background = "#FFC21A", color = "#1D1D1F", size = 32 }: { name: string; background?: string; color?: string; size?: number }) {
  const initials = name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase())
    .join("");
  return (
    <span aria-hidden style={{ width: size, height: size, borderRadius: "50%", background, color, fontWeight: 700, fontSize: 12, display: "flex", alignItems: "center", justifyContent: "center", flex: "none" }}>
      {initials}
    </span>
  );
}

export function Banner({ tone = "info", icon, children }: { tone?: "info" | "warn" | "ok" | "err"; icon?: ReactNode; children: ReactNode }) {
  return (
    <div className={tone} style={{ display: "flex", gap: 10, padding: "12px 14px", borderRadius: 10, fontSize: 14, alignItems: "flex-start" }}>
      {icon}
      <div style={{ minWidth: 0 }}>{children}</div>
    </div>
  );
}
