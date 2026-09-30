import type { Forecast, SolutionPhoto, SubjectCode, TaskView } from "./api";

export interface SolveRequest {
  subject: SubjectCode;
  number?: number;
  taskId?: string;
  mode?: "mistakes" | "recommended";
  testId?: string;
  assignmentId?: string;
  title?: string;
  resumeAttemptId?: string;
}

export interface Answered { taskId: string; number: number; correct: boolean; pending?: boolean; solution?: string[]; }

export interface SavedSolveSession {
  version: 1;
  request: SolveRequest;
  attemptId: string;
  tasks: TaskView[];
  index: number;
  drafts: Record<string, string>;
  done: Answered[];
  skipped: string[];
  photos: Record<string, SolutionPhoto[]>;
  requireSolution: boolean;
  startedAt: number;
  updatedAt: number;
  combo: number;
  level?: number;
  forecastBefore?: Forecast | null;
}

type SessionStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;
const storageKey = (userId: string) => `egeism.solve.${userId}`;

export function readSolveSessions(userId: string, storage: SessionStorage = localStorage): SavedSolveSession[] {
  if (!userId) return [];
  try {
    const data: unknown = JSON.parse(storage.getItem(storageKey(userId)) ?? "[]");
    if (!Array.isArray(data)) return [];
    return data.filter((s): s is SavedSolveSession => !!s && s.version === 1 &&
      typeof s.attemptId === "string" && !!s.request &&
      ["rus", "math", "inf", "soc"].includes(s.request.subject) &&
      Array.isArray(s.tasks) && s.tasks.length > 0 && s.tasks.every((t: TaskView) => t && typeof t.id === "string") &&
      Number.isInteger(s.index) && s.index >= 0 && s.index < s.tasks.length &&
      !!s.drafts && typeof s.drafts === "object" && !!s.photos && typeof s.photos === "object" &&
      Array.isArray(s.done) && Array.isArray(s.skipped) && Number.isFinite(s.startedAt) && Number.isFinite(s.updatedAt))
      .sort((a, b) => b.updatedAt - a.updatedAt);
  } catch { return []; }
}

export function saveSolveSession(userId: string, session: SavedSolveSession, storage: SessionStorage = localStorage): boolean {
  if (!userId) return false;
  try {
    const sessions = readSolveSessions(userId, storage).filter((s) => s.attemptId !== session.attemptId);
    storage.setItem(storageKey(userId), JSON.stringify([session, ...sessions]));
    return true;
  } catch { return false; }
}

export function removeSolveSession(userId: string, attemptId: string, storage: SessionStorage = localStorage) {
  try {
    const sessions = readSolveSessions(userId, storage).filter((s) => s.attemptId !== attemptId);
    if (sessions.length) storage.setItem(storageKey(userId), JSON.stringify(sessions));
    else storage.removeItem(storageKey(userId));
  } catch { /* A blocked browser store must not turn a successful finish into an error. */ }
}

// First finish the untouched tasks, then return to postponed ones. Reaching
// the last position is never a reason to finish while an unanswered task remains.
export function nextUnansweredIndex(tasks: Pick<TaskView, "id">[], current: number, done: Pick<Answered, "taskId">[], skipped: string[]): number {
  const answered = new Set(done.map((a) => a.taskId));
  const postponed = new Set(skipped);
  const order = Array.from({ length: tasks.length }, (_, i) => (current + i + 1) % tasks.length)
    .filter((i) => !answered.has(tasks[i].id));
  return order.find((i) => !postponed.has(tasks[i].id)) ?? order[0] ?? -1;
}
