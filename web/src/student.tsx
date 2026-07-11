import { useEffect, useMemo, useRef, useState } from "react";
import {
  api, SubjectCode, TaskView, DayAnswer, AssignmentCard, AttemptReviewItem, Forecast, useForecast, useHeatmap,
  useWeakSpots, useMastery, useMasterySeries, useAssignments, useAttempts, useInvalidate, usePracticeOverview,
} from "./api";
import { useApp } from "./state";
import { Card, Label, Pill, Button, Async, Empty, Loading, Modal, accColor, SUBJECT_TITLES, SubjectSwitch, testTitle, MediaBlock, StatementView, AttemptReviewGrid, useIsMobile } from "./ui";
import { ScoreGauge, Ring, Heatmap, computeStreak, WeakSpotsList, Section, MasteryChart, Sparkline } from "./charts";
import { AnswerInput } from "./answer";
import { Icon, IconName } from "./icons";
import { deadlineInfo } from "./deadline";
import { confettiBurst } from "./confetti";
import { dayKey, todayTotal, dailyGoal, streakAtRisk, streakCelebration, streakColor } from "./engage";
import { pluralRu } from "./plural";

// Russian plural for «день» (1 день · 2 дня · 5 дней).
function pluralDays(n: number): string {
  const m10 = n % 10, m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return "день";
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return "дня";
  return "дней";
}

// A flame glyph + streak label, sized for use inside a <Pill>. The flame comes
// alive (a gentle flicker + glow) once the streak is non-zero, and climbs the
// milestone ramp (--warn → --accent → --hm4 at 3/7/14, blazing harder at 30+)
// so a long streak visibly burns hotter. `ember` is the at-risk state: today
// is still empty, yesterday's run of `days` is about to be lost.
export function StreakBadge({ days, ember }: { days: number; ember?: boolean }) {
  if (ember) {
    return <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>
      <Icon name="flame" size={13} className="flame-ember" /> серия {days} — под угрозой
    </span>;
  }
  const color = streakColor(days);
  return <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>
    <span style={{ display: "inline-flex", ...(color ? { color } : {}) }}>
      <Icon name="flame" size={13} className={days > 0 ? "flame-live" : undefined}
        style={days >= 30 ? { filter: "drop-shadow(0 0 6px currentColor)" } : undefined} />
    </span>
    {days} {pluralDays(days)} подряд
  </span>;
}

// Solve request handoff (set before navigating to the solve view). Modes:
// free practice (subject — random unmastered tasks), a drill (+number — one
// задание, server-filtered), mode:"mistakes" (the wrong-answer queue),
// mode:"recommended" (the smart mix: ошибки → слабые номера → новое), and a
// test (testId — tasks are exactly the variant's items; assignmentId, when it
// came from a teacher, marks the assignment done on finish).
export interface SolveRequest {
  subject: SubjectCode;
  number?: number;
  mode?: "mistakes" | "recommended";
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

// min(300px, 100%) keeps the track from exceeding a narrow phone's content
// width (a bare 300px minimum forced horizontal scroll on 320–360px screens).
const grid12 = { display: "grid", gap: "var(--gap)", gridTemplateColumns: "repeat(auto-fit, minmax(min(300px, 100%), 1fr))" } as const;

// useAttemptReview loads a solved assignment's per-task review into a modal — the
// «как решил» drill-down. It owns the modal, so a screen just renders `modal` and
// calls `open(card)`. Shared by the dashboard's assigned cards and the History
// screen's assigned-tests list.
export function useAttemptReview() {
  const [review, setReview] = useState<{ title: string; items: AttemptReviewItem[] } | null>(null);
  const open = async (card: { attempt_id?: string; title: string }) => {
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
          <div key={a.id} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, flexWrap: "wrap", padding: 12, background: "var(--surface-2)", borderRadius: 12 }}>
            <div style={{ minWidth: 0 }}>
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
  const { subject, go, user, showToast } = useApp();
  const sid = user?.id ?? "";
  const forecast = useForecast(sid, subject);
  const heat = useHeatmap(sid);
  const weak = useWeakSpots(sid, subject);
  const assignments = useAssignments(sid);
  const overview = usePracticeOverview(sid, subject);
  const { open: openReview, modal: reviewModal } = useAttemptReview();

