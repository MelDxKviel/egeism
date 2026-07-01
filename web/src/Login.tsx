import { useState } from "react";
import { useApp } from "./state";
import { Role } from "./api";
import { Button, Label } from "./ui";
import { Icon } from "./icons";

// Login / registration screen. Role is chosen at registration and tied to the
// account — there is no role toggle; the logged-in account decides student vs
// teacher UI.
export function Login() {
  const { theme, login, register } = useApp();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [role, setRole] = useState<Role>("student");
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string>();

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr(undefined); setBusy(true);
    try {
      if (mode === "login") await login(username, password);
      else await register(role, username, password, name);
    } catch (e) {
      setErr(mode === "login" ? "Неверный логин или пароль" : String((e as Error).message));
    } finally { setBusy(false); }
  };

  return (
    <div className="app" data-theme={theme} style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", padding: 16 }}>
      <form onSubmit={submit} className="fade" style={{
        width: "100%", maxWidth: 380, background: "var(--surface)", border: "1px solid var(--border)",
        borderRadius: 20, padding: 28, boxShadow: "var(--shadow-lg)",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 11, marginBottom: 22 }}>
          <div style={{ width: 38, height: 38, borderRadius: 11, background: "var(--accent)", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--on-accent)" }}>
            <Icon name="logo" size={20} strokeWidth={2.4} />
          </div>
          <div>
            <div style={{ fontWeight: 800, fontSize: 19 }}>Вектор</div>
            <div className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>подготовка · ЕГЭ</div>
          </div>
        </div>

        <div style={{ display: "flex", gap: 6, background: "var(--bg-2)", borderRadius: 10, padding: 3, marginBottom: 20 }}>
          <Tab active={mode === "login"} onClick={() => setMode("login")}>Вход</Tab>
          <Tab active={mode === "register"} onClick={() => setMode("register")}>Регистрация</Tab>
        </div>

        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          {mode === "register" && (
            <>
              <div>
                <Label>Кто ты</Label>
                <div style={{ display: "flex", gap: 6, marginTop: 6 }}>
                  <RolePick active={role === "student"} onClick={() => setRole("student")}>Ученик</RolePick>
                  <RolePick active={role === "teacher"} onClick={() => setRole("teacher")}>Учитель</RolePick>
                </div>
              </div>
              <label><Label>Имя</Label>
                <input value={name} onChange={(e) => setName(e.target.value)} placeholder="как тебя зовут" style={{ width: "100%", marginTop: 6 }} /></label>
            </>
          )}
          <label><Label>Логин</Label>
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" style={{ width: "100%", marginTop: 6 }} /></label>
          <label><Label>Пароль</Label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete={mode === "login" ? "current-password" : "new-password"} style={{ width: "100%", marginTop: 6 }} /></label>

          {err && <div style={{ color: "var(--bad)", fontSize: 13 }}>{err}</div>}

          <Button type="submit" disabled={busy} style={{ width: "100%", padding: "12px 0" }}>
            {busy ? "…" : mode === "login" ? "Войти" : "Создать аккаунт"}
          </Button>
        </div>
      </form>
    </div>
  );
}

function Tab({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} style={{
    flex: 1, border: "none", borderRadius: 8, padding: "8px 0", fontSize: 14, fontWeight: 600,
    background: active ? "var(--surface)" : "transparent", color: active ? "var(--text)" : "var(--text-3)",
    boxShadow: active ? "var(--shadow)" : "none",
  }}>{children}</button>;
}
function RolePick({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return <button type="button" onClick={onClick} style={{
    flex: 1, borderRadius: 10, padding: "9px 0", fontSize: 14, fontWeight: 600,
    border: "1px solid " + (active ? "var(--accent)" : "var(--border-2)"),
    background: active ? "var(--accent-soft)" : "transparent",
    color: active ? "var(--accent-2)" : "var(--text-2)",
  }}>{children}</button>;
}
