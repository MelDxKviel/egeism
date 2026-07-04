import { useState } from "react";
import { useApp } from "./state";
import { Button, Label } from "./ui";
import { Icon } from "./icons";

// Login screen. Self-registration is gone: accounts are created by the admin
// (admin panel) or by a teacher (student accounts), so this is login-only. The
// account's role decides the UI — student, teacher or admin.
export function Login() {
  const { theme, login } = useApp();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string>();

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
            <div style={{ fontWeight: 800, fontSize: 19 }}>ЕГЭизм</div>
            <div className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>подготовка · ЕГЭ</div>
          </div>
        </div>

        <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <label><Label>Логин</Label>
            <input value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="username" style={{ width: "100%", marginTop: 6 }} /></label>
          <label><Label>Пароль</Label>
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" style={{ width: "100%", marginTop: 6 }} /></label>

          {err && <div style={{ color: "var(--bad)", fontSize: 13 }}>{err}</div>}

          <Button type="submit" disabled={busy} style={{ width: "100%", padding: "12px 0" }}>
            {busy ? "…" : "Войти"}
          </Button>
          <div style={{ color: "var(--text-3)", fontSize: 12.5, textAlign: "center" }}>
            Аккаунты выдаёт администратор или учитель.
          </div>
        </div>
      </form>
    </div>
  );
}
