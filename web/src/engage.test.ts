import { describe, it, expect } from "vitest";
import { HeatCell } from "./api";
import { dayKey, todayTotal, streakCelebration, streakColor, streakAtRisk, effectiveStreak, dailyGoal } from "./engage";

// A fixed "now" keeps the tests deterministic; helpers below build heatmap
// cells relative to it, using the same UTC day keys engage.ts reads.
const TODAY = new Date("2026-07-11T12:00:00Z");

function daysAgo(n: number): string {
  const d = new Date(TODAY);
  d.setDate(d.getDate() - n);
  return dayKey(d);
}
const cell = (ago: number, total: number, correct = 0): HeatCell =>
  ({ day: daysAgo(ago), total, correct });

describe("todayTotal", () => {
  it("reads today's cell", () => {
    expect(todayTotal([cell(1, 5), cell(0, 3)], TODAY)).toBe(3);
  });
  it("is 0 without a today cell", () => {
    expect(todayTotal([cell(1, 5)], TODAY)).toBe(0);
    expect(todayTotal([], TODAY)).toBe(0);
  });
});

describe("streakCelebration", () => {
  it("celebrates the first crossing of a milestone", () => {
    expect(streakCelebration(3, 0)).toEqual({ milestone: 3, seen: 3 });
    expect(streakCelebration(7, 3)).toEqual({ milestone: 7, seen: 7 });
  });
  it("celebrates only the highest newly crossed milestone", () => {
    // Jumped straight past 3 and 7 (e.g. first visit in a while).
    expect(streakCelebration(8, 0)).toEqual({ milestone: 7, seen: 8 });
  });
  it("stays silent between milestones", () => {
    expect(streakCelebration(4, 3)).toEqual({ milestone: null, seen: 3 });
    expect(streakCelebration(13, 8)).toEqual({ milestone: null, seen: 8 });
    expect(streakCelebration(2, 0)).toEqual({ milestone: null, seen: 0 });
  });
  it("never re-celebrates the same milestone on revisits", () => {
    const first = streakCelebration(7, 0);
    expect(first.milestone).toBe(7);
    expect(streakCelebration(7, first.seen)).toEqual({ milestone: null, seen: 7 });
  });
  it("re-arms after the streak breaks", () => {
    const broken = streakCelebration(1, 8); // was 8, broke, restarted
    expect(broken).toEqual({ milestone: null, seen: 1 });
    expect(streakCelebration(3, broken.seen).milestone).toBe(3);
  });
});

describe("streakColor", () => {
  it("steps warn → accent → hm4 at 3/7/14", () => {
    expect(streakColor(0)).toBeUndefined();
    expect(streakColor(2)).toBeUndefined();
    expect(streakColor(3)).toBe("var(--warn)");
    expect(streakColor(6)).toBe("var(--warn)");
    expect(streakColor(7)).toBe("var(--accent)");
    expect(streakColor(13)).toBe("var(--accent)");
    expect(streakColor(14)).toBe("var(--hm4)");
    expect(streakColor(30)).toBe("var(--hm4)");
  });
});

describe("streakAtRisk", () => {
  it("returns yesterday's run when today is empty", () => {
    expect(streakAtRisk([cell(3, 2), cell(2, 1), cell(1, 4)], TODAY)).toBe(3);
  });
  it("is 0 once today has activity", () => {
    expect(streakAtRisk([cell(1, 4), cell(0, 1)], TODAY)).toBe(0);
  });
  it("is 0 when yesterday was empty too (already lost)", () => {
    expect(streakAtRisk([cell(2, 4)], TODAY)).toBe(0);
    expect(streakAtRisk([], TODAY)).toBe(0);
  });
});

describe("effectiveStreak", () => {
  it("counts the run through today when today is active", () => {
    expect(effectiveStreak([cell(2, 1), cell(1, 2), cell(0, 3)], TODAY)).toBe(3);
  });
  it("keeps yesterday's run alive while today is still empty", () => {
    expect(effectiveStreak([cell(3, 2), cell(2, 1), cell(1, 4)], TODAY)).toBe(3);
  });
  it("is 0 only when the streak is genuinely broken (yesterday empty too)", () => {
    expect(effectiveStreak([cell(2, 4)], TODAY)).toBe(0);
    expect(effectiveStreak([], TODAY)).toBe(0);
  });
  it("a morning visit must not re-arm celebrated milestones", () => {
    // 8-day run ending yesterday, seen=7 (the 7-day веха already celebrated).
    // Before today's first solve the effective streak is still 8, so `seen`
    // stays put and the 7-day веха can NOT re-fire after today's solve.
    const cells = Array.from({ length: 8 }, (_, i) => cell(i + 1, 3));
    const alive = effectiveStreak(cells, TODAY);
    expect(alive).toBe(8);
    expect(streakCelebration(alive, 7)).toEqual({ milestone: null, seen: 7 });
    // ...and after the first solve of the day (streak now 9): still silent.
    expect(streakCelebration(9, 7)).toEqual({ milestone: null, seen: 7 });
  });
});

describe("dailyGoal", () => {
  it("defaults to 10 for a fresh account", () => {
    expect(dailyGoal([], TODAY)).toBe(10);
  });
  it("ignores today (the day in progress)", () => {
    expect(dailyGoal([cell(0, 50)], TODAY)).toBe(10);
  });
  it("averages recent active days only", () => {
    // 8 and 12 over two active days → 10; the empty days between don't drag it down.
    expect(dailyGoal([cell(1, 8), cell(5, 12)], TODAY)).toBe(10);
  });
  it("clamps to 5..20", () => {
    expect(dailyGoal([cell(1, 1), cell(2, 2)], TODAY)).toBe(5);
    expect(dailyGoal([cell(1, 60), cell(2, 40)], TODAY)).toBe(20);
  });
  it("looks back only two weeks", () => {
    expect(dailyGoal([cell(20, 60)], TODAY)).toBe(10);
  });
});
