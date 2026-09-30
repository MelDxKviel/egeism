import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api, BankSyncJob } from "./api";
import { preparePracticeBank } from "./student-bank";

vi.mock("./api", () => ({ api: { fetchStudentBank: vi.fn(), studentBankSyncJob: vi.fn() } }));

const job = (state: BankSyncJob["state"], error?: string): BankSyncJob => ({
  id: "job-1", kind: "fetch", subject: "math", number: 0, limit: 30, active: true,
  state, created_at: "2026-09-30T09:00:00Z", error,
});

describe("cold practice preparation", () => {
  beforeEach(() => { vi.useFakeTimers(); vi.resetAllMocks(); });
  afterEach(() => vi.useRealTimers());

  it("does not poll an already completed refill", async () => {
    vi.mocked(api.fetchStudentBank).mockResolvedValue(job("succeeded"));
    await preparePracticeBank("math", undefined, () => true);
    expect(api.fetchStudentBank).toHaveBeenCalledTimes(1);
    expect(api.studentBankSyncJob).not.toHaveBeenCalled();
  });

  it("polls the accepted job without sending more fetch requests", async () => {
    vi.mocked(api.fetchStudentBank).mockResolvedValue(job("queued"));
    vi.mocked(api.studentBankSyncJob).mockResolvedValueOnce(job("running")).mockResolvedValueOnce(job("succeeded"));
    const prepare = preparePracticeBank("math", 3, () => true);
    await vi.runAllTimersAsync();
    await prepare;
    expect(api.fetchStudentBank).toHaveBeenCalledTimes(1);
    expect(api.fetchStudentBank).toHaveBeenCalledWith("math", 3);
    expect(api.studentBankSyncJob).toHaveBeenCalledTimes(2);
  });

  it("stops polling when the student leaves while the job continues server-side", async () => {
    let active = true;
    vi.mocked(api.fetchStudentBank).mockResolvedValue(job("running"));
    const prepare = preparePracticeBank("math", undefined, () => active);
    await vi.advanceTimersByTimeAsync(0);
    active = false;
    await vi.runAllTimersAsync();
    await prepare;
    expect(api.studentBankSyncJob).not.toHaveBeenCalled();
  });

  it("starts practice as soon as a partial batch is available", async () => {
    vi.mocked(api.fetchStudentBank).mockResolvedValue(job("running"));
    const ready = vi.fn().mockResolvedValue(true);
    const prepare = preparePracticeBank("math", undefined, () => true, ready);
    await vi.runAllTimersAsync();
    await prepare;
    expect(ready).toHaveBeenCalledTimes(1);
    expect(api.studentBankSyncJob).not.toHaveBeenCalled();
  });

  it("surfaces a source failure without looping indefinitely", async () => {
    vi.mocked(api.fetchStudentBank).mockResolvedValue(job("failed", "Нет подтверждённых свежих заданий"));
    await expect(preparePracticeBank("math", undefined, () => true)).rejects.toThrow("Нет подтверждённых свежих заданий");
    expect(api.studentBankSyncJob).not.toHaveBeenCalled();
  });
});
