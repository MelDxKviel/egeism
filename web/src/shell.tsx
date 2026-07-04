import { ReactNode, useEffect, useRef, useState } from "react";
import { useApp, View } from "./state";
import { Icon, IconName } from "./icons";
import { api, User, NotificationItem, useNotifications, useSubjects, useInvalidate } from "./api";
import { Button, Loading, Modal, Spinner, SUBJECT_TITLES, testTitle } from "./ui";
import { requestSolve } from "./student";
import { requestTestView } from "./teacher";

function useIsMobile() {
  const [m, setM] = useState(window.innerWidth < 900);
  useEffect(() => {
    const on = () => setM(window.innerWidth < 900);
    window.addEventListener("resize", on);
    return () => window.removeEventListener("resize", on);
  }, []);
  return m;
}

const STUDENT_NAV: { v: View; label: string; icon: IconName }[] = [
  { v: "dashboard", label: "Дашборд", icon: "dashboard" },
  { v: "subject", label: "Предмет", icon: "target" },
  { v: "history", label: "История", icon: "history" },
  { v: "profile", label: "Профиль", icon: "user" },
];
const TEACHER_NAV: { v: View; label: string; icon: IconName }[] = [
  { v: "t-dashboard", label: "Ученики", icon: "overview" },
  { v: "t-builder", label: "Тесты", icon: "tests" },
  { v: "t-assign", label: "Назначить", icon: "assign" },
  { v: "t-bank", label: "Банк", icon: "bank" },
  { v: "profile", label: "Профиль", icon: "user" },
];
const ADMIN_NAV: { v: View; label: string; icon: IconName }[] = [
  { v: "a-stats", label: "Обзор", icon: "overview" },
  { v: "a-users", label: "Пользователи", icon: "user" },
  { v: "profile", label: "Профиль", icon: "user" },
];
const ROLE_RU: Record<string, string> = { teacher: "учитель", student: "ученик", admin: "админ" };

