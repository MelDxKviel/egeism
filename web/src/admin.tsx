import { useState } from "react";
import {
  api, Role, SubjectCode, User,
  useAdminUsers, useAdminStats, useAdminClasses, useInvalidate,
} from "./api";
import { useApp } from "./state";
import { Card, Label, Pill, Button, Async, Empty, Modal, PasswordInput, Seg, accColor, SUBJECT_TITLES } from "./ui";
import { Icon } from "./icons";
import { ResetLinkModal } from "./reset";

const ROLE_RU: Record<Role, string> = { student: "ученик", teacher: "учитель", admin: "админ" };
const SUBJECTS: SubjectCode[] = ["rus", "math", "inf", "soc"];

// ---------- Обзор платформы ----------

function StatTile({ label, value, sub }: { label: string; value: number | string; sub?: string }) {
  return (
    <Card style={{ padding: 18 }}>
      <Label>{label}</Label>
      <div className="mono" style={{ fontSize: 30, fontWeight: 700, letterSpacing: "-0.02em", marginTop: 6 }}>{value}</div>
      {sub && <div className="mono" style={{ color: "var(--text-3)", fontSize: 12, marginTop: 2 }}>{sub}</div>}
    </Card>
  );
}

// AdminStats — общая статистика платформы (переработка №1): счётчики по людям,
// контенту и активности + разбивка по предметам и список всех классов.
export function AdminStats() {
  const stats = useAdminStats(true);
  const classes = useAdminClasses(true);

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <Async q={stats}>{(s) => {
        const acc = s.answers ? Math.round((s.correct_answers / s.answers) * 100) : 0;
        return (
          <>
            <div style={{ display: "grid", gap: "var(--gap)", gridTemplateColumns: "repeat(auto-fill, minmax(min(170px, 100%), 1fr))" }}>
              <StatTile label="Ученики" value={s.students} />
              <StatTile label="Учителя" value={s.teachers} sub={`админов: ${s.admins}`} />
              <StatTile label="Классы" value={s.classes} />
              <StatTile label="Отключённые" value={s.inactive_users} sub="аккаунты" />
              <StatTile label="Задания в банке" value={s.tasks} sub={`активных: ${s.active_tasks}`} />
              <StatTile label="Тесты" value={s.tests} sub={`назначений: ${s.assignments}`} />
              <StatTile label="Решено заданий" value={s.answers} sub={`точность: ${acc}%`} />
              <StatTile label="За 7 дней" value={s.answers_7d} sub="ответов" />
            </div>

            <Card>
              <Label>По предметам</Label>
              <div style={{ overflowX: "auto", marginTop: 10 }}>
                <table className="stmt-table" style={{ width: "100%" }}>
                  <thead>
                    <tr><th>Предмет</th><th>Активных заданий</th><th>Ответов</th><th>Точность</th></tr>
                  </thead>
                  <tbody>
                    {s.subjects.map((r) => {
                      const pct = r.answers ? Math.round((r.correct / r.answers) * 100) : 0;
                      return (
                        <tr key={r.code}>
                          <td>{SUBJECT_TITLES[r.code]}</td>
                          <td className="mono">{r.active_tasks}</td>
                          <td className="mono">{r.answers}</td>
                          <td className="mono" style={{ color: r.answers ? accColor(pct) : "var(--text-3)", fontWeight: 700 }}>
                            {r.answers ? `${pct}%` : "—"}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </Card>
          </>
        );
      }}</Async>

      <Card>
        <Label>Все классы</Label>
        <Async q={classes}>{(list) => list.length === 0
          ? <div style={{ color: "var(--text-2)", fontSize: 14, marginTop: 8 }}>Классов пока нет — их создают учителя.</div>
          : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8, marginTop: 10 }}>
              {list.map((c) => (
                <div key={c.id} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8, flexWrap: "wrap", padding: "10px 12px", background: "var(--surface-2)", borderRadius: 12 }}>
                  <span style={{ fontWeight: 600, minWidth: 0 }}>{c.name}</span>
                  <span className="mono" style={{ color: "var(--text-3)", fontSize: 12 }}>
                    {c.teacher_name} · {c.member_count} уч.
                  </span>
                </div>
              ))}
            </div>
          )}</Async>
      </Card>
    </div>
  );
}

// ---------- Пользователи ----------

