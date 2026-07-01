import { ReactNode, useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useApp, View } from "./state";
import { Icon, IconName } from "./icons";
import { api, User } from "./api";
import { Button, Spinner } from "./ui";

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
];
const TEACHER_NAV: { v: View; label: string; icon: IconName }[] = [
  { v: "t-dashboard", label: "Обзор", icon: "overview" },
  { v: "t-builder", label: "Тесты", icon: "tests" },
  { v: "t-assign", label: "Назначить", icon: "assign" },
  { v: "t-bank", label: "Банк", icon: "bank" },
];

export function Shell({ title, cta, children }: { title: string; cta?: ReactNode; children: ReactNode }) {
  const { theme, role, view, user, go, logout, setTheme } = useApp();
  const isMobile = useIsMobile();
  const nav = role === "teacher" ? TEACHER_NAV : STUDENT_NAV;

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
                  <span className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>{role === "teacher" ? "учитель" : "ученик"}</span>
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

function TelegramLinkModal({ linked, onClose }: { linked: boolean; onClose: () => void }) {
  const [data, setData] = useState<{ code: string; deep_link?: string } | null>(null);
  const [error, setError] = useState<string>();
  useEffect(() => {
    api.telegramLinkCode()
      .then((d) => setData(d))
      .catch((e) => setError(String((e as Error)?.message ?? e)));
  }, []);
  return createPortal(
    <div onClick={onClose} style={{
      position: "fixed", inset: 0, zIndex: 2000, background: "rgba(0,0,0,.55)",
      display: "flex", alignItems: "center", justifyContent: "center", padding: 20,
    }}>
      <div onClick={(e) => e.stopPropagation()} style={{
        background: "var(--surface)", border: "1px solid var(--border)", borderRadius: 16,
        padding: 24, width: "min(100%, 420px)", boxShadow: "var(--shadow)",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 12 }}>
          <Icon name="bot" size={20} />
          <div style={{ fontWeight: 700, fontSize: 17 }}>Привязать Telegram</div>
        </div>
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
      </div>
    </div>,
    document.body,
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
