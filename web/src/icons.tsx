import { CSSProperties, ReactNode } from "react";

// A small, dependency-free icon set (Lucide/Feather-style strokes). Every icon
// shares one 24×24 viewBox and draws with `currentColor`, so an icon inherits
// the text color of wherever it's placed and works in both themes. Prefer these
// over emoji/unicode glyphs for anything chrome-level (nav, buttons, badges).

export type IconName =
  | "dashboard" | "target" | "history" | "dumbbell"  // student nav
  | "overview" | "user" | "tests" | "assign" | "bank"  // teacher nav
  | "moon" | "sun" | "logout" | "logo"        // shell chrome
  | "flame" | "paperclip" | "bot" | "image" | "check"  // inline markers
  | "close" | "arrowRight" | "arrowLeft" | "chevronDown" | "trash" | "pencil"  // affordances
  | "bell"   // notifications
  | "download"  // PDF export
  | "eye" | "eyeOff" | "key";  // password visibility + reset

const PATHS: Record<IconName, ReactNode> = {
  dashboard: (
    <>
      <rect x="3" y="3" width="7.5" height="7.5" rx="1.6" />
      <rect x="13.5" y="3" width="7.5" height="7.5" rx="1.6" />
      <rect x="3" y="13.5" width="7.5" height="7.5" rx="1.6" />
      <rect x="13.5" y="13.5" width="7.5" height="7.5" rx="1.6" />
    </>
  ),
  target: (
    <>
      <circle cx="12" cy="12" r="9" />
      <circle cx="12" cy="12" r="5" />
      <circle cx="12" cy="12" r="1.4" />
    </>
  ),
  history: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7.5V12l3 1.8" />
    </>
  ),
  dumbbell: (
    <>
      <path d="M7.5 8v8M16.5 8v8" />
      <rect x="4" y="9.5" width="3.5" height="5" rx="1" />
      <rect x="16.5" y="9.5" width="3.5" height="5" rx="1" />
      <path d="M7.5 12h9" />
    </>
  ),
  overview: <path d="M3 12h4l2.5 7 5-14 2.5 7H21" />,
  user: (
    <>
      <circle cx="12" cy="8" r="4" />
      <path d="M5 20c0-3.87 3.13-6.5 7-6.5s7 2.63 7 6.5" />
    </>
  ),
  tests: (
    <>
      <rect x="4.5" y="4.5" width="15" height="16.5" rx="2.4" />
      <rect x="8.5" y="2.5" width="7" height="4" rx="1.4" />
      <path d="M8.7 13l2.2 2.2 4.4-4.6" />
    </>
  ),
  assign: (
    <>
      <rect x="3.5" y="5" width="17" height="15.5" rx="2.4" />
      <path d="M3.5 9.5h17" />
      <path d="M8 3v3.5M16 3v3.5" />
      <path d="M8.8 14.8l2.2 2.2 4-4.2" />
    </>
  ),
  bank: (
    <>
      <ellipse cx="12" cy="6" rx="7.5" ry="3" />
      <path d="M4.5 6v6c0 1.66 3.36 3 7.5 3s7.5-1.34 7.5-3V6" />
      <path d="M4.5 12v6c0 1.66 3.36 3 7.5 3s7.5-1.34 7.5-3v-6" />
    </>
  ),
  moon: <path d="M20.5 13.3A8.5 8.5 0 1 1 10.7 3.5a6.6 6.6 0 0 0 9.8 9.8z" />,
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2.5v2M12 19.5v2M4.6 4.6 6 6M18 18l1.4 1.4M2.5 12h2M19.5 12h2M4.6 19.4 6 18M18 6l1.4-1.4" />
    </>
  ),
  logout: (
    <>
      <path d="M9.5 21H6a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3.5" />
      <path d="m16 16 4-4-4-4M20 12H9" />
    </>
  ),
  // "ЕГЭизм" — a vector arrow (thematic wordmark glyph).
  logo: <path d="M6 18 18 6M18 6h-7M18 6v7" />,
  flame: (
    <path d="M8.5 14.5A2.5 2.5 0 0 0 11 12c0-1.38-.5-2-1-3-1.07-2.14-.22-4.05 2-6 .5 2.5 2 4.9 4 6.5 2 1.6 3 3.5 3 5.5a7 7 0 1 1-14 0c0-1.15.43-2.29 1-3a2.5 2.5 0 0 0 2.5 2.5z" />
  ),
  paperclip: (
    <path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48" />
  ),
  bot: (
    <>
      <rect x="4" y="8.5" width="16" height="11.5" rx="2.6" />
      <path d="M12 8.5V6" />
      <circle cx="12" cy="4.5" r="1.3" />
      <path d="M2 14.5h2M20 14.5h2" />
      <circle cx="9" cy="14" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="15" cy="14" r="1.1" fill="currentColor" stroke="none" />
    </>
  ),
  image: (
    <>
      <rect x="3" y="3" width="18" height="18" rx="2.6" />
      <circle cx="8.5" cy="8.5" r="1.8" />
      <path d="m20.5 15-4.5-4.5L5 21" />
    </>
  ),
  check: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="m8.2 12.2 2.6 2.6L16 9.5" />
    </>
  ),
  trash: (
    <>
      <path d="M4 7h16" />
      <path d="M9 7V5.5A1.5 1.5 0 0 1 10.5 4h3A1.5 1.5 0 0 1 15 5.5V7" />
      <path d="M6 7l1 12.5A1.5 1.5 0 0 0 8.5 21h7a1.5 1.5 0 0 0 1.5-1.5L18 7" />
      <path d="M10 11v6M14 11v6" />
    </>
  ),
  pencil: (
    <>
      <path d="M4 20h4L18.5 9.5a2.12 2.12 0 0 0-3-3L5 17v3z" />
      <path d="M13.5 6.5l3 3" />
    </>
  ),
  bell: (
    <>
      <path d="M18.5 9.5a6.5 6.5 0 1 0-13 0c0 5-2 6.5-2 6.5h17s-2-1.5-2-6.5" />
      <path d="M10.2 19.5a2 2 0 0 0 3.6 0" />
    </>
  ),
  close: <path d="M18 6 6 18M6 6l12 12" />,
  download: (
    <>
      <path d="M12 4v11" />
      <path d="M7 10.5 12 15l5-4.5" />
      <path d="M4.5 19.5h15" />
    </>
  ),
  arrowRight: <path d="M4 12h15M13 5.5l6.5 6.5-6.5 6.5" />,
  arrowLeft: <path d="M20 12H5M11 18.5 4.5 12 11 5.5" />,
  chevronDown: <path d="m6 9.5 6 6 6-6" />,
  eye: (
    <>
      <path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z" />
      <circle cx="12" cy="12" r="2.8" />
    </>
  ),
  eyeOff: (
    <>
      <path d="M10.6 6c.46-.07.93-.1 1.4-.1 6 0 9.5 6.1 9.5 6.1a17.6 17.6 0 0 1-2.2 2.9" />
      <path d="M6.4 7.4A17 17 0 0 0 2.5 12S6 18.1 12 18.1c1.5 0 2.86-.38 4.06-.96" />
      <path d="M9.9 9.9a2.8 2.8 0 0 0 4.2 4.2" />
      <path d="m4 4 16 16" />
    </>
  ),
  key: (
    <>
      <circle cx="7.5" cy="16.5" r="4.5" />
      <path d="m10.8 13.2 9.7-9.7" />
      <path d="m15.5 8.5 3 3" />
      <path d="m19 5 2 2" />
    </>
  ),
};

export function Icon({ name, size = 20, strokeWidth = 1.75, style, className }:
  { name: IconName; size?: number; strokeWidth?: number; style?: CSSProperties; className?: string }) {
  return (
    <svg
      width={size} height={size} viewBox="0 0 24 24" fill="none"
      stroke="currentColor" strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round"
      className={className} style={{ display: "block", flex: "none", ...style }} aria-hidden="true"
    >
      {PATHS[name]}
    </svg>
  );
}
