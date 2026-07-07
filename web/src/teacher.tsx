import { useEffect, useMemo, useRef, useState } from "react";
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell, CartesianGrid } from "recharts";
import {
  api, SubjectCode, TestKind, Task, TaskStatus, AnswerSchema, AttemptSummary, AttemptReviewItem,
  StudentSummary, User, uploadTasks, downloadTestPDF,
  useForecast, useHeatmap, useWeakSpots, useMastery, useMasterySeries, useAttempts, useAssignments,
  useAdminTasks, useTests, useTestDetail, useInvalidate, useClasses, useClassDetail, useClassOverview, useStudents,
} from "./api";
import { useApp } from "./state";
import { Card, Label, Pill, Button, Async, Empty, Loading, Modal, PasswordInput, accColor, SUBJECT_TITLES, testTitle, MediaBlock, StatementView, AttemptReviewGrid } from "./ui";
import { ScoreGauge, computeStreak, WeakSpotsList, Section, MasteryChart } from "./charts";
import { StreakBadge, ASSIGNMENT_STATUS_RU } from "./student";
import { deadlineInfo } from "./deadline";
import { Icon } from "./icons";
import { ResetLinkModal } from "./reset";

const SUBJECTS: SubjectCode[] = ["rus", "math", "inf", "soc"];
// Which live source feeds a subject (per CLAUDE.md: openfipi serves информатика,
// РЕШУ/sdamgia the rest). Shown in toasts so the teacher knows where tasks came from.
const SOURCE_TITLE: Record<SubjectCode, string> = {
  rus: "РЕШУ ЕГЭ", math: "РЕШУ ЕГЭ", soc: "РЕШУ ЕГЭ", inf: "открытый банк ФИПИ (openfipi)",
};
const grid = { display: "grid", gap: "var(--gap)", gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))" } as const;

// Builder prefill handoff (set before navigating to the builder). Lets the
// student page's "Прокачать" open the builder already in drill mode, pre-aimed
// at the weak number and 10 tasks — one click from creating the drill variant.
let builderRequest: { kind: TestKind; number?: number; count?: number } | null = null;
export const requestBuilder = (r: NonNullable<typeof builderRequest>) => { builderRequest = r; };

// Detail-page handoffs (same pattern as requestTestView below): set before go().
let viewClassId = "";
export const requestClassView = (id: string) => { viewClassId = id; };
let viewStudent: { id: string; name: string } | null = null;
export const requestStudentView = (id: string, name: string) => { viewStudent = { id, name }; };
// Assign prefill: «Назначить тест» from a student/class page lands pre-targeted.
let assignRequest: { studentId?: string; classId?: string } | null = null;
export const requestAssign = (r: NonNullable<typeof assignRequest>) => { assignRequest = r; };

// useAllowedSubjects respects the teacher's subject role (переработка №3): a
// scoped teacher works in one subject, the сверхучитель in all four.
function useAllowedSubjects(): SubjectCode[] {
  const { user } = useApp();
  return user?.role === "teacher" && user.subject ? [user.subject] : SUBJECTS;
}

function SubjectTabs({ value, onChange }: { value: SubjectCode; onChange: (s: SubjectCode) => void }) {
  const allowed = useAllowedSubjects();
  // A scoped teacher can't sit on a foreign subject (e.g. restored from an
  // older session) — snap to their own.
  useEffect(() => {
    if (!allowed.includes(value)) onChange(allowed[0]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [allowed.join(","), value]);
  if (allowed.length === 1) {
    return <div><Pill tone="accent">{SUBJECT_TITLES[allowed[0]]} · ваш предмет</Pill></div>;
  }
  return (
    <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
      {allowed.map((c) => (
        <button key={c} onClick={() => onChange(c)} style={{
          padding: "8px 16px", borderRadius: 999, fontWeight: 600, fontSize: 14,
          border: "1px solid " + (value === c ? "var(--accent)" : "var(--border-2)"),
          background: value === c ? "var(--accent-soft)" : "transparent",
          color: value === c ? "var(--accent-2)" : "var(--text-2)",
        }}>{SUBJECT_TITLES[c]}</button>
      ))}
    </div>
  );
}

// accuracyPct is the shared "how are they doing" number for color coding.
const accuracyPct = (correct: number, total: number) => (total ? Math.round((correct / total) * 100) : 0);

// ---------- Обзор: классы + ученики (t-dashboard) ----------

// CreateStudentModal — учитель создаёт аккаунт ученика (переработка №2/№6),
// сразу со своей привязкой и, опционально, в один из своих классов.
function CreateStudentModal({ classId, onClose, onDone }: { classId?: string; onClose: () => void; onDone: () => void }) {
  const { showToast } = useApp();
  const classes = useClasses(true);
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [klass, setKlass] = useState(classId ?? "");
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setBusy(true);
    try {
      await api.createStudent(name, username, password, klass || undefined);
      showToast(`Аккаунт создан: ${username}`);
      onDone();
      onClose();
    } catch (e) {
      // Занятый логин почти всегда значит «ученик уже есть на платформе» (у
      // другого учителя) — подскажи взять существующего, а не плодить аккаунты.
      const msg = String((e as Error).message);
      showToast(msg.includes("занят") ? `${msg} — если это твой ученик, возьми его через «Взять ученика»` : msg);
    }
    finally { setBusy(false); }
  };

  return (
    <Modal onClose={onClose} maxWidth={420} title={<><Icon name="user" size={20} /> Новый ученик</>}>
      <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        <label><Label>Имя</Label>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Фамилия Имя" style={{ width: "100%", marginTop: 6 }} /></label>
        <label><Label>Логин</Label>
          <input value={username} onChange={(e) => setUsername(e.target.value)} style={{ width: "100%", marginTop: 6 }} /></label>
        <label><Label>Пароль</Label>
          <PasswordInput value={password} onChange={setPassword} autoComplete="new-password"
            placeholder="минимум 6 символов" style={{ marginTop: 6 }} /></label>
        <label>
          <Label>Класс (необязательно)</Label>
          <select value={klass} onChange={(e) => setKlass(e.target.value)} style={{ width: "100%", marginTop: 6 }}>
            <option value="">Без класса (индивидуально)</option>
            {(classes.data || []).map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
        </label>
        <div style={{ color: "var(--text-3)", fontSize: 12.5 }}>
          Передай логин и пароль ученику — он войдёт на сайте и сможет привязать Telegram.
        </div>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
          <Button variant="ghost" onClick={onClose}>Отмена</Button>
          <Button disabled={busy} onClick={submit}>{busy ? "…" : "Создать"}</Button>
        </div>
      </div>
    </Modal>
  );
}

// EnrollStudentModal — «взять ученика»: у ученика может быть НЕСКОЛЬКО учителей
// (школьный + репетитор), поэтому существующего на платформе ученика берут к
// себе как есть — без класса и без второго аккаунта. Поиск по всей платформе;
// уже свои отфильтрованы.
function EnrollStudentModal({ mine, onClose, onDone }: {
  mine: StudentSummary[]; onClose: () => void; onDone: () => void;
}) {
  const { showToast } = useApp();
  const all = useStudents(true, "all");
  const [q, setQ] = useState("");
  const [creating, setCreating] = useState(false);
  const enrolled = new Set(mine.map((m) => m.id));

  const enroll = async (s: User) => {
    try {
      await api.enrollStudent(s.id);
      showToast(`${s.name} теперь твой ученик`);
      onDone();
    } catch (e) { showToast(String((e as Error).message)); }
  };

  return (
    <>
      <Modal onClose={onClose} maxWidth={460} title="Взять ученика">
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ color: "var(--text-2)", fontSize: 13.5 }}>
            Ученик уже занимается на платформе (например, у другого учителя)? Возьми его к себе —
            аккаунт один, у ученика может быть несколько учителей.
          </div>
          <div style={{ display: "flex", gap: 8 }}>
            <input placeholder="поиск: имя или логин" value={q} onChange={(e) => setQ(e.target.value)} style={{ flex: 1 }} />
            <Button variant="soft" onClick={() => setCreating(true)}>+ Новый</Button>
          </div>
          <Async q={all}>{(list) => {
            const qq = q.trim().toLowerCase();
            const rows = list
              .filter((s) => !enrolled.has(s.id))
              .filter((s) => !qq || s.name.toLowerCase().includes(qq) || (s.username || "").toLowerCase().includes(qq));
            return rows.length === 0
              ? <div style={{ color: "var(--text-2)", fontSize: 14 }}>Свободных учеников не нашлось — создай нового.</div>
              : (
                <div style={{ display: "flex", flexDirection: "column", gap: 8, maxHeight: 320, overflowY: "auto" }}>
                  {rows.map((s) => (
                    <StudentRow key={s.id} s={s} onOpen={() => enroll(s)}
                      right={<span style={{ color: "var(--accent-2)", fontSize: 13, fontWeight: 600 }}>взять</span>} />
                  ))}
                </div>
              );
          }}</Async>
        </div>
      </Modal>
      {creating && (
        <CreateStudentModal onClose={() => setCreating(false)}
          onDone={() => { setCreating(false); onDone(); }} />
      )}
    </>
  );
}

