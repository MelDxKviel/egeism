import { describe, expect, it } from "vitest";
import { nextUnansweredIndex, readSolveSessions, removeSolveSession, saveSolveSession, SavedSolveSession } from "./solve-session";
import type { TaskView } from "./api";

const tasks: TaskView[] = ["one", "two", "three"].map((id, i) => ({
  id, number: i + 1, part: 1, grading_mode: "auto", max_points: 1,
  subject_id: "math", statement: "Условие из банка", media: [], status: "active", answer_kind: "number", bot_solvable: true,
}));

function memoryStorage() {
  const entries = new Map<string, string>();
  return {
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => { entries.set(key, value); },
    removeItem: (key: string) => { entries.delete(key); },
  };
}

function session(attemptId = "attempt-1"): SavedSolveSession {
  return {
    version: 1, request: { subject: "math" }, attemptId, tasks, index: 1,
    drafts: { one: "12,5" }, done: [], skipped: ["one"],
    photos: { one: [{ id: "photo-1", attempt_id: attemptId, task_id: "one", size_bytes: 42, content_type: "image/png" }] },
    requireSolution: false, startedAt: 100, updatedAt: 200, combo: 0,
  };
}

describe("returning to skipped tasks", () => {
  it("continues with untouched tasks before returning to skipped tasks", () => {
    expect(nextUnansweredIndex(tasks, 0, [], ["one"])).toBe(1);
    expect(nextUnansweredIndex(tasks, 1, [], ["one", "two"])).toBe(2);
    expect(nextUnansweredIndex(tasks, 2, [{ taskId: "three" }], ["one", "two"])).toBe(0);
  });
  it("does not finish when the last-position task is answered but earlier tasks were skipped", () => {
    expect(nextUnansweredIndex(tasks, 2, [{ taskId: "two" }, { taskId: "three" }], ["one"])).toBe(0);
  });
  it("keeps an all-skipped session available and handles a single task", () => {
    expect(nextUnansweredIndex(tasks, 2, [], tasks.map((t) => t.id))).toBe(0);
    expect(nextUnansweredIndex(tasks.slice(0, 1), 0, [], ["one"])).toBe(0);
  });
  it("finishes only when every task has an answer", () => {
    expect(nextUnansweredIndex(tasks, 0, tasks.map((t) => ({ taskId: t.id })), [])).toBe(-1);
  });
});

describe("resumable session storage", () => {
  it("restores task order, drafts, photos and skipped state after a reload", () => {
    const storage = memoryStorage();
    const saved = session();
    expect(saveSolveSession("student-a", saved, storage)).toBe(true);
    expect(readSolveSessions("student-a", storage)).toEqual([saved]);
  });
  it("separates students and keeps distinct unfinished attempts", () => {
    const storage = memoryStorage();
    saveSolveSession("student-a", session(), storage);
    saveSolveSession("student-a", { ...session("attempt-2"), updatedAt: 300 }, storage);
    expect(readSolveSessions("student-b", storage)).toEqual([]);
    expect(readSolveSessions("student-a", storage).map((s) => s.attemptId)).toEqual(["attempt-2", "attempt-1"]);
    removeSolveSession("student-a", "attempt-2", storage);
    expect(readSolveSessions("student-a", storage)).toEqual([session()]);
  });
  it("updates an existing session without duplicating it", () => {
    const storage = memoryStorage();
    saveSolveSession("student-a", session(), storage);
    const edited = { ...session(), index: 2, drafts: { one: "12,5", two: "7" } };
    saveSolveSession("student-a", edited, storage);
    expect(readSolveSessions("student-a", storage)).toEqual([edited]);
  });
  it("ignores corrupt or unsupported saved sessions", () => {
    const storage = memoryStorage();
    storage.setItem("egeism.solve.student-a", "invalid-json");
    expect(readSolveSessions("student-a", storage)).toEqual([]);
    storage.setItem("egeism.solve.student-a", JSON.stringify([{ ...session(), index: 99 }, { ...session(), version: 2 }]));
    expect(readSolveSessions("student-a", storage)).toEqual([]);
  });
  it("reports storage failure so the UI cannot promise persistence", () => {
    const storage = { ...memoryStorage(), setItem: () => { throw new Error("Storage quota"); } };
    expect(saveSolveSession("student-a", session(), storage)).toBe(false);
  });
});
