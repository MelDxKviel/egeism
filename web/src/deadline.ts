import type { AssignmentCard } from "./api";

// fmtDue formats a deadline compactly for the meta line: "15.07, 18:00".
export const fmtDue = (iso: string) =>
  new Date(iso).toLocaleString("ru", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });

export type DeadlineKind = "none" | "upcoming" | "overdue" | "late" | "ontime";

export interface DeadlineInfo {
  kind: DeadlineKind;
  text: string;
  pill?: { tone: "bad" | "warn" | "ok"; label: string };
}

// deadlineInfo derives the deadline state for an assignment card so both the
// student's list and the teacher's roster render it consistently. The deadline
// is soft: "overdue" still lets the student solve (the button stays), and a
// later solve shows as "late". The status flag may lag the sweep by up to a
// minute, so "overdue" is computed from due_at < now regardless of status.
//
// `now` is a parameter (default Date.now()) so the state machine is fully
// deterministic under test — no real clock leaks into the assertions.
export function deadlineInfo(card: AssignmentCard, now: number = Date.now()): DeadlineInfo {
  if (!card.due_at) return { kind: "none", text: "" };
  const due = new Date(card.due_at);
  const text = `до ${fmtDue(card.due_at)}`;
  if (card.finished_at) {
    const solvedLate = new Date(card.finished_at) > due;
    return solvedLate
      ? { kind: "late", text, pill: { tone: "warn", label: "с опозданием" } }
      : { kind: "ontime", text, pill: { tone: "ok", label: "вовремя" } };
  }
  if (due.getTime() <= now) {
    return { kind: "overdue", text, pill: { tone: "bad", label: "просрочен" } };
  }
  return { kind: "upcoming", text };
}
