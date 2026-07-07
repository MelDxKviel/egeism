import { describe, it, expect } from "vitest";
import { deadlineInfo } from "./deadline";
import type { AssignmentCard } from "./api";

// Base card; each case overrides the deadline-relevant fields. task_count etc.
// are irrelevant to deadlineInfo — only due_at / finished_at / status matter.
const base = (over: Partial<AssignmentCard>): AssignmentCard => ({
  id: "x", test_id: "t", title: "T", kind: "classic", subject_id: "s",
  scheduled_at: "2026-07-07T12:00:00.000Z", status: "scheduled", task_count: 1,
  correct: 0, total: 0, ...over,
});

// Fixed "now" so the overdue/upcoming boundary is deterministic.
const NOW = Date.parse("2026-07-10T12:00:00.000Z");
const day = 86_400_000;

describe("deadlineInfo", () => {
  it("no deadline -> none", () => {
    const d = deadlineInfo(base({}), NOW);
    expect(d.kind).toBe("none");
    expect(d.text).toBe("");
    expect(d.pill).toBeUndefined();
  });

  it("deadline in the future, unsolved -> upcoming, no pill", () => {
    const d = deadlineInfo(base({ due_at: new Date(NOW + day).toISOString() }), NOW);
    expect(d.kind).toBe("upcoming");
    expect(d.pill).toBeUndefined();
    expect(d.text).toContain("до ");
  });

  it("deadline passed, unsolved -> overdue + red pill", () => {
    const d = deadlineInfo(base({ due_at: new Date(NOW - day).toISOString() }), NOW);
    expect(d.kind).toBe("overdue");
    expect(d.pill).toEqual({ tone: "bad", label: "просрочен" });
  });

  it("solved before the deadline -> ontime + green pill", () => {
    const due = new Date(NOW + day).toISOString();
    const finished = new Date(NOW - day).toISOString();
    const d = deadlineInfo(base({ due_at: due, finished_at: finished }), NOW);
    expect(d.kind).toBe("ontime");
    expect(d.pill).toEqual({ tone: "accent", label: "вовремя" });
  });

  it("solved after the deadline -> late + orange pill", () => {
    const due = new Date(NOW - day).toISOString();
    const finished = new Date(NOW + 10_000).toISOString();
    const d = deadlineInfo(base({ due_at: due, finished_at: finished }), NOW);
    expect(d.kind).toBe("late");
    expect(d.pill).toEqual({ tone: "warn", label: "с опозданием" });
  });

  it("boundary: finished exactly at the deadline counts as on time (not >)", () => {
    const dueISO = new Date(NOW + day).toISOString();
    const d = deadlineInfo(base({ due_at: dueISO, finished_at: dueISO }), NOW);
    expect(d.kind).toBe("ontime");
  });
});