  // One-time celebrations, guarded by per-user localStorage marks so a reload
  // or refocus never re-fires them: a crossed streak milestone (3/7/14/30) and
  // the closed daily goal. Fires on the dashboard — where the student lands
  // right after a session.
  const heatData = heat.data;
  useEffect(() => {
    if (!heatData || !sid) return;
    const goal = dailyGoal(heatData);
    const goalKey = `egeism.goalDone.${sid}`;
    const today = dayKey(new Date());
    if (todayTotal(heatData) >= goal && localStorage.getItem(goalKey) !== today) {
      localStorage.setItem(goalKey, today);
      confettiBurst({ count: 70 });
      showToast("Цель на день выполнена!");
    }
    const seenKey = `egeism.streakSeen.${sid}`;
    const seen = Number(localStorage.getItem(seenKey) || 0);
    const { milestone, seen: next } = streakCelebration(computeStreak(heatData), seen);
    if (next !== seen) localStorage.setItem(seenKey, String(next));
    if (milestone) {
      confettiBurst({ count: 90 });
      // The rarer toast wins if both fire at once (the burst still stacks).
      showToast(`${milestone} ${pluralDays(milestone)} подряд — так держать!`);
    }
  }, [heatData, sid, showToast]);

  const startPractice = () => { requestSolve({ subject }); go("solve"); };
  const startMistakes = () => { requestSolve({ subject, mode: "mistakes", title: "Работа над ошибками" }); go("solve"); };
  const startRecommended = () => { requestSolve({ subject, mode: "recommended", title: "Умная тренировка" }); go("solve"); };
  const drill = (n: number) => { requestSolve({ subject, number: n, title: `Тренировка №${n}` }); go("solve"); };
  const solveAssigned = (a: AssignmentCard) => {
    requestSolve({ subject, testId: a.test_id, assignmentId: a.id, title: testTitle(a.title) });
    go("solve");
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      {/* Switch the working subject right on the dashboard — the forecast, weak
          spots and training cards below all follow it. */}
      <SubjectSwitch />
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
            {/* An alive streak = success → green ok-tone; a run about to break
                shows as the warn-tinted ember; a cold one stays neutral. */}
            <Async q={heat}>{(h) => {
              const days = computeStreak(h);
              const risk = days === 0 ? streakAtRisk(h) : 0;
              if (risk > 0) return <Pill tone="warn"><StreakBadge days={risk} ember /></Pill>;
              return <Pill tone={days > 0 ? "ok" : "neutral"}><StreakBadge days={days} /></Pill>;
            }}</Async>
          </div>
          <Async q={heat}>{(h) => <>
            <Heatmap cells={h} onDay={() => go("history")} />
            {computeStreak(h) === 0 && streakAtRisk(h) > 0
              ? <div style={{ color: "var(--warn)", fontSize: 12, marginTop: 10 }}>Серия догорает — спаси её: реши одну задачу сегодня.</div>
              : <div style={{ color: "var(--text-3)", fontSize: 12, marginTop: 10 }}>Клетки — активность по дням. Открой историю для разбора.</div>}
          </>}</Async>
        </Card>

        <Card>
          <Label>Цель на день</Label>
          <Async q={heat}>{(h) => {
            const goal = dailyGoal(h);
            const done = todayTotal(h);
            const met = done >= goal;
            return (
              <div style={{ display: "flex", alignItems: "center", gap: 18, marginTop: 14 }}>
                <Ring value={done} max={goal} color={met ? "var(--ok)" : "var(--accent)"}>
                  <div className="mono" style={{ fontSize: 24, fontWeight: 700, letterSpacing: "-0.02em", lineHeight: 1 }}>
                    {done}<span style={{ fontSize: 12, fontWeight: 600, color: "var(--text-3)" }}>/{goal}</span>
                  </div>
                </Ring>
                <div style={{ display: "flex", flexDirection: "column", gap: 10, minWidth: 0 }}>
                  <div style={{ color: "var(--text-2)", fontSize: 13, lineHeight: 1.45 }}>
                    {met
                      ? <>Цель на сегодня закрыта — день зачтён в серию. Отличный темп!</>
                      : <>Реши ещё {goal - done} {pluralRu(goal - done, ["задачу", "задачи", "задач"])} — и день зачтётся в серию.</>}
                  </div>
                  {met
                    ? <div><Pill tone="ok">выполнено ✓</Pill></div>
                    : <Button variant="soft" style={{ alignSelf: "flex-start", padding: "6px 14px", fontSize: 13 }} onClick={startPractice}>Решать</Button>}
                </div>
              </div>
            );
          }}</Async>
        </Card>

        <Card>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 14 }}>
            <Label>Тренировка</Label>
            <Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => go("train")}>Все тренировки</Button>
          </div>
          <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
            <div style={{ color: "var(--text-2)", fontSize: 13, lineHeight: 1.45 }}>
              Умная тренировка соберёт сессию под тебя: сначала ошибки, потом слабые номера, потом новое.
            </div>
            <Button onClick={startRecommended}>Умная тренировка</Button>
            <Async q={overview}>{(o) => o.mistakes > 0
              ? <Button variant="soft" onClick={startMistakes}>Работа над ошибками · {o.mistakes}</Button>
              : <div style={{ color: "var(--text-3)", fontSize: 12 }}>Ошибок на разбор нет — так держать!</div>}</Async>
          </div>
        </Card>
      </div>

      <div style={grid12}>
        <Section title="Слабые места" right={<Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => go("subject")}>Все номера</Button>}>
          <Async q={weak}>{(w) => <WeakSpotsList spots={w} onDrill={drill} />}</Async>
        </Section>

        <Section title="Назначено тебе" right={<Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => go("history")}>Вся история</Button>}>
          <Async q={assignments}>{(list) => list.length === 0
            ? <Empty art="telescope" title="Пока ничего не назначено" hint="Учитель запланирует тест — он появится здесь." action={<Button onClick={startPractice}>Решать самому</Button>} />
            : <AssignedTestsList cards={list} onSolve={solveAssigned} onReview={openReview} />}</Async>
        </Section>
      </div>
      {reviewModal}
    </div>
  );
}

