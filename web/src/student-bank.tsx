import { useEffect } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, SubjectCode } from "./api";

// Prepare the selected subject as soon as the student enters the app. Existing
// tasks remain available while a short request schedules the refill job.
export function StudentPracticeWarmup({ subject, userId }: { subject: SubjectCode; userId: string }) {
  const client = useQueryClient();
  const start = useQuery({
    queryKey: ["practice-warmup", userId, subject],
    queryFn: () => api.fetchStudentBank(subject),
    staleTime: 10 * 60_000,
    retry: false,
  });
  const initial = start.data;
  const job = useQuery({
    queryKey: ["practice-warmup-job", userId, initial?.id],
    queryFn: () => api.studentBankSyncJob(initial!.id),
    enabled: !!initial && ["queued", "running"].includes(initial.state),
    refetchInterval: (q) => q.state.error || (q.state.data && !["queued", "running"].includes(q.state.data.state)) ? false : 2000,
    retry: 1,
  });
  const state = job.data?.state ?? initial?.state;
  useEffect(() => {
    if (state === "succeeded") void client.invalidateQueries({ queryKey: ["practice-overview"] });
  }, [state, client]);
  return null;
}

// Only a completely empty pool waits for the initial import. The solve screen
// remains navigable throughout, and leaving it stops further polling.
export async function preparePracticeBank(subject: SubjectCode, number: number | undefined, active: () => boolean, ready?: () => Promise<boolean>): Promise<void> {
  let job = await api.fetchStudentBank(subject, number);
  const deadline = Date.now() + 150_000;
  while (active() && ["queued", "running"].includes(job.state) && Date.now() < deadline) {
    await new Promise((resolve) => setTimeout(resolve, 2000));
    if (!active()) return;
    // Ingestion saves tasks incrementally. Start with the first usable batch
    // instead of waiting for every remaining source attachment to download.
    if (ready && await ready()) return;
    job = await api.studentBankSyncJob(job.id);
  }
  if (!active()) return;
  if (job.state === "failed" || job.state === "cancelled") throw new Error(job.error || "Источник пока недоступен. Попробуй тренировку позже.");
}