// StudentRow — одна строка ростера: имя, классы, точность быстрого взгляда нет
// (она на странице ученика); клик открывает полную статистику.
function StudentRow({ s, onOpen, right }: { s: StudentSummary | User; onOpen: () => void; right?: React.ReactNode }) {
  const classes = "classes" in s ? s.classes : [];
  return (
    <div onClick={onOpen} style={{
      display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10,
      padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10, cursor: "pointer",
      opacity: s.is_active === false ? 0.55 : 1,
    }}>
      <div style={{ minWidth: 0 }}>
        <div style={{ fontWeight: 600, display: "flex", alignItems: "center", gap: 8 }}>
          {s.name}
          {s.is_active === false && <Pill tone="bad">отключён</Pill>}
        </div>
        <div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>
          {s.username || "—"}{classes.length > 0 ? " · " + classes.map((c) => c.name).join(", ") : ""}
        </div>
      </div>
      {right ?? <Icon name="arrowRight" size={16} />}
    </div>
  );
}

// TeacherDashboard — «Ученики и классы»: все классы учителя + ученики без
// класса (репетиторский случай) + создание класса/ученика (переработка №2).
export function TeacherDashboard() {
  const { go, showToast } = useApp();
  const invalidate = useInvalidate();
  const classes = useClasses(true);
  const students = useStudents(true);
  const [creatingClass, setCreatingClass] = useState(false);
  const [className, setClassName] = useState("");
  const [creatingStudent, setCreatingStudent] = useState(false);
  const [enrolling, setEnrolling] = useState(false);
  const [busy, setBusy] = useState(false);

  const refresh = () => {
    invalidate("classes"); invalidate("students");
    // A student may have been created straight into a class — refresh the
    // class page's roster/grid too (staleTime would otherwise hide them 30s).
    invalidate("class-detail"); invalidate("class-overview");
  };

  const createClass = async () => {
    if (!className.trim()) { showToast("Укажи название класса"); return; }
    setBusy(true);
    try {
      const c = await api.createClass(className.trim());
      showToast(`Класс «${c.name}» создан`);
      setCreatingClass(false); setClassName("");
      refresh();
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
  };

  const openStudent = (s: { id: string; name: string }) => { requestStudentView(s.id, s.name); go("t-student"); };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
        <Button onClick={() => setCreatingClass(true)}>+ Новый класс</Button>
        <Button variant="soft" onClick={() => setCreatingStudent(true)}>+ Аккаунт ученика</Button>
        <Button variant="ghost" onClick={() => setEnrolling(true)}>Взять ученика</Button>
      </div>

      <Section title="Классы">
        <Async q={classes}>{(list) => list.length === 0
          ? <div style={{ color: "var(--text-2)" }}>Классов пока нет. Создай класс и добавь в него учеников — или веди учеников индивидуально.</div>
          : (
            <div style={{ display: "grid", gap: 12, gridTemplateColumns: "repeat(auto-fill, minmax(220px, 1fr))" }}>
              {list.map((c) => (
                <Card key={c.id} onClick={() => { requestClassView(c.id); go("t-class"); }} style={{ cursor: "pointer", padding: 18 }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                    <div style={{
                      width: 40, height: 40, borderRadius: 12, background: "var(--accent-soft)",
                      display: "flex", alignItems: "center", justifyContent: "center", color: "var(--accent-2)",
                    }}><Icon name="overview" size={20} /></div>
                    <div style={{ minWidth: 0 }}>
                      <div style={{ fontWeight: 700, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{c.name}</div>
                      <div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>{c.member_count} уч.</div>
                    </div>
                  </div>
                </Card>
              ))}
            </div>
          )}</Async>
      </Section>

      <Section title="Ученики без класса">
        <Async q={students}>{(list) => {
          const loose = list.filter((s) => s.classes.length === 0);
          return loose.length === 0
            ? <div style={{ color: "var(--text-2)" }}>Все твои ученики распределены по классам.</div>
            : (
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {loose.map((s) => <StudentRow key={s.id} s={s} onOpen={() => openStudent(s)} />)}
              </div>
            );
        }}</Async>
      </Section>

      <Section title="Все мои ученики">
        <Async q={students}>{(list) => list.length === 0
          ? <div style={{ color: "var(--text-2)" }}>Пока нет учеников. Создай аккаунт ученика — или попроси администратора.</div>
          : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {list.map((s) => <StudentRow key={s.id} s={s} onOpen={() => openStudent(s)} />)}
            </div>
          )}</Async>
      </Section>

      {creatingClass && (
        <Modal onClose={() => setCreatingClass(false)} maxWidth={400} title="Новый класс">
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <label><Label>Название</Label>
              <input autoFocus value={className} onChange={(e) => setClassName(e.target.value)}
                onKeyDown={(e) => { if (e.key === "Enter") createClass(); }}
                placeholder="напр. 11А · информатика" style={{ width: "100%", marginTop: 6 }} /></label>
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
              <Button variant="ghost" onClick={() => setCreatingClass(false)}>Отмена</Button>
              <Button disabled={busy} onClick={createClass}>{busy ? "…" : "Создать"}</Button>
            </div>
          </div>
        </Modal>
      )}
      {creatingStudent && <CreateStudentModal onClose={() => setCreatingStudent(false)} onDone={refresh} />}
      {enrolling && (
        <EnrollStudentModal mine={students.data || []}
          onClose={() => setEnrolling(false)} onDone={refresh} />
      )}
    </div>
  );
}

// ---------- Страница класса (t-class): состав + цветовая сетка ----------

// gridCellColor: фон ячейки по точности — учитель сразу видит, кто и какой
// номер проседает (переработка №5). Пустые ячейки нейтральные.
function GridCell({ total, correct }: { total: number; correct: number }) {
  if (!total) {
    return <td className="mono" style={{ textAlign: "center", color: "var(--text-3)" }}>—</td>;
  }
  const pct = accuracyPct(correct, total);
  return (
    <td className="mono" title={`${correct}/${total} верно`} style={{
      textAlign: "center", fontWeight: 700, color: "#fff",
      background: accColor(pct),
    }}>{pct}%</td>
  );
}

// AddMemberModal — добавить ученика в класс: из всех учеников платформы (поиск
// по имени/логину), либо создать нового прямо отсюда.
function AddMemberModal({ classId, members, onClose, onDone }: {
  classId: string; members: User[]; onClose: () => void; onDone: () => void;
}) {
  const { showToast } = useApp();
  const all = useStudents(true, "all");
  const [q, setQ] = useState("");
  const [creating, setCreating] = useState(false);
  const inClass = new Set(members.map((m) => m.id));

  const add = async (s: User) => {
    try {
      await api.addClassMember(classId, s.id);
      showToast(`${s.name} — в классе`);
      onDone();
    } catch (e) { showToast(String((e as Error).message)); }
  };

  return (
    <>
      <Modal onClose={onClose} maxWidth={460} title="Добавить ученика в класс">
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ display: "flex", gap: 8 }}>
            <input placeholder="поиск: имя или логин" value={q} onChange={(e) => setQ(e.target.value)} style={{ flex: 1 }} />
            <Button variant="soft" onClick={() => setCreating(true)}>+ Новый</Button>
          </div>
          <Async q={all}>{(list) => {
            const qq = q.trim().toLowerCase();
            const rows = list
              .filter((s) => !inClass.has(s.id))
              .filter((s) => !qq || s.name.toLowerCase().includes(qq) || (s.username || "").toLowerCase().includes(qq));
            return rows.length === 0
              ? <div style={{ color: "var(--text-2)", fontSize: 14 }}>Свободных учеников не нашлось — создай нового.</div>
              : (
                <div style={{ display: "flex", flexDirection: "column", gap: 8, maxHeight: 320, overflowY: "auto" }}>
                  {rows.map((s) => (
                    <StudentRow key={s.id} s={s} onOpen={() => add(s)}
                      right={<span style={{ color: "var(--accent-2)", fontSize: 13, fontWeight: 600 }}>добавить</span>} />
                  ))}
                </div>
              );
          }}</Async>
        </div>
      </Modal>
      {creating && (
        <CreateStudentModal classId={classId} onClose={() => setCreating(false)}
          onDone={() => { setCreating(false); onDone(); }} />
      )}
    </>
  );
}

