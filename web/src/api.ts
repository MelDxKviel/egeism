// API client + types + React Query hooks. Talks to the Go API; the session is
// a JWT sent as `Authorization: Bearer` (see setToken / req below).
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

export type Role = "student" | "teacher" | "admin";
export type SubjectCode = "rus" | "math" | "inf" | "soc";
export type AnswerKind = "number" | "string" | "set" | "sequence";
export type TaskStatus = "draft" | "active" | "rejected";
export type TestKind = "classic" | "drill" | "composed";

export interface User {
  id: string; role: Role; name: string; username?: string; telegram_id?: number;
  // Teacher subject scope: set = ведёт один предмет, absent = сверхучитель.
  subject?: SubjectCode;
  is_active: boolean; created_at?: string;
}
export interface ClassRef { id: string; name: string; }
// A roster row: the student plus which of MY classes they're in (teacher view).
export interface StudentSummary extends User { classes: ClassRef[]; }
// Klass = класс (group of students). "Class" is a reserved-ish word in TS/JSX contexts.
export interface Klass {
  id: string; teacher_id: string; name: string; created_at: string;
  member_count: number; teacher_name?: string;
}
export interface ClassDetail { class: Klass; students: User[]; }
export interface ClassNumberStat { number: number; total: number; correct: number; }
// One row of the class overview color grid.
export interface ClassStudentStats {
  student_id: string; name: string; total: number; correct: number;
  by_number: ClassNumberStat[];
}
export interface SubjectActivity { code: SubjectCode; active_tasks: number; answers: number; correct: number; }
export interface PlatformStats {
  students: number; teachers: number; admins: number; inactive_users: number;
  classes: number; tasks: number; active_tasks: number; tests: number;
  assignments: number; attempts: number; answers: number; correct_answers: number;
  answers_7d: number; subjects: SubjectActivity[];
}
// Student profile carries teachers — the full enrollment list: a student may
// have several teachers at once (school + репетитор), including classless ones.
export interface Profile { user: User; classes: Klass[]; teachers?: User[]; students_count?: number; }
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
// One reviewed answer in an attempt: the task's condition + media, the student's
// answer, the verdict, and the correct answer — for the teacher's attempt review.
export interface AttemptReviewItem {
  answer_id: string; task_id: string; number: number; statement: string;
  media: Media[]; answer_kind: AnswerKind; raw_answer: string;
  is_correct: boolean; correct: string[]; time_spent_ms: number; answered_at: string;
}
export interface AssignmentCard {
  id: string; test_id: string; title: string; kind: TestKind; subject_id: string;
  scheduled_at: string; notified_at?: string; status: string; task_count: number;
  // Optional deadline (absent = no deadline, self-paced). The UI marks an
  // assignment overdue when due_at < now and still unsolved, "late" when
  // solved after due_at.
  due_at?: string;
  // Result of the latest finished attempt (the assigned test's history): present
  // only once solved. finished_at is the reliable "was it solved" signal.
  attempt_id?: string; finished_at?: string; correct: number; total: number;
}
export interface AttemptSummary {
  id: string; test_id: string; title: string; kind: TestKind; subject_id: string;
  started_at: string; finished_at?: string; total: number; correct: number; time_ms: number;
}
// In-app notification (the bell): assignment_created goes to the student,
// assignment_done to the teacher who assigned, password_reset_requested to the
// teachers/admins of a user who hit «забыл пароль». Assignment kinds carry the
// assignment/test context; the password kind only the subject user.
export type NotificationKind = "assignment_created" | "assignment_done" | "password_reset_requested";
export interface NotificationItem {
  id: string; kind: NotificationKind; assignment_id: string;
  test_id: string; test_title: string; subject_id: string;
  student_id: string; student_name: string;
  scheduled_at: string; due_at?: string; assignment_status: string;
  subject_user_id?: string; subject_user_name?: string;
  read_at?: string; created_at: string;
}
export interface NotificationsFeed { unread: number; items: NotificationItem[]; }
export interface Test {
  id: string; subject_id: string; kind: TestKind; title: string;
  created_by: string; created_at: string;
}
export interface TestDetail { test: Test; tasks: Task[]; }

// Composed builder: one slot = «N заданий номера number».
export interface VariantSlot { number: number; count: number; }
// Per-номер bank availability (active = usable in a generated variant).
export interface NumberAvailability { number: number; active: number; total: number; }
export interface TaskSummary { subject: SubjectCode; numbers: NumberAvailability[]; }

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

export interface ImportResult { fetched: number; inserted: number; skipped: number; invalid: number; promoted?: number; source?: string; }

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

