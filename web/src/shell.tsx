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
// assigned, so kind alone decides the wording.
function notifText(n: NotificationItem, subjectCode?: string): { title: string; sub: string } {
  const when = (iso: string) =>
    new Date(iso).toLocaleString("ru", { day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" });
  const subj = subjectCode ? SUBJECT_TITLES[subjectCode] : undefined;
  if (n.kind === "assignment_created") {
    return {
      title: `Тебе назначен тест «${testTitle(n.test_title)}»`,
      sub: [subj, `на ${when(n.scheduled_at)}`].filter(Boolean).join(" · "),
    };
  }
  return {
    title: `${n.student_name} решил(а) тест «${testTitle(n.test_title)}»`,
    sub: [subj, when(n.created_at)].filter(Boolean).join(" · "),
  };
}

// NotificationsBell polls the in-app feed, shows the unread badge, and opens
// the list in the shared themed Modal. Clicking a notification marks it read
// and jumps to the test: the student straight into solving the assigned
// variant, the teacher into the test view.
function NotificationsBell() {
  const { go, role, subject, user, showToast } = useApp();
  const invalidate = useInvalidate();
  const feed = useNotifications(user?.id ?? "");
  const subjects = useSubjects();
  const [open, setOpen] = useState(false);

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

  const markAll = () => api.markAllNotificationsRead().then(() => invalidate("notifications")).catch(() => {});

  const openItem = (n: NotificationItem) => {
    if (!n.read_at) api.markNotificationRead(n.id).then(() => invalidate("notifications")).catch(() => {});
    if (n.kind === "assignment_created") {
      if (n.assignment_status === "done") { showToast("Этот тест уже решён ✓"); return; }
      requestSolve({
        subject: codeOf(n.subject_id) ?? subject,
        testId: n.test_id, assignmentId: n.assignment_id, title: testTitle(n.test_title),
      });
      setOpen(false);
      go("solve");
    } else {
      requestTestView(n.test_id);
      setOpen(false);
      go("t-test");
    }
  };

  const unread = feed.data?.unread ?? 0;
  return (
    <>
      <button onClick={() => setOpen(true)} title="Уведомления" style={{
        display: "flex", alignItems: "center", position: "relative",
        background: "transparent", border: "1px solid var(--border)", borderRadius: 10, padding: 8, color: "var(--text-2)",
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
        <Modal onClose={() => setOpen(false)} maxWidth={480}
          title={<><Icon name="bell" size={20} /> Уведомления</>}>
          {feed.isLoading && <Loading label="Загружаем…" />}
          {feed.data && feed.data.items.length === 0 && (
            <div style={{ color: "var(--text-2)", fontSize: 14, padding: "4px 0 8px" }}>
              Пока нет уведомлений.{role === "teacher"
                ? " Здесь появится, когда ученик решит назначенный тест."
                : role === "student" ? " Здесь появится, когда учитель назначит тебе тест." : ""}
            </div>
          )}
          {feed.data && feed.data.items.length > 0 && (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {unread > 0 && (
                <button onClick={markAll} style={{
                  alignSelf: "flex-end", background: "none", border: "none",
                  color: "var(--accent-2)", fontSize: 13, padding: "0 0 2px",
                }}>Прочитать все</button>
              )}
              {feed.data.items.map((n) => {
                const { title, sub } = notifText(n, codeOf(n.subject_id));
                const isUnread = !n.read_at;
                return (
                  <button key={n.id} onClick={() => openItem(n)} style={{
                    display: "flex", gap: 10, alignItems: "flex-start", textAlign: "left", width: "100%",
                    background: isUnread ? "var(--accent-soft)" : "var(--surface-2)",
                    border: "none", borderRadius: 12, padding: "10px 12px", cursor: "pointer",
                  }}>
                    <span style={{ color: isUnread ? "var(--accent-2)" : "var(--text-3)", marginTop: 2 }}>
                      <Icon name={n.kind === "assignment_created" ? "assign" : "check"} size={18} />
                    </span>
                    <span style={{ display: "flex", flexDirection: "column", gap: 3, minWidth: 0, flex: 1 }}>
                      <span style={{ fontWeight: isUnread ? 700 : 600, fontSize: 14, color: "var(--text)" }}>{title}</span>
                      {sub && <span className="mono" style={{ fontSize: 11.5, color: "var(--text-3)" }}>{sub}</span>}
                      <span style={{ fontSize: 12.5, color: "var(--accent-2)", display: "inline-flex", alignItems: "center", gap: 5 }}>
                        {n.kind === "assignment_created" && n.assignment_status === "done"
                          ? "уже решён ✓"
                          : <>Перейти к тесту <Icon name="arrowRight" size={13} /></>}
                      </span>
                    </span>
                    {isUnread && <span style={{ width: 8, height: 8, borderRadius: 999, background: "var(--accent)", marginTop: 5, flex: "none" }} />}
                  </button>
                );
              })}
            </div>
          )}
        </Modal>
      )}
    </>
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
