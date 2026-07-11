// Pure engagement logic — streak milestones, the "streak at risk" ember and
// the adaptive daily goal. Kept free of React/DOM so vitest covers it the same
// way deadline.ts is covered. Day keys follow computeStreak's convention
// (toISOString → UTC date), so both read the same heatmap cells identically.

import { HeatCell } from "./api";

export const dayKey = (d: Date) => d.toISOString().slice(0, 10);

const totalsByDay = (cells: HeatCell[]) =>
  new Map(cells.map((c) => [c.day.slice(0, 10), c.total]));

/** Today's solved count (the daily-goal ring's live value). */
export function todayTotal(cells: HeatCell[], today = new Date()): number {
  return totalsByDay(cells).get(dayKey(today)) ?? 0;
}

/** Streak milestones worth a one-time celebration, ascending. */
export const STREAK_MILESTONES = [3, 7, 14, 30];

// streakCelebration decides whether crossing a milestone should be celebrated
// right now. `seen` is the last celebrated-at streak length (from localStorage);
// returns the milestone to celebrate (the highest newly crossed one) and the
// value to store back. A broken streak (days < seen) re-arms every milestone,
// so rebuilding the series celebrates again.
export function streakCelebration(days: number, seen: number): { milestone: number | null; seen: number } {
  if (days < seen) return { milestone: null, seen: days };
  const crossed = STREAK_MILESTONES.filter((m) => m <= days && m > seen);
  if (crossed.length === 0) return { milestone: null, seen };
  return { milestone: crossed[crossed.length - 1], seen: days };
}

// streakColor — the flame's milestone tint (§checklist: --warn → --accent →
// --hm4 at 3/7/14; ≥30 additionally blazes, see StreakBadge). Below 3 the
// flame keeps whatever color the pill gives it.
export function streakColor(days: number): string | undefined {
  if (days >= 14) return "var(--hm4)";
  if (days >= 7) return "var(--accent)";
  if (days >= 3) return "var(--warn)";
  return undefined;
}

// streakAtRisk — the "догорающий уголёк": today is still empty but yesterday
// ended an active run. Returns that run's length (what the student is about to
// lose), or 0 when today already counts / there is nothing to lose.
export function streakAtRisk(cells: HeatCell[], today = new Date()): number {
  const byDay = totalsByDay(cells);
  const d = new Date(today);
  if ((byDay.get(dayKey(d)) ?? 0) > 0) return 0;
  let run = 0;
  d.setDate(d.getDate() - 1);
  while ((byDay.get(dayKey(d)) ?? 0) > 0) {
    run++;
    d.setDate(d.getDate() - 1);
  }
  return run;
}

// dailyGoal — «реши N сегодня». Adapts to the student's recent pace: the mean
// of ACTIVE days over the last two weeks (today excluded — it's in progress),
// clamped to a sane 5..20; a fresh account starts at 10.
export function dailyGoal(cells: HeatCell[], today = new Date()): number {
  const byDay = totalsByDay(cells);
  const d = new Date(today);
  const totals: number[] = [];
  for (let i = 0; i < 14; i++) {
    d.setDate(d.getDate() - 1);
    const t = byDay.get(dayKey(d)) ?? 0;
    if (t > 0) totals.push(t);
  }
  if (totals.length === 0) return 10;
  const avg = totals.reduce((a, b) => a + b, 0) / totals.length;
  return Math.max(5, Math.min(20, Math.round(avg)));
}