// downloadTestPDF fetches the printable variant PDF and hands it to the browser
// as a download. Kept separate from req() because the response is a blob, and
// from a plain <a href> because the endpoint needs the Bearer header.
// answers=true appends the teacher's answer-key page — never give that file to
// a student.
export async function downloadTestPDF(id: string, title: string, answers: boolean): Promise<void> {
  const headers: Record<string, string> = {};
  if (currentToken) headers["Authorization"] = `Bearer ${currentToken}`;
  const res = await fetch(`${API_BASE}/api/admin/tests/${id}/export.pdf${answers ? "?answers=1" : ""}`, { headers });
  if (!res.ok) {
    let msg = `${res.status}`;
    try { msg = (await res.json()).error || msg; } catch { /* ignore */ }
    throw new ApiError(res.status, msg);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${title}${answers ? " (с ответами)" : ""}.pdf`;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export interface AuthResult { token: string; user: User; }

export const api = {
  // Self-registration is gone: accounts come from the admin panel or a teacher.
  login: (username: string, password: string) =>
    req<AuthResult>("POST", "/api/auth/login", { username, password }),
  me: () => req<User>("GET", "/api/auth/me"),
  // Password recovery: «забыл пароль» pings the user's teachers/admins; one of
  // them issues a one-hour reset link; the link holder sets a new password.
  forgotPassword: (username: string) =>
    req<{ ok: boolean }>("POST", "/api/auth/forgot-password", { username }),
  peekResetToken: (token: string) =>
    req<{ name: string; expires_at: string }>("GET", `/api/auth/reset-password/${token}`),
  resetPassword: (token: string, password: string) =>
    req<AuthResult>("POST", "/api/auth/reset-password", { token, password }),
  createPasswordResetLink: (userId: string) =>
    req<{ token: string; expires_at: string }>("POST", `/api/users/${userId}/password-reset-link`),
  profile: () => req<Profile>("GET", "/api/profile"),
  telegramLinkCode: () =>
    req<{ code: string; deep_link?: string; expires_at: string }>("POST", "/api/auth/telegram/link-code"),
  // Teacher roster (enrolled students, tagged with class names); scope=all
  // widens to every student on the platform (the add-to-class picker).
  students: (scope?: "mine" | "all") =>
    req<StudentSummary[]>("GET", `/api/students${scope === "all" ? "?scope=all" : ""}`),
  createStudent: (name: string, username: string, password: string, class_id?: string) =>
    req<User>("POST", "/api/students", { name, username, password, class_id }),
  // Enrollment link (m2m — a student may have several teachers): take an
  // existing student onto my roster / drop them from it (and my classes).
  enrollStudent: (studentId: string) => req<User>("POST", `/api/students/${studentId}/enroll`),
  unenrollStudent: (studentId: string) => req<void>("DELETE", `/api/students/${studentId}/enroll`),

  // Classes (teacher).
  classes: () => req<Klass[]>("GET", "/api/classes"),
  createClass: (name: string) => req<Klass>("POST", "/api/classes", { name }),
  classDetail: (id: string) => req<ClassDetail>("GET", `/api/classes/${id}`),
  renameClass: (id: string, name: string) => req<Klass>("PATCH", `/api/classes/${id}`, { name }),
  deleteClass: (id: string) => req<void>("DELETE", `/api/classes/${id}`),
  addClassMember: (id: string, student_id: string) =>
    req<User>("POST", `/api/classes/${id}/members`, { student_id }),
  removeClassMember: (id: string, studentId: string) =>
    req<void>("DELETE", `/api/classes/${id}/members/${studentId}`),
  classOverview: (id: string, subject: SubjectCode) =>
    req<ClassStudentStats[]>("GET", `/api/classes/${id}/overview?subject=${subject}`),

  // Admin panel.
  adminUsers: () => req<User[]>("GET", "/api/admin/users"),
  adminCreateUser: (u: { role: Role; name: string; username: string; password: string; subject?: SubjectCode }) =>
    req<User>("POST", "/api/admin/users", u),
  adminUpdateUser: (id: string, patch: { name?: string; role?: Role; subject?: SubjectCode | ""; is_active?: boolean; password?: string }) =>
    req<User>("PATCH", `/api/admin/users/${id}`, patch),
  adminDeleteUser: (id: string) => req<void>("DELETE", `/api/admin/users/${id}`),
  adminStats: () => req<PlatformStats>("GET", "/api/admin/stats"),
  adminClasses: () => req<Klass[]>("GET", "/api/admin/classes"),
  subjects: () => req<Subject[]>("GET", "/api/subjects"),
  task: (id: string) => req<TaskView>("GET", `/api/tasks/${id}`),
  tasks: (q: string) => req<TaskView[]>("GET", `/api/tasks${q}`),
  startPractice: (subject: SubjectCode) =>
    req<{ test_id: string; attempt_id: string }>("POST", "/api/practice", { subject }),
  practiceTasks: (subject: SubjectCode, limit: number) =>
    req<TaskView[]>("GET", `/api/practice/tasks?subject=${subject}&limit=${limit}`),
  // Student-safe task list of a composed/assigned test (no answers).
  testTasks: (testId: string) => req<TaskView[]>("GET", `/api/tests/${testId}/tasks`),
  startAttempt: (test_id: string, assignment_id?: string) =>
    req<Attempt>("POST", "/api/attempts", { test_id, assignment_id }),
  submit: (attemptId: string, task_id: string, raw_answer: string, time_spent_ms: number) =>
    req<SubmitResult>("POST", `/api/attempts/${attemptId}/answers`, { task_id, raw_answer, time_spent_ms }),
  finish: (attemptId: string) => req<Attempt>("POST", `/api/attempts/${attemptId}/finish`),
  attemptAnswers: (attemptId: string) => req<DayAnswer[]>("GET", `/api/attempts/${attemptId}/answers`),
  attemptReview: (attemptId: string) => req<AttemptReviewItem[]>("GET", `/api/attempts/${attemptId}/review`),

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
  notifications: (limit = 30) => req<NotificationsFeed>("GET", `/api/notifications?limit=${limit}`),
  markNotificationRead: (id: string) => req<void>("POST", `/api/notifications/${id}/read`),
  markAllNotificationsRead: () => req<void>("POST", "/api/notifications/read-all"),

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
  taskSummary: (subject: SubjectCode) =>
    req<TaskSummary>("GET", `/api/admin/tasks/summary?subject=${subject}`),
  generateVariant: (
    subject: SubjectCode,
    kind: TestKind,
    opts: { number?: number; count?: number; title?: string; slots?: VariantSlot[] } = {},
  ) =>
    req<{ test: Test; task_count: number; requested?: number; source: string }>("POST", "/api/admin/tests/generate", { subject, kind, ...opts }),
  // Assign to one student OR fan out to a whole class (exactly one target).
  // individual=true generates every target their own random variant of the
  // picked test (same numbers, random bank tasks) — the anti-cheating mode.
  // due_at is the optional deadline (absent = no deadline); must be after
  // scheduled_at.
  createAssignment: (
    test_id: string,
    target: { student_id?: string; class_id?: string },
    scheduled_at: string,
    opts: { notify?: boolean; individual?: boolean; due_at?: string } = {},
  ) =>
    req<{ created: number }>("POST", "/api/admin/assignments", {
      test_id, ...target, scheduled_at,
      notify: opts.notify ?? true,
      individual: opts.individual ?? false,
      ...(opts.due_at ? { due_at: opts.due_at } : {}),
    }),
};

// --- hooks ---
export const useSubjects = () => useQuery({ queryKey: ["subjects"], queryFn: api.subjects });
export const useStudents = (enabled: boolean, scope: "mine" | "all" = "mine") =>
  useQuery({ queryKey: ["students", scope], queryFn: () => api.students(scope), enabled });
export const useClasses = (enabled: boolean) =>
  useQuery({ queryKey: ["classes"], queryFn: api.classes, enabled });
export const useClassDetail = (id: string | null) =>
  useQuery({ queryKey: ["class-detail", id], queryFn: () => api.classDetail(id!), enabled: !!id });
export const useClassOverview = (id: string | null, s: SubjectCode) =>
  useQuery({ queryKey: ["class-overview", id, s], queryFn: () => api.classOverview(id!, s), enabled: !!id });
export const useProfile = () => useQuery({ queryKey: ["profile"], queryFn: api.profile });
export const useAdminUsers = (enabled: boolean) =>
  useQuery({ queryKey: ["admin-users"], queryFn: api.adminUsers, enabled });
export const useAdminStats = (enabled: boolean) =>
  useQuery({ queryKey: ["admin-stats"], queryFn: api.adminStats, enabled });
export const useAdminClasses = (enabled: boolean) =>
  useQuery({ queryKey: ["admin-classes"], queryFn: api.adminClasses, enabled });
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
// The bell polls so a fresh assignment / completed test shows up without a
// reload (30s is plenty for a 1-teacher-1-student stage). Keyed by the acting
// user id: the cache must not leak across a logout→login on one browser.
export const useNotifications = (uid: string) =>
  useQuery({
    queryKey: ["notifications", uid],
    queryFn: () => api.notifications(),
    refetchInterval: 30_000,
    enabled: !!uid,
  });
export const useAttempts = (sid: string) =>
  useQuery({ queryKey: ["attempts", sid], queryFn: () => api.attempts(sid), enabled: !!sid });
export const useAdminTasks = (q: string) =>
  useQuery({ queryKey: ["admin-tasks", q], queryFn: () => api.adminTasks(q) });
export const useTests = (subject?: SubjectCode) =>
  useQuery({ queryKey: ["tests", subject], queryFn: () => api.tests(subject) });
export const useTaskSummary = (subject: SubjectCode) =>
  useQuery({ queryKey: ["task-summary", subject], queryFn: () => api.taskSummary(subject) });
export const useTestDetail = (id: string | null) =>
  useQuery({ queryKey: ["test-detail", id], queryFn: () => api.testDetail(id!), enabled: !!id });

export function useInvalidate() {
  const qc = useQueryClient();
  return (key: string) => qc.invalidateQueries({ queryKey: [key] });
}
export { useMutation };
