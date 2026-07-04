import { useState } from "react";
import { useApp } from "./state";
import { api } from "./api";
import { Button, Label, PasswordInput } from "./ui";
import { Icon } from "./icons";

// Login screen. Self-registration is gone: accounts are created by the admin
// (admin panel) or by a teacher (student accounts), so this is login-only. The
// account's role decides the UI — student, teacher or admin. «Забыли пароль?»
// switches to a small form that pings the user's teachers/admins (the bell) so
// one of them issues a reset link — there is no email flow on the platform.
export function Login() {
  const { theme, login } = useApp();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string>();
  const [mode, setMode] = useState<"login" | "forgot" | "forgot-sent">("login");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setErr(undefined); setBusy(true);
    try {
      await login(username, password);
    } catch (e) {
      const msg = String((e as Error).message ?? e);
      setErr(msg.includes("отключён") ? msg : "Неверный логин или пароль");
    } finally { setBusy(false); }
  };

  const submitForgot = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!username.trim()) { setErr("Укажи свой логин"); return; }
    setErr(undefined); setBusy(true);
    try {
      await api.forgotPassword(username);
      setMode("forgot-sent");
    } catch (e) {
      setErr(String((e as Error).message ?? e));
    } finally { setBusy(false); }
  };

  const swap = (m: "login" | "forgot") => { setMode(m); setErr(undefined); };

  return (
    <div className="app" data-theme={theme} style={{ minHeight: "100vh", display: "flex", alignItems: "center", justifyContent: "center", padding: 16 }}>
      <form onSubmit={mode === "login" ? submit : submitForgot} className="fade" style={{
        width: "100%", maxWidth: 380, background: "var(--surface)", border: "1px solid var(--border)",
        borderRadius: 20, padding: 28, boxShadow: "var(--shadow-lg)",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 11, marginBottom: 22 }}>
          <div style={{ width: 38, height: 38, borderRadius: 11, background: "var(--accent)", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--on-accent)" }}>
            <Icon name="logo" size={20} strokeWidth={2.4} />
          </div>
          <div>
            <div style={{ fontWeight: 800, fontSize: 19 }}>ЕГЭизм</div>
            <div className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>подготовка · ЕГЭ</div>
          </div>
        </div>

        {mode === "login" && (
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <label><Label>Логин</Label>
              <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" style={{ width: "100%", marginTop: 6 }} /></label>
            <label><Label>Пароль</Label>
              <PasswordInput value={password} onChange={setPassword} autoComplete="current-password" style={{ marginTop: 6 }} /></label>

            {err && <div style={{ color: "var(--bad)", fontSize: 13 }}>{err}</div>}

            <Button type="submit" disabled={busy} style={{ width: "100%", padding: "12px 0" }}>
              {busy ? "…" : "Войти"}
            </Button>
            <button type="button" onClick={() => swap("forgot")} style={{
              background: "none", border: "none", color: "var(--accent-2)", fontSize: 13, cursor: "pointer", padding: 0,
            }}>Забыли пароль?</button>
            <div style={{ color: "var(--text-3)", fontSize: 12.5, textAlign: "center" }}>
              Аккаунты выдаёт администратор или учитель.
            </div>
          </div>
        )}

        {mode === "forgot" && (
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div style={{ fontWeight: 700, fontSize: 15 }}>Забыли пароль?</div>
            <div style={{ color: "var(--text-2)", fontSize: 13.5 }}>
              Укажи свой логин — учитель и администратор получат уведомление,
              сбросят пароль и пришлют тебе ссылку для смены.
            </div>
            <label><Label>Логин</Label>
              <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" autoFocus style={{ width: "100%", marginTop: 6 }} /></label>

            {err && <div style={{ color: "var(--bad)", fontSize: 13 }}>{err}</div>}

            <Button type="submit" disabled={busy} style={{ width: "100%", padding: "12px 0" }}>
              {busy ? "…" : "Сообщить учителю"}
            </Button>
            <button type="button" onClick={() => swap("login")} style={{
              background: "none", border: "none", color: "var(--text-3)", fontSize: 13, cursor: "pointer", padding: 0,
            }}>← Назад ко входу</button>
          </div>
        )}

        {mode === "forgot-sent" && (
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div style={{ fontWeight: 700, fontSize: 15, color: "var(--accent-2)" }}>Готово ✓</div>
            <div style={{ color: "var(--text-2)", fontSize: 13.5 }}>
              Если такой логин есть, учитель и администратор уже получили
              уведомление. Они пришлют тебе ссылку для смены пароля — она
              действует 1 час.
            </div>
            <button type="button" onClick={() => swap("login")} style={{
              background: "none", border: "none", color: "var(--accent-2)", fontSize: 13, cursor: "pointer", padding: 0,
            }}>← Назад ко входу</button>
          </div>
        )}
      </form>
    </div>
  );
}