// ---------- Subject screen ----------
export function SubjectScreen() {
  const { subject, go, user } = useApp();
  const sid = user?.id ?? "";
  const mastery = useMastery(sid, subject);
  const series = useMasterySeries(sid, subject);
  const [open, setOpen] = useState<number | null>(null);

  const drill = (n: number) => { requestSolve({ subject, number: n, title: `Тренировка №${n}` }); go("solve"); };
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
      <SubjectSwitch />

      <Async q={mastery}>{(rows) => rows.length === 0
        ? <Empty art="sprout" title="Нет данных по номерам" hint="Начни решать — здесь появится прогресс по каждому заданию." action={<Button onClick={() => { requestSolve({ subject }); go("solve"); }}>Решать</Button>} />
        : (
          <div style={{ ...grid12, gridTemplateColumns: "repeat(auto-fill, minmax(180px, 1fr))" }}>
            {rows.map((r) => {
              const pct = r.total ? Math.round((r.correct / r.total) * 100) : 0;
              return (
                <Card key={r.number} onClick={() => setOpen(r.number)} style={{ padding: 16 }}>
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

// A dead-end before the session could start. Not always a failure: the
// mistake queue may be empty (medal) or the bank may have nothing new
// (telescope) — pick the art and title to match.
interface SolveStop { title: string; hint: string; art?: IconName; }

export function Solve() {
  const { go, showToast, user } = useApp();
  const isMobile = useIsMobile();
  const invalidate = useInvalidate();
  const req = useRef(solveRequest).current;
  const sid = user?.id ?? "";
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
  const [err, setErr] = useState<SolveStop>();
  const taskStart = useRef(Date.now());
  const sessionStart = useRef(Date.now());
  const [finished, setFinished] = useState(false);

  // Snapshot the forecast as it was BEFORE this session, so the Results screen
  // can show the honest «Прогноз 58 → 60» delta after the refetch.
  const forecast = useForecast(req ? sid : "", req?.subject ?? "math");
  const forecastBefore = useRef<Forecast | null>(null);
  useEffect(() => {
    if (forecast.data && !forecastBefore.current) forecastBefore.current = forecast.data;
  }, [forecast.data]);

  useEffect(() => {
    if (!req) { setErr({ title: "Не получилось", hint: "Не задан предмет" }); setLoading(false); return; }
    (async () => {
      try {
        if (req.testId) {
          // Assigned/composed variant: solve exactly the test's tasks; the
          // attempt carries assignment_id so finishing marks it done.
          const list = await api.testTasks(req.testId);
          if (list.length === 0) { setErr({ title: "Пока пусто", hint: "В этом тесте нет заданий.", art: "telescope" }); setLoading(false); return; }
          const att = await api.startAttempt(req.testId, req.assignmentId);
          setAttemptId(att.id); setTasks(list);
        } else {
          const { attempt_id } = await api.startPractice(req.subject);
          // The pools are assembled server-side and all exclude what's already
          // mastered (solved correctly ≥2×): the mistake queue, the smart mix,
          // the per-номер drill, or free practice across the subject.
          let list: TaskView[];
          if (req.mode === "mistakes") list = await api.mistakeTasks(req.subject, 15);
          else if (req.mode === "recommended") list = (await api.recommended(req.subject, 12)).tasks;
          else list = await api.practiceTasks(req.subject, req.number ? 15 : 20, req.number);
          if (list.length === 0) {
            setErr(req.mode === "mistakes"
              ? { title: "Так держать!", hint: "Ошибок на разбор нет — очередь пуста.", art: "medal" }
              : req.number
                ? { title: "Номер освоен", hint: "Ты уже решил все задания этого номера — молодец!", art: "medal" }
                : { title: "Пока нет новых заданий", hint: "Либо всё освоено, либо банк пуст. Учитель может собрать вариант — он подтянет задания.", art: "telescope" });
            setLoading(false); return;
          }
          setAttemptId(attempt_id); setTasks(list.slice(0, 15));
        }
        setLoading(false);
        taskStart.current = Date.now();
        sessionStart.current = Date.now();
      } catch (e) { setErr({ title: "Не получилось", hint: String((e as Error).message) }); setLoading(false); }
    })();
  }, [req]);

  const finishSession = () => {
    api.finish(attemptId).catch(() => {});
    // An assigned test just became "done" — refresh the dashboard feed.
    if (req?.assignmentId) invalidate("assignments");
    invalidate("attempts");
    // The session just moved the training state: mistakes solved correctly left
    // the queue, drilled tasks may be mastered now, a пробник got its score.
    invalidate("practice-overview");
    invalidate("self-variants");
    // The answers also moved the score forecast (Results shows the delta), the
    // heatmap and with it the streak and the daily-goal ring.
    invalidate("forecast");
    invalidate("heatmap");
    setFinished(true);
  };

  if (loading) return <Loading label="Готовим задания…" />;
  if (err) return <Empty title={err.title} hint={err.hint} art={err.art} action={<Button onClick={() => go("dashboard")}>На главную</Button>} />;
  if (finished) return <Results tasks={tasks} done={done} forecast={forecast.data} forecastBefore={forecastBefore.current} onExit={() => go("dashboard")} />;

  const task = tasks[idx];
  const submit = async () => {
    if (!draft.trim()) { showToast("Введите ответ"); return; }
    const dt = Date.now() - taskStart.current;
    try {
      const r = await api.submit(attemptId, task.id, draft, dt);
      setSubmitted({ ok: r.is_correct, solution: r.solution });
      setCombo((c) => (r.is_correct ? c + 1 : 0));
      setDone((d) => [...d.filter((x) => x.taskId !== task.id), { taskId: task.id, number: task.number, correct: r.is_correct }]);
      if (r.is_correct) {
        // The salute scales with the run (3/5/10) — a combo earns a bigger sky.
        const run = combo + 1;
        confettiBurst({ count: run >= 10 ? 110 : run >= 5 ? 70 : run >= 3 ? 46 : 26 });
      }
    } catch (e) { showToast(String((e as Error).message)); }
  };
  const next = () => {
    if (idx >= tasks.length - 1) { finishSession(); return; }
    setIdx(idx + 1); setDraft(""); setSubmitted(null); taskStart.current = Date.now();
  };

  return (
    <div style={{ maxWidth: 720, margin: "0 auto", display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      {req?.title && <div style={{ fontWeight: 700, fontSize: 16 }}>{req.title}</div>}
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <span className="mono" style={{ color: "var(--text-2)", display: "inline-flex", alignItems: "center", gap: 12 }}>
          {idx + 1} / {tasks.length}
          <SessionTimer since={sessionStart.current} />
        </span>
        {/* Per-task dots (they wrap instead of squashing the row); on a PHONE a
            big composed variant gets a slim progress bar instead — 100 dots
            crushed the counter and «Завершить» off the screen. Desktop keeps
            the per-task green/red dots at any size. */}
        {tasks.length <= 30 || !isMobile ? (
          <div style={{ display: "flex", gap: 6, flexWrap: "wrap", justifyContent: "center", minWidth: 0, flex: "1 1 120px" }}>
            {tasks.map((t, i) => {
              const a = done.find((x) => x.taskId === t.id);
              return <div key={t.id} style={{
                width: 9, height: 9, borderRadius: 999,
                background: a ? (a.correct ? "var(--ok)" : "var(--bad)") : i === idx ? "var(--text-3)" : "var(--border-2)",
              }} />;
            })}
          </div>
        ) : (
          <div style={{ flex: "1 1 120px", height: 6, borderRadius: 999, background: "color-mix(in srgb, var(--text) 8%, transparent)", overflow: "hidden" }}>
            <div style={{ width: `${Math.round((done.length / tasks.length) * 100)}%`, height: "100%", background: "var(--accent)" }} />
          </div>
        )}
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
            background: submitted.ok ? "var(--ok-soft)" : "var(--bad-soft)",
            color: submitted.ok ? "var(--ok)" : "var(--bad)",
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
              <div className="mono" style={{ fontSize: 14, overflowWrap: "anywhere" }}>Правильный ответ: {submitted.solution.join(" / ")}</div>
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
// ForecastDelta — the «Прогноз 58 → 60 · +2 первичных» pill: the session's
// visible effect on the score. Growth celebrates in accent, a dip warns
// softly, no movement stays neutral; the forecast is honestly labelled an
// ориентир (scoring.Predict is a placeholder table).
function ForecastDelta({ before, after }: { before: Forecast; after: Forecast }) {
  const dTest = after.test_score - before.test_score;
  const dPrim = after.primary_estimate - before.primary_estimate;
  const moved = dTest !== 0 || dPrim !== 0;
  const up = dTest > 0 || (dTest === 0 && dPrim > 0);
  const [bg, color] = !moved
    ? ["color-mix(in srgb, var(--text) 8%, transparent)", "var(--text-3)"]
    : up ? ["var(--accent-soft)", "var(--accent-2)"] : ["var(--warn-soft)", "var(--warn)"];
  return (
    <div style={{ marginTop: 14, display: "flex", flexDirection: "column", alignItems: "center", gap: 6 }}>
      <span className="mono" style={{ background: bg, color, borderRadius: 999, padding: "6px 14px", fontSize: 13, fontWeight: 700 }}>
        {dTest !== 0
          ? <>Прогноз {before.test_score} → {after.test_score}{dPrim !== 0 && <> · {dPrim > 0 ? `▲ +${dPrim}` : `▼ ${dPrim}`} первичн.</>}</>
          : dPrim !== 0
            ? <>{dPrim > 0 ? `▲ +${dPrim}` : `▼ ${dPrim}`} {pluralRu(Math.abs(dPrim), ["первичный балл", "первичных балла", "первичных баллов"])}</>
            : <>Прогноз {after.test_score} — без изменений</>}
      </span>
      <span style={{ color: "var(--text-3)", fontSize: 11 }}>Прогноз — ориентир, не гарантия.</span>
    </div>
  );
}

function Results({ tasks, done, forecast, forecastBefore, onExit }: {
  tasks: TaskView[]; done: Answered[];
  forecast?: Forecast; forecastBefore?: Forecast | null;
  onExit: () => void;
}) {
  const correct = done.filter((d) => d.correct).length;
  const pct = tasks.length ? Math.round((correct / tasks.length) * 100) : 0;
  const perfect = pct === 100 && tasks.length > 0;
  // A perfect variant earns one big final salute (reduced-motion → silence,
  // handled inside confettiBurst).
  useEffect(() => {
    if (perfect) confettiBurst({ count: 160 });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return (
    <div style={{ maxWidth: 640, margin: "0 auto", display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <Card style={{ textAlign: "center", padding: "clamp(20px, 6vw, 34px)" }}>
        <Label>Итоги</Label>
        <div className="mono" style={{ fontSize: 54, fontWeight: 700, letterSpacing: "-0.02em", color: accColor(pct), margin: "10px 0" }}>{pct}%</div>
        <div style={{ color: "var(--text-2)" }}>{correct} из {tasks.length} верно{perfect ? " — идеально!" : ""}</div>
        {forecast && forecastBefore && <ForecastDelta before={forecastBefore} after={forecast} />}
      </Card>
      <Section title="По заданиям">
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          {tasks.map((t) => {
            const a = done.find((d) => d.taskId === t.id);
            return (
              <div key={t.id} style={{ display: "flex", justifyContent: "space-between", padding: "8px 12px", background: "var(--surface-2)", borderRadius: 12 }}>
                <span className="mono">№{t.number}</span>
                {/* «верно» = success → green ok token (blue is reserved for actions). */}
                <span style={{ color: a ? (a.correct ? "var(--ok)" : "var(--bad)") : "var(--text-3)" }}>
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
                <div key={a.id} style={{ display: "flex", justifyContent: "space-between", padding: "10px 12px", background: "var(--surface-2)", borderRadius: 12 }}>
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
                <div key={it.answer_id} style={{ display: "flex", justifyContent: "space-between", gap: 10, flexWrap: "wrap", padding: "8px 12px", background: "var(--surface-2)", borderRadius: 12 }}>
                  <span className="mono" style={{ minWidth: 0, overflowWrap: "anywhere" }}>№{it.number} · {it.raw_answer}</span>
                  {/* «верно» = success → green ok token (blue is reserved for actions). */}
                  <span style={{ color: it.is_correct ? "var(--ok)" : "var(--bad)" }}>{it.is_correct ? "верно" : "неверно"}</span>
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
