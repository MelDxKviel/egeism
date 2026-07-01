import { useMemo, useRef, useState } from "react";
import { BarChart, Bar, XAxis, YAxis, Tooltip, ResponsiveContainer, Cell, CartesianGrid } from "recharts";
import {
  api, SubjectCode, TestKind, Task, TaskStatus, AnswerSchema, uploadTasks,
  useForecast, useHeatmap, useWeakSpots, useMastery, useAttempts,
  useAdminTasks, useTests, useTestDetail, useInvalidate,
} from "./api";
import { useApp, useStudentId } from "./state";
import { Card, Label, Pill, Button, Async, Empty, Loading, accColor, SUBJECT_TITLES, testTitle, MediaBlock, StatementView } from "./ui";
import { ScoreGauge, computeStreak, WeakSpotsList, Section } from "./charts";
import { StreakBadge } from "./student";
import { Icon } from "./icons";

const SUBJECTS: SubjectCode[] = ["rus", "math", "inf", "soc"];
const grid = { display: "grid", gap: "var(--gap)", gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))" } as const;

function SubjectTabs({ value, onChange }: { value: SubjectCode; onChange: (s: SubjectCode) => void }) {
  return (
    <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
      {SUBJECTS.map((c) => (
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

// ---------- Teacher dashboard ----------
export function TeacherDashboard() {
  const { subject, setSubject, go } = useApp();
  const sid = useStudentId();
  const forecast = useForecast(sid, subject);
  const heat = useHeatmap(sid);
  const weak = useWeakSpots(sid, subject);
  const attempts = useAttempts(sid);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <SubjectTabs value={subject} onChange={setSubject} />
      <div style={grid}>
        <Card>
          <Label>Ученик · {SUBJECT_TITLES[subject]}</Label>
          <Async q={forecast}>{(f) => (
            <div style={{ display: "flex", alignItems: "center", gap: 20, marginTop: 8 }}>
              <ScoreGauge score={f.test_score} />
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                <div><span className="mono" style={{ fontSize: 24, fontWeight: 800 }}>{Math.round(f.accuracy * 100)}%</span><div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>точность</div></div>
                <Async q={heat}>{(h) => <Pill tone="accent"><StreakBadge>{computeStreak(h)} дней</StreakBadge></Pill>}</Async>
              </div>
            </div>
          )}</Async>
          <div style={{ marginTop: 14 }}><Button variant="ghost" onClick={() => go("t-student")}>Подробная статистика</Button></div>
        </Card>

        <Section title="Слабые места" right={<Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => go("t-builder")}>Собрать дрилл</Button>}>
          <Async q={weak}>{(w) => <WeakSpotsList spots={w} onDrill={() => go("t-builder")} />}</Async>
        </Section>
      </div>

      <Section title="Свежие попытки">
        <Async q={attempts}>{(list) => list.length === 0 ? <div style={{ color: "var(--text-2)" }}>Ученик ещё не решал.</div> : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {list.map((a) => (
              <div key={a.id} style={{ display: "flex", justifyContent: "space-between", padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10 }}>
                <div><div style={{ fontWeight: 600 }}>{testTitle(a.title)}</div><div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>{new Date(a.started_at).toLocaleString("ru")}</div></div>
                <span className="mono" style={{ color: accColor(a.total ? (Number(a.correct) / Number(a.total)) * 100 : 0), fontWeight: 700 }}>{a.correct}/{a.total}</span>
              </div>
            ))}
          </div>
        )}</Async>
      </Section>
    </div>
  );
}

// ---------- Student stats ----------
export function StudentStats() {
  const { subject, setSubject } = useApp();
  const sid = useStudentId();
  const mastery = useMastery(sid, subject);
  const weak = useWeakSpots(sid, subject);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <SubjectTabs value={subject} onChange={setSubject} />
      <Section title="Успешность по номерам">
        <Async q={mastery}>{(rows) => rows.length === 0 ? <div style={{ color: "var(--text-2)" }}>Нет данных.</div> : (
          <ResponsiveContainer width="100%" height={260}>
            <BarChart data={rows.map((r) => ({ number: `№${r.number}`, acc: r.total ? Math.round((r.correct / r.total) * 100) : 0 }))} margin={{ top: 8, right: 8, left: -20, bottom: 0 }}>
              <CartesianGrid stroke="var(--border)" vertical={false} />
              <XAxis dataKey="number" stroke="var(--text-3)" fontSize={10} tickLine={false} interval={0} angle={-30} textAnchor="end" height={44} />
              <YAxis domain={[0, 100]} stroke="var(--text-3)" fontSize={11} tickLine={false} />
              <Tooltip contentStyle={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 10, fontSize: 12 }} />
              <Bar dataKey="acc" radius={[4, 4, 0, 0]}>
                {rows.map((r) => { const p = r.total ? (r.correct / r.total) * 100 : 0; return <Cell key={r.number} fill={accColor(p)} />; })}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        )}</Async>
      </Section>
      <div style={grid}>
        <Section title="Слабые места"><Async q={weak}>{(w) => <WeakSpotsList spots={w} onDrill={() => {}} />}</Async></Section>
        <Section title="Дольше всего решает">
          <Async q={mastery}>{(rows) => {
            const top = [...rows].sort((a, b) => b.avg_time_ms - a.avg_time_ms).slice(0, 5);
            return <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {top.map((r) => (
                <div key={r.number} style={{ display: "flex", justifyContent: "space-between", padding: "8px 12px", background: "var(--surface-2)", borderRadius: 10 }}>
                  <span className="mono">№{r.number}</span>
                  <span className="mono" style={{ color: "var(--warn)" }}>{Math.round(r.avg_time_ms / 1000)} с</span>
                </div>
              ))}
            </div>;
          }}</Async>
        </Section>
      </div>
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
  const [kind, setKind] = useState<TestKind>("classic");
  const [number, setNumber] = useState(1);
  const [count, setCount] = useState(10);
  const [busy, setBusy] = useState(false);
  const tests = useTests(subject);
  const activeTasks = useAdminTasks(`?subject=${subject}&status=active&limit=200`);

  const generate = async () => {
    setBusy(true);
    try {
      const res = await api.generateVariant(subject, kind, kind === "drill" ? { number, count } : {});
      const src = res.source === "mock" ? "демо-заглушки (РЕШУ недоступен)" : "РЕШУ";
      showToast(`Вариант собран: ${res.task_count} задач · задания с «${src}» добавлены в банк`);
      invalidate("tests");
      invalidate("admin-tasks");
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
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
          Сам подтянет нужные задания с источника (РЕШУ), сохранит их в банк и соберёт вариант.
          То есть тесты наполняют банк, а не наоборот. Может занять несколько секунд.
        </div>
        <Button onClick={generate} disabled={busy}>{busy ? "Собираю…" : "Собрать вариант"}</Button>
      </Section>

      <Section title="Готовые тесты">
        <Async q={tests}>{(list) => list.filter((t) => t.title !== "__practice__").length === 0 ? <div style={{ color: "var(--text-2)" }}>Пока нет тестов.</div> : (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {list.filter((t) => t.title !== "__practice__").map((t) => (
              <div key={t.id} onClick={() => { requestTestView(t.id); go("t-test"); }} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10, cursor: "pointer" }}>
                <div><div style={{ fontWeight: 600 }}>{t.title}</div><div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>{new Date(t.created_at).toLocaleDateString("ru")}</div></div>
                <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
                  <Pill>{t.kind}</Pill>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 5, color: "var(--accent-2)", fontSize: 13 }}>смотреть <Icon name="arrowRight" size={15} /></span>
                </div>
              </div>
            ))}
          </div>
        )}</Async>
      </Section>

      <Section title={`Банк · активные задачи (${activeTasks.data?.length ?? 0})`}>
        <Async q={activeTasks}>{(rows) => (
          <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
            {rows.map((t) => <Pill key={t.id}>№{t.number} {t.answer_kind}</Pill>)}
            {rows.length === 0 && <div style={{ color: "var(--text-2)" }}>Нет одобренных задач — одобри в банке.</div>}
          </div>
        )}</Async>
      </Section>
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
            <div style={{ fontWeight: 700, fontSize: 18 }}>{testTitle(d.test.title)}</div>
            <div className="mono" style={{ color: "var(--text-3)", fontSize: 13, marginTop: 4 }}>{d.test.kind} · {d.tasks.length} задач</div>
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
export function Assign() {
  const { subject, setSubject, showToast } = useApp();
  const tests = useTests(subject);
  const [testId, setTestId] = useState("");
  const [when, setWhen] = useState("");
  const [notify, setNotify] = useState(true);
  const [busy, setBusy] = useState(false);
  const sid = useStudentId();

  const submit = async () => {
    if (!sid) { showToast("Нет учеников — попроси ученика зарегистрироваться"); return; }
    if (!testId) { showToast("Выбери тест"); return; }
    if (!when) { showToast("Укажи время"); return; }
    setBusy(true);
    try {
      await api.createAssignment(testId, sid, new Date(when).toISOString());
      showToast(notify ? "Назначено · уведомление в Telegram запланировано" : "Назначено");
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
  };

  return (
    <div style={{ maxWidth: 560, display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <SubjectTabs value={subject} onChange={setSubject} />
      <Section title="Назначить тест">
        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
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
          <label style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 14 }}>
            <input type="checkbox" checked={notify} onChange={(e) => setNotify(e.target.checked)} style={{ width: "auto" }} />
            Уведомить в Telegram
          </label>
          <div style={{ display: "flex", gap: 10 }}>
            <Button onClick={submit} disabled={busy}>{busy ? "…" : "Назначить"}</Button>
            <Button variant="ghost" onClick={() => showToast("PDF-экспорт бланка — в разработке")}>Экспорт в PDF</Button>
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

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <SourcePanel subject={subject} onDone={refresh} />
      <SubjectTabs value={subject} onChange={setSubject} />
      <div style={{ display: "flex", gap: 8 }}>
        {(["", "draft", "active", "rejected"] as const).map((s) => (
          <ChoiceTab key={s} active={status === s} onClick={() => setStatus(s)}>{s === "" ? "Все" : s}</ChoiceTab>
        ))}
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
// subject; a file upload is kept as a secondary fallback. Both run the same
// ingest (media → MinIO, dedup, draft for curation unless "сразу активными").
function SourcePanel({ subject, onDone }: { subject: SubjectCode; onDone: () => void }) {
  const { showToast } = useApp();
  const inputRef = useRef<HTMLInputElement>(null);
  const [count, setCount] = useState(30);
  const [active, setActive] = useState(false);
  const [busy, setBusy] = useState<"fetch" | "upload" | null>(null);

  const summary = (r: { inserted: number; skipped: number; invalid: number }) => {
    if (r.inserted > 0) return `Добавлено ${r.inserted} заданий${r.skipped ? ` (${r.skipped} уже были в банке)` : ""}`;
    if (r.skipped > 0) return `Новых нет — все ${r.skipped} уже в банке`;
    return "Источник не вернул заданий";
  };

  const fetchNow = async () => {
    setBusy("fetch");
    try {
      const r = await api.fetchTasks(subject, count, active);
      const src = r.source === "mock" ? "демо-заглушки (РЕШУ недоступен)" : "РЕШУ";
      if (r.inserted > 0) showToast(`Добавлено ${r.inserted} заданий с «${src}»${r.skipped ? ` (${r.skipped} уже были)` : ""}`);
      else if (r.skipped > 0) showToast(`Новых нет — все ${r.skipped} уже в банке`);
      else showToast(`Источник (${src}) не вернул заданий`);
      onDone();
    } catch (err) {
      showToast("Не удалось подтянуть: " + String((err as Error).message));
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
      <div style={{ marginTop: 12, borderTop: "1px solid var(--border)", paddingTop: 10, display: "flex", alignItems: "center", gap: 10 }}>
        <span style={{ fontSize: 13, color: "var(--text-3)" }}>или загрузить файлом</span>
        <input ref={inputRef} type="file" accept=".json,.jsonl,application/json" onChange={onPick} style={{ display: "none" }} />
        <button onClick={() => inputRef.current?.click()} disabled={busy !== null} style={{
          background: "transparent", border: "1px solid var(--border-2)", borderRadius: 9,
          padding: "6px 12px", fontSize: 13, color: "var(--text-2)",
        }}>{busy === "upload" ? "Загрузка…" : ".json / .jsonl"}</button>
      </div>
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