// SubjectPick — выбор предметной роли учителя; пусто = сверхучитель.
function SubjectPick({ value, onChange }: { value: SubjectCode | ""; onChange: (s: SubjectCode | "") => void }) {
  return (
    <select value={value} onChange={(e) => onChange(e.target.value as SubjectCode | "")} style={{ width: "100%", marginTop: 6 }}>
      <option value="">Сверхучитель (все предметы)</option>
      {SUBJECTS.map((s) => <option key={s} value={s}>{SUBJECT_TITLES[s]}</option>)}
    </select>
  );
}

// UserForm обслуживает и создание, и редактирование: у редактирования логин
// зафиксирован, пароль опционален («оставить пустым — не менять»).
function UserForm({ initial, onSubmit, onClose, busy }: {
  initial?: User;
  onSubmit: (v: { role: Role; name: string; username: string; password: string; subject: SubjectCode | "" }) => void;
  onClose: () => void;
  busy: boolean;
}) {
  const [role, setRole] = useState<Role>(initial?.role ?? "student");
  const [name, setName] = useState(initial?.name ?? "");
  const [username, setUsername] = useState(initial?.username ?? "");
  const [password, setPassword] = useState("");
  const [subject, setSubject] = useState<SubjectCode | "">(initial?.subject ?? "");
  const editing = !!initial;

  return (
    <Modal onClose={onClose} maxWidth={440}
      title={<><Icon name="user" size={20} /> {editing ? "Редактировать пользователя" : "Новый пользователь"}</>}>
      <div style={{ display: "flex", flexDirection: "column", gap: 14 }}>
        <div>
          <Label>Роль</Label>
          <Seg style={{ display: "flex", marginTop: 6 }}>
            {(["student", "teacher", "admin"] as Role[]).map((r) => (
              <button key={r} type="button" onClick={() => setRole(r)}
                data-active={role === r ? "1" : undefined} style={{ flex: 1 }}>{ROLE_RU[r]}</button>
            ))}
          </Seg>
        </div>
        {role === "teacher" && (
          <label>
            <Label>Предмет учителя</Label>
            <SubjectPick value={subject} onChange={setSubject} />
          </label>
        )}
        <label><Label>Имя</Label>
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder="Фамилия Имя" style={{ width: "100%", marginTop: 6 }} /></label>
        <label><Label>Логин</Label>
          <input value={username} onChange={(e) => setUsername(e.target.value)} disabled={editing}
            style={{ width: "100%", marginTop: 6, opacity: editing ? 0.6 : 1 }} /></label>
        <label><Label>{editing ? "Новый пароль (пусто — не менять)" : "Пароль"}</Label>
          <PasswordInput value={password} onChange={setPassword} autoComplete="new-password"
            placeholder={editing ? "" : "минимум 6 символов"} style={{ marginTop: 6 }} /></label>
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 10 }}>
          <Button variant="ghost" onClick={onClose}>Отмена</Button>
          <Button disabled={busy} onClick={() => onSubmit({ role, name, username, password, subject })}>
            {busy ? "…" : editing ? "Сохранить" : "Создать"}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

