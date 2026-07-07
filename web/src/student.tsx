import { useEffect, useMemo, useRef, useState } from "react";
import {
  api, SubjectCode, TaskView, DayAnswer, AssignmentCard, AttemptReviewItem, useForecast, useHeatmap, useWeakSpots,
  useMastery, useMasterySeries, useAssignments, useAttempts, useInvalidate,
} from "./api";
import { useApp } from "./state";
import { Card, Label, Pill, Button, Async, Empty, Loading, Modal, accColor, SUBJECT_TITLES, testTitle, MediaBlock, StatementView, AttemptReviewGrid } from "./ui";
import { ScoreGauge, Heatmap, computeStreak, WeakSpotsList, Section, MasteryChart, Sparkline } from "./charts";
import { AnswerInput } from "./answer";
import { Icon } from "./icons";
import { deadlineInfo } from "./deadline";

// Russian plural for «день» (1 день · 2 дня · 5 дней).
function pluralDays(n: number): string {
  const m10 = n % 10, m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return "день";
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return "дня";
  return "дней";
}

// A flame glyph + streak label, sized for use inside a <Pill>. The flame comes
// alive (a gentle flicker + glow) once the streak is non-zero, so an active
// streak reads as "burning" while a broken one (0 days) sits cold.
export function StreakBadge({ days }: { days: number }) {
  return <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>
    <Icon name="flame" size={13} className={days > 0 ? "flame-live" : undefined} /> {days} {pluralDays(days)} подряд
  </span>;
}

// Solve request handoff (set before navigating to the solve view). Two modes:
// free practice (subject [+ number] — tasks come from the practice pool) and an
// assigned/composed test (testId — tasks are exactly the variant's items, and
// assignmentId marks the assignment done on finish).
export interface SolveRequest {
  subject: SubjectCode;
  number?: number;
  testId?: string;
  assignmentId?: string;
  title?: string;
}
let solveRequest: SolveRequest | null = null;
export function requestSolve(r: SolveRequest) { solveRequest = r; }

// Assignment statuses come from the API in English; the UI speaks Russian.
export const ASSIGNMENT_STATUS_RU: Record<string, string> = {
  scheduled: "запланирован", done: "решён", missed: "просрочен",
};

const grid12 = { display: "grid", gap: "var(--gap)", gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))" } as const;

// useAttemptReview loads a solved assignment's per-task review into a modal — the
// «как решил» drill-down. It owns the modal, so a screen just renders `modal` and
// calls `open(card)`. Shared by the dashboard's assigned cards and the History
// screen's assigned-tests list.
function useAttemptReview() {
  const [review, setReview] = useState<{ title: string; items: AttemptReviewItem[] } | null>(null);
  const open = async (card: AssignmentCard) => {
    if (!card.attempt_id) return;
    const title = testTitle(card.title);
    try { const items = await api.attemptReview(card.attempt_id); setReview({ title, items }); }
    catch { setReview({ title, items: [] }); }
  };
  const modal = review && (
    <Modal onClose={() => setReview(null)} title={`Разбор · ${review.title}`} maxWidth="min(1200px, 96vw)">
      <AttemptReviewGrid items={review.items} selfView />
    </Modal>
  );
  return { open, modal };
}

// AssignedTestsList renders the assigned-tests history: each assignment with its
// schedule, and — once solved — the score and a «Разбор» drill-down (что/как решил).
// Not-yet-solved rows show «Начать» (whether it was solved is read straight from
// the presence of a score). onSolve starts the variant; onReview opens the review.
function AssignedTestsList({ cards, onSolve, onReview }: {
  cards: AssignmentCard[];
  onSolve: (c: AssignmentCard) => void;
  onReview: (c: AssignmentCard) => void;
}) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
      {cards.map((a) => {
        // "Solved" reads off an actual finished attempt, not just the status flag
        // (which the finish handler sets best-effort) — the honest «решал ли».
        const solved = !!a.finished_at;
        const pct = a.total ? Math.round((a.correct / a.total) * 100) : 0;
        const dl = deadlineInfo(a);
        return (
          <div key={a.id} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, padding: 12, background: "var(--surface-2)", borderRadius: 12 }}>
            <div>
              <div style={{ fontWeight: 600, display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                {testTitle(a.title)}
                {dl.pill && <Pill tone={dl.pill.tone}>{dl.pill.label}</Pill>}
              </div>
              <div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>
                {new Date(a.scheduled_at).toLocaleString("ru")} · {a.task_count} зад.
                {solved && a.finished_at
                  ? ` · решён ${new Date(a.finished_at).toLocaleString("ru")}`
                  : dl.kind === "none"
                    ? ` · ${ASSIGNMENT_STATUS_RU[a.status] || a.status}`
                    : ` · ${dl.text}`}
              </div>
            </div>
            {solved
              ? (
                <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  <span className="mono" title={`${pct}% верно`} style={{ color: accColor(pct), fontWeight: 700 }}>{a.correct}/{a.total}</span>
                  {a.attempt_id && <Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => onReview(a)}>Разбор</Button>}
                </div>
              )
              : <Button variant="soft" onClick={() => onSolve(a)}>Начать</Button>}
          </div>
        );
      })}
    </div>
  );
}

