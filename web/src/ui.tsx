import { CSSProperties, ReactNode } from "react";
import { Media, mediaUrl } from "./api";
import { Icon } from "./icons";

// MediaBlock renders a task's block figures and attached files (§8: file/image
// tasks are web-only). Inline formulas (m.inline) are skipped here — they are
// drawn mid-sentence by StatementView via their ⟦img:N⟧ placeholders.
export function MediaBlock({ media }: { media?: Media[] }) {
  const blocks = (media || []).filter((m) => !m.inline);
  if (blocks.length === 0) return null;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10, margin: "6px 0 16px" }}>
      {blocks.map((m, i) => m.kind === "file" ? (
        <a key={i} href={mediaUrl(m.key)} target="_blank" rel="noreferrer" className="mono" style={{
          display: "inline-flex", alignItems: "center", gap: 8, alignSelf: "flex-start",
          background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 10,
          padding: "8px 12px", fontSize: 13, textDecoration: "none", color: "var(--accent-2)",
        }}><Icon name="paperclip" size={15} /> {m.alt || "Скачать файл"}</a>
      ) : (
        <img key={i} src={mediaUrl(m.key)} alt={m.alt || ""} loading="lazy" style={{
          maxWidth: "100%", borderRadius: 12, border: "1px solid var(--border)", display: "block",
        }} />
      ))}
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
      blocks.push(
        <div key={blocks.length} style={{ overflowX: "auto", margin: "10px 0" }}>
          <table className="stmt-table">
            {header.length > 0 && (
              <thead>{header.map((r, ri) => (
                <tr key={ri}>{r.map((c, ci) => <th key={ci}>{c ? renderInline(c, media) : " "}</th>)}</tr>
              ))}</thead>
            )}
            <tbody>{body.map((r, ri) => (
              <tr key={ri}>{r.map((c, ci) => <td key={ci}>{c ? renderInline(c, media) : " "}</td>)}</tr>
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

// Accuracy → token color (§ design: ≥78 accent · 62–77 L3 · 48–61 warn · <48 bad).
export function accColor(pct: number): string {
  if (pct >= 78) return "var(--accent)";
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
  return (
    <div className={className} onClick={onClick} style={{
      background: "var(--surface)", border: "1px solid var(--border)",
      borderRadius: 16, padding: 22, boxShadow: "var(--shadow)", ...style,
    }}>{children}</div>
  );
}

export function Label({ children, style }: { children: ReactNode; style?: CSSProperties }) {
  return <div className="mono" style={{
    fontSize: 11, letterSpacing: ".06em", textTransform: "uppercase",
    color: "var(--text-3)", ...style,
  }}>{children}</div>;
}

export function Pill({ children, tone = "neutral" }: { children: ReactNode; tone?: "accent" | "bad" | "warn" | "neutral" }) {
  const map = {
    accent: ["var(--accent-soft)", "var(--accent-2)"],
    bad: ["var(--bad-soft)", "var(--bad)"],
    warn: ["var(--warn-soft)", "var(--warn)"],
    neutral: ["var(--bg-2)", "var(--text-2)"],
  }[tone];
  return <span className="mono" style={{
    background: map[0], color: map[1], borderRadius: 999, padding: "3px 10px",
    fontSize: 11, fontWeight: 600, letterSpacing: ".02em",
  }}>{children}</span>;
}

export function Button({ children, onClick, variant = "primary", disabled, style, type }:
  { children: ReactNode; onClick?: () => void; variant?: "primary" | "ghost" | "soft"; disabled?: boolean; style?: CSSProperties; type?: "button" | "submit" }) {
  const base: CSSProperties = {
    borderRadius: 11, padding: "10px 16px", fontWeight: 600, fontSize: 14,
    border: "1px solid transparent", opacity: disabled ? 0.5 : 1,
    pointerEvents: disabled ? "none" : "auto", ...style,
  };
  const styles: Record<string, CSSProperties> = {
    primary: { background: "var(--accent)", color: "var(--on-accent)" },
    ghost: { background: "transparent", color: "var(--text)", border: "1px solid var(--border-2)" },
    soft: { background: "var(--accent-soft)", color: "var(--accent-2)" },
  };
  return <button type={type || "button"} onClick={onClick} disabled={disabled} style={{ ...base, ...styles[variant] }}>{children}</button>;
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
