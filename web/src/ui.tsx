import { CSSProperties, ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { AttemptReviewItem, Media, mediaUrl } from "./api";
import { useApp } from "./state";
import { Icon } from "./icons";

// ---------- Modal (the ONE portaled dialog) ----------
// maxWidth defaults to a compact dialog; pass a larger value (e.g. a near-full
// "min(1200px, 96vw)") for content-heavy modals like the attempt review. The body
// scrolls internally so the panel never exceeds the viewport.
export function Modal({ title, children, onClose, maxWidth = 560 }:
  { title: ReactNode; children: ReactNode; onClose: () => void; maxWidth?: number | string }) {
  const { theme } = useApp();
  // Portaled to <body> so the fixed backdrop covers the whole viewport — the page
  // content lives inside a `.fade` wrapper whose transform animation makes it the
  // containing block for position:fixed, which would otherwise clip the backdrop
  // to the centered content column (same reason the MediaBlock lightbox portals).
  // The overlay re-establishes the theme scope (`.app` + data-theme) because the
  // design tokens (var(--surface)…) are defined there and would be undefined at
  // <body> level, leaving the panel transparent and unstyled. Every dialog must
  // go through this component — a bare createPortal loses the theme again.
  return createPortal(
    <div className="app overlay" data-theme={theme} onClick={onClose} style={{ position: "fixed", inset: 0, display: "flex", alignItems: "center", justifyContent: "center", zIndex: 50, padding: 16, minHeight: 0 }}>
      <div onClick={(e) => e.stopPropagation()} className="pop" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 20, padding: 24, maxWidth, width: "100%", maxHeight: "92vh", display: "flex", flexDirection: "column", boxShadow: "var(--shadow-lg)" }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
          <div style={{ fontWeight: 700, fontSize: 17, letterSpacing: "-0.01em", display: "flex", alignItems: "center", gap: 10 }}>{title}</div>
          <button onClick={onClose} title="Закрыть" className="icon-btn" style={{ border: "none", borderRadius: 999, padding: 6, color: "var(--text-3)" }}><Icon name="close" size={19} /></button>
        </div>
        <div className="scroll" style={{ overflowY: "auto", flex: 1, minHeight: 0 }}>{children}</div>
      </div>
    </div>,
    document.body,
  );
}

// MediaBlock renders a task's block figures and attached files (§8: file/image
// tasks are web-only). Inline formulas (m.inline) are skipped here — they are
// drawn mid-sentence by StatementView via their ⟦img:N⟧ placeholders.
export function MediaBlock({ media }: { media?: Media[] }) {
  const blocks = (media || []).filter((m) => !m.inline);
  // Which image is enlarged in the lightbox (its media key, null = closed).
  const [zoom, setZoom] = useState<string | null>(null);
  // Esc closes the lightbox while it is open.
  useEffect(() => {
    if (!zoom) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setZoom(null); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [zoom]);
  if (blocks.length === 0) return null;
  const zoomed = zoom ? blocks.find((m) => m.key === zoom) : undefined;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10, margin: "6px 0 16px" }}>
      {blocks.map((m, i) => m.kind === "file" ? (
        <a key={i} href={mediaUrl(m.key)} target="_blank" rel="noreferrer" className="mono" style={{
          display: "inline-flex", alignItems: "center", gap: 8, alignSelf: "flex-start",
          background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 10,
          padding: "8px 12px", fontSize: 13, textDecoration: "none", color: "var(--accent-2)",
        }}><Icon name="paperclip" size={15} /> {m.alt || "Скачать файл"}</a>
      ) : (
        // Bounded so a scheme/diagram sits at a modest size instead of filling
        // the card; click opens an in-page lightbox. The container is always
        // white (even in dark theme) so transparent PNG figures stay legible.
        <div key={i} role="button" tabIndex={0} aria-label="Увеличить изображение"
          className="card-tap"
          onClick={() => setZoom(m.key)}
          onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); setZoom(m.key); } }}
          style={{
            alignSelf: "flex-start", display: "block", width: "min(100%, 340px)", lineHeight: 0,
            background: "#FFFFFF", padding: 6, borderRadius: 14, border: "1px solid var(--border)",
            cursor: "zoom-in", boxSizing: "border-box",
          }}>
          <img src={mediaUrl(m.key)} alt={m.alt || ""} loading="lazy" style={{
            width: "100%", height: "auto", maxHeight: 440, display: "block",
          }} />
        </div>
      ))}
      {zoomed && createPortal(
        // Telegram-style lightbox: dark backdrop, centered image on a white
        // panel; click backdrop or ✕ (or Esc) to close, click image = no-op.
        // Portaled to <body> so the fixed backdrop covers the whole viewport
        // regardless of any transformed/clipping ancestor in the card tree.
        <div onClick={() => setZoom(null)} style={{
          position: "fixed", inset: 0, zIndex: 2000, background: "rgba(0,0,0,.72)",
          WebkitBackdropFilter: "blur(14px)", backdropFilter: "blur(14px)",
          display: "flex", alignItems: "center", justifyContent: "center", padding: 20,
        }}>
          <button type="button" aria-label="Закрыть" onClick={() => setZoom(null)} style={{
            position: "fixed", top: 16, right: 16, zIndex: 2001,
            display: "inline-flex", alignItems: "center", justifyContent: "center",
            width: 40, height: 40, borderRadius: 999, border: "none", cursor: "pointer",
            background: "rgba(0,0,0,.5)", color: "#fff",
          }}><Icon name="close" size={20} /></button>
          <img src={mediaUrl(zoomed.key)} alt={zoomed.alt || ""}
            onClick={(e) => e.stopPropagation()} style={{
              // Scale the figure UP to fill a large white panel (not its tiny
              // natural size): object-fit contain keeps the aspect ratio and
              // letterboxes onto white, so small schemes actually get bigger.
              width: "min(92vw, 960px)", height: "min(86vh, 720px)",
              objectFit: "contain", display: "block",
              background: "#FFFFFF", padding: 12, borderRadius: 12, boxSizing: "border-box",
            }} />
        </div>,
        document.body,
      )}
    </div>
  );
}

