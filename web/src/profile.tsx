import { useProfile } from "./api";
import { useApp } from "./state";
import { Card, Label, Pill, Async, SUBJECT_TITLES } from "./ui";
import { Icon } from "./icons";
import { requestClassView } from "./teacher";

const ROLE_RU: Record<string, string> = { student: "Ученик", teacher: "Учитель", admin: "Администратор" };

// ProfilePage — личный профиль (переработка №4). Students see their personal
// info + classes (and whose they are); teachers additionally their subject
// scope, classes and roster size; admins just the account card. Stats stay on
// the dashboards — the profile is identity.
export function ProfilePage() {
  const { user, go, role } = useApp();
  const q = useProfile();

  if (!user) return null;
  return (
    <div style={{ maxWidth: 640, display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <Card>
        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <div style={{
            width: 56, height: 56, borderRadius: 16, background: "var(--accent-soft)",
            display: "flex", alignItems: "center", justifyContent: "center", color: "var(--accent-2)",
          }}><Icon name="user" size={28} /></div>
          <div style={{ minWidth: 0 }}>
            <div style={{ fontWeight: 800, fontSize: 20 }}>{user.name}</div>
            <div style={{ display: "flex", gap: 8, marginTop: 6, flexWrap: "wrap" }}>
              <Pill tone="accent">{ROLE_RU[user.role]}</Pill>
              {user.role === "teacher" && (
                <Pill tone={user.subject ? "neutral" : "warn"}>
                  {user.subject ? SUBJECT_TITLES[user.subject] : "сверхучитель · все предметы"}
                </Pill>
              )}
            </div>
          </div>
        </div>
        <div style={{ marginTop: 18, display: "grid", gap: 12, gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))" }}>
          <div>
            <Label>Логин</Label>
            <div className="mono" style={{ marginTop: 4, fontSize: 15 }}>{user.username || "—"}</div>
          </div>
          <div>
            <Label>Telegram</Label>
            <div style={{ marginTop: 4, fontSize: 15, color: user.telegram_id ? "var(--accent-2)" : "var(--text-3)" }}>
              {user.telegram_id ? "привязан ✓" : "не привязан (кнопка в меню слева)"}
            </div>
          </div>
          {user.created_at && (
            <div>
              <Label>Аккаунт создан</Label>
              <div className="mono" style={{ marginTop: 4, fontSize: 15 }}>
                {new Date(user.created_at).toLocaleDateString("ru")}
              </div>
            </div>
          )}
        </div>
      </Card>

      <Async q={q}>{(p) => (
        <>
          {role === "student" && (
            <Card>
              <Label>Мои учителя</Label>
              {(p.teachers ?? []).length === 0 ? (
                <div style={{ color: "var(--text-2)", fontSize: 14, marginTop: 8 }}>
                  Пока ни один учитель не взял тебя к себе.
                </div>
              ) : (
                <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 10 }}>
                  {(p.teachers ?? []).map((t) => (
                    <div key={t.id} style={{
                      display: "flex", justifyContent: "space-between", alignItems: "center",
                      padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10,
                    }}>
                      <span style={{ fontWeight: 600 }}>{t.name}</span>
                      <span className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>
                        {t.subject ? SUBJECT_TITLES[t.subject] : "все предметы"}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          )}
          {role !== "admin" && (
            <Card>
              <Label>Мои классы</Label>
              {p.classes.length === 0 ? (
                <div style={{ color: "var(--text-2)", fontSize: 14, marginTop: 8 }}>
                  {role === "teacher"
                    ? "Классов пока нет — создай на вкладке «Ученики»."
                    : "Ты пока не состоишь в классе (занимаешься индивидуально)."}
                </div>
              ) : (
                <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 10 }}>
                  {p.classes.map((c) => (
                    <div key={c.id}
                      onClick={role === "teacher" ? () => { requestClassView(c.id); go("t-class"); } : undefined}
                      style={{
                        display: "flex", justifyContent: "space-between", alignItems: "center",
                        padding: "10px 12px", background: "var(--surface-2)", borderRadius: 10,
                        cursor: role === "teacher" ? "pointer" : "default",
                      }}>
                      <span style={{ fontWeight: 600 }}>{c.name}</span>
                      <span className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>
                        {role === "teacher" ? `${c.member_count} уч.` : c.teacher_name}
                      </span>
                    </div>
                  ))}
                </div>
              )}
              {role === "teacher" && (
                <div className="mono" style={{ color: "var(--text-3)", fontSize: 12, marginTop: 10 }}>
                  всего учеников: {p.students_count ?? 0}
                </div>
              )}
            </Card>
          )}
        </>
      )}</Async>
    </div>
  );
}
