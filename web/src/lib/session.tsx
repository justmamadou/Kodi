"use client";

import { createContext, useContext } from "react";
import type { Organization, User } from "./api";

export interface Session {
  user: User;
  organization: Organization;
}

export const SessionContext = createContext<Session | null>(null);

export function useSession(): Session {
  const session = useContext(SessionContext);
  if (!session) throw new Error("useSession hors de l'application authentifiée");
  return session;
}

/**
 * Masque ce qu'un rôle ne peut pas faire. C'est du confort d'interface : le serveur reste seul
 * juge et refuse de toute façon (voir internal/rbac).
 */
export const can = {
  manageOrg: (r: string) => r === "owner",
  manageMembers: (r: string) => r === "owner",
  manageProjects: (r: string) => r === "owner" || r === "manager",
  editBudget: (r: string) => r === "owner" || r === "manager",
  viewBudget: (r: string) => r !== "site_worker",
  createExpense: (r: string) => r !== "viewer",
  approve: (r: string) => r === "owner" || r === "accountant" || r === "manager",
  pay: (r: string) => r === "owner" || r === "accountant",
  viewFinance: (r: string) => r === "owner" || r === "accountant" || r === "manager",
  exportData: (r: string) => r === "owner" || r === "accountant",
  manageCatalog: (r: string) => r === "owner" || r === "manager",
  moveStock: (r: string) => r === "owner" || r === "manager" || r === "site_worker",
  manageTasks: (r: string) => r === "owner" || r === "manager",
  writeReport: (r: string) => r === "owner" || r === "manager" || r === "site_worker",
  viewRFQ: (r: string) => r !== "site_worker",
  createSupplier: (r: string) => r !== "viewer",
  awardRFQ: (r: string) => r === "owner" || r === "manager",
  managePO: (r: string) => r === "owner" || r === "manager",
  approvePO: (r: string) => r === "owner" || r === "accountant",
  receivePO: (r: string) => r === "owner" || r === "manager" || r === "site_worker",
  editSupplier: (r: string) => r === "owner" || r === "manager" || r === "accountant",
  supplierTrade: (r: string) => r !== "site_worker",
  manageRFQ: (r: string) => r === "owner" || r === "manager",
};
