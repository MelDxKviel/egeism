import { ReactNode, useEffect, useState } from "react";
import { useApp, View } from "./state";
import { Icon, IconName } from "./icons";

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