// AdminUsers — управление аккаунтами: создать, отредактировать (роль/предмет/
// имя/пароль), включить/отключить, удалить (только без истории — иначе 409 и
// подсказка отключить).
export function AdminUsers() {
  const { user: me, showToast } = useApp();
  const invalidate = useInvalidate();
  const users = useAdminUsers(true);
  const [filter, setFilter] = useState<Role | "">("");
  const [search, setSearch] = useState("");
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<User | null>(null);
  const [resetFor, setResetFor] = useState<User | null>(null);
  const [busy, setBusy] = useState(false);

  const refresh = () => { invalidate("admin-users"); invalidate("admin-stats"); };

  const create = async (v: { role: Role; name: string; username: string; password: string; subject: SubjectCode | "" }) => {
    setBusy(true);
    try {
      await api.adminCreateUser({
        role: v.role, name: v.name, username: v.username, password: v.password,
        subject: v.role === "teacher" && v.subject ? v.subject : undefined,
      });
      showToast("Пользователь создан");
      setCreating(false);
      refresh();
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
  };

  const saveEdit = async (u: User, v: { role: Role; name: string; password: string; subject: SubjectCode | "" }) => {
    setBusy(true);
    try {
      await api.adminUpdateUser(u.id, {
        name: v.name !== u.name ? v.name : undefined,
        role: v.role !== u.role ? v.role : undefined,
        subject: v.role === "teacher" ? v.subject : undefined,
        password: v.password ? v.password : undefined,
      });
      showToast("Сохранено");
      setEditing(null);
      refresh();
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
  };

  const toggleActive = async (u: User) => {
    try {
      await api.adminUpdateUser(u.id, { is_active: !u.is_active });
      showToast(u.is_active ? `${u.name} отключён` : `${u.name} снова активен`);
      refresh();
    } catch (e) { showToast(String((e as Error).message)); }
  };

  const del = async (u: User) => {
    if (!window.confirm(`Удалить аккаунт «${u.name}» безвозвратно?\nЕсли у него есть история решений — удаление откажет, тогда лучше отключить.`)) return;
    try {
      await api.adminDeleteUser(u.id);
      showToast("Аккаунт удалён");
      refresh();
    } catch (e) { showToast(String((e as Error).message)); }
  };

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
        <Seg>
          {(["", "student", "teacher", "admin"] as const).map((rr) => (
            <button key={rr} onClick={() => setFilter(rr)} data-active={filter === rr ? "1" : undefined}>
              {rr === "" ? "Все" : ROLE_RU[rr]}
            </button>
          ))}
        </Seg>
        <input placeholder="поиск: имя или логин" value={search} onChange={(e) => setSearch(e.target.value)}
          style={{ marginLeft: "auto", width: 200, maxWidth: "100%", minWidth: 0 }} />
        <Button onClick={() => setCreating(true)}>+ Пользователь</Button>
      </div>

      <Async q={users}>{(list) => {
        const q = search.trim().toLowerCase();
        const rows = list
          .filter((u) => !filter || u.role === filter)
          .filter((u) => !q || u.name.toLowerCase().includes(q) || (u.username || "").toLowerCase().includes(q));
        if (rows.length === 0) return <Empty title="Никого не нашлось" />;
        return (
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            {rows.map((u) => (
              <Card key={u.id} style={{ padding: 14, opacity: u.is_active ? 1 : 0.55 }}>
                <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
                  <div style={{ minWidth: 0, flex: 1 }}>
                    <div style={{ fontWeight: 700, display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap" }}>
                      {u.name}
                      {u.id === me?.id && <Pill tone="accent">это вы</Pill>}
                      {!u.is_active && <Pill tone="bad">отключён</Pill>}
                    </div>
                    <div className="mono" style={{ color: "var(--text-3)", fontSize: 12, marginTop: 3 }}>
                      {u.username || "—"}
                      {u.telegram_id ? <span style={{ color: "var(--ok)" }}> · tg ✓</span> : null}
                      {u.created_at ? ` · с ${new Date(u.created_at).toLocaleDateString("ru")}` : ""}
                    </div>
                  </div>
                  <Pill tone={u.role === "admin" ? "warn" : u.role === "teacher" ? "accent" : "neutral"}>
                    {ROLE_RU[u.role]}
                    {u.role === "teacher" && ` · ${u.subject ? SUBJECT_TITLES[u.subject] : "все предметы"}`}
                  </Pill>
                  <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
                    <Button variant="ghost" style={{ padding: "6px 12px", fontSize: 13 }} onClick={() => setEditing(u)}>
                      Изменить
                    </Button>
                    {u.id !== me?.id && (
                      <>
                        <Button variant={u.is_active ? "ghost" : "soft"} style={{ padding: "6px 12px", fontSize: 13 }}
                          onClick={() => toggleActive(u)}>
                          {u.is_active ? "Отключить" : "Включить"}
                        </Button>
                        <button className="icon-btn" onClick={() => setResetFor(u)} title="Ссылка для сброса пароля">
                          <Icon name="key" size={16} /></button>
                        <button className="btn btn-danger" onClick={() => del(u)} title="Удалить аккаунт"
                          style={{ padding: "8px 10px" }}><Icon name="trash" size={16} /></button>
                      </>
                    )}
                  </div>
                </div>
              </Card>
            ))}
          </div>
        );
      }}</Async>

      {creating && <UserForm busy={busy} onClose={() => setCreating(false)} onSubmit={create} />}
      {editing && (
        <UserForm busy={busy} initial={editing} onClose={() => setEditing(null)}
          onSubmit={(v) => saveEdit(editing, v)} />
      )}
      {resetFor && <ResetLinkModal user={resetFor} onClose={() => setResetFor(null)} />}
    </div>
  );
}