// ---------- Dashboard ----------
export function Dashboard() {
  const { subject, go, user } = useApp();
  const sid = user?.id ?? "";
  const forecast = useForecast(sid, subject);
  const heat = useHeatmap(sid);
  const weak = useWeakSpots(sid, subject);
  const assignments = useAssignments(sid);
  const { open: openReview, modal: reviewModal } = useAttemptReview();

  const startPractice = () => { requestSolve({ subject }); go("solve"); };
  const drill = (n: number) => { requestSolve({ subject, number: n }); go("solve"); };
  const solveAssigned = (a: AssignmentCard) => {
    requestSolve({ subject, testId: a.test_id, assignmentId: a.id, title: testTitle(a.title) });
    go("solve");
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <div style={grid12}>
        <Card>
          <Label>Прогноз балла · {SUBJECT_TITLES[subject]}</Label>
          <Async q={forecast}>{(f) => (
            <div style={{ display: "flex", flexDirection: "column", alignItems: "center", marginTop: 8 }}>
              <ScoreGauge score={f.test_score} />
              <div className="mono" style={{ color: "var(--text-2)", fontSize: 13, marginTop: 4 }}>
                {f.primary_estimate} из {f.primary_max} первичных · точность {Math.round(f.accuracy * 100)}%
              </div>
              <div style={{ color: "var(--text-2)", fontSize: 13, marginTop: 10, textAlign: "center" }}>{f.note}</div>
              <div style={{ marginTop: 16 }}><Button onClick={startPractice}>Решать</Button></div>
            </div>
          )}</Async>
        </Card>

        <Card>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 14 }}>
            <Label>Активность</Label>
            <Async q={heat}>{(h) => <Pill tone="accent"><StreakBadge days={computeStreak(h)} /></Pill>}</Async>
          </div>
          <Async q={heat}>{(h) => <Heatmap cells={h} onDay={() => go("history")} />}</Async>
          <div style={{ color: "var(--text-3)", fontSize: 12, marginTop: 10 }}>Клетки — активность по дням. Открой историю для разбора.</div>
        </Card>
      </div>

      <div style={grid12}>
        <Section title="Слабые места" right={<Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => go("subject")}>Все номера</Button>}>
          <Async q={weak}>{(w) => <WeakSpotsList spots={w} onDrill={drill} />}</Async>
        </Section>

        <Section title="Назначено тебе" right={<Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => go("history")}>Вся история</Button>}>
          <Async q={assignments}>{(list) => list.length === 0
            ? <Empty title="Пока ничего не назначено" hint="Учитель запланирует тест — он появится здесь." action={<Button onClick={startPractice}>Решать самому</Button>} />
            : <AssignedTestsList cards={list} onSolve={solveAssigned} onReview={openReview} />}</Async>
        </Section>
      </div>
      {reviewModal}
    </div>
  );
}