// ClassPage — интерфейс класса (переработка №5): состав, добавление/удаление
// учеников и цветовая сетка успеваемости: строки — ученики, столбцы — номера
// заданий; последняя строка — весь класс по номеру.
export function ClassPage() {
  const { go, subject, setSubject, showToast } = useApp();
  const invalidate = useInvalidate();
  const detail = useClassDetail(viewClassId || null);
  const overview = useClassOverview(viewClassId || null, subject);
  const [adding, setAdding] = useState(false);

  const refresh = () => { invalidate("class-detail"); invalidate("class-overview"); invalidate("classes"); invalidate("students"); };

  const removeMember = async (s: User) => {
    if (!window.confirm(`Убрать «${s.name}» из класса? Ученик и его история останутся.`)) return;
    try {
      await api.removeClassMember(viewClassId, s.id);
      showToast(`${s.name} убран из класса`);
      refresh();
    } catch (e) { showToast(String((e as Error).message)); }
  };

  const deleteClass = async (name: string) => {
    if (!window.confirm(`Удалить класс «${name}»? Ученики останутся (без класса).`)) return;
    try {
      await api.deleteClass(viewClassId);
      showToast("Класс удалён");
      invalidate("classes"); invalidate("students");
      go("t-dashboard");
    } catch (e) { showToast(String((e as Error).message)); }
  };

  const openStudent = (s: { id: string; name: string }) => { requestStudentView(s.id, s.name); go("t-student"); };

  if (!viewClassId) {
    return <Empty title="Класс не выбран" action={<Button onClick={() => go("t-dashboard")}>К ученикам</Button>} />;
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <button onClick={() => go("t-dashboard")} style={{
        display: "inline-flex", alignItems: "center", gap: 7, alignSelf: "flex-start",
        background: "transparent", border: "1px solid var(--border-2)",
        borderRadius: 10, padding: "8px 14px", fontSize: 14, color: "var(--text-2)",
      }}><Icon name="arrowLeft" size={16} /> Ко всем ученикам</button>

      <Async q={detail}>{(d) => (
        <>
          <Card>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
              <div>
                <EditableClassTitle id={d.class.id} title={d.class.name} onRenamed={refresh} />
                <div className="mono" style={{ color: "var(--text-3)", fontSize: 13, marginTop: 4 }}>{d.students.length} уч.</div>
              </div>
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                <Button variant="soft" onClick={() => setAdding(true)}>+ Ученик</Button>
                <Button variant="ghost" onClick={() => { requestAssign({ classId: d.class.id }); go("t-assign"); }}>Назначить тест классу</Button>
                <button onClick={() => deleteClass(d.class.name)} title="Удалить класс" style={{
                  display: "inline-flex", alignItems: "center", background: "transparent",
                  border: "1px solid var(--bad)", color: "var(--bad)", borderRadius: 10, padding: "8px 12px",
                }}><Icon name="trash" size={15} /></button>
              </div>
            </div>
          </Card>

          <Section title="Успеваемость класса">
            <SubjectTabs value={subject} onChange={setSubject} />
            <div style={{ marginTop: 12 }}>
              <Async q={overview}>{(rows) => <ClassGrid rows={rows} onOpen={openStudent} />}</Async>
            </div>
            <div className="mono" style={{ color: "var(--text-3)", fontSize: 12, marginTop: 8 }}>
              Строки — ученики, столбцы — номера заданий. Красное — проседает, зелёное — уверенно. Клик по ученику — полная статистика.
            </div>
          </Section>

          <Section title="Состав класса">
            {d.students.length === 0
              ? <div style={{ color: "var(--text-2)" }}>В классе пока пусто — добавь учеников.</div>
              : (
                <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                  {d.students.map((s) => (
                    <StudentRow key={s.id} s={s} onOpen={() => openStudent(s)}
                      right={
                        <button onClick={(e) => { e.stopPropagation(); removeMember(s); }} title="Убрать из класса" style={{
                          display: "inline-flex", alignItems: "center", background: "transparent",
                          border: "none", color: "var(--text-3)", padding: 4,
                        }}><Icon name="close" size={16} /></button>
                      } />
                  ))}
                </div>
              )}
          </Section>

          {adding && (
            <AddMemberModal classId={d.class.id} members={d.students}
              onClose={() => setAdding(false)} onDone={refresh} />
          )}
        </>
      )}</Async>
    </div>
  );
}

// ClassGrid — сама цветовая сетка. numbers = объединение номеров всех учеников;
// итоговая строка агрегирует класс по номеру (что проседает «в общем»).
function ClassGrid({ rows, onOpen }: {
  rows: import("./api").ClassStudentStats[];
  onOpen: (s: { id: string; name: string }) => void;
}) {
  const numbers = useMemo(() => {
    const set = new Set<number>();
    rows.forEach((r) => r.by_number.forEach((n) => set.add(n.number)));
    return [...set].sort((a, b) => a - b);
  }, [rows]);

  if (rows.length === 0) return <div style={{ color: "var(--text-2)" }}>В классе пока нет учеников.</div>;
  if (numbers.length === 0) return <div style={{ color: "var(--text-2)" }}>По этому предмету ученики ещё не решали.</div>;

  const byNum = (r: import("./api").ClassStudentStats, n: number) =>
    r.by_number.find((x) => x.number === n) ?? { number: n, total: 0, correct: 0 };
  const classTotal = numbers.map((n) => rows.reduce(
    (acc, r) => { const c = byNum(r, n); return { total: acc.total + c.total, correct: acc.correct + c.correct }; },
    { total: 0, correct: 0 },
  ));

  return (
    <div style={{ overflowX: "auto" }}>
      <table className="stmt-table" style={{ width: "100%" }}>
        <thead>
          <tr>
            <th>Ученик</th>
            {numbers.map((n) => <th key={n} className="mono">№{n}</th>)}
            <th className="mono">Итого</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => {
            const pct = accuracyPct(r.correct, r.total);
            return (
              <tr key={r.student_id}>
                <th>
                  <button onClick={() => onOpen({ id: r.student_id, name: r.name })} style={{
                    background: "none", border: "none", padding: 0, cursor: "pointer",
                    color: "var(--text)", fontWeight: 600, display: "inline-flex", alignItems: "center", gap: 7,
                  }}>
                    <span style={{ width: 9, height: 9, borderRadius: 999, background: r.total ? accColor(pct) : "var(--border-2)", flex: "none" }} />
                    {r.name}
                  </button>
                </th>
                {numbers.map((n) => { const c = byNum(r, n); return <GridCell key={n} total={c.total} correct={c.correct} />; })}
                <td className="mono" style={{ textAlign: "center", fontWeight: 700, color: r.total ? accColor(pct) : "var(--text-3)" }}>
                  {r.total ? `${pct}%` : "—"}
                </td>
              </tr>
            );
          })}
          <tr>
            <th>Весь класс</th>
            {classTotal.map((c, i) => <GridCell key={numbers[i]} total={c.total} correct={c.correct} />)}
            <td className="mono" style={{ textAlign: "center", fontWeight: 700 }}>
              {(() => {
                const t = classTotal.reduce((a, c) => ({ total: a.total + c.total, correct: a.correct + c.correct }), { total: 0, correct: 0 });
                return t.total ? <span style={{ color: accColor(accuracyPct(t.correct, t.total)) }}>{accuracyPct(t.correct, t.total)}%</span> : "—";
              })()}
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  );
}