export function Shell({ title, cta, children }: { title: string; cta?: ReactNode; children: ReactNode }) {
  const { theme, role, view, user, go, logout, setTheme } = useApp();
  const isMobile = useIsMobile();
  const nav = role === "admin" ? ADMIN_NAV : role === "teacher" ? TEACHER_NAV : STUDENT_NAV;

  return (
    <div className="app" data-theme={theme} style={{ minHeight: "100vh" }}>
      <div style={{ display: "flex", minHeight: "100vh" }}>
        {!isMobile && (
          <aside style={{
            width: 246, flex: "none", position: "sticky", top: 0, height: "100vh",
            background: "var(--surface)", borderRight: "1px solid var(--border)",
            display: "flex", flexDirection: "column", padding: "22px 16px 18px",
          }}>
            <div style={{ display: "flex", alignItems: "center", gap: 11, padding: "0 6px 18px" }}>
              <div style={{ width: 34, height: 34, borderRadius: 10, background: "var(--accent)", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--on-accent)" }}>
                <Icon name="logo" size={18} strokeWidth={2.4} />
              </div>
              <div style={{ display: "flex", flexDirection: "column", lineHeight: 1.1 }}>
                <span style={{ fontWeight: 800, fontSize: 17 }}>ЕГЭизм</span>
                <span className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>подготовка · ЕГЭ</span>
              </div>
            </div>
            <nav style={{ display: "flex", flexDirection: "column", gap: 3, flex: 1 }}>
              {nav.map((n) => (
                <NavBtn key={n.v} active={view === n.v} onClick={() => go(n.v)} icon={n.icon} label={n.label} />
              ))}
            </nav>
            <div style={{ borderTop: "1px solid var(--border)", paddingTop: 12, display: "flex", flexDirection: "column", gap: 10 }}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "0 4px" }}>
                <div style={{ display: "flex", flexDirection: "column", lineHeight: 1.2, minWidth: 0 }}>
                  <span style={{ fontWeight: 600, fontSize: 14, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{user?.name}</span>
                  <span className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>{ROLE_RU[role || "student"]}</span>
                </div>
                <button onClick={logout} title="Выйти" style={{
                  display: "flex", alignItems: "center", justifyContent: "center",
                  background: "transparent", border: "1px solid var(--border)", borderRadius: 8,
                  padding: 8, color: "var(--text-2)",
                }}><Icon name="logout" size={17} /></button>
              </div>
              <button onClick={() => setTheme(theme === "light" ? "dark" : "light")} style={{
                display: "flex", alignItems: "center", gap: 9,
                background: "transparent", border: "1px solid var(--border)", borderRadius: 10,
                padding: "8px 12px", color: "var(--text-2)", fontSize: 13,
              }}>
                <Icon name={theme === "light" ? "moon" : "sun"} size={16} />
                {theme === "light" ? "Тёмная тема" : "Светлая тема"}
              </button>
              <TelegramLink user={user} />
            </div>
          </aside>
        )}

        <main style={{ flex: 1, minWidth: 0, paddingBottom: isMobile ? 90 : 0 }}>
          <header style={{
            position: "sticky", top: 0, zIndex: 5, height: 60, background: "var(--bg)",
            borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center",
            justifyContent: "space-between", padding: "0 var(--main-pad)",
          }}>
            <div style={{ fontWeight: 700, fontSize: 18 }}>{title}</div>
            <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
              {cta}
              <NotificationsBell />
              {isMobile && (
                <>
                  <TelegramLink user={user} compact />
                  <button onClick={() => setTheme(theme === "light" ? "dark" : "light")} title="Сменить тему" style={{
                    display: "flex", alignItems: "center", background: "transparent", border: "1px solid var(--border)", borderRadius: 10, padding: 8, color: "var(--text-2)",
                  }}><Icon name={theme === "light" ? "moon" : "sun"} size={17} /></button>
                  <button onClick={logout} title="Выйти" style={{
                    display: "flex", alignItems: "center", background: "transparent", border: "1px solid var(--border)", borderRadius: 10, padding: 8, color: "var(--text-2)",
                  }}><Icon name="logout" size={17} /></button>
                </>
              )}
            </div>
          </header>
          <div className="fade" style={{ padding: "var(--main-pad)", maxWidth: 1200, margin: "0 auto" }}>{children}</div>
        </main>
      </div>

      {isMobile && (
        <nav style={{
          position: "fixed", bottom: 0, left: 0, right: 0, height: 66, background: "var(--surface)",
          borderTop: "1px solid var(--border)", display: "flex", justifyContent: "space-around", alignItems: "center", zIndex: 10,
        }}>
          {nav.map((n) => (
            <button key={n.v} onClick={() => go(n.v)} style={{
              background: "none", border: "none", display: "flex", flexDirection: "column", alignItems: "center", gap: 4,
              color: view === n.v ? "var(--accent)" : "var(--text-3)", fontSize: 11,
            }}>
              <Icon name={n.icon} size={21} strokeWidth={view === n.v ? 2.1 : 1.75} />{n.label}
            </button>
          ))}
        </nav>
      )}
    </div>
  );
}

// ---------- Notifications (the header bell) ----------

// notifText builds the human line for one notification. assignment_created is
// only ever delivered to the student, assignment_done to the teacher who
// assigned, so kind alone decides the wording. The arrival time is shown
// separately (relTime), so the sub only carries the distinct extra fact — the
// due date for an assignment, the subject for a solved test.
function notifText(n: NotificationItem, subjectCode?: string): { title: string; sub: string } {
  const subj = subjectCode ? SUBJECT_TITLES[subjectCode] : undefined;
  if (n.kind === "assignment_created") {
    const due = new Date(n.scheduled_at).toLocaleString("ru", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
    return {
      title: `Тебе назначен тест «${testTitle(n.test_title)}»`,
      sub: [subj, `на ${due}`].filter(Boolean).join(" · "),
    };
  }
  return {
    title: `${n.student_name} решил(а) тест «${testTitle(n.test_title)}»`,
    sub: subj ?? "",
  };
}

// relTime renders how long ago a notification arrived — «только что», «5 мин»,
// «3 ч», «2 дн» — then falls back to an absolute date for anything older than a
// week. The exact timestamp rides along as a title tooltip (fullTime).
function relTime(iso: string): string {
  const mins = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 60000));
  if (mins < 1) return "только что";
  if (mins < 60) return `${mins} мин`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs} ч`;
  const days = Math.floor(hrs / 24);
  if (days < 7) return `${days} дн`;
  return new Date(iso).toLocaleDateString("ru", { day: "2-digit", month: "2-digit" });
}
const fullTime = (iso: string) =>
  new Date(iso).toLocaleString("ru", { day: "2-digit", month: "2-digit", year: "numeric", hour: "2-digit", minute: "2-digit" });

// NotificationsBell polls the in-app feed, shows the unread badge, and drops a
// themed popover list anchored under the icon — no dimming backdrop; a tap
// outside or Esc closes it. On desktop it's a compact dropdown right below the
// bell; on mobile it becomes a near-full-width sheet pinned just under the
// header (comfortable to read and tap, kept clear of the bottom nav). Each row
// carries its arrival time. Clicking a notification marks it read and jumps to
// the test: the student straight into solving the assigned variant, the teacher
// into the test view.
function NotificationsBell() {
  const { go, role, subject, user, showToast } = useApp();
  const isMobile = useIsMobile();
  const invalidate = useInvalidate();
  const feed = useNotifications(user?.id ?? "");
  const subjects = useSubjects();
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLDivElement>(null);

  const codeOf = (subjectId: string) => subjects.data?.find((s) => s.id === subjectId)?.code;

  // Toast genuinely new notifications (not the initial load) and refresh the
  // feeds they imply — a fresh assignment / a just-solved test.
  const seen = useRef<Set<string> | null>(null);
  useEffect(() => {
    const items = feed.data?.items;
    if (!items) return;
    if (seen.current === null) { seen.current = new Set(items.map((i) => i.id)); return; }
    const fresh = items.filter((i) => !seen.current!.has(i.id) && !i.read_at);
    items.forEach((i) => seen.current!.add(i.id));
    if (fresh.length === 0) return;
    invalidate("assignments");
    invalidate("attempts");
    showToast(`🔔 ${notifText(fresh[0], codeOf(fresh[0].subject_id)).title}`);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [feed.data]);

  // Close the popover on an outside pointer (mouse or touch) or Esc. The bell
  // itself lives inside wrapRef, so tapping it toggles rather than double-fires.
  useEffect(() => {
    if (!open) return;
    const onDown = (e: PointerEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false); };
    document.addEventListener("pointerdown", onDown);
    window.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      window.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const markAll = () => api.markAllNotificationsRead().then(() => invalidate("notifications")).catch(() => {});

  const openItem = (n: NotificationItem) => {
    if (!n.read_at) api.markNotificationRead(n.id).then(() => invalidate("notifications")).catch(() => {});
    setOpen(false);
    if (n.kind === "assignment_created") {
      if (n.assignment_status === "done") { showToast("Этот тест уже решён ✓"); return; }
      requestSolve({
        subject: codeOf(n.subject_id) ?? subject,
        testId: n.test_id, assignmentId: n.assignment_id, title: testTitle(n.test_title),
      });
      go("solve");
    } else {
      requestTestView(n.test_id);
      go("t-test");
    }
  };

  const unread = feed.data?.unread ?? 0;
  const items = feed.data?.items ?? [];

  return (
    <div ref={wrapRef} style={{ position: "relative", display: "inline-flex" }}>
      <button onClick={() => setOpen((o) => !o)} title="Уведомления" style={{
        display: "flex", alignItems: "center", position: "relative",
        background: open ? "var(--accent-soft)" : "transparent",
        border: "1px solid var(--border)", borderRadius: 10, padding: 8,
        color: open ? "var(--accent-2)" : "var(--text-2)", transition: "background .15s, color .15s",
      }}>
        <Icon name="bell" size={17} />
        {unread > 0 && (
          <span className="mono" style={{
            position: "absolute", top: -5, right: -5, minWidth: 16, height: 16, padding: "0 4px",
            borderRadius: 999, background: "var(--bad)", color: "#fff", fontSize: 10, fontWeight: 700,
            display: "flex", alignItems: "center", justifyContent: "center", boxSizing: "border-box",
          }}>{unread > 9 ? "9+" : unread}</span>
        )}
      </button>

      {open && (
        <div className="popdown" style={{
          position: isMobile ? "fixed" : "absolute",
          ...(isMobile
            ? { top: 68, left: 8, right: 8, maxHeight: "calc(100vh - 150px)" }
            : { top: "calc(100% + 10px)", right: 0, width: 384, maxHeight: "min(72vh, 540px)" }),
          background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 16,
          boxShadow: "var(--shadow-lg)", zIndex: 60, transformOrigin: "top right",
          display: "flex", flexDirection: "column", overflow: "hidden",
        }}>
          <div style={{
            display: "flex", alignItems: "center", justifyContent: "space-between", gap: 10,
            padding: "13px 16px", borderBottom: "1px solid var(--border)", flex: "none",
          }}>
            <div style={{ display: "flex", alignItems: "center", gap: 9, fontWeight: 700, fontSize: 15 }}>
              <Icon name="bell" size={18} /> Уведомления
              {unread > 0 && (
                <span className="mono" style={{
                  background: "var(--bad)", color: "#fff", borderRadius: 999,
                  fontSize: 10.5, fontWeight: 700, padding: "1px 7px",
                }}>{unread}</span>
              )}
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
              {unread > 0 && (
                <button onClick={markAll} style={{
                  background: "none", border: "none", color: "var(--accent-2)",
                  fontSize: 13, fontWeight: 600, padding: "4px 6px", borderRadius: 8, whiteSpace: "nowrap",
                }}>Прочитать все</button>
              )}
              {isMobile && (
                <button onClick={() => setOpen(false)} title="Закрыть" style={{
                  display: "flex", background: "none", border: "none", color: "var(--text-3)", padding: 4,
                }}><Icon name="close" size={19} /></button>
              )}
            </div>
          </div>

          <div className="scroll" style={{ overflowY: "auto", padding: 8, minHeight: 0 }}>
            {feed.isLoading && <Loading label="Загружаем…" />}
            {feed.data && items.length === 0 && (
              <div style={{ color: "var(--text-2)", fontSize: 14, padding: "22px 16px", textAlign: "center", lineHeight: 1.5 }}>
                Пока нет уведомлений.{role === "teacher"
                  ? " Здесь появится, когда ученик решит назначенный тест."
                  : role === "student" ? " Здесь появится, когда учитель назначит тебе тест." : ""}
              </div>
            )}
            {items.length > 0 && (
              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                {items.map((n) => {
                  const { title, sub } = notifText(n, codeOf(n.subject_id));
                  const isUnread = !n.read_at;
                  const done = n.kind === "assignment_created" && n.assignment_status === "done";
                  return (
                    <button key={n.id} onClick={() => openItem(n)} title={fullTime(n.created_at)} style={{
                      display: "flex", gap: 11, alignItems: "flex-start", textAlign: "left", width: "100%",
                      background: isUnread ? "var(--accent-soft)" : "transparent",
                      border: isUnread ? "1px solid transparent" : "1px solid var(--border)",
                      borderRadius: 12, padding: "10px 12px", cursor: "pointer",
                    }}>
                      <span style={{
                        display: "flex", flex: "none", width: 30, height: 30, borderRadius: 9,
                        alignItems: "center", justifyContent: "center", color: "var(--accent-2)",
                        background: isUnread ? "var(--surface)" : "var(--surface-2)",
                      }}>
                        <Icon name={n.kind === "assignment_created" ? "assign" : "check"} size={17} />
                      </span>
                      <span style={{ display: "flex", flexDirection: "column", gap: 3, minWidth: 0, flex: 1 }}>
                        <span style={{ display: "flex", gap: 8, alignItems: "baseline", justifyContent: "space-between" }}>
                          <span style={{ fontWeight: isUnread ? 700 : 600, fontSize: 13.5, color: "var(--text)", lineHeight: 1.35 }}>{title}</span>
                          <span className="mono" style={{ fontSize: 11, color: "var(--text-3)", flex: "none", whiteSpace: "nowrap" }}>{relTime(n.created_at)}</span>
                        </span>
                        {sub && <span className="mono" style={{ fontSize: 11.5, color: "var(--text-3)" }}>{sub}</span>}
                        <span style={{ fontSize: 12.5, color: "var(--accent-2)", display: "inline-flex", alignItems: "center", gap: 5 }}>
                          {done ? "уже решён ✓" : <>Перейти к тесту <Icon name="arrowRight" size={13} /></>}
                        </span>
                      </span>
                      {isUnread && <span style={{ width: 8, height: 8, borderRadius: 999, background: "var(--accent)", marginTop: 6, flex: "none" }} />}
                    </button>
                  );
                })}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

// TelegramLink lets a user bind this web account to the Telegram bot: it opens a
// modal that requests a one-time code and shows the deep link / `/link <code>`.
function TelegramLink({ user, compact }: { user?: User | null; compact?: boolean }) {
  const [open, setOpen] = useState(false);
  const linked = !!user?.telegram_id;
  return (
    <>
      {compact ? (
        <button onClick={() => setOpen(true)} title={linked ? "Telegram привязан" : "Привязать Telegram"} style={{
          display: "flex", alignItems: "center", position: "relative",
          background: "transparent", border: "1px solid var(--border)", borderRadius: 10, padding: 8, color: "var(--text-2)",
        }}>
          <Icon name="bot" size={17} />
          {linked && <span style={{ position: "absolute", top: 3, right: 3, width: 7, height: 7, borderRadius: 999, background: "var(--accent)" }} />}
        </button>
      ) : (
        <button onClick={() => setOpen(true)} style={{
          display: "flex", alignItems: "center", gap: 9,
          background: "transparent", border: "1px solid var(--border)", borderRadius: 10,
          padding: "8px 12px", color: "var(--text-2)", fontSize: 13,
        }}>
          <Icon name="bot" size={16} />
          {linked ? "Telegram привязан ✓" : "Привязать Telegram"}
        </button>
      )}
      {open && <TelegramLinkModal linked={linked} onClose={() => setOpen(false)} />}
    </>
  );
}

// TelegramLinkModal renders through the shared themed Modal (ui.tsx). Its old
// hand-rolled portal to <body> lost the .app/data-theme scope, so every design
// token resolved to nothing — transparent panel, system font (the bug).
function TelegramLinkModal({ linked, onClose }: { linked: boolean; onClose: () => void }) {
  const [data, setData] = useState<{ code: string; deep_link?: string } | null>(null);
  const [error, setError] = useState<string>();
  useEffect(() => {
    api.telegramLinkCode()
      .then((d) => setData(d))
      .catch((e) => setError(String((e as Error)?.message ?? e)));
  }, []);
  return (
    <Modal onClose={onClose} maxWidth={420}
      title={<><Icon name="bot" size={20} /> Привязать Telegram</>}>
      {linked && (
        <div style={{ color: "var(--text-2)", fontSize: 13, marginBottom: 12 }}>
          Аккаунт уже привязан. Можно перепривязать новым кодом (например, другой Telegram).
        </div>
      )}
      {error && <div style={{ color: "var(--bad)", fontSize: 14, marginBottom: 12 }}>{error}</div>}
      {!data && !error && <Spinner />}
      {data && (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ color: "var(--text-2)", fontSize: 14 }}>
            Открой бота и подтверди привязку:
          </div>
          {data.deep_link && (
            <a href={data.deep_link} target="_blank" rel="noreferrer">
              <Button variant="primary" style={{ width: "100%" }}>Открыть бота</Button>
            </a>
          )}
          <div style={{ color: "var(--text-2)", fontSize: 14 }}>
            Или отправь боту команду:
          </div>
          <div className="mono" style={{
            background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 10,
            padding: "10px 12px", fontSize: 15, textAlign: "center", userSelect: "all",
          }}>/link {data.code}</div>
          <div className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>Код действует 15 минут.</div>
        </div>
      )}
      <div style={{ marginTop: 16, display: "flex", justifyContent: "flex-end" }}>
        <Button variant="ghost" onClick={onClose}>Закрыть</Button>
      </div>
    </Modal>
  );
}

function NavBtn({ active, onClick, icon, label }: { active: boolean; onClick: () => void; icon: IconName; label: string }) {
  return (
    <button onClick={onClick} style={{
      display: "flex", alignItems: "center", gap: 11, padding: "10px 12px", borderRadius: 10,
      border: "none", textAlign: "left", fontSize: 14, fontWeight: active ? 700 : 500,
      background: active ? "var(--accent-soft)" : "transparent",
      color: active ? "var(--accent-2)" : "var(--text-2)",
    }}>
      <Icon name={icon} size={19} strokeWidth={active ? 2.1 : 1.75} />{label}
    </button>
  );
}