// renderInline swaps a statement's ⟦img:N⟧ formula placeholders (emitted by the
// РЕШУ fetcher for <img class=tex> chunks) for the matching inline media image,
// so formulas sit mid-sentence at text size instead of as detached blocks.
function renderInline(text: string, media?: Media[]): ReactNode[] {
  if (!media || media.length === 0 || !text.includes("⟦img:")) return [text];
  const parts: ReactNode[] = [];
  const re = /⟦img:(\d+)⟧/g;
  let last = 0, k = 0, m: RegExpExecArray | null;
  while ((m = re.exec(text))) {
    if (m.index > last) parts.push(text.slice(last, m.index));
    const mm = media[Number(m[1])];
    parts.push(mm
      ? <img key={`f${k++}`} className="stmt-formula" src={mediaUrl(mm.key)} alt={mm.alt || ""} loading="lazy" />
      : m[0]);
    last = m.index + m[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));
  return parts;
}

// StatementView renders a task statement, drawing the Markdown tables the
// content fetcher emits (truth tables задание 2, DB headers задание 3, …) as
// real styled <table>s, inline formula placeholders (⟦img:N⟧) as inline images,
// and everything else as text (line breaks preserved).
export function StatementView({ text, media, style }: { text?: string; media?: Media[]; style?: CSSProperties }) {
  const lines = (text || "").split("\n");
  const isRow = (l: string) => /^\s*\|.*\|\s*$/.test(l);
  const cellsOf = (l: string) =>
    l.trim().replace(/^\|/, "").replace(/\|$/, "").split("|").map((c) => c.trim());
  const isSep = (l: string) => isRow(l) && cellsOf(l).every((c) => c === "" || /^-+$/.test(c));

  const blocks: ReactNode[] = [];
  let para: string[] = [];
  const flush = () => {
    if (para.join("").trim())
      blocks.push(<div key={blocks.length} style={{ whiteSpace: "pre-wrap" }}>{renderInline(para.join("\n").trim(), media)}</div>);
    para = [];
  };
  for (let i = 0; i < lines.length; ) {
    if (isRow(lines[i])) {
      flush();
      const rows: string[] = [];
      while (i < lines.length && isRow(lines[i])) rows.push(lines[i++]);
      const sep = rows.findIndex(isSep);
      const header = sep > 0 ? rows.slice(0, sep).map(cellsOf) : [];
      const body = (sep >= 0 ? rows.slice(sep + 1) : rows).map(cellsOf);
      // A corner matrix (empty top-left header cell) labels rows in its first
      // column — render those as <th> too, so both axes read as headers.
      const rowLabels = header.length > 0 && header[0][0] === "";
      blocks.push(
        <div key={blocks.length} style={{ overflowX: "auto", margin: "10px 0" }}>
          <table className="stmt-table">
            {header.length > 0 && (
              <thead>{header.map((r, ri) => (
                <tr key={ri}>{r.map((c, ci) => <th key={ci}>{c ? renderInline(c, media) : " "}</th>)}</tr>
              ))}</thead>
            )}
            <tbody>{body.map((r, ri) => (
              <tr key={ri}>{r.map((c, ci) => rowLabels && ci === 0
                ? <th key={ci}>{c ? renderInline(c, media) : " "}</th>
                : <td key={ci}>{c ? renderInline(c, media) : " "}</td>)}</tr>
            ))}</tbody>
          </table>
        </div>,
      );
    } else {
      para.push(lines[i++]);
    }
  }
  flush();
  return <div style={style}>{blocks}</div>;
}

