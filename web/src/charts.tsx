import { ReactNode } from "react";
import { LineChart, Line, XAxis, YAxis, Tooltip, ResponsiveContainer, CartesianGrid } from "recharts";
import { HeatCell, WeakSpot, MasteryPoint } from "./api";
import { accColor, heatColor, Card, Label, Button } from "./ui";

// ScoreGauge — the hero forecast number with a semicircle arc.
export function ScoreGauge({ score, max = 100 }: { score: number; max?: number }) {
  const r = 78, cx = 96, cy = 96;
  const frac = Math.max(0, Math.min(1, score / max));
  const a0 = Math.PI, a1 = Math.PI * (1 - frac);
  const p = (a: number) => [cx + r * Math.cos(a), cy - r * Math.sin(a)];
  const [sx, sy] = p(a0), [ex, ey] = p(a1);
  // The gauge sweeps at most 180°, so the SVG large-arc flag is ALWAYS 0.
  // (A previous `frac>0.5?1:0` made it draw the long way round — the bug.)
  return (
    <div style={{ position: "relative", width: 192, height: 116 }}>
      <svg width={192} height={116} viewBox="0 0 192 116">
        <path d={`M ${sx} ${sy} A ${r} ${r} 0 0 1 ${cx + r} ${cy}`} fill="none" stroke="var(--bg-2)" strokeWidth={12} strokeLinecap="round" />
        {frac > 0 && (
          <path d={`M ${sx} ${sy} A ${r} ${r} 0 0 1 ${ex} ${ey}`} fill="none" stroke={accColor((score / max) * 100)} strokeWidth={12} strokeLinecap="round" />
        )}
      </svg>
      {/* The number sits ON the arc's baseline (cy=96), well clear of the apex
          (y=18): digits end at y≈92, «из N» tucks under the baseline. A top-
          anchored block used to shove the digits into the arc. */}
      <div className="mono" style={{ position: "absolute", left: 0, right: 0, bottom: 24, textAlign: "center", fontSize: 42, fontWeight: 800, lineHeight: 1 }}>{score}</div>
      <div className="mono" style={{ position: "absolute", left: 0, right: 0, bottom: 6, textAlign: "center", fontSize: 12, color: "var(--text-3)" }}>из {max}</div>
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

// Heatmap — a year of activity, github-style. Returns cells grouped into weeks.
export function Heatmap({ cells, onDay, big }: { cells: HeatCell[]; onDay?: (c: HeatCell) => void; big?: boolean }) {
  const byDay = new Map(cells.map((c) => [c.day.slice(0, 10), c]));
  const size = big ? 13 : 11, gap = 3;
  const today = new Date();
  const days: HeatCell[] = [];
  const total = big ? 371 : 182;
  for (let i = total - 1; i >= 0; i--) {
    const d = new Date(today); d.setDate(d.getDate() - i);
    const key = d.toISOString().slice(0, 10);
    days.push(byDay.get(key) || { day: d.toISOString(), total: 0, correct: 0 });
  }
  const weeks: HeatCell[][] = [];
  for (let i = 0; i < days.length; i += 7) weeks.push(days.slice(i, i + 7));
  return (
    <div style={{ display: "flex", gap, overflowX: "auto" }} className="scroll">
      {weeks.map((wk, wi) => (
        <div key={wi} style={{ display: "flex", flexDirection: "column", gap }}>
          {wk.map((c, di) => (
            <div key={di} title={`${c.day.slice(0, 10)} · ${c.total} задач${c.total ? ` · ${c.correct} верно` : ""}`} onClick={() => onDay?.(c)}
              style={{ width: size, height: size, borderRadius: 3, background: heatColor(c.total), cursor: onDay ? "pointer" : "default" }} />
          ))}
        </div>
      ))}
    </div>
  );
}

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
        <Tooltip contentStyle={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 10, fontSize: 12 }} />
        <Line type="monotone" dataKey="acc" stroke="var(--accent)" strokeWidth={2.4} dot={{ r: 3 }} />
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
          <div key={s.number} style={{ display: "flex", alignItems: "center", gap: 12 }}>
            <div className="mono" style={{ width: 34, fontWeight: 700 }}>№{s.number}</div>
            <div style={{ flex: 1 }}>
              <div style={{ height: 8, borderRadius: 999, background: "var(--bg-2)", overflow: "hidden" }}>
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
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
        <Label>{title}</Label>{right}
      </div>
      {children}
    </Card>
  );
}
