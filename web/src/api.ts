// API client + types + React Query hooks. Talks to the Go API; the acting user
// is sent via the X-User-ID header (stage-1 placeholder auth).
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

export type Role = "student" | "teacher";
export type SubjectCode = "rus" | "math" | "inf" | "soc";
export type AnswerKind = "number" | "string" | "set" | "sequence";
export type TaskStatus = "draft" | "active" | "rejected";
export type TestKind = "classic" | "drill";

export interface User { id: string; role: Role; name: string; telegram_id?: number; }
export interface Subject { id: string; code: SubjectCode; title: string; }
export interface Media { key: string; kind: "image" | "table" | "file"; alt?: string; inline?: boolean; }
export interface AnswerSchema {
  type: AnswerKind; correct: string[]; tolerance?: number;
  ci?: boolean; yo_fold?: boolean; token?: "char" | "split";
}
export interface TaskView {
  id: string; subject_id: string; number: number; statement: string;
  media: Media[]; status: TaskStatus; answer_kind: AnswerKind; bot_solvable: boolean;
}
export interface Task extends TaskView { answer_schema: AnswerSchema; }
export interface Attempt {
  id: string; test_id: string; assignment_id?: string;
  student_id: string; started_at: string; finished_at?: string;
}
export interface SubmitResult { is_correct: boolean; answer_id: string; solution?: string[]; }
export interface Forecast {
  subject: string; accuracy: number; primary_estimate: number;
  primary_max: number; test_score: number; note: string;
}
export interface HeatCell { day: string; total: number; correct: number; }
export interface NumberMastery { number: number; total: number; correct: number; avg_time_ms: number; }
export interface WeakSpot extends NumberMastery { accuracy: number; }
export interface MasteryPoint { number: number; week: string; total: number; correct: number; }
export interface DayAnswer {
  answer_id: string; task_id: string; number: number; subject_id: string;
  raw_answer: string; is_correct: boolean; time_spent_ms: number; answered_at: string;
}
export interface AssignmentCard {
  id: string; test_id: string; title: string; kind: TestKind; subject_id: string;
  scheduled_at: string; notified_at?: string; status: string; task_count: number;
}
export interface AttemptSummary {
  id: string; test_id: string; title: string; kind: TestKind; subject_id: string;
  started_at: string; finished_at?: string; total: number; correct: number; time_ms: number;
}
export interface Test {
  id: string; subject_id: string; kind: TestKind; title: string;
  created_by: string; created_at: string;
}
export interface TestDetail { test: Test; tasks: Task[]; }

// --- session token ---
const TOKEN_KEY = "egeism.token";
let currentToken = localStorage.getItem(TOKEN_KEY) || "";
export function setToken(t: string) { currentToken = t; localStorage.setItem(TOKEN_KEY, t); }
export function clearToken() { currentToken = ""; localStorage.removeItem(TOKEN_KEY); }
export function hasToken() { return !!currentToken; }

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) { super(message); this.status = status; }
}

// API origin. Empty (default) = same-origin: the dev server proxies /api and
// prod is served by nginx behind the same host. Set VITE_API_BASE to hit a
// remote API directly (no proxy).
const API_BASE = import.meta.env.VITE_API_BASE ?? "";