// Accuracy → token color (≥78 green · 62–77 light green · 48–61 warn · <48 bad).
// Success is GREEN (system semantics), the blue accent is reserved for actions.
export function accColor(pct: number): string {
  if (pct >= 78) return "var(--ok)";
  if (pct >= 62) return "var(--hm3)";
  if (pct >= 48) return "var(--warn)";
  return "var(--bad)";
}
// Heatmap level by daily total (0 → L0; 1–2 L1; 3–5 L2; 6–9 L3; ≥10 L4).
export function heatColor(total: number): string {
  if (total <= 0) return "var(--hm0)";
  if (total <= 2) return "var(--hm1)";
  if (total <= 5) return "var(--hm2)";
  if (total <= 9) return "var(--hm3)";
  return "var(--hm4)";
}

export function Card({ children, style, className, onClick }:
  { children: ReactNode; style?: CSSProperties; className?: string; onClick?: () => void }) {
  // Clickable cards get the .card-tap raise/settle states from theme.css.
  return (
    <div className={["card", onClick ? "card-tap" : "", className || ""].filter(Boolean).join(" ")}
      onClick={onClick} style={{ padding: 22, ...style }}>{children}</div>
  );
}

export function Label({ children, style }: { children: ReactNode; style?: CSSProperties }) {
  return <div className="mono" style={{
    fontSize: 11, fontWeight: 600, letterSpacing: ".08em", textTransform: "uppercase",
    color: "var(--text-3)", ...style,
  }}>{children}</div>;
}

export function Pill({ children, tone = "neutral" }: { children: ReactNode; tone?: "accent" | "ok" | "bad" | "warn" | "neutral" }) {
  // ok = green success («верно», «решён»); accent (blue) = informational/selected.
  // The neutral tint is text-mixed so it reads on any surface level.
  const map = {
    accent: ["var(--accent-soft)", "var(--accent-2)"],
    ok: ["var(--ok-soft)", "var(--ok)"],
    bad: ["var(--bad-soft)", "var(--bad)"],
    warn: ["var(--warn-soft)", "var(--warn)"],
    neutral: ["color-mix(in srgb, var(--text) 8%, transparent)", "var(--text-2)"],
  }[tone];
  return <span className="mono" style={{
    background: map[0], color: map[1], borderRadius: 999, padding: "3px 10px",
    fontSize: 11, fontWeight: 600, letterSpacing: ".02em",
  }}>{children}</span>;
}

// Pill button (apple.com CTA shape). Colors + hover/active/focus states live in
// theme.css (.btn-*); the style prop stays for layout overrides only.
export function Button({ children, onClick, variant = "primary", disabled, style, type }:
  { children: ReactNode; onClick?: () => void; variant?: "primary" | "ghost" | "soft" | "danger" | "ok"; disabled?: boolean; style?: CSSProperties; type?: "button" | "submit" }) {
  return <button type={type || "button"} onClick={onClick} disabled={disabled}
    className={`btn btn-${variant}`} style={style}>{children}</button>;
}

// PasswordInput — a password field with the «глазик»: a trailing eye button
// toggles between the masked and plain view. Use it for EVERY password field
// (login, reset page, admin/teacher account forms) so the affordance is
// uniform. tabIndex=-1 on the toggle keeps Tab flowing input → submit.
export function PasswordInput({ value, onChange, placeholder, autoComplete, autoFocus, style }: {
  value: string; onChange: (v: string) => void; placeholder?: string;
  autoComplete?: string; autoFocus?: boolean; style?: CSSProperties;
}) {
  const [show, setShow] = useState(false);
  return (
    <div style={{ position: "relative", width: "100%", ...style }}>
      <input
        type={show ? "text" : "password"} value={value} onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder} autoComplete={autoComplete} autoFocus={autoFocus}
        style={{ width: "100%", paddingRight: 42 }}
      />
      <button
        type="button" onClick={() => setShow((s) => !s)} tabIndex={-1}
        title={show ? "Скрыть пароль" : "Показать пароль"}
        aria-label={show ? "Скрыть пароль" : "Показать пароль"}
        style={{
          position: "absolute", right: 4, top: "50%", transform: "translateY(-50%)",
          display: "flex", alignItems: "center", background: "none", border: "none",
          color: "var(--text-3)", padding: 8, cursor: "pointer",
        }}
      >
        <Icon name={show ? "eyeOff" : "eye"} size={17} />
      </button>
    </div>
  );
}

