import { useState } from "react";
import {
  api, SubjectCode, SelfVariant, usePracticeOverview, useSelfVariants, useWeakSpots, useInvalidate,
} from "./api";
import { useApp } from "./state";
import { Card, Label, Button, Async, Empty, accColor, SUBJECT_TITLES } from "./ui";
import { Section } from "./charts";
import { requestSolve, useAttemptReview } from "./student";
import { pluralRu } from "./plural";

// TrainingHub is the student's self-study home: the mistake queue, the smart
// session, self-generated пробники and the per-номер training map — everything
// a student can start without waiting for the teacher.

const grid3 = { display: "grid", gap: "var(--gap)", gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))" } as const;

export function TrainingHub() {
  const { subject, setSubject, go, user, showToast } = useApp();
  const uid = user?.id ?? "";
  const overview = usePracticeOverview(uid, subject);
  const variants = useSelfVariants(uid, subject);
  const weak = useWeakSpots(uid, subject);
  const invalidate = useInvalidate();
  const [generating, setGenerating] = useState(false);
  const { open: openReview, modal: reviewModal } = useAttemptReview();

  const startMistakes = () => { requestSolve({ subject, mode: "mistakes", title: "Работа над ошибками" }); go("solve"); };
  const startRecommended = () => { requestSolve({ subject, mode: "recommended", title: "Умная тренировка" }); go("solve"); };
  const drill = (n: number) => { requestSolve({ subject, number: n, title: `Тренировка №${n}` }); go("solve"); };
  const startVariant = (v: { id: string; title: string }) => {
    requestSolve({ subject, testId: v.id, title: v.title });
    go("solve");
  };
  const generate = async () => {
    setGenerating(true);
    try {
      const r = await api.createSelfVariant(subject);
      invalidate("self-variants");
      startVariant({ id: r.test.id, title: r.test.title });
    } catch (e) {
      showToast(String((e as Error).message));
    } finally {
      setGenerating(false);
    }
  };

  // The weak-номер hint for the smart-session card («…слабые номера (№7, №12)…»).
  const weakNums = (weak.data || []).filter((w) => w.accuracy < 0.7).slice(0, 3).map((w) => `№${w.number}`);

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

      <div style={grid3}>
        <Card>
          <Label>Работа над ошибками</Label>
          <Async q={overview}>{(o) => (
            <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 10 }}>
              {o.mistakes > 0 ? (
                <>
                  <div className="mono" style={{ fontSize: 30, fontWeight: 800, color: "var(--warn)" }}>{o.mistakes}</div>
                  <div style={{ color: "var(--text-2)", fontSize: 13, lineHeight: 1.45 }}>
                    {pluralRu(o.mistakes, ["задание ждёт", "задания ждут", "заданий ждут"])} второго шанса.
                    Реши верно — и они уйдут из очереди.
                  </div>
                  <Button onClick={startMistakes}>Разобрать ошибки</Button>
                </>
              ) : (
                <div style={{ color: "var(--text-2)", fontSize: 13, lineHeight: 1.45 }}>
                  Очередь пуста — все ошибки исправлены. Новые попадут сюда сами, как только появятся.
                </div>
              )}
            </div>
          )}</Async>
        </Card>

        <Card>
          <Label>Умная тренировка</Label>
          <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 10 }}>
            <div style={{ color: "var(--text-2)", fontSize: 13, lineHeight: 1.45 }}>
              Короткая сессия под тебя: сначала ошибки, потом слабые номера{weakNums.length > 0 ? ` (${weakNums.join(", ")})` : ""}, потом новое.
            </div>
            <Button onClick={startRecommended}>Начать</Button>
          </div>
        </Card>

        <Card>
          <Label>Пробный вариант</Label>
          <div style={{ display: "flex", flexDirection: "column", gap: 10, marginTop: 10 }}>
            <div style={{ color: "var(--text-2)", fontSize: 13, lineHeight: 1.45 }}>
              Полный вариант из банка — по случайному заданию каждого номера, как на экзамене. Таймер идёт.
            </div>
            <Button onClick={generate} disabled={generating}>{generating ? "Собираем…" : "Собрать пробник"}</Button>
          </div>
        </Card>
      </div>

      <Section title="Тренировка по номерам" right={
        <span style={{ color: "var(--text-3)", fontSize: 12 }}>реши задание верно дважды — номер зачтётся</span>
      }>
        <Async q={overview}>{(o) => o.numbers.length === 0
          ? <Empty title="Банк пока пуст" hint="Попроси учителя подтянуть задания — карта номеров появится здесь." />
          : (
            <div style={{ display: "grid", gap: 10, gridTemplateColumns: "repeat(auto-fill, minmax(150px, 1fr))" }}>
              {o.numbers.map((n) => {
                const pct = n.answers_total > 0 ? Math.round((n.answers_correct / n.answers_total) * 100) : null;
                const empty = n.bank_active === 0;
                const done = !empty && n.mastered >= n.bank_active;
                const progress = n.bank_active > 0 ? Math.min(100, Math.round((n.mastered / n.bank_active) * 100)) : 0;
                return (
                  <Card key={n.number} onClick={empty ? undefined : () => drill(n.number)}
                    style={{ padding: 14, cursor: empty ? "default" : "pointer", opacity: empty ? 0.55 : 1 }}>
                    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                      <span className="mono" style={{ fontWeight: 700, fontSize: 15 }}>№{n.number}</span>
                      {pct !== null
                        ? <span className="mono" style={{ color: accColor(pct), fontWeight: 700, fontSize: 13 }} title="точность твоих ответов">{pct}%</span>
                        : <span className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>новое</span>}
                    </div>
                    <div style={{ marginTop: 10, height: 6, borderRadius: 999, background: "var(--surface-2)", overflow: "hidden" }}>
                      <div style={{ width: `${progress}%`, height: "100%", borderRadius: 999, background: done ? "var(--accent)" : "var(--accent-2)" }} />
                    </div>
                    <div className="mono" style={{ color: "var(--text-3)", fontSize: 11, marginTop: 6 }}>
                      {empty ? "нет заданий в банке" : done ? "освоено ✓" : `освоено ${n.mastered} из ${n.bank_active}`}
                    </div>
                  </Card>
                );
              })}
            </div>
          )}</Async>
      </Section>

      <Section title="Мои пробники">
        <Async q={variants}>{(list) => list.length === 0
          ? <div style={{ color: "var(--text-2)", fontSize: 14 }}>Собери первый пробник — он появится здесь вместе с результатом.</div>
          : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {list.map((v) => <SelfVariantRow key={v.id} v={v} onStart={startVariant} onReview={openReview} />)}
            </div>
          )}</Async>
      </Section>
      {reviewModal}
    </div>
  );
}

