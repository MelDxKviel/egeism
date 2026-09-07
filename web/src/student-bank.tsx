import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, SubjectCode, useInvalidate } from "./api";
import { useApp } from "./state";
import { Async, Button, Card, Empty, Pill, StatementView, MediaBlock, SubjectSwitch } from "./ui";
import { requestSolve } from "./student";

export function StudentBankFetch({ subject, autoWhenEmpty = false }: { subject: SubjectCode; autoWhenEmpty?: boolean }) {
  const invalidate = useInvalidate();
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const attempted = useRef("");
  const fetchBank = async () => {
    if (busy) return;
    setBusy(true); setMessage("");
    try {
      const result = await api.fetchStudentBank(subject);
      const added = result.inserted + (result.promoted ?? 0);
      setMessage(added > 0 ? `Добавлено заданий: ${added}` : "Банк обновлён. Задания из этой загрузки уже есть в банке");
      ["student-bank", "practice-overview", "task-summary"].forEach(invalidate);
    } catch (e) { setMessage((e as Error).message); }
    finally { setBusy(false); }
  };
  useEffect(() => {
    if (autoWhenEmpty && attempted.current !== subject) { attempted.current = subject; void fetchBank(); }
  }, [subject, autoWhenEmpty]);
  return <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 12 }}>
    <Button disabled={busy} onClick={fetchBank}>{busy ? "Загружаем задания…" : "Подтянуть задания"}</Button>
    {message && <span role="status" style={{ fontSize: 13, color: "var(--text-2)" }}>{message}</span>}
  </div>;
}

export function StudentBank() {
  const { subject, go } = useApp();
  const [number, setNumber] = useState("");
  const [page, setPage] = useState(0);
  useEffect(() => { setPage(0); setNumber(""); }, [subject]);
  const tasks = useQuery({ queryKey: ["student-bank", subject, number, page], queryFn: () => api.tasks(`?subject=${subject}&status=active&limit=20&offset=${page * 20}${number ? `&number=${number}` : ""}`) });
  return <div style={{ display: "grid", gap: "var(--gap)" }}>
    <SubjectSwitch />
    <Card>
      <h2 style={{ marginTop: 0 }}>Банк заданий</h2>
      <p style={{ color: "var(--text-2)" }}>Выбирай задания для самостоятельной подготовки. Банк общий с учителем и доступен по всем предметам.</p>
      <StudentBankFetch key={subject} subject={subject} autoWhenEmpty={!number && page === 0 && tasks.isSuccess && tasks.data.length === 0} />
      <label style={{ display: "block", marginTop: 16 }}>Номер задания <input aria-label="Номер задания" type="number" min={1} max={99} placeholder="Все" value={number} onChange={(e) => { setNumber(e.target.value); setPage(0); }} style={{ width: 90, marginLeft: 8 }} /></label>
    </Card>
    <Async q={tasks}>{(list) => list.length === 0 ? <Empty title="Заданий пока нет" hint="Подтяни задания или выбери другой номер." /> : <>
      {list.map((t) => <Card key={t.id}>
        <div style={{ display: "flex", gap: 8, marginBottom: 12 }}><Pill>№{t.number}</Pill>{t.grading_mode === "manual" && <Pill tone="neutral">Часть 2 · проверяет учитель</Pill>}</div>
        <StatementView text={t.statement} media={t.media} />
        <MediaBlock media={t.media} />
        <Button style={{ marginTop: 12 }} onClick={() => { requestSolve({ subject, taskId: t.id, title: `Задание №${t.number}` }); go("solve"); }}>Решать</Button>
      </Card>)}
    </>}</Async>
    <div style={{ display: "flex", gap: 10 }}>
      <Button variant="ghost" disabled={page === 0 || tasks.isFetching} onClick={() => setPage((p) => p - 1)}>Назад</Button>
      <Button variant="ghost" disabled={(tasks.data?.length ?? 0) < 20 || tasks.isFetching} onClick={() => setPage((p) => p + 1)}>Ещё задания</Button>
    </div>
  </div>;
}