// ---------- Subject screen ----------
export function SubjectScreen() {
  const { subject, setSubject, go, user } = useApp();
  const sid = user?.id ?? "";
  const mastery = useMastery(sid, subject);
  const series = useMasterySeries(sid, subject);
  const [open, setOpen] = useState<number | null>(null);

  const drill = (n: number) => { requestSolve({ subject, number: n }); go("solve"); };
  const seriesByNumber = useMemo(() => {
    const m = new Map<number, number[]>();
    (series.data || []).forEach((p) => {
      const arr = m.get(p.number) || [];
      arr.push(p.total ? p.correct / p.total : 0);
      m.set(p.number, arr);
    });
    return m;
  }, [series.data]);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {(["rus", "math", "inf", "soc"] as SubjectCode[]).map((c) => (
          <button key={c} onClick={() => setSubject(c)} style={{
            padding: "8px 16px", borderRadius: 999, fontWeight: 600, fontSize: 14,
            border: "1px solid " + (subject === c ? "var(--accent)" : "var(--border-2)"),
            background: subject === c ? "var(--accent-soft)" : "transparent",
            color: subject === c ? "var(--accent-2)" : "var(--text-2)",
          }}>{SUBJECT_TITLES[c]}</button>
        ))}
      </div>

      <Async q={mastery}>{(rows) => rows.length === 0
        ? <Empty title="Нет данных по номерам" hint="Начни решать — здесь появится прогресс по каждому заданию." action={<Button onClick={() => { requestSolve({ subject }); go("solve"); }}>Решать</Button>} />
        : (
          <div style={{ ...grid12, gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))" }}>
            {rows.map((r) => {
              const pct = r.total ? Math.round((r.correct / r.total) * 100) : 0;
              return (
                <Card key={r.number} onClick={() => setOpen(r.number)} style={{ cursor: "pointer", padding: 16 }}>
                  <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                    <span className="mono" style={{ fontWeight: 700, fontSize: 15 }}>№{r.number}</span>
                    <span className="mono" style={{ color: accColor(pct), fontWeight: 700 }}>{pct}%</span>
                  </div>
                  <div style={{ marginTop: 8 }}><Sparkline data={seriesByNumber.get(r.number) || [pct / 100]} color={accColor(pct)} w={148} /></div>
                  <div className="mono" style={{ color: "var(--text-3)", fontSize: 11, marginTop: 6 }}>{r.total} попыток</div>
                </Card>
              );
            })}
          </div>
        )}</Async>

      {open !== null && (
        <Modal onClose={() => setOpen(null)} title={`Задание №${open} · динамика`}>
          <MasteryChart points={(series.data || []).filter((p) => p.number === open)} />
          <div style={{ marginTop: 16, display: "flex", justifyContent: "flex-end" }}>
            <Button onClick={() => { setOpen(null); drill(open); }}>Решать такие</Button>
          </div>
        </Modal>
      )}
    </div>
  );
}

// ---------- Solve session ----------
interface Answered { taskId: string; number: number; correct: boolean; }

// SessionTimer shows elapsed time since the session started (design §3.3: the
// exam is timed). Per-task time is measured separately for time_spent_ms.
function SessionTimer({ since }: { since: number }) {
  const [, tick] = useState(0);
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);
  const sec = Math.max(0, Math.floor((Date.now() - since) / 1000));
  const mm = String(Math.floor(sec / 60)).padStart(2, "0");
  const ss = String(sec % 60).padStart(2, "0");
  return <span className="mono" title="Время с начала решения" style={{ color: "var(--text-3)", fontSize: 13, display: "inline-flex", alignItems: "center", gap: 5 }}>
    <Icon name="history" size={14} /> {mm}:{ss}
  </span>;
}