// mediaUrl resolves a task media key to a servable URL. Keys are MinIO object
// keys served by GET /api/media/<key>; a full http(s) key is used as-is.
export const mediaUrl = (key: string) =>
  key.startsWith("http") ? key : `${API_BASE}/api/media/${key}`;

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (currentToken) headers["Authorization"] = `Bearer ${currentToken}`;
  const res = await fetch(API_BASE + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  if (!res.ok) {
    let msg = `${res.status}`;
    try { msg = (await res.json()).error || msg; } catch { /* ignore */ }
    throw new ApiError(res.status, msg);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export interface ImportResult { fetched: number; inserted: number; skipped: number; invalid: number; source?: string; }

// uploadTasks posts a JSON/JSONL task file as multipart form-data. Kept separate
// from req() because FormData must set its own Content-Type (boundary).
export async function uploadTasks(file: File, opts: { provider?: string; active?: boolean } = {}): Promise<ImportResult> {
  const fd = new FormData();
  fd.append("file", file);
  if (opts.provider) fd.append("provider", opts.provider);
  if (opts.active) fd.append("status", "active");
  const headers: Record<string, string> = {};
  if (currentToken) headers["Authorization"] = `Bearer ${currentToken}`;
  const res = await fetch(`${API_BASE}/api/admin/tasks/import`, { method: "POST", headers, body: fd });
  if (!res.ok) {
    let msg = `${res.status}`;
    try { msg = (await res.json()).error || msg; } catch { /* ignore */ }
    throw new ApiError(res.status, msg);
  }
  return res.json();
}

export interface AuthResult { token: string; user: User; }

export const api = {
  register: (role: Role, username: string, password: string, name: string) =>
    req<AuthResult>("POST", "/api/auth/register", { role, username, password, name }),
  login: (username: string, password: string) =>
    req<AuthResult>("POST", "/api/auth/login", { username, password }),
  me: () => req<User>("GET", "/api/auth/me"),
  students: () => req<User[]>("GET", "/api/students"),
  subjects: () => req<Subject[]>("GET", "/api/subjects"),
  task: (id: string) => req<TaskView>("GET", `/api/tasks/${id}`),
  tasks: (q: string) => req<TaskView[]>("GET", `/api/tasks${q}`),
  startPractice: (subject: SubjectCode) =>
    req<{ test_id: string; attempt_id: string }>("POST", "/api/practice", { subject }),
  practiceTasks: (subject: SubjectCode, limit: number) =>
    req<TaskView[]>("GET", `/api/practice/tasks?subject=${subject}&limit=${limit}`),
  startAttempt: (test_id: string, assignment_id?: string) =>
    req<Attempt>("POST", "/api/attempts", { test_id, assignment_id }),
  submit: (attemptId: string, task_id: string, raw_answer: string, time_spent_ms: number) =>
    req<SubmitResult>("POST", `/api/attempts/${attemptId}/answers`, { task_id, raw_answer, time_spent_ms }),
  finish: (attemptId: string) => req<Attempt>("POST", `/api/attempts/${attemptId}/finish`),
  attemptAnswers: (attemptId: string) => req<DayAnswer[]>("GET", `/api/attempts/${attemptId}/answers`),

  forecast: (sid: string, subject: SubjectCode) =>
    req<Forecast>("GET", `/api/students/${sid}/stats/forecast?subject=${subject}`),
  heatmap: (sid: string, since?: string) =>
    req<HeatCell[]>("GET", `/api/students/${sid}/stats/heatmap${since ? `?since=${since}` : ""}`),
  mastery: (sid: string, subject: SubjectCode) =>
    req<NumberMastery[]>("GET", `/api/students/${sid}/stats/mastery?subject=${subject}`),
  masterySeries: (sid: string, subject: SubjectCode) =>
    req<MasteryPoint[]>("GET", `/api/students/${sid}/stats/mastery-series?subject=${subject}`),
  weakSpots: (sid: string, subject: SubjectCode) =>
    req<WeakSpot[]>("GET", `/api/students/${sid}/stats/weak-spots?subject=${subject}&min=1&limit=4`),
  day: (sid: string, date: string) =>
    req<DayAnswer[]>("GET", `/api/students/${sid}/stats/day?date=${date}`),
  assignments: (sid: string) => req<AssignmentCard[]>("GET", `/api/students/${sid}/assignments`),
  attempts: (sid: string, limit = 12) =>
    req<AttemptSummary[]>("GET", `/api/students/${sid}/attempts?limit=${limit}`),

  adminTasks: (q: string) => req<Task[]>("GET", `/api/admin/tasks${q}`),
  fetchTasks: (subject: SubjectCode, limit: number, active: boolean) =>
    req<ImportResult>("POST", "/api/admin/tasks/fetch", { subject, limit, active }),
  clearBank: (subject: SubjectCode) =>
    req<{ deleted: number; kept: number }>("DELETE", `/api/admin/tasks?subject=${subject}`),
  setTaskStatus: (id: string, status: TaskStatus) =>
    req<Task>("PATCH", `/api/admin/tasks/${id}/status`, { status }),
  setTaskAnswer: (id: string, answer_schema: AnswerSchema) =>
    req<Task>("PATCH", `/api/admin/tasks/${id}/answer`, { answer_schema }),
  tests: (subject?: SubjectCode) =>
    req<Test[]>("GET", `/api/admin/tests${subject ? `?subject=${subject}` : ""}`),
  testDetail: (id: string) => req<TestDetail>("GET", `/api/admin/tests/${id}`),
  createTest: (subject: SubjectCode, kind: TestKind, title: string) =>
    req<Test>("POST", "/api/admin/tests", { subject, kind, title }),
  deleteTest: (id: string) => req<void>("DELETE", `/api/admin/tests/${id}`),
  renameTest: (id: string, title: string) => req<Test>("PATCH", `/api/admin/tests/${id}`, { title }),
  refetchFormulas: () =>
    req<{ updated: number; scanned: number; by_subject: Record<string, number> }>("POST", "/api/admin/tasks/refetch-formulas"),
  addItem: (testId: string, task_id: string, position: number) =>
    req("POST", `/api/admin/tests/${testId}/items`, { task_id, position }),
  generateVariant: (subject: SubjectCode, kind: TestKind, opts: { number?: number; count?: number; title?: string } = {}) =>
    req<{ test: Test; task_count: number; source: string }>("POST", "/api/admin/tests/generate", { subject, kind, ...opts }),
  createAssignment: (test_id: string, student_id: string, scheduled_at: string) =>
    req("POST", "/api/admin/assignments", { test_id, student_id, scheduled_at }),
};

// --- hooks ---
export const useSubjects = () => useQuery({ queryKey: ["subjects"], queryFn: api.subjects });
export const useStudents = (enabled: boolean) =>
  useQuery({ queryKey: ["students"], queryFn: api.students, enabled });
export const useForecast = (sid: string, s: SubjectCode) =>
  useQuery({ queryKey: ["forecast", sid, s], queryFn: () => api.forecast(sid, s), enabled: !!sid });
export const useHeatmap = (sid: string) =>
  useQuery({ queryKey: ["heatmap", sid], queryFn: () => api.heatmap(sid), enabled: !!sid });
export const useMastery = (sid: string, s: SubjectCode) =>
  useQuery({ queryKey: ["mastery", sid, s], queryFn: () => api.mastery(sid, s), enabled: !!sid });
export const useMasterySeries = (sid: string, s: SubjectCode) =>
  useQuery({ queryKey: ["mastery-series", sid, s], queryFn: () => api.masterySeries(sid, s), enabled: !!sid });
export const useWeakSpots = (sid: string, s: SubjectCode) =>
  useQuery({ queryKey: ["weak", sid, s], queryFn: () => api.weakSpots(sid, s), enabled: !!sid });
export const useAssignments = (sid: string) =>
  useQuery({ queryKey: ["assignments", sid], queryFn: () => api.assignments(sid), enabled: !!sid });
export const useAttempts = (sid: string) =>
  useQuery({ queryKey: ["attempts", sid], queryFn: () => api.attempts(sid), enabled: !!sid });
export const useAdminTasks = (q: string) =>
  useQuery({ queryKey: ["admin-tasks", q], queryFn: () => api.adminTasks(q) });
export const useTests = (subject?: SubjectCode) =>
  useQuery({ queryKey: ["tests", subject], queryFn: () => api.tests(subject) });
export const useTestDetail = (id: string | null) =>
  useQuery({ queryKey: ["test-detail", id], queryFn: () => api.testDetail(id!), enabled: !!id });

export function useInvalidate() {
  const qc = useQueryClient();
  return (key: string) => qc.invalidateQueries({ queryKey: [key] });
}
export { useMutation };