// SelfVariantRow is one generated пробник: unsolved shows «Начать», solved shows
// the score, «Разбор» and «Ещё раз» (a fresh attempt on the same variant).
function SelfVariantRow({ v, onStart, onReview }: {
  v: SelfVariant;
  onStart: (v: { id: string; title: string }) => void;
  onReview: (card: { attempt_id?: string; title: string }) => void;
}) {
  const solved = !!v.finished_at;
  const pct = v.total > 0 ? Math.round((v.correct / v.total) * 100) : 0;
  return (
    <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, padding: 12, background: "var(--surface-2)", borderRadius: 12 }}>
      <div>
        <div style={{ fontWeight: 600 }}>{v.title}</div>
        <div className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>
          {new Date(v.created_at).toLocaleString("ru")} · {v.task_count} {pluralRu(v.task_count, ["задание", "задания", "заданий"])}
          {solved && v.finished_at ? ` · решён ${new Date(v.finished_at).toLocaleString("ru")}` : ""}
        </div>
      </div>
      {solved ? (
        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span className="mono" title={`${pct}% верно`} style={{ color: accColor(pct), fontWeight: 700 }}>{v.correct}/{v.total}</span>
          {v.attempt_id && <Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => onReview({ attempt_id: v.attempt_id, title: v.title })}>Разбор</Button>}
          <Button variant="soft" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => onStart({ id: v.id, title: v.title })}>Ещё раз</Button>
        </div>
      ) : (
        <Button variant="soft" onClick={() => onStart({ id: v.id, title: v.title })}>Начать</Button>
      )}
    </div>
  );
}