export function Spinner() {
  return <div style={{
    width: 22, height: 22, border: "2px solid var(--border-2)", borderTopColor: "var(--accent)",
    borderRadius: "50%", animation: "spin .8s linear infinite",
  }} />;
}

// Unified loading / error / empty patterns.
export function Loading({ label = "Загрузка…" }: { label?: string }) {
  return <div style={{ display: "flex", gap: 12, alignItems: "center", color: "var(--text-2)", padding: 20 }}>
    <Spinner /> <span>{label}</span>
  </div>;
}
export function Skeleton({ h = 120 }: { h?: number }) {
  return <div className="skel" style={{ height: h, width: "100%" }} />;
}
export function ErrorBox({ error, onRetry }: { error: unknown; onRetry?: () => void }) {
  return <Card style={{ borderColor: "var(--bad)" }}>
    <div style={{ color: "var(--bad)", fontWeight: 600, marginBottom: 6 }}>Ошибка</div>
    <div style={{ color: "var(--text-2)", fontSize: 14 }}>{String((error as Error)?.message ?? error)}</div>
    {onRetry && <div style={{ marginTop: 12 }}><Button variant="ghost" onClick={onRetry}>Повторить</Button></div>}
  </Card>;
}
export function Empty({ title, hint, action }: { title: string; hint?: string; action?: ReactNode }) {
  return <Card style={{ textAlign: "center", padding: 34 }}>
    <div style={{ fontWeight: 700, fontSize: 16 }}>{title}</div>
    {hint && <div style={{ color: "var(--text-2)", marginTop: 6, fontSize: 14 }}>{hint}</div>}
    {action && <div style={{ marginTop: 16 }}>{action}</div>}
  </Card>;
}

// Query-state wrapper: shows loading/error, else renders children with data.
export function Async<T>({ q, children, loading }:
  { q: { data?: T; isLoading: boolean; error: unknown; refetch: () => void }; children: (d: T) => ReactNode; loading?: ReactNode }) {
  if (q.isLoading) return <>{loading ?? <Loading />}</>;
  if (q.error) return <ErrorBox error={q.error} onRetry={q.refetch} />;
  if (q.data === undefined) return null;
  return <>{children(q.data)}</>;
}

export const SUBJECT_TITLES: Record<string, string> = {
  rus: "Русский язык", math: "Математика", inf: "Информатика", soc: "Обществознание",
};

// The internal practice test carries a sentinel title; show it nicely in feeds.
export const testTitle = (t: string) => (t === "__practice__" ? "Свободное решение" : t);

// AttemptReviewGrid renders a solved attempt as reviewable cards — each task's
// condition + media, the answer given, the verdict, and the correct answer.
// Shared by the teacher's attempt review and the student's assigned-tests
// history (the «как решил» drill-down), so the two stay identical. Pass
// selfView when the solver is viewing their own attempt («твой ответ» vs the
// teacher's «ответ ученика»). Wrap it in a <Modal maxWidth="min(1200px, 96vw)">.
export function AttemptReviewGrid({ items, selfView }: { items: AttemptReviewItem[]; selfView?: boolean }) {
  if (items.length === 0) return <div style={{ color: "var(--text-2)" }}>В этой попытке нет ответов.</div>;
  return (
    <div style={{ display: "grid", gap: 12, gridTemplateColumns: "repeat(auto-fill, minmax(340px, 1fr))", alignItems: "start" }}>
      {items.map((it) => (
        <div key={it.answer_id} style={{ padding: 14, background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 14 }}>
          <div style={{ display: "flex", gap: 8, marginBottom: 8, alignItems: "center" }}>
            <Pill tone="neutral">№{it.number}</Pill>
            <Pill tone={it.is_correct ? "ok" : "bad"}>{it.is_correct ? "верно" : "неверно"}</Pill>
          </div>
          <StatementView text={it.statement} media={it.media} style={{ fontSize: 14, lineHeight: 1.45, marginBottom: 8 }} />
          <MediaBlock media={it.media} />
          <div className="mono" style={{ fontSize: 13, display: "flex", flexDirection: "column", gap: 4 }}>
            <div><span style={{ color: "var(--text-3)" }}>{selfView ? "твой ответ" : "ответ ученика"}: </span><b style={{ color: it.is_correct ? "var(--ok)" : "var(--bad)" }}>{it.raw_answer || "—"}</b></div>
            <div><span style={{ color: "var(--text-3)" }}>верный ответ: </span><b style={{ color: "var(--ok)" }}>{it.correct.join(" / ")}</b></div>
          </div>
        </div>
      ))}
    </div>
  );
}