// EditableClassTitle — переименование класса по клику (как у тестов).
function EditableClassTitle({ id, title, onRenamed }: { id: string; title: string; onRenamed: () => void }) {
  const { showToast } = useApp();
  const [editing, setEditing] = useState(false);
  const [val, setVal] = useState(title);
  useEffect(() => { setVal(title); }, [title]);

  const save = async () => {
    setEditing(false);
    const next = val.trim();
    if (!next || next === title) { setVal(title); return; }
    try {
      await api.renameClass(id, next);
      showToast("Класс переименован");
      onRenamed();
    } catch (e) { showToast(String((e as Error).message)); setVal(title); }
  };

  if (editing) {
    return (
      <input autoFocus value={val} onChange={(e) => setVal(e.target.value)} onBlur={save}
        onKeyDown={(e) => { if (e.key === "Enter") save(); else if (e.key === "Escape") { setVal(title); setEditing(false); } }}
        style={{ fontWeight: 700, fontSize: 18, width: "100%", maxWidth: 420 }} />
    );
  }
  return (
    <button onClick={() => setEditing(true)} title="Нажми, чтобы переименовать класс"
      style={{ display: "inline-flex", alignItems: "center", gap: 8, background: "transparent", border: "none", padding: 0, cursor: "text", color: "var(--text)", fontWeight: 700, fontSize: 18 }}>
      {title}
      <Icon name="pencil" size={15} />
    </button>
  );
}

// ---------- Страница ученика (t-student): полная статистика ----------

// StudentStatsPage — статистика одного ученика для учителя (переработка №4):
// прогноз, слабые места, успешность по номерам, свежие попытки с разбором и
// назначенные тесты. Это бывший обзор — теперь на каждого ученика свой.
export function StudentStatsPage() {
  const { subject, setSubject, go, showToast } = useApp();
  const invalidate = useInvalidate();
  const student = useRef(viewStudent).current;
  const sid = student?.id ?? "";
  const forecast = useForecast(sid, subject);
  const heat = useHeatmap(sid);
  const weak = useWeakSpots(sid, subject);
  const mastery = useMastery(sid, subject);
  const series = useMasterySeries(sid, subject);
  const attempts = useAttempts(sid);
  const assignments = useAssignments(sid);
  const [open, setOpen] = useState<number | null>(null);
  const [review, setReview] = useState<{ title: string; items: AttemptReviewItem[] } | null>(null);
  const [resetOpen, setResetOpen] = useState(false);

  if (!student) {
    return <Empty title="Ученик не выбран" action={<Button onClick={() => go("t-dashboard")}>К ученикам</Button>} />;
  }

  // Open a solved attempt (even a free-practice "Свободное решение" one) as its
  // reviewable variant: each task's condition + correct answer, plus the
  // student's answer and verdict.
  const openAttempt = async (a: AttemptSummary) => {
    try { const items = await api.attemptReview(a.id); setReview({ title: testTitle(a.title), items }); }
    catch { setReview({ title: testTitle(a.title), items: [] }); }
  };
  const toDrill = (number?: number) => { requestBuilder({ kind: "drill", number, count: 10 }); go("t-builder"); };

  // «Отчислить» разрывает только СВОЮ связь с учеником: аккаунт, история и
  // другие учителя (у ученика их может быть несколько) не трогаются.
  const unenroll = async () => {
    if (!window.confirm(`Отчислить «${student.name}»? Ученик уйдёт из твоего списка и твоих классов; аккаунт, история решений и другие его учителя останутся.`)) return;
    try {
      await api.unenrollStudent(student.id);
      showToast(`${student.name} отчислен — аккаунт и история сохранены`);
      invalidate("students"); invalidate("classes"); invalidate("class-detail"); invalidate("class-overview");
      go("t-dashboard");
    } catch (e) { showToast(String((e as Error).message)); }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <button onClick={() => go("t-dashboard")} style={{
          display: "inline-flex", alignItems: "center", gap: 7,
          background: "transparent", border: "1px solid var(--border-2)",
          borderRadius: 10, padding: "8px 14px", fontSize: 14, color: "var(--text-2)",
        }}><Icon name="arrowLeft" size={16} /> Ко всем ученикам</button>
        <div style={{ fontWeight: 800, fontSize: 18 }}>{student.name}</div>
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <Button variant="ghost" onClick={() => setResetOpen(true)}>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 7 }}>
              <Icon name="key" size={15} /> Сбросить пароль
            </span>
          </Button>
          <Button variant="soft" onClick={() => { requestAssign({ studentId: student.id }); go("t-assign"); }}>Назначить тест</Button>
          <button onClick={unenroll} title="Убрать из моих учеников (аккаунт и история останутся)" style={{
            display: "inline-flex", alignItems: "center", gap: 7, background: "transparent",
            border: "1px solid var(--bad)", color: "var(--bad)", borderRadius: 10, padding: "8px 14px", fontSize: 14,
          }}><Icon name="close" size={15} /> Отчислить</button>
        </div>
      </div>

      <SubjectTabs value={subject} onChange={setSubject} />
      <div style={grid}>
        <Card>
          <Label>Прогноз балла · {SUBJECT_TITLES[subject]}</Label>
          <Async q={forecast}>{(f) => (
            <div style={{ display: "flex", flexDirection: "column", alignItems: "center", marginTop: 8 }}>
              <ScoreGauge score={f.test_score} />
              <div className="mono" style={{ color: "var(--text-2)", fontSize: 13, marginTop: 4 }}>
                {f.primary_estimate} из {f.primary_max} первичных · точность {Math.round(f.accuracy * 100)}%
              </div>
              <div style={{ color: "var(--text-2)", fontSize: 13, marginTop: 10, textAlign: "center" }}>{f.note}</div>
              <div style={{ marginTop: 12 }}>
                <Async q={heat}>{(h) => <Pill tone="accent"><StreakBadge>{computeStreak(h)} дней подряд</StreakBadge></Pill>}</Async>
              </div>
            </div>
          )}</Async>
        </Card>

        <Section title="Слабые места" right={<Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => toDrill()}>Собрать дрилл</Button>}>
          <Async q={weak}>{(w) => <WeakSpotsList spots={w} onDrill={(n) => toDrill(n)} />}</Async>
        </Section>
      </div>

      <Section title="Успешность по номерам">
        <Async q={mastery}>{(rows) => rows.length === 0 ? <div style={{ color: "var(--text-2)" }}>Нет данных.</div> : (
          <ResponsiveContainer width="100%" height={260}>
            <BarChart data={rows.map((r) => ({ number: r.number, label: `№${r.number}`, acc: r.total ? Math.round((r.correct / r.total) * 100) : 0 }))} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
              <CartesianGrid stroke="var(--border)" vertical={false} />
              <XAxis dataKey="label" stroke="var(--text-3)" fontSize={10} tickLine={false} interval={0} angle={-30} textAnchor="end" height={44} />
              <YAxis domain={[0, 100]} stroke="var(--text-3)" fontSize={11} tickLine={false} />
              <Tooltip contentStyle={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 10, fontSize: 12 }} />
              <Bar dataKey="acc" minPointSize={2} radius={[4, 4, 0, 0]} cursor="pointer" onClick={(d: any) => { if (d && typeof d.number === "number") setOpen(d.number); }}>
                {rows.map((r) => { const p = r.total ? (r.correct / r.total) * 100 : 0; return <Cell key={r.number} fill={accColor(p)} />; })}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        )}</Async>
        <div className="mono" style={{ color: "var(--text-3)", fontSize: 12, marginTop: 8 }}>Кликни по столбцу, чтобы увидеть, как менялась прогрессия по номеру.</div>
      </Section>

      <Section title="Назначенные тесты">
        <Async q={assignments}>{(list) => list.length === 0 ? <div style={{ color: "var(--text-2)" }}>Пока ничего не назначено.</div> : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {list.map((a) => {
              const solved = !!a.finished_at;
              const pct = a.total ? Math.round((Number(a.correct) / Number(a.total)) * 100) : 0;
              const dl = deadlineInfo(a);
              return (
                <div key={a.id} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10 }}>
                  <div>
                    <div style={{ fontWeight: 600, display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                      {testTitle(a.title)}
                      {dl.pill && <Pill tone={dl.pill.tone}>{dl.pill.label}</Pill>}
                    </div>
                    <div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>
                      {new Date(a.scheduled_at).toLocaleString("ru")}
                      {solved && a.finished_at ? ` · решён ${new Date(a.finished_at).toLocaleString("ru")}` : dl.kind === "none" ? ` · ${ASSIGNMENT_STATUS_RU[a.status] || a.status}` : ` · ${dl.text}`}
                    </div>
                  </div>
                  {solved && <span className="mono" style={{ color: accColor(pct), fontWeight: 700 }}>{a.correct}/{a.total}</span>}
                </div>
              );
            })}
          </div>
        )}</Async>
      </Section>

      <Section title="Дольше всего решает">
        <Async q={mastery}>{(rows) => {
          const top = [...rows].sort((a, b) => b.avg_time_ms - a.avg_time_ms).slice(0, 5);
          return top.length === 0 ? <div style={{ color: "var(--text-2)" }}>Нет данных.</div> : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {top.map((r) => (
                <div key={r.number} style={{ display: "flex", justifyContent: "space-between", padding: "8px 12px", background: "var(--surface-2)", borderRadius: 10 }}>
                  <span className="mono">№{r.number}</span>
                  <span className="mono" style={{ color: "var(--warn)" }}>{Math.round(r.avg_time_ms / 1000)} с</span>
                </div>
              ))}
            </div>
          );
        }}</Async>
      </Section>

      <Section title="Свежие попытки">
        <Async q={attempts}>{(list) => list.length === 0 ? <div style={{ color: "var(--text-2)" }}>Ученик ещё не решал.</div> : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {list.map((a) => (
              <div key={a.id} onClick={() => openAttempt(a)} title="Открыть решённый вариант"
                style={{ display: "flex", justifyContent: "space-between", padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10, cursor: "pointer" }}>
                <div><div style={{ fontWeight: 600 }}>{testTitle(a.title)}</div><div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>{new Date(a.started_at).toLocaleString("ru")}</div></div>
                <span className="mono" style={{ color: accColor(a.total ? (Number(a.correct) / Number(a.total)) * 100 : 0), fontWeight: 700 }}>{a.correct}/{a.total}</span>
              </div>
            ))}
          </div>
        )}</Async>
      </Section>

      {open !== null && (
        <Modal onClose={() => setOpen(null)} title={`Задание №${open} · динамика`}>
          <MasteryChart points={(series.data || []).filter((p) => p.number === open)} />
        </Modal>
      )}

      {review && (
        <Modal onClose={() => setReview(null)} title={`Разбор · ${review.title}`} maxWidth="min(1200px, 96vw)">
          <AttemptReviewGrid items={review.items} />
        </Modal>
      )}

      {resetOpen && <ResetLinkModal user={student} onClose={() => setResetOpen(false)} />}
    </div>
  );
}