export function Solve() {
  const { go, showToast } = useApp();
  const invalidate = useInvalidate();
  const req = useRef(solveRequest).current;
  const [attemptId, setAttemptId] = useState("");
  const [tasks, setTasks] = useState<TaskView[]>([]);
  const [idx, setIdx] = useState(0);
  const [draft, setDraft] = useState("");
  const [submitted, setSubmitted] = useState<{ ok: boolean; solution?: string[] } | null>(null);
  const [done, setDone] = useState<Answered[]>([]);
  // Consecutive-correct run, shown as «серия ×N» once it reaches 2. Resets on a
  // wrong answer; drives the in-session momentum without any server state.
  const [combo, setCombo] = useState(0);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string>();
  const taskStart = useRef(Date.now());
  const sessionStart = useRef(Date.now());
  const [finished, setFinished] = useState(false);

  useEffect(() => {
    if (!req) { setErr("Не задан предмет"); setLoading(false); return; }
    (async () => {
      try {
        if (req.testId) {
          // Assigned/composed variant: solve exactly the test's tasks; the
          // attempt carries assignment_id so finishing marks it done.
          const list = await api.testTasks(req.testId);
          if (list.length === 0) { setErr("В этом тесте нет заданий."); setLoading(false); return; }
          const att = await api.startAttempt(req.testId, req.assignmentId);
          setAttemptId(att.id); setTasks(list);
        } else {
          const { attempt_id } = await api.startPractice(req.subject);
          // practiceTasks excludes tasks the student already mastered (solved
          // correctly ≥2×), so they don't repeat. Drill pulls more, then filters.
          let list = await api.practiceTasks(req.subject, req.number ? 60 : 20);
          if (req.number) list = list.filter((t) => t.number === req.number);
          if (list.length === 0) { setErr(req.number ? "Ты уже освоил все задания этого номера — молодец!" : "Пока нет новых заданий: либо всё освоено, либо банк пуст. Учитель может собрать вариант — он подтянет задания."); setLoading(false); return; }
          setAttemptId(attempt_id); setTasks(list.slice(0, 15));
        }
        setLoading(false);
        taskStart.current = Date.now();
        sessionStart.current = Date.now();
      } catch (e) { setErr(String((e as Error).message)); setLoading(false); }
    })();
  }, [req]);

  const finishSession = () => {
    api.finish(attemptId).catch(() => {});
    // An assigned test just became "done" — refresh the dashboard feed.
    if (req?.assignmentId) invalidate("assignments");
    invalidate("attempts");
    setFinished(true);
  };

  if (loading) return <Loading label="Готовим задания…" />;
  if (err) return <Empty title="Не получилось" hint={err} action={<Button onClick={() => go("dashboard")}>На главную</Button>} />;
  if (finished) return <Results tasks={tasks} done={done} onExit={() => go("dashboard")} />;

  const task = tasks[idx];
  const submit = async () => {
    if (!draft.trim()) { showToast("Введите ответ"); return; }
    const dt = Date.now() - taskStart.current;
    try {
      const r = await api.submit(attemptId, task.id, draft, dt);
      setSubmitted({ ok: r.is_correct, solution: r.solution });
      setCombo((c) => (r.is_correct ? c + 1 : 0));
      setDone((d) => [...d.filter((x) => x.taskId !== task.id), { taskId: task.id, number: task.number, correct: r.is_correct }]);
    } catch (e) { showToast(String((e as Error).message)); }
  };
  const next = () => {
    if (idx >= tasks.length - 1) { finishSession(); return; }
    setIdx(idx + 1); setDraft(""); setSubmitted(null); taskStart.current = Date.now();
  };

  return (
    <div style={{ maxWidth: 720, margin: "0 auto", display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      {req?.title && <div style={{ fontWeight: 700, fontSize: 16 }}>{req.title}</div>}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12 }}>
        <span className="mono" style={{ color: "var(--text-2)", display: "inline-flex", alignItems: "center", gap: 12 }}>
          {idx + 1} / {tasks.length}
          <SessionTimer since={sessionStart.current} />
        </span>
        <div style={{ display: "flex", gap: 6 }}>
          {tasks.map((t, i) => {
            const a = done.find((x) => x.taskId === t.id);
            return <div key={t.id} style={{
              width: 9, height: 9, borderRadius: 999,
              background: a ? (a.correct ? "var(--accent)" : "var(--bad)") : i === idx ? "var(--text-3)" : "var(--border-2)",
            }} />;
          })}
        </div>
        <Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={finishSession}>Завершить</Button>
      </div>

      <Card>
        <div style={{ display: "flex", gap: 8, marginBottom: 12 }}>
          <Pill tone="neutral">№{task.number}</Pill>
          <Pill>{task.answer_kind}</Pill>
        </div>
        <StatementView text={task.statement} media={task.media} style={{ fontSize: 17, lineHeight: 1.5, marginBottom: 18 }} />
        <MediaBlock media={task.media} />
        {!submitted && <AnswerInput kind={task.answer_kind} value={draft} onChange={setDraft} />}
        {submitted && (
          <div className={submitted.ok ? "celebrate" : undefined} style={{
            padding: 16, borderRadius: 12, marginTop: 4,
            background: submitted.ok ? "var(--accent-soft)" : "var(--bad-soft)",
            color: submitted.ok ? "var(--accent-2)" : "var(--bad)",
          }}>
            <div style={{ display: "flex", alignItems: "center", gap: 7, fontWeight: 700, marginBottom: submitted.solution?.length ? 8 : 0 }}>
              {submitted.ok && <Icon name="check" size={18} className="checkpop" />}
              {submitted.ok ? "Верно!" : "Пока неверно"}
              {submitted.ok && combo >= 2 && (
                <span className="mono" style={{
                  marginLeft: "auto", background: "var(--warn-soft)", color: "var(--warn)",
                  borderRadius: 999, padding: "2px 10px", fontSize: 12, fontWeight: 700,
                }}>серия ×{combo}</span>
              )}
            </div>
            {!submitted.ok && submitted.solution && submitted.solution.length > 0 && (
              <div className="mono" style={{ fontSize: 14 }}>Правильный ответ: {submitted.solution.join(" / ")}</div>
            )}
          </div>
        )}
      </Card>

      <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
        {!submitted
          ? <Button onClick={submit}>Ответить</Button>
          : <Button onClick={next}>
              {idx >= tasks.length - 1 ? "Итоги" : (
                <span style={{ display: "inline-flex", alignItems: "center", gap: 7 }}>Дальше <Icon name="arrowRight" size={16} /></span>
              )}
            </Button>}
      </div>
    </div>
  );
}

