import { useEffect, useState } from "react";
import { api, setToken } from "./api";
import { useApp } from "./state";
import { Button, Label, Modal, PasswordInput, Spinner } from "./ui";
import { Icon } from "./icons";

// Password-reset flow (no email on the platform): a teacher/admin issues a
// one-time link (ResetLinkModal), the user opens it (#reset=<token>) and sets
// a new password here (ResetPasswordPage). The link lives 1 hour; an expired
// one is simply reissued.

// resetLink builds the absolute reset URL from the token. The web composes it
// from its own origin, so the API needs no WEB_URL config: the link a teacher
// copies points at the very host they are using.
const resetLink = (token: string) =>
  `${window.location.origin}${window.location.pathname}#reset=${token}`;

// ResetPasswordPage — the standalone pre-auth screen behind a reset link. It
// validates the token up front (an expired link fails fast, before any typing),
// takes the new password twice, and logs the user straight in on success.
export function ResetPasswordPage({ token }: { token: string }) {
  const { theme } = useApp();
  const [peek, setPeek] = useState<{ name: string; expires_at: string } | null>(null);
  const [invalid, setInvalid] = useState(false);
  const [p1, setP1] = useState("");
  const [p2, setP2] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string>();

  useEffect(() => {
    api.peekResetToken(token).then(setPeek).catch(() => setInvalid(true));
  }, [token]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (p1.length < 6) { setErr("Пароль должен быть не короче 6 символов"); return; }
    if (p1 !== p2) { setErr("Пароли не совпадают"); return; }
    setErr(undefined); setBusy(true);
    try {
      const res = await api.resetPassword(token, p1);
      setToken(res.token);
      // Drop the #reset hash and reload: the stored session token logs the
      // user in through the normal restore path (state.tsx).
      window.location.replace(window.location.pathname);
    } catch (e) {
      setErr(String((e as Error).message ?? e));
      setBusy(false);
    }
  };

  return (
    <div className="app auth-screen" data-theme={theme}>
      <form onSubmit={submit} className="pop" style={{
        width: "100%", maxWidth: 380, background: "var(--surface)", border: "1px solid var(--border)",
        borderRadius: 24, padding: "clamp(18px, 5vw, 28px)", boxShadow: "var(--shadow-lg)",
      }}>
        <div style={{ display: "flex", alignItems: "center", gap: 11, marginBottom: 22 }}>
          <div style={{ width: 38, height: 38, borderRadius: 12, background: "var(--accent)", display: "flex", alignItems: "center", justifyContent: "center", color: "var(--on-accent)" }}>
            <Icon name="key" size={20} strokeWidth={2.4} />
          </div>
          <div>
            <div style={{ fontWeight: 700, fontSize: 19, letterSpacing: "-0.01em" }}>Новый пароль</div>
            <div className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>ЕГЭизм · сброс пароля</div>
          </div>
        </div>

        {invalid ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div style={{ color: "var(--bad)", fontSize: 14 }}>
              Ссылка недействительна или истекла (она работает 1 час).
            </div>
            <div style={{ color: "var(--text-2)", fontSize: 13.5 }}>
              Попроси учителя или администратора сгенерировать новую ссылку.
            </div>
            <Button variant="ghost" onClick={() => window.location.replace(window.location.pathname)}>
              На страницу входа
            </Button>
          </div>
        ) : !peek ? (
          <Spinner />
        ) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
            <div style={{ color: "var(--text-2)", fontSize: 14 }}>
              {peek.name}, придумай новый пароль для входа.
            </div>
            <label><Label>Новый пароль</Label>
              <PasswordInput value={p1} onChange={setP1} autoComplete="new-password" autoFocus
                placeholder="минимум 6 символов" style={{ marginTop: 6 }} /></label>
            <label><Label>Ещё раз</Label>
              <PasswordInput value={p2} onChange={setP2} autoComplete="new-password" style={{ marginTop: 6 }} /></label>

            {err && <div style={{ color: "var(--bad)", fontSize: 13 }}>{err}</div>}

            <Button type="submit" disabled={busy} style={{ width: "100%", padding: "12px 0" }}>
              {busy ? "…" : "Сохранить и войти"}
            </Button>
          </div>
        )}
      </form>
    </div>
  );
}

// ResetLinkModal — the teacher/admin side: issue a one-time reset link for a
// user and hand it over (copy button). Reached from the bell notification
// («N забыл пароль»), the admin users list and the teacher's student page.
export function ResetLinkModal({ user, onClose }: { user: { id: string; name: string }; onClose: () => void }) {
  const [data, setData] = useState<{ token: string; expires_at: string } | null>(null);
  const [error, setError] = useState<string>();
  const [copied, setCopied] = useState(false);

  const generate = () => {
    setData(null); setError(undefined); setCopied(false);
    api.createPasswordResetLink(user.id)
      .then(setData)
      .catch((e) => setError(String((e as Error)?.message ?? e)));
  };
  // A fresh link on open: each click of «Новая ссылка» re-issues (the old one
  // stays valid until its own expiry, so an accidental re-open loses nothing).
  useEffect(generate, [user.id]); // eslint-disable-line react-hooks/exhaustive-deps

  const link = data ? resetLink(data.token) : "";
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(link);
      setCopied(true);
    } catch {
      // Clipboard may be unavailable (http origin) — the box is select-all.
      setCopied(false);
    }
  };

  return (
    <Modal onClose={onClose} maxWidth={440}
      title={<><Icon name="key" size={20} /> Сброс пароля · {user.name}</>}>
      {error && <div style={{ color: "var(--bad)", fontSize: 14, marginBottom: 12 }}>{error}</div>}
      {!data && !error && <Spinner />}
      {data && (
        <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
          <div style={{ color: "var(--text-2)", fontSize: 14 }}>
            Передай эту ссылку — по ней {user.name} сам(а) задаст новый пароль:
          </div>
          <div className="mono" style={{
            background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 12,
            padding: "10px 12px", fontSize: 12.5, userSelect: "all", wordBreak: "break-all",
          }}>{link}</div>
          <Button variant={copied ? "ok" : "primary"} onClick={copy} style={{ width: "100%" }}>
            {copied ? "Скопировано ✓" : "Скопировать ссылку"}
          </Button>
          <div className="mono" style={{ fontSize: 11, color: "var(--text-3)" }}>
            Ссылка действует 1 час. Не успели — сгенерируй новую.
          </div>
        </div>
      )}
      <div style={{ marginTop: 16, display: "flex", justifyContent: "space-between", gap: 10 }}>
        <Button variant="ghost" onClick={generate} disabled={!data && !error}>Новая ссылка</Button>
        <Button variant="ghost" onClick={onClose}>Закрыть</Button>
      </div>
    </Modal>
  );
}