// ---------- Test builder (with random-variant generation) ----------
// Selected test to view on the standalone test page (set before navigating).
let viewTestId = "";
export const requestTestView = (id: string) => { viewTestId = id; };

export function Builder() {
  const { subject, setSubject, showToast, go } = useApp();
  const invalidate = useInvalidate();
  // Prefill from the student page's "Прокачать": drill mode, aimed at a weak number.
  const prefill = useRef(builderRequest).current;
  useEffect(() => { builderRequest = null; }, []);
  const [kind, setKind] = useState<TestKind>(prefill?.kind ?? "classic");
  const [number, setNumber] = useState(prefill?.number ?? 1);
  const [count, setCount] = useState(prefill?.count ?? 10);
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState("");
  const tests = useTests(subject);

  const generate = async () => {
    setBusy(true);
    try {
      const res = await api.generateVariant(subject, kind, kind === "drill" ? { number, count } : {});
      // A drill can come out shorter than asked: the bank + source ran dry for
      // that номер. Say so instead of silently handing over a smaller variant.
      const short = kind === "drill" && res.task_count < count
        ? ` (просили ${count} — активных заданий №${number} пока столько; попробуй собрать ещё раз)`
        : "";
      showToast(`Вариант собран: ${res.task_count} задач${short} · задания с «${SOURCE_TITLE[subject]}» добавлены в банк`);
      invalidate("tests");
      invalidate("admin-tasks");
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
  };

  const del = async (id: string, title: string) => {
    if (!window.confirm(`Удалить тест «${testTitle(title)}»? Это действие необратимо.`)) return;
    setDeleting(id);
    try {
      await api.deleteTest(id);
      showToast("Тест удалён");
      invalidate("tests");
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setDeleting(""); }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <SubjectTabs value={subject} onChange={setSubject} />
      <Section title="Собрать вариант в один клик">
        <div style={{ display: "flex", gap: 8, marginBottom: 14, flexWrap: "wrap" }}>
          <ChoiceTab active={kind === "classic"} onClick={() => setKind("classic")}>Классический (по одному на номер)</ChoiceTab>
          <ChoiceTab active={kind === "drill"} onClick={() => setKind("drill")}>Дрилл (N одного номера)</ChoiceTab>
        </div>
        {kind === "drill" && (
          <div style={{ display: "flex", gap: 12, marginBottom: 14, alignItems: "center" }}>
            <label className="mono" style={{ fontSize: 13, color: "var(--text-2)" }}>Номер
              <input type="number" min={1} value={number} onChange={(e) => setNumber(+e.target.value)} style={{ width: 70, marginLeft: 8 }} /></label>
            <label className="mono" style={{ fontSize: 13, color: "var(--text-2)" }}>Сколько
              <input type="number" min={1} value={count} onChange={(e) => setCount(+e.target.value)} style={{ width: 70, marginLeft: 8 }} /></label>
          </div>
        )}
        <div style={{ color: "var(--text-2)", fontSize: 14, marginBottom: 14 }}>
          Сам подтянет нужные задания с источника ({SOURCE_TITLE[subject]}), сохранит их в банк
          и соберёт вариант. То есть тесты наполняют банк, а не наоборот. Может занять несколько секунд.
        </div>
        <Button onClick={generate} disabled={busy}>{busy ? "Собираю…" : "Собрать вариант"}</Button>
      </Section>

      <Section title="Готовые тесты">
        <Async q={tests}>{(list) => list.filter((t) => t.title !== "__practice__").length === 0 ? <div style={{ color: "var(--text-2)" }}>Пока нет тестов.</div> : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {list.filter((t) => t.title !== "__practice__").map((t) => (
              <div key={t.id} onClick={() => { requestTestView(t.id); go("t-test"); }} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10, cursor: "pointer" }}>
                <div><div style={{ fontWeight: 600 }}>{t.title}</div><div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>{new Date(t.created_at).toLocaleString("ru", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" })}</div></div>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <Pill>{t.kind}</Pill>
                  <button onClick={(e) => { e.stopPropagation(); del(t.id, t.title); }} disabled={deleting === t.id}
                    title="Удалить тест" aria-label="Удалить тест"
                    style={{ display: "inline-flex", alignItems: "center", background: "transparent", border: "none", color: "var(--text-3)", padding: 4, opacity: deleting === t.id ? 0.4 : 1 }}>
                    <Icon name="trash" size={16} />
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}</Async>
      </Section>
    </div>
  );
}

// EditableTitle renames a variant in place: click the name to turn it into an
// input, Enter/blur saves (Esc cancels). No prompt dialog — you rename the open
// variant right where you're looking at it.
function EditableTitle({ id, title }: { id: string; title: string }) {
  const { showToast } = useApp();
  const invalidate = useInvalidate();
  const [editing, setEditing] = useState(false);
  const [val, setVal] = useState(title);
  useEffect(() => { setVal(title); }, [title]);

  const save = async () => {
    setEditing(false);
    const next = val.trim();
    if (!next || next === title) { setVal(title); return; }
    try {
      await api.renameTest(id, next);
      showToast("Вариант переименован");
      invalidate("tests");
      invalidate("test-detail");
    } catch (e) { showToast(String((e as Error).message)); setVal(title); }
  };

  if (editing) {
    return (
      <input autoFocus value={val} onChange={(e) => setVal(e.target.value)} onBlur={save}
        onKeyDown={(e) => { if (e.key === "Enter") save(); else if (e.key === "Escape") { setVal(title); setEditing(false); } }}
        style={{ fontWeight: 700, fontSize: 18, width: "100%", maxWidth: 420 }} />
    );
  }
  return (
    <button onClick={() => setEditing(true)} title="Нажми, чтобы переименовать вариант"
      style={{ display: "inline-flex", alignItems: "center", gap: 8, background: "transparent", border: "none", padding: 0, cursor: "text", color: "var(--text)", fontWeight: 700, fontSize: 18 }}>
      {testTitle(title)}
      <Icon name="pencil" size={15} />
    </button>
  );
}

// PdfExportButtons downloads the printable variant: a clean copy for the
// student and one with the answer-key page for the teacher.
function PdfExportButtons({ id, title }: { id: string; title: string }) {
  const { showToast } = useApp();
  const [busy, setBusy] = useState(false);
  const dl = async (answers: boolean) => {
    setBusy(true);
    try { await downloadTestPDF(id, testTitle(title), answers); }
    catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
  };
  const style = {
    display: "inline-flex", alignItems: "center", gap: 7,
    background: "transparent", border: "1px solid var(--border-2)",
    borderRadius: 10, padding: "8px 14px", fontSize: 14, color: "var(--text-2)",
    opacity: busy ? 0.6 : 1,
  } as const;
  return (
    <div style={{ display: "flex", gap: 10, flexWrap: "wrap", marginTop: 12 }}>
      <button disabled={busy} onClick={() => dl(false)} style={style}>
        <Icon name="download" size={16} /> PDF для ученика
      </button>
      <button disabled={busy} onClick={() => dl(true)} style={style}>
        <Icon name="download" size={16} /> PDF с ответами
      </button>
    </div>
  );
}

// TestDetailPage is a standalone page showing a composed test's tasks WITH the
// correct answers, so the teacher can review a variant before assigning.
export function TestDetailPage() {
  const { go } = useApp();
  const q = useTestDetail(viewTestId || null);
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <button onClick={() => go("t-builder")} style={{
        display: "inline-flex", alignItems: "center", gap: 7, alignSelf: "flex-start",
        background: "transparent", border: "1px solid var(--border-2)",
        borderRadius: 10, padding: "8px 14px", fontSize: 14, color: "var(--text-2)",
      }}><Icon name="arrowLeft" size={16} /> К тестам</button>
      <Async q={q}>{(d) => (
        <>
          <Card>
            <EditableTitle id={d.test.id} title={d.test.title} />
            <div className="mono" style={{ color: "var(--text-3)", fontSize: 13, marginTop: 4 }}>{d.test.kind} · {d.tasks.length} задач</div>
            {d.tasks.length > 0 && <PdfExportButtons id={d.test.id} title={d.test.title} />}
          </Card>
          {d.tasks.length === 0 ? <Empty title="В тесте пока нет заданий" /> : (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              {d.tasks.map((t) => (
                <Card key={t.id}>
                  <div style={{ display: "flex", gap: 8, marginBottom: 8 }}>
                    <Pill tone="neutral">№{t.number}</Pill>
                    <Pill>{t.answer_schema.type}</Pill>
                  </div>
                  <StatementView text={t.statement} media={t.media} style={{ fontSize: 15, lineHeight: 1.45, marginBottom: 8 }} />
                  <MediaBlock media={t.media} />
                  <div className="mono" style={{ fontSize: 14, background: "var(--surface-2)", borderRadius: 8, padding: "8px 12px", display: "inline-block" }}>
                    <span style={{ color: "var(--text-3)" }}>ответ: </span>
                    <b style={{ color: "var(--accent-2)" }}>{t.answer_schema.correct.join(" / ")}</b>
                  </div>
                </Card>
              ))}
            </div>
          )}
        </>
      )}</Async>
    </div>
  );
}

// ---------- Assign ----------

// toLocalInput formats a Date as the local "YYYY-MM-DDTHH:MM" string that an
// <input type="datetime-local"> expects (the preset chips build deadline values
// from it).
function toLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// Assign — назначение теста ученику ИЛИ целому классу (переработка №2): при
// назначении на класс сервер создаёт назначение каждому участнику.
export function Assign() {
  const { subject, setSubject, showToast } = useApp();
  const tests = useTests(subject);
  const classes = useClasses(true);
  const students = useStudents(true);
  const prefill = useRef(assignRequest).current;
  useEffect(() => { assignRequest = null; }, []);
  const [mode, setMode] = useState<"student" | "class">(prefill?.classId ? "class" : "student");
  const [studentId, setStudentId] = useState(prefill?.studentId ?? "");
  const [classId, setClassId] = useState(prefill?.classId ?? "");
  const [testId, setTestId] = useState("");
  const [when, setWhen] = useState("");
  const [due, setDue] = useState("");
  const [notify, setNotify] = useState(true);
  // Каждому свой вариант (анти-списывание): по умолчанию ВКЛ для класса —
  // одинаковый тест классу почти гарантирует обмен ответами.
  const [individual, setIndividual] = useState(true);
  const [busy, setBusy] = useState(false);
  const invalidate = useInvalidate();
  // Tests are per subject: switching the tab must drop the picked test, or
  // submit would silently assign a test the UI no longer shows as selected.
  useEffect(() => { setTestId(""); }, [subject]);

  const submit = async () => {
    const target = mode === "class" ? { class_id: classId } : { student_id: studentId };
    if (mode === "class" && !classId) { showToast("Выбери класс"); return; }
    if (mode === "student" && !studentId) { showToast("Выбери ученика"); return; }
    if (!testId) { showToast("Выбери тест"); return; }
    if (!when) { showToast("Укажи время"); return; }
    const dueISO = due ? new Date(due).toISOString() : undefined;
    if (dueISO && dueISO <= new Date(when).toISOString()) {
      showToast("Срок сдачи должен быть позже времени назначения");
      return;
    }
    setBusy(true);
    try {
      const indiv = mode === "class" && individual;
      const r = await api.createAssignment(testId, target, new Date(when).toISOString(), { notify, individual: indiv, due_at: dueISO });
      const who = mode === "class" ? `классу (${r.created} уч.${indiv ? ", у каждого свой вариант" : ""})` : "ученику";
      showToast(notify ? `Назначено ${who} · уведомления в Telegram запланированы` : `Назначено ${who} · без уведомлений`);
      invalidate("assignments"); // the student pages' «Назначенные тесты» feed
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
  };

  return (
    <div style={{ maxWidth: 560, display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <SubjectTabs value={subject} onChange={setSubject} />
      <Section title="Назначить тест">
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div>
            <Label>Кому</Label>
            <div style={{ display: "flex", gap: 6, marginTop: 6 }}>
              <ChoiceTab active={mode === "student"} onClick={() => setMode("student")}>Ученику</ChoiceTab>
              <ChoiceTab active={mode === "class"} onClick={() => setMode("class")}>Классу</ChoiceTab>
            </div>
          </div>
          {mode === "student" ? (
            <label>
              <Label>Ученик</Label>
              <Async q={students}>{(list) => (
                <select value={studentId} onChange={(e) => setStudentId(e.target.value)} style={{ width: "100%", marginTop: 6 }}>
                  <option value="">— выбери ученика —</option>
                  {list.map((s) => (
                    <option key={s.id} value={s.id}>
                      {s.name}{s.classes.length > 0 ? ` (${s.classes.map((c) => c.name).join(", ")})` : ""}
                    </option>
                  ))}
                </select>
              )}</Async>
            </label>
          ) : (
            <label>
              <Label>Класс</Label>
              <Async q={classes}>{(list) => (
                <select value={classId} onChange={(e) => setClassId(e.target.value)} style={{ width: "100%", marginTop: 6 }}>
                  <option value="">— выбери класс —</option>
                  {list.map((c) => <option key={c.id} value={c.id}>{c.name} · {c.member_count} уч.</option>)}
                </select>
              )}</Async>
            </label>
          )}
          <label>
            <Label>Тест</Label>
            <Async q={tests}>{(list) => (
              <select value={testId} onChange={(e) => setTestId(e.target.value)} style={{ width: "100%", marginTop: 6 }}>
                <option value="">— выбери тест —</option>
                {list.filter((t) => t.title !== "__practice__").map((t) => <option key={t.id} value={t.id}>{t.title}</option>)}
              </select>
            )}</Async>
          </label>
          <label><Label>Когда</Label>
            <input type="datetime-local" value={when} onChange={(e) => setWhen(e.target.value)} style={{ width: "100%", marginTop: 6 }} /></label>
          <div>
            <Label>Срок сдачи <span style={{ textTransform: "none", letterSpacing: 0 }}>· необязательно</span></Label>
            <input type="datetime-local" value={due} onChange={(e) => setDue(e.target.value)} style={{ width: "100%", marginTop: 6 }} />
            <div style={{ display: "flex", gap: 6, marginTop: 8, flexWrap: "wrap" }}>
              {[["+1 день", 1], ["+3 дня", 3], ["+1 неделя", 7]].map(([label, days]) => (
                <button key={label as string} type="button" onClick={() => {
                  const base = when ? new Date(when) : new Date();
                  base.setDate(base.getDate() + (days as number));
                  setDue(toLocalInput(base));
                }} style={{
                  background: "var(--surface-2)", border: "1px solid var(--border-2)", color: "var(--text-2)",
                  borderRadius: 999, padding: "4px 12px", fontSize: 12.5, cursor: "pointer",
                }}>{label}</button>
              ))}
              {due && (
                <button type="button" onClick={() => setDue("")} style={{
                  background: "transparent", border: "1px solid var(--border-2)", color: "var(--text-3)",
                  borderRadius: 999, padding: "4px 12px", fontSize: 12.5, cursor: "pointer",
                }}>без срока</button>
              )}
            </div>
            <div style={{ color: "var(--text-3)", fontSize: 12.5, marginTop: 6 }}>
              Без срока ученик решает в любое время. Со сроком — просроченные задания подсвечиваются у ученика и в твоём списке.
            </div>
          </div>
          {mode === "class" && (
            <label style={{ display: "flex", alignItems: "flex-start", gap: 10, fontSize: 14 }}>
              <input type="checkbox" checked={individual} onChange={(e) => setIndividual(e.target.checked)} style={{ width: "auto", marginTop: 3 }} />
              <span>
                Каждому свой вариант
                <span style={{ display: "block", color: "var(--text-3)", fontSize: 12.5 }}>
                  та же структура номеров, но задания у всех разные (из банка) — чтобы не списывали
                </span>
              </span>
            </label>
          )}
          <label style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 14 }}>
            <input type="checkbox" checked={notify} onChange={(e) => setNotify(e.target.checked)} style={{ width: "auto" }} />
            Уведомить в Telegram
          </label>
          <div style={{ display: "flex", gap: 10 }}>
            <Button onClick={submit} disabled={busy}>{busy ? "…" : "Назначить"}</Button>
          </div>
        </div>
      </Section>
    </div>
  );
}

// ---------- Task bank (curation) ----------
export function Bank() {
  const { subject, setSubject, showToast } = useApp();
  const invalidate = useInvalidate();
  const [status, setStatus] = useState<TaskStatus | "">("");
  const q = `?subject=${subject}${status ? `&status=${status}` : ""}&limit=200`;
  const tasks = useAdminTasks(q);

  const refresh = () => invalidate("admin-tasks");
  const setTaskStatus = async (id: string, s: TaskStatus) => {
    try { await api.setTaskStatus(id, s); showToast(s === "active" ? "Одобрено — в бою" : s === "rejected" ? "Отклонено" : "В черновики"); refresh(); }
    catch (e) { showToast(String((e as Error).message)); }
  };
  const clearBank = async () => {
    if (!window.confirm(`Очистить банк по предмету «${SUBJECT_TITLES[subject]}»?\nВсе задания без истории решений будут удалены безвозвратно.`)) return;
    try {
      const r = await api.clearBank(subject);
      showToast(r.kept > 0 ? `Удалено ${r.deleted} · сохранено ${r.kept} (есть решения)` : `Удалено заданий: ${r.deleted}`);
      refresh();
    } catch (e) { showToast(String((e as Error).message)); }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <SourcePanel subject={subject} onDone={refresh} />
      <SubjectTabs value={subject} onChange={setSubject} />
      <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
        {(["", "draft", "active", "rejected"] as const).map((s) => (
          <ChoiceTab key={s} active={status === s} onClick={() => setStatus(s)}>{s === "" ? "Все" : s}</ChoiceTab>
        ))}
        <button onClick={clearBank} title="Удалить все задания предмета (кроме решённых)"
          style={{
            marginLeft: "auto", display: "inline-flex", alignItems: "center", gap: 6,
            background: "transparent", border: "1px solid var(--bad)", color: "var(--bad)",
            borderRadius: 999, padding: "7px 14px", fontSize: 13, fontWeight: 600,
          }}>
          <Icon name="trash" size={15} /> Очистить банк
        </button>
      </div>
      <Async q={tasks}>{(rows) => rows.length === 0 ? <Empty title="Пусто" hint="Запусти ингест, чтобы наполнить банк." /> : (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          {rows.map((t) => <BankCard key={t.id} task={t} onStatus={setTaskStatus} onEditAnswer={async (schema) => {
            try { await api.setTaskAnswer(t.id, schema); showToast("Ответ обновлён"); refresh(); }
            catch (e) { showToast(String((e as Error).message)); }
          }} />)}
        </div>
      )}</Async>
    </div>
  );
}

// SourcePanel populates the bank: the primary action is a button that pulls
// tasks from the source (РЕШУ/ФИПИ via the fetcher service) for the current
// subject; a file upload is kept as a secondary fallback (super-teacher only —
// a file can mix subjects). Both run the same ingest (media → MinIO, dedup,
// draft for curation unless "сразу активными").
function SourcePanel({ subject, onDone }: { subject: SubjectCode; onDone: () => void }) {
  const { showToast, user } = useApp();
  const inputRef = useRef<HTMLInputElement>(null);
  const [count, setCount] = useState(30);
  const [active, setActive] = useState(false);
  const [busy, setBusy] = useState<"fetch" | "upload" | "refresh" | null>(null);
  const superTeacher = !user?.subject;

  const summary = (r: { inserted: number; skipped: number; invalid: number }) => {
    if (r.inserted > 0) return `Добавлено ${r.inserted} заданий${r.skipped ? ` (${r.skipped} уже были в банке)` : ""}`;
    if (r.skipped > 0) return `Новых нет — все ${r.skipped} уже в банке`;
    return "Источник не вернул заданий";
  };

  const fetchNow = async () => {
    setBusy("fetch");
    try {
      const r = await api.fetchTasks(subject, count, active);
      const src = SOURCE_TITLE[subject];
      const promoted = r.promoted ? `, ${r.promoted} черновиков активировано` : "";
      if (r.inserted > 0) showToast(`Добавлено ${r.inserted} заданий с «${src}»${r.skipped ? ` (${r.skipped} уже были${promoted})` : ""}`);
      else if (r.skipped > 0) showToast(`Новых нет — все ${r.skipped} уже в банке${promoted}`);
      else showToast(`Источник (${src}) не вернул заданий`);
      onDone();
    } catch (err) {
      showToast("Не удалось подтянуть: " + String((err as Error).message));
    } finally { setBusy(null); }
  };

  // Re-reads the condition of already-imported tasks whose text is broken —
  // РЕШУ: theory dumped instead of the question / formulas as detached blocks;
  // openfipi (информатика): statements re-parsed by the current parser (e.g.
  // mangled distance-matrix tables) — and rewrites it in place. Answers/статус
  // сохраняются, дубли не плодятся.
  const refreshStale = async () => {
    setBusy("refresh");
    try {
      const r = await api.refetchFormulas();
      if (r.updated > 0) showToast(`Обновлено условий: ${r.updated} (проверено ${r.scanned})`);
      else showToast(r.scanned > 0 ? "Все условия уже в порядке" : "Нет РЕШУ-заданий для проверки");
      onDone();
    } catch (err) {
      showToast("Не удалось обновить: " + String((err as Error).message));
    } finally { setBusy(null); }
  };

  const onPick = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    setBusy("upload");
    try {
      const r = await uploadTasks(file, { provider: file.name.replace(/\.(jsonl?|txt)$/i, ""), active });
      showToast(summary(r));
      onDone();
    } catch (err) {
      showToast("Ошибка загрузки: " + String((err as Error).message));
    } finally { setBusy(null); }
  };

  return (
    <Card>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
        <div>
          <div style={{ fontWeight: 700, fontSize: 15 }}>Подтянуть задания из источника</div>
          <div style={{ color: "var(--text-2)", fontSize: 13, marginTop: 4 }}>
            Загрузит задания ({SUBJECT_TITLES[subject]}) с РЕШУ/ФИПИ прямо в банк.
            Медиа уедет в хранилище, дубли отсеются, ответы можно поправить ниже.
          </div>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <label className="mono" style={{ fontSize: 13, color: "var(--text-2)" }}>
            сколько
            <input type="number" min={1} max={200} value={count} onChange={(e) => setCount(+e.target.value)} style={{ width: 68, marginLeft: 8 }} />
          </label>
          <label style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 13, color: "var(--text-2)" }}>
            <input type="checkbox" checked={active} onChange={(e) => setActive(e.target.checked)} style={{ width: "auto" }} />
            сразу активными
          </label>
          <Button onClick={fetchNow} disabled={busy !== null}>{busy === "fetch" ? "Подтягиваю…" : "Подтянуть задания"}</Button>
        </div>
      </div>
      <div style={{ marginTop: 10, display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <span title="Перечитывает условия у заданий с битым текстом: теория/справка вместо задания, формулы-блоки (РЕШУ), разъехавшиеся таблицы (ФИПИ/информатика). Ответы и статус сохранятся.">
          <Button variant="soft" onClick={refreshStale} disabled={busy !== null} style={{ padding: "8px 14px", fontSize: 13 }}>
            {busy === "refresh" ? "Обновляю…" : "Обновить условия у старых заданий"}
          </Button>
        </span>
        <span style={{ fontSize: 12, color: "var(--text-3)" }}>чинит битые условия: теория вместо текста, разъехавшиеся таблицы</span>
      </div>
      {superTeacher && (
        <div style={{ marginTop: 12, borderTop: "1px solid var(--border)", paddingTop: 10, display: "flex", alignItems: "center", gap: 10 }}>
          <span style={{ fontSize: 13, color: "var(--text-3)" }}>или загрузить файлом</span>
          <input ref={inputRef} type="file" accept=".json,.jsonl,application/json" onChange={onPick} style={{ display: "none" }} />
          <button onClick={() => inputRef.current?.click()} disabled={busy !== null} style={{
            background: "transparent", border: "1px solid var(--border-2)", borderRadius: 9,
            padding: "6px 12px", fontSize: 13, color: "var(--text-2)",
          }}>{busy === "upload" ? "Загрузка…" : ".json / .jsonl"}</button>
        </div>
      )}
    </Card>
  );
}

function BankCard({ task, onStatus, onEditAnswer }: { task: Task; onStatus: (id: string, s: TaskStatus) => void; onEditAnswer: (s: AnswerSchema) => void }) {
  const [editing, setEditing] = useState(false);
  const [val, setVal] = useState(task.answer_schema.correct.join(", "));
  const tone = task.status === "active" ? "accent" : task.status === "rejected" ? "bad" : "warn";
  return (
    <Card>
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12, marginBottom: 10 }}>
        <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
          <Pill tone="neutral">№{task.number}</Pill>
          <Pill>{task.answer_kind}</Pill>
          <Pill tone={tone as any}>{task.status}</Pill>
          {task.bot_solvable && <span title="бот решает" style={{ display: "inline-flex", color: "var(--text-3)" }}><Icon name="bot" size={16} /></span>}
          {(task.media?.length ?? 0) > 0 && <span title="есть медиа" style={{ display: "inline-flex", color: "var(--text-3)" }}><Icon name="image" size={16} /></span>}
        </div>
      </div>
      <StatementView text={task.statement} media={task.media} style={{ fontSize: 15, lineHeight: 1.45, marginBottom: 12 }} />
      <MediaBlock media={task.media} />
      <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 12 }}>
        <Label>Ответ</Label>
        {!editing ? (
          <>
            <span className="mono" style={{ background: "var(--surface-2)", padding: "4px 10px", borderRadius: 8 }}>{task.answer_schema.correct.join(" / ")}</span>
            <button onClick={() => setEditing(true)} style={{ background: "none", border: "none", color: "var(--accent-2)", fontSize: 13 }}>править</button>
          </>
        ) : (
          <>
            <input value={val} onChange={(e) => setVal(e.target.value)} className="mono" style={{ flex: 1 }} />
            <Button variant="soft" style={{ padding: "6px 12px" }} onClick={() => { onEditAnswer({ ...task.answer_schema, correct: val.split(",").map((s) => s.trim()).filter(Boolean) }); setEditing(false); }}>Сохранить</Button>
          </>
        )}
      </div>
      <div style={{ display: "flex", gap: 8 }}>
        <Button variant="soft" style={{ padding: "6px 14px" }} onClick={() => onStatus(task.id, "active")}>Одобрить</Button>
        <Button variant="ghost" style={{ padding: "6px 14px" }} onClick={() => onStatus(task.id, "rejected")}>Отклонить</Button>
      </div>
    </Card>
  );
}

function ChoiceTab({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return <button onClick={onClick} style={{
    padding: "7px 14px", borderRadius: 999, fontSize: 13, fontWeight: 600,
    border: "1px solid " + (active ? "var(--accent)" : "var(--border-2)"),
    background: active ? "var(--accent-soft)" : "transparent",
    color: active ? "var(--accent-2)" : "var(--text-2)",
  }}>{children}</button>;
}