// ---------- Results ----------
function Results({ tasks, done, onExit }: { tasks: TaskView[]; done: Answered[]; onExit: () => void }) {
  const correct = done.filter((d) => d.correct).length;
  const pct = tasks.length ? Math.round((correct / tasks.length) * 100) : 0;
  return (
    <div style={{ maxWidth: 640, margin: "0 auto", display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <Card style={{ textAlign: "center", padding: 34 }}>
        <Label>Итоги</Label>
        <div className="mono" style={{ fontSize: 54, fontWeight: 800, color: accColor(pct), margin: "10px 0" }}>{pct}%</div>
        <div style={{ color: "var(--text-2)" }}>{correct} из {tasks.length} верно</div>
      </Card>
      <Section title="По заданиям">
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {tasks.map((t) => {
            const a = done.find((d) => d.taskId === t.id);
            return (
              <div key={t.id} style={{ display: "flex", justifyContent: "space-between", padding: "8px 12px", background: "var(--surface-2)", borderRadius: 10 }}>
                <span className="mono">№{t.number}</span>
                <span style={{ color: a ? (a.correct ? "var(--accent)" : "var(--bad)") : "var(--text-3)" }}>
                  {a ? (a.correct ? "верно" : "неверно") : "пропущено"}
                </span>
              </div>
            );
          })}
        </div>
      </Section>
      <div style={{ display: "flex", justifyContent: "center" }}><Button onClick={onExit}>На главную</Button></div>
    </div>
  );
}

// ---------- History ----------
export function History() {
  const { subject, go, user } = useApp();
  const sid = user?.id ?? "";
  const heat = useHeatmap(sid);
  const attempts = useAttempts(sid);
  const assignments = useAssignments(sid);
  const { open: openReview, modal: reviewModal } = useAttemptReview();
  const [day, setDay] = useState<{ date: string; items: DayAnswer[] } | null>(null);

  const openDay = async (dateISO: string) => {
    const date = dateISO.slice(0, 10);
    try { const items = await api.day(sid, date); setDay({ date, items }); } catch { setDay({ date, items: [] }); }
  };
  const solveAssigned = (a: AssignmentCard) => {
    requestSolve({ subject, testId: a.test_id, assignmentId: a.id, title: testTitle(a.title) });
    go("solve");
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <Section title="Назначенные тесты">
        <Async q={assignments}>{(list) => list.length === 0
          ? <div style={{ color: "var(--text-2)" }}>Учитель ещё ничего не назначал.</div>
          : <AssignedTestsList cards={list} onSolve={solveAssigned} onReview={openReview} />}</Async>
      </Section>
      <Section title="Активность за год">
        <Async q={heat}>{(h) => <Heatmap cells={h} big onDay={(c) => c.total > 0 && openDay(c.day)} />}</Async>
      </Section>
      <Section title="Недавние решения">
        <Async q={attempts}>{(list) => list.length === 0
          ? <div style={{ color: "var(--text-2)" }}>Пока пусто.</div>
          : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {list.map((a) => (
                <div key={a.id} style={{ display: "flex", justifyContent: "space-between", padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10 }}>
                  <div>
                    <div style={{ fontWeight: 600 }}>{testTitle(a.title)}</div>
                    <div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>{new Date(a.started_at).toLocaleString("ru")}</div>
                  </div>
                  <span className="mono" style={{ color: accColor(a.total ? (Number(a.correct) / Number(a.total)) * 100 : 0), fontWeight: 700 }}>
                    {a.correct}/{a.total}
                  </span>
                </div>
              ))}
            </div>
          )}</Async>
      </Section>
      {day && (
        <Modal onClose={() => setDay(null)} title={`Разбор дня · ${day.date}`}>
          {day.items.length === 0 ? <div style={{ color: "var(--text-2)" }}>В этот день решений не найдено.</div> : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {day.items.map((it) => (
                <div key={it.answer_id} style={{ display: "flex", justifyContent: "space-between", padding: "8px 12px", background: "var(--surface-2)", borderRadius: 10 }}>
                  <span className="mono">№{it.number} · {it.raw_answer}</span>
                  <span style={{ color: it.is_correct ? "var(--accent)" : "var(--bad)" }}>{it.is_correct ? "верно" : "неверно"}</span>
                </div>
              ))}
            </div>
          )}
        </Modal>
      )}
      {reviewModal}
    </div>
  );
}

// Modal moved to ui.tsx (the shared kit) so every dialog — including ones used
// outside student screens, e.g. the Telegram link modal in the shell — renders
// through the one theme-scoped portal.
