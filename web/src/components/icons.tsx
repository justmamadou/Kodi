import type { ReactNode } from "react";

/** Icônes reprises à l'identique des maquettes. */
function I({ size = 16, width = 1.9, children, label }: { size?: number; width?: number; children: ReactNode; label?: string }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={width} strokeLinecap="round" strokeLinejoin="round" aria-hidden={label ? undefined : true} aria-label={label} style={{ flex: "none" }}>
      {children}
    </svg>
  );
}

type P = { size?: number; width?: number };

export const IconHome = (p: P) => <I {...p}><path d="M3 11l9-8 9 8" /><path d="M5 10v10h14V10" /></I>;
export const IconBuilding = (p: P) => <I {...p}><rect x="4" y="3" width="16" height="18" rx="2" /><path d="M9 7h2M13 7h2M9 11h2M13 11h2M10 21v-4h4v4" /></I>;
export const IconCard = (p: P) => <I {...p}><rect x="3" y="6" width="18" height="14" rx="2" /><path d="M3 10h18M16 15h2" /></I>;
export const IconBox = (p: P) => <I {...p}><path d="M21 8l-9-5-9 5 9 5 9-5z" /><path d="M3 8v8l9 5 9-5V8M12 13v8" /></I>;
export const IconBars = (p: P) => <I {...p}><path d="M4 20V10M10 20V4M16 20v-7" /></I>;
export const IconUsers = (p: P) => <I {...p}><circle cx="9" cy="8" r="3.5" /><path d="M2.5 20a6.5 6.5 0 0 1 13 0M16 4.5a3.5 3.5 0 0 1 0 7M21.5 20a6 6 0 0 0-4-5.6" /></I>;
export const IconTruck = (p: P) => <I {...p}><path d="M2 6h12v10H2zM14 10h4l3 3v3h-7" /><circle cx="6" cy="18" r="2" /><circle cx="17" cy="18" r="2" /></I>;
export const IconGear = (p: P) => <I {...p}><circle cx="12" cy="12" r="3" /><path d="M12 2v3M12 19v3M2 12h3M19 12h3M4.9 4.9l2.1 2.1M17 17l2.1 2.1M4.9 19.1L7 17M17 7l2.1-2.1" /></I>;
export const IconHelmet = (p: P) => <I {...p}><path d="M2 18h20M4 18v-2a8 8 0 0 1 16 0v2M10 8V5h4v3" /></I>;
export const IconPlus = (p: P) => <I width={2.4} {...p}><path d="M12 5v14M5 12h14" /></I>;
export const IconCheck = (p: P) => <I width={2.2} {...p}><path d="M5 12l5 5 9-10" /></I>;
export const IconChevron = (p: P) => <I width={2.4} {...p}><path d="M9 6l6 6-6 6" /></I>;
export const IconBack = (p: P) => <I width={2.2} size={22} {...p}><path d="M15 6l-6 6 6 6" /></I>;
export const IconDown = (p: P) => <I width={2.6} {...p}><path d="M6 9l6 6 6-6" /></I>;
export const IconDownload = (p: P) => <I width={2} {...p}><path d="M12 4v11M7 10l5 5 5-5M4 20h16" /></I>;
export const IconDoc = (p: P) => <I width={2} {...p}><path d="M14 3H6v18h12V7z" /><path d="M14 3v4h4" /></I>;
export const IconDocLines = (p: P) => <I width={2} {...p}><path d="M14 3H6v18h12V7z" /><path d="M14 3v4h4M9 13h6M9 17h6" /></I>;
export const IconAlert = (p: P) => <I width={2.2} {...p}><path d="M12 7v6M12 17h.01" /></I>;
export const IconAlertCircle = (p: P) => <I width={2} {...p}><circle cx="12" cy="12" r="9" /><path d="M12 8v5M12 16h.01" /></I>;
export const IconInfo = (p: P) => <I width={2} {...p}><circle cx="12" cy="12" r="9" /><path d="M12 11v5M12 8h.01" /></I>;
export const IconWarn = (p: P) => <I width={2} {...p}><path d="M12 3l10 18H2z" /><path d="M12 10v5M12 18h.01" /></I>;
export const IconCopy = (p: P) => <I width={2} {...p}><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V4H4v12h4" /></I>;
export const IconCamera = (p: P) => <I width={2} {...p}><path d="M3 8h4l2-3h6l2 3h4v12H3z" /><circle cx="12" cy="13.5" r="3.5" /></I>;
export const IconImage = (p: P) => <I width={2} {...p}><rect x="3" y="5" width="18" height="14" rx="2" /><path d="M3 16l5-5 4 4 3-3 6 6" /></I>;
export const IconOffline = (p: P) => <I width={2.2} {...p}><path d="M4 4l16 16" /><path d="M7 18h10a4 4 0 0 0 1.5-7.7A6 6 0 0 0 9 7.3" /></I>;
export const IconClose = (p: P) => <I width={2.4} {...p}><path d="M6 6l12 12M18 6L6 18" /></I>;
export const IconSearch = (p: P) => <I width={2} {...p}><circle cx="11" cy="11" r="7" /><path d="M20 20l-4-4" /></I>;
export const IconArrowDown = (p: P) => <I width={2.4} {...p}><path d="M12 5v14M5 12l7 7 7-7" /></I>;
export const IconArrowUp = (p: P) => <I width={2.4} {...p}><path d="M12 19V5M5 12l7-7 7 7" /></I>;
export const IconAdjust = (p: P) => <I width={2.2} {...p}><path d="M4 8h14l-3-3M20 16H6l3 3" /></I>;
export const IconBell = (p: P) => <I {...p}><path d="M6 16v-5a6 6 0 0 1 12 0v5l2 2H4z" /><path d="M10 21h4" /></I>;
export const IconUser = (p: P) => <I {...p}><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></I>;
