import { useApp } from "./state";
import { Shell } from "./shell";
import { Login } from "./Login";
import { Dashboard, SubjectScreen, Solve, History } from "./student";
import { TeacherDashboard, ClassPage, StudentStatsPage, Builder, Assign, Bank, TestDetailPage } from "./teacher";
import { AdminStats, AdminUsers } from "./admin";
import { ProfilePage } from "./profile";
import { Loading } from "./ui";

const TITLES: Record<string, string> = {
  dashboard: "Дашборд", subject: "Предмет", solve: "Решение", results: "Итоги", history: "История",
  "t-dashboard": "Ученики и классы", "t-class": "Класс", "t-student": "Ученик",
  "t-builder": "Конструктор тестов",
  "t-test": "Просмотр теста", "t-assign": "Назначение", "t-bank": "Банк задач",
  "a-stats": "Платформа", "a-users": "Пользователи",
  profile: "Профиль",
};

export default function App() {
  const { view, ready, user, toast, theme } = useApp();

  if (!ready) {
    return <div className="app" data-theme={theme} style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center" }}>
      <Loading label="Загрузка…" />
    </div>;
  }
  if (!user) return <Login />;

  let screen: React.ReactNode;
  switch (view) {
    case "dashboard": screen = <Dashboard />; break;
    case "subject": screen = <SubjectScreen />; break;
    case "solve": case "results": screen = <Solve />; break;
    case "history": screen = <History />; break;
    case "t-dashboard": screen = <TeacherDashboard />; break;
    case "t-class": screen = <ClassPage />; break;
    case "t-student": screen = <StudentStatsPage />; break;
    case "t-builder": screen = <Builder />; break;
    case "t-test": screen = <TestDetailPage />; break;
    case "t-assign": screen = <Assign />; break;
    case "t-bank": screen = <Bank />; break;
    case "a-stats": screen = <AdminStats />; break;
    case "a-users": screen = <AdminUsers />; break;
    case "profile": screen = <ProfilePage />; break;
    default: screen = <Dashboard />;
  }

  return (
    <Shell title={TITLES[view] || "ЕГЭизм"}>
      {screen}
      {toast && (
        <div className="fade" style={{
          position: "fixed", top: 74, left: "50%", transform: "translateX(-50%)",
          background: "var(--accent)", color: "var(--on-accent)", padding: "12px 22px",
          borderRadius: 12, fontSize: 14, fontWeight: 600, zIndex: 1000,
          boxShadow: "var(--shadow-lg)", maxWidth: "90vw", textAlign: "center",
        }}>{toast}</div>
      )}
    </Shell>
  );
}
