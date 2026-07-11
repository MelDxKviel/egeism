import { ReactNode, memo, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from "recharts";
import { HeatCell, WeakSpot, MasteryPoint } from "./api";
import { accColor, heatColor, Card, Label, Button } from "./ui";
import { prefersReducedMotion } from "./confetti";

// useCountUp animates a number toward `target` (rAF, easeOutCubic): mounting
// counts up from 0, a later target change glides from the current value.
// Reduced-motion lands on the target immediately.
function useCountUp(target: number, ms = 600): number {
  const [v, setV] = useState(() => (prefersReducedMotion() ? target : 0));
  const cur = useRef(v);
  useEffect(() => {
    if (prefersReducedMotion()) { cur.current = target; setV(target); return; }
    const from = cur.current;
    if (from === target) return;
    let raf = 0;
    const t0 = performance.now();
    const step = (t: number) => {
      const k = Math.min(1, (t - t0) / ms);
      const eased = 1 - Math.pow(1 - k, 3);
      const val = from + (target - from) * eased;
      cur.current = val;
      setV(val);
      if (k < 1) raf = requestAnimationFrame(step);
    };
    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [target, ms]);
  return v;
}

// ScoreGauge — the hero forecast number with a semicircle arc. The number
// counts up and the arc sweeps along with it (§3.5 of the design brief).
export function ScoreGauge({ score, max = 100 }: { score: number; max?: number }) {
  const disp = useCountUp(score);
  const r = 78, cx = 96, cy = 96;
  const frac = Math.max(0, Math.min(1, disp / max));
  const a0 = Math.PI, a1 = Math.PI * (1 - frac);
  const p = (a: number) => [cx + r * Math.cos(a), cy - r * Math.sin(a)];
  const [sx, sy] = p(a0), [ex, ey] = p(a1);
  // The gauge sweeps at most 180°, so the SVG large-arc flag is ALWAYS 0.
  // (A previous `frac>0.5?1:0` made it draw the long way round — the bug.)
  return (
    <div style={{ position: "relative", width: 192, height: 116 }}>
      <svg width={192} height={116} viewBox="0 0 192 116">
        <path d={`M ${sx} ${sy} A ${r} ${r} 0 0 1 ${cx + r} ${cy}`} fill="none" stroke="color-mix(in srgb, var(--text) 8%, transparent)" strokeWidth={12} strokeLinecap="round" />
        {frac > 0 && (
          <path d={`M ${sx} ${sy} A ${r} ${r} 0 0 1 ${ex} ${ey}`} fill="none" stroke={accColor((disp / max) * 100)} strokeWidth={12} strokeLinecap="round" />
        )}
      </svg>
      {/* The number sits ON the arc's baseline (cy=96), well clear of the apex
          (y=18): digits end at y≈92, «из N» tucks under the baseline. A top-
          anchored block used to shove the digits into the arc. */}
      <div className="mono" style={{ position: "absolute", left: 0, right: 0, bottom: 24, textAlign: "center", fontSize: 42, fontWeight: 700, letterSpacing: "-0.02em", lineHeight: 1 }}>{Math.round(disp)}</div>
      <div className="mono" style={{ position: "absolute", left: 0, right: 0, bottom: 6, textAlign: "center", fontSize: 12, color: "var(--text-3)" }}>из {max}</div>
    </div>
  );
}

// Ring — a closing progress ring (the daily-goal circle). Mounts empty and
// lets the CSS transition (.ring-anim, killed under reduced-motion) sweep the
// arc to its value, so progress visibly "closes" on every visit. Children
// render centered inside the ring.
export function Ring({ value, max, size = 104, stroke = 11, color = "var(--accent)", children }: {
  value: number; max: number; size?: number; stroke?: number; color?: string; children?: ReactNode;
}) {
  const frac = max > 0 ? Math.max(0, Math.min(1, value / max)) : 0;
  const r = (size - stroke) / 2;
  const C = 2 * Math.PI * r;
  const [drawn, setDrawn] = useState(0);
  useEffect(() => {
    // Double rAF: the first paint must land with the old offset, or the
    // transition has nothing to animate from.
    let id2 = 0;
    const id = requestAnimationFrame(() => { id2 = requestAnimationFrame(() => setDrawn(frac)); });
    return () => { cancelAnimationFrame(id); cancelAnimationFrame(id2); };
  }, [frac]);
  return (
    <div style={{ position: "relative", width: size, height: size, flex: "none" }}>
      {/* rotate(-90°): progress starts at 12 o'clock and closes clockwise. */}
      <svg width={size} height={size} viewBox={`0 0 ${size} ${size}`} style={{ transform: "rotate(-90deg)" }}>
        <circle cx={size / 2} cy={size / 2} r={r} fill="none" stroke="color-mix(in srgb, var(--text) 8%, transparent)" strokeWidth={stroke} />
        {frac > 0 && (
          <circle className="ring-anim" cx={size / 2} cy={size / 2} r={r} fill="none" stroke={color}
            strokeWidth={stroke} strokeLinecap="round" strokeDasharray={C} strokeDashoffset={C * (1 - drawn)} />
        )}
      </svg>
      <div style={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center" }}>
        {children}
      </div>
    </div>
  );
}

// Sparkline — tiny trend line from a numeric series [0..1].
export function Sparkline({ data, w = 84, h = 26, color }: { data: number[]; w?: number; h?: number; color?: string }) {
  if (!data.length) return <svg width={w} height={h} />;
  const max = Math.max(...data, 1), min = Math.min(...data, 0);
  const pts = data.map((v, i) => {
    const x = (i / (data.length - 1 || 1)) * (w - 2) + 1;
    const y = h - 2 - ((v - min) / (max - min || 1)) * (h - 4);
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  }).join(" ");
  return <svg width={w} height={h}><polyline points={pts} fill="none" stroke={color || "var(--accent)"} strokeWidth={1.8} strokeLinejoin="round" strokeLinecap="round" /></svg>;
}

// Heatmap — daily activity, github-style. The grid is RESPONSIVE: it measures
// its container and shows exactly as many trailing weeks as fit (ending today),
// stretching the cells to fill the row edge-to-edge — so it never grows a
// horizontal scrollbar. `big` raises the cap to a full year (the History page);
// the compact dashboard card tops out at half a year.
export function Heatmap({ cells, onDay, big }: { cells: HeatCell[]; onDay?: (c: HeatCell) => void; big?: boolean }) {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  // Hovered cell → the custom portaled tooltip (replaces the native title=).
  const [tip, setTip] = useState<{ x: number; y: number; c: HeatCell } | null>(null);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const ro = new ResizeObserver((es) => setWidth(es[0].contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const gap = 3;
  const base = big ? 13 : 11;      // preferred cell size, px
  const maxWeeks = big ? 53 : 26;  // a year / half a year
  const weeks = width > 0 ? Math.max(4, Math.min(maxWeeks, Math.floor((width + gap) / (base + gap)))) : 0;
  // Grow the cells a touch (up to +5px) to consume the leftover width, so the
  // grid lands flush with the card edge instead of a ragged right gutter.
  const cell = weeks > 0 ? Math.min(base + 5, (width - (weeks - 1) * gap) / weeks) : base;

  // Memoized: the day map + a Date/ISO pair per cell would otherwise rebuild
  // on every tooltip show/hide re-render (371 cells on the History page).
  const cols = useMemo(() => {
    const byDay = new Map(cells.map((c) => [c.day.slice(0, 10), c]));
    const today = new Date();
    const days: HeatCell[] = [];
    for (let i = weeks * 7 - 1; i >= 0; i--) {
      const d = new Date(today); d.setDate(d.getDate() - i);
      const key = d.toISOString().slice(0, 10);
      days.push(byDay.get(key) || { day: d.toISOString(), total: 0, correct: 0 });
    }
    const out: HeatCell[][] = [];
    for (let i = 0; i < days.length; i += 7) out.push(days.slice(i, i + 7));
    return out;
  }, [cells, weeks]);

  const onHover = useCallback((c: HeatCell | null, el?: HTMLElement) => {
    if (!c || !el) { setTip(null); return; }
    const r = el.getBoundingClientRect();
    setTip({ x: r.left + r.width / 2, y: r.top, c });
  }, []);

  return (
    <div ref={ref}>
      {weeks === 0
        ? <div style={{ height: 7 * (base + gap) - gap }} /> /* pre-measure placeholder, keeps the card height stable */
        : <HeatGrid cols={cols} cell={cell} gap={gap} onDay={onDay} onHover={onHover} />}
      {/* The tooltip portals into the .app scope (NOT bare <body>) so the theme
          tokens resolve — same trap the Modal comment in ui.tsx describes. It
          sits UNDER the modal z-index (50), so an open dialog covers it. */}
      {tip && createPortal(
        <div key={tip.c.day} className="mono"
          // Clamp into the viewport after layout: the most-hovered cells are
          // the newest days at the grid's RIGHT edge, where a centered nowrap
          // tooltip would run off screen and lose its «N верно» tail.
          ref={(el) => {
            if (!el) return;
            const r = el.getBoundingClientRect();
            const pad = 8;
            const dx = r.left < pad ? pad - r.left
              : r.right > window.innerWidth - pad ? window.innerWidth - pad - r.right : 0;
            if (dx !== 0) el.style.left = `${tip.x + dx}px`;
          }}
          style={{
            position: "fixed", left: tip.x, top: tip.y - 9, transform: "translate(-50%, -100%)",
            zIndex: 45, pointerEvents: "none", whiteSpace: "nowrap",
            background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 10,
            boxShadow: "var(--shadow-2)", padding: "7px 11px", fontSize: 12, color: "var(--text-2)",
          }}>
          <b style={{ color: "var(--text)", fontWeight: 600 }}>
            {/* Format from the UTC day KEY, not the timestamp — new Date(iso)
                in a western timezone would render the previous day. */}
            {(() => {
              const [y, m, d] = tip.c.day.slice(0, 10).split("-").map(Number);
              return new Date(y, m - 1, d).toLocaleDateString("ru", { day: "numeric", month: "long" });
            })()}
          </b>
          {tip.c.total > 0
            ? <> · {tip.c.total} решено · <span style={{ color: "var(--ok)" }}>{tip.c.correct} верно</span></>
            : " · нет решений"}
        </div>,
        document.querySelector(".app") ?? document.body,
      )}
    </div>
  );
}

// HeatGrid — the memoized cell grid. Tooltip state lives in the Heatmap
// wrapper, so hovering a day re-renders only that wrapper; without the memo a
// cursor sweep re-reconciled every cell div twice per crossing.
const HeatGrid = memo(function HeatGrid({ cols, cell, gap, onDay, onHover }: {
  cols: HeatCell[][]; cell: number; gap: number;
  onDay?: (c: HeatCell) => void;
  onHover: (c: HeatCell | null, el?: HTMLElement) => void;
}) {
  return (
    <div style={{ display: "flex", gap }}>
      {cols.map((wk, wi) => (
        <div key={wi} style={{ display: "flex", flexDirection: "column", gap, flex: "none" }}>
          {wk.map((c, di) => (
            <div key={di} className={onDay ? "hm-cell hm-tap" : "hm-cell"}
              onMouseEnter={(e) => onHover(c, e.currentTarget)}
              onMouseLeave={() => onHover(null)}
              onClick={() => { onHover(null); onDay?.(c); }}
              style={{ width: cell, height: cell, borderRadius: cell >= 13 ? 4 : 3, background: heatColor(c.total), cursor: onDay ? "pointer" : "default" }} />
          ))}
        </div>
      ))}
    </div>
  );
});

// streak = consecutive days up to today with any activity.
export function computeStreak(cells: HeatCell[]): number {
  const byDay = new Map(cells.map((c) => [c.day.slice(0, 10), c.total]));
  let streak = 0;
  const d = new Date();
  for (;;) {
    const key = d.toISOString().slice(0, 10);
    if ((byDay.get(key) || 0) > 0) { streak++; d.setDate(d.getDate() - 1); }
    else break;
  }
  return streak;
}

// MasteryChart — accuracy over weeks for one task number (Recharts line).
export function MasteryChart({ points }: { points: MasteryPoint[] }) {
  const data = points.map((p) => ({
    week: p.week.slice(5, 10),
    acc: p.total ? Math.round((p.correct / p.total) * 100) : 0,
  }));
  if (data.length < 2) {
    return <div style={{ color: "var(--text-2)", fontSize: 14, padding: 20 }}>Недостаточно данных для графика — реши больше заданий этого номера.</div>;
  }
  return (
    <ResponsiveContainer width="100%" height={200}>
      <LineChart data={data} margin={{ top: 8, right: 8, bottom: 0, left: -18 }}>
        <CartesianGrid stroke="var(--border)" vertical={false} />
        <XAxis dataKey="week" stroke="var(--text-3)" fontSize={11} tickLine={false} />
        <YAxis domain={[0, 100]} stroke="var(--text-3)" fontSize={11} tickLine={false} />
        <Tooltip contentStyle={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 12, boxShadow: "var(--shadow-2)", fontSize: 12 }} />
        <Line type="monotone" dataKey="acc" stroke="var(--accent)" strokeWidth={2.4} dot={{ r: 3 }} isAnimationActive={false} />
      </LineChart>
    </ResponsiveContainer>
  );
}

// WeakSpotsList — worst numbers with accuracy bar + drill CTA.
export function WeakSpotsList({ spots, onDrill }: { spots: WeakSpot[]; onDrill: (n: number) => void }) {
  if (!spots.length) return <div style={{ color: "var(--text-2)", fontSize: 14 }}>Пока нет данных — реши несколько заданий.</div>;
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      {spots.map((s) => {
        const pct = Math.round(s.accuracy * 100);
        return (
          <div key={s.number} style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
            <div className="mono" style={{ width: 34, fontWeight: 700 }}>№{s.number}</div>
            {/* The bar keeps a readable minimum; when the row runs out the CTA
                wraps to the next line instead of crushing the bar to a sliver. */}
            <div style={{ flex: "1 1 90px", minWidth: 90 }}>
              <div style={{ height: 8, borderRadius: 999, background: "color-mix(in srgb, var(--text) 8%, transparent)", overflow: "hidden" }}>
                <div style={{ width: `${pct}%`, height: "100%", background: accColor(pct) }} />
              </div>
            </div>
            <div className="mono" style={{ width: 40, textAlign: "right", color: accColor(pct), fontWeight: 700 }}>{pct}%</div>
            <Button variant="soft" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => onDrill(s.number)}>Прокачать</Button>
          </div>
        );
      })}
    </div>
  );
}

export function Section({ title, right, children }: { title: string; right?: ReactNode; children: ReactNode }) {
  return (
    <Card>
      {/* flexWrap: a long `right` node (a button, a hint) drops below the title
          on a phone instead of colliding with it. */}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, rowGap: 6, flexWrap: "wrap", marginBottom: 16 }}>
        <Label>{title}</Label>{right}
      </div>
      {children}
    </Card>
  );
}
