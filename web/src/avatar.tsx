import { CSSProperties, ReactNode, useState } from "react";
import { AvatarConfig, api, useInvalidate } from "./api";
import { useApp } from "./state";
import { Button, Label, Modal } from "./ui";
import { Icon } from "./icons";

// ---------- Аватары: параметрический SVG-билдер ----------
// Конфиг (AvatarConfig) хранится на сервере в users.avatar; РИСУЕТ его только
// клиент — этот файл. Добавить причёску/цвет = поменять массивы ниже, API не
// трогается (сервер ограничивает индексы 0..31). У пользователя без своего
// аватара всё равно есть лицо: детерминированный дефолт из id (defaultAvatar),
// одинаковый на всех экранах и устройствах. kind="photo" зарезервирован под
// будущие загружаемые фото: renderAvatar на неизвестный kind падает в дефолт.

export const BG_COLORS = [
  "#C7D2FE", "#BAE6FD", "#BBF7D0", "#D9F99D", "#FDE68A",
  "#FED7AA", "#FECACA", "#F5D0FE", "#99F6E4", "#E5E7EB",
];
export const SKIN_COLORS = ["#FFD5B3", "#F8C09A", "#EAA772", "#C98D5E", "#9C6B43", "#7A4F2E"];
export const HAIR_COLORS = [
  "#2F2A26", "#4A3728", "#8B5A2B", "#E8C468",
  "#B93A2B", "#8E8E93", "#7C5CDB", "#3B82F6",
];

// Число вариантов каждой части (индексы 0..N-1). Названия — для конструктора.
export const HAIR_NAMES = ["лысина", "короткая", "чёлка", "кудри", "длинные", "пучок", "ёжик", "хвостики"];
export const EYES_NAMES = ["точки", "круглые", "довольные", "сонные", "подмигивание"];
export const MOUTH_NAMES = ["улыбка", "смех", "спокойный", "«о»", "ухмылка", "грустный"];
export const ACC_NAMES = ["ничего", "очки", "тёмные очки", "румянец"];

const INK = "#26303B"; // черты лица: единый тёмный, читается на любой коже

// ---------- SVG-части ----------
// Геометрия: viewBox 0 0 64 64, голова — круг c=(32,42) r=21, кадр круглый
// (обрезается контейнером), так что низ головы уходит за край как портрет.

function hairBack(v: number, c: string): ReactNode {
  switch (v) {
    case 4: // длинные: полотно за головой до низа кадра
      return <path d="M13 30 Q13 17 32 17 Q51 17 51 30 L51 60 Q51 64 46 64 L18 64 Q13 64 13 60 Z" fill={c} />;
    case 7: // хвостики по бокам
      return <g fill={c}><circle cx="9" cy="42" r="7" /><circle cx="55" cy="42" r="7" /></g>;
    default:
      return null;
  }
}

function hairFront(v: number, c: string): ReactNode {
  // «Шапочка» — сегмент круга головы выше хорды y=36 (докручен чуть шире).
  const cap = "M11.9 36 A21 21 0 0 1 52.1 36 Q32 30 11.9 36 Z";
  switch (v) {
    case 1: return <path d="M11.9 37 A21 21 0 0 1 52.1 37 Q32 30 11.9 37 Z" fill={c} />;
    case 2: // чёлка набок
      return <path d="M11.9 39 A21 21 0 0 1 52.1 33 Q44 32 37 31 Q22 29 15 36 Q13 37.5 11.9 39 Z" fill={c} />;
    case 3: // кудри: облако из кругов по макушке
      return (
        <g fill={c}>
          <circle cx="17" cy="31" r="8" /><circle cx="25" cy="25" r="8" />
          <circle cx="34" cy="23" r="8.5" /><circle cx="43" cy="26" r="8" />
          <circle cx="48" cy="33" r="7" />
        </g>
      );
    case 4: return <path d={cap} fill={c} />; // длинные: чёлка поверх
    case 5: // пучок
      return <g fill={c}><circle cx="32" cy="16" r="6.5" /><path d={cap} /></g>;
    case 6: // ёжик: зубцы над шапочкой
      return <path d="M13 36 L17 24 L22 30 L27 20 L32 27 L37 19 L42 29 L47 23 L51 36 Q32 29 13 36 Z" fill={c} />;
    case 7: return <path d={cap} fill={c} />; // хвостики: шапочка + хвосты сзади
    default: return null; // 0 — лысина
  }
}

function eyes(v: number): ReactNode {
  const L = { x: 24.5, y: 41 }, R = { x: 39.5, y: 41 };
  const stroke = { stroke: INK, strokeWidth: 2.2, strokeLinecap: "round" as const, fill: "none" };
  switch (v) {
    case 1: // круглые с бликом
      return (
        <g>
          <circle cx={L.x} cy={L.y} r="3.4" fill={INK} /><circle cx={R.x} cy={R.y} r="3.4" fill={INK} />
          <circle cx={L.x - 1.1} cy={L.y - 1.1} r="1.1" fill="#fff" /><circle cx={R.x - 1.1} cy={R.y - 1.1} r="1.1" fill="#fff" />
        </g>
      );
    case 2: // довольные (дуги домиком)
      return <g {...stroke}><path d={`M${L.x - 3.5} ${L.y + 1} q3.5 -4.5 7 0`} /><path d={`M${R.x - 3.5} ${R.y + 1} q3.5 -4.5 7 0`} /></g>;
    case 3: // сонные (полуприкрытые)
      return <g {...stroke}><path d={`M${L.x - 3.5} ${L.y} h7`} /><path d={`M${R.x - 3.5} ${R.y} h7`} /></g>;
    case 4: // подмигивание
      return (
        <g>
          <circle cx={L.x} cy={L.y} r="2.6" fill={INK} />
          <path d={`M${R.x - 3.5} ${R.y + 1} q3.5 -4.5 7 0`} {...stroke} />
        </g>
      );
    default: // точки
      return <g fill={INK}><circle cx={L.x} cy={L.y} r="2.3" /><circle cx={R.x} cy={R.y} r="2.3" /></g>;
  }
}

function mouth(v: number): ReactNode {
  const stroke = { stroke: INK, strokeWidth: 2.2, strokeLinecap: "round" as const, fill: "none" };
  switch (v) {
    case 1: // смех: заполненная улыбка + язычок
      return (
        <g>
          <path d="M25 49.5 q7 9 14 0 z" fill={INK} />
          <path d="M28.5 53 q3.5 3 7 0 q-1 2.6 -3.5 2.6 q-2.5 0 -3.5 -2.6 z" fill="#F1808C" />
        </g>
      );
    case 2: return <path d="M26.5 52 h11" {...stroke} />;
    case 3: return <circle cx="32" cy="52" r="3" fill={INK} />;
    case 4: return <path d="M26 53.5 q6 2.5 12 -3.5" {...stroke} />;
    case 5: return <path d="M25.5 55 q6.5 -6 13 0" {...stroke} />;
    default: return <path d="M25.5 50 q6.5 6.5 13 0" {...stroke} />;
  }
}

function accessory(v: number): ReactNode {
  switch (v) {
    case 1: // очки
      return (
        <g stroke={INK} strokeWidth="1.9" fill="none">
          <circle cx="24.5" cy="41" r="6.2" /><circle cx="39.5" cy="41" r="6.2" />
          <path d="M30.7 40 q1.3 -1.6 2.6 0" /><path d="M18.3 40 L14 38.5" /><path d="M45.7 40 L50 38.5" />
        </g>
      );
    case 2: // тёмные очки
      return (
        <g>
          <g fill={INK}>
            <rect x="17.5" y="35.5" width="13" height="10" rx="4.4" />
            <rect x="33.5" y="35.5" width="13" height="10" rx="4.4" />
          </g>
          <g stroke={INK} strokeWidth="1.9" fill="none">
            <path d="M30.5 39.5 q1.5 -1.8 3 0" /><path d="M17.5 39.5 L13.5 38" /><path d="M46.5 39.5 L50.5 38" />
          </g>
        </g>
      );
    case 3: // румянец
      return <g fill="#F1808C" opacity=".55"><ellipse cx="20.5" cy="48" rx="3.4" ry="2" /><ellipse cx="43.5" cy="48" rx="3.4" ry="2" /></g>;
    default:
      return null;
  }
}

// renderAvatar — чистая функция конфиг → SVG-содержимое (без контейнера).
function renderAvatar(cfg: AvatarConfig): ReactNode {
  const bg = cfg.bg || BG_COLORS[0];
  const skin = cfg.skin || SKIN_COLORS[1];
  const hairC = cfg.hair_color || HAIR_COLORS[0];
  return (
    <>
      <rect width="64" height="64" fill={bg} />
      {hairBack(cfg.hair, hairC)}
      <circle cx="32" cy="42" r="21" fill={skin} />
      {hairFront(cfg.hair, hairC)}
      {eyes(cfg.eyes)}
      {mouth(cfg.mouth)}
      {accessory(cfg.accessory)}
    </>
  );
}

// ---------- Конфиги: дефолт и рандом ----------

// fnv1a — крошечный стабильный хеш строки (id пользователя) для дефолта.
function fnv1a(s: string): number {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) { h ^= s.charCodeAt(i); h = Math.imul(h, 0x01000193); }
  return h >>> 0;
}

// defaultAvatar — детерминированный аватар «из коробки» для пользователя без
// своего: одинаковый везде, где виден этот id.
export function defaultAvatar(seed: string): AvatarConfig {
  const h = fnv1a(seed);
  return {
    kind: "builder",
    bg: BG_COLORS[h % BG_COLORS.length],
    skin: SKIN_COLORS[(h >>> 4) % SKIN_COLORS.length],
    hair_color: HAIR_COLORS[(h >>> 8) % HAIR_COLORS.length],
    hair: (h >>> 12) % HAIR_NAMES.length,
    eyes: (h >>> 17) % EYES_NAMES.length,
    mouth: (h >>> 22) % MOUTH_NAMES.length,
    accessory: (h >>> 27) % ACC_NAMES.length,
  };
}

export function randomAvatar(): AvatarConfig {
  const pick = <T,>(a: T[]) => a[Math.floor(Math.random() * a.length)];
  return {
    kind: "builder",
    bg: pick(BG_COLORS), skin: pick(SKIN_COLORS), hair_color: pick(HAIR_COLORS),
    hair: Math.floor(Math.random() * HAIR_NAMES.length),
    eyes: Math.floor(Math.random() * EYES_NAMES.length),
    mouth: Math.floor(Math.random() * MOUTH_NAMES.length),
    accessory: Math.floor(Math.random() * ACC_NAMES.length),
  };
}

// ---------- Компоненты ----------

// Avatar — круглый аватар. Передавай либо готовый config, либо
// {avatar?, id} пользователя — без avatar рисуется дефолт из id. Неизвестный
// kind (будущее "photo" на старом клиенте) тоже падает в дефолт.
export function Avatar({ user, config, size = 34, style }: {
  user?: { id: string; avatar?: AvatarConfig };
  config?: AvatarConfig;
  size?: number;
  style?: CSSProperties;
}) {
  let cfg = config ?? user?.avatar;
  if (!cfg || cfg.kind !== "builder") cfg = defaultAvatar(user?.id ?? "");
  return (
    <span aria-hidden style={{
      width: size, height: size, borderRadius: 999, flex: "none", display: "inline-flex",
      overflow: "hidden", boxShadow: "inset 0 0 0 1px color-mix(in srgb, var(--text) 12%, transparent)",
      ...style,
    }}>
      <svg viewBox="0 0 64 64" width={size} height={size} style={{ display: "block" }}>
        {renderAvatar(cfg)}
      </svg>
    </span>
  );
}

// ColorDots — ряд цветных кружков-свотчей (фон/кожа/волосы).
function ColorDots({ colors, value, onPick }: { colors: string[]; value: string; onPick: (c: string) => void }) {
  return (
    <div style={{ display: "flex", gap: 7, flexWrap: "wrap" }}>
      {colors.map((c) => {
        const active = c.toLowerCase() === value.toLowerCase();
        return (
          <button key={c} type="button" onClick={() => onPick(c)} title={c}
            style={{
              width: 27, height: 27, borderRadius: 999, background: c, cursor: "pointer", padding: 0,
              border: active ? "2px solid var(--accent)" : "2px solid color-mix(in srgb, var(--text) 14%, transparent)",
              boxShadow: active ? "0 0 0 2px color-mix(in srgb, var(--accent) 30%, transparent)" : "none",
            }} />
        );
      })}
    </div>
  );
}

// VariantRow — выбор варианта части: мини-аватары, у каждого применён свой
// вариант поверх текущего конфига — видно «как будет» прямо в ряду.
function VariantRow({ cfg, part, count, names, onPick }: {
  cfg: AvatarConfig; part: "hair" | "eyes" | "mouth" | "accessory";
  count: number; names: string[]; onPick: (v: number) => void;
}) {
  return (
    <div style={{ display: "flex", gap: 7, flexWrap: "wrap" }}>
      {Array.from({ length: count }, (_, v) => {
        const active = cfg[part] === v;
        return (
          <button key={v} type="button" onClick={() => onPick(v)} title={names[v]}
            style={{
              padding: 2, borderRadius: 999, cursor: "pointer", background: "none", lineHeight: 0,
              border: active ? "2px solid var(--accent)" : "2px solid color-mix(in srgb, var(--text) 14%, transparent)",
              boxShadow: active ? "0 0 0 2px color-mix(in srgb, var(--accent) 30%, transparent)" : "none",
            }}>
            <Avatar config={{ ...cfg, [part]: v }} size={38} />
          </button>
        );
      })}
    </div>
  );
}

// AvatarBuilderModal — ОЧЕНЬ простой конструктор: большое превью, кнопка
// «Случайный» и по ряду свотчей на каждую часть. Сохранение шлёт конфиг на
// сервер и обновляет пользователя в контексте + все списки, где он виден.
export function AvatarBuilderModal({ onClose }: { onClose: () => void }) {
  const { user, updateUser, showToast } = useApp();
  const invalidate = useInvalidate();
  const [cfg, setCfg] = useState<AvatarConfig>(() =>
    user?.avatar && user.avatar.kind === "builder" ? { ...user.avatar } : defaultAvatar(user?.id ?? ""));
  const [busy, setBusy] = useState(false);

  const set = (patch: Partial<AvatarConfig>) => setCfg((c) => ({ ...c, ...patch }));

  const save = async () => {
    setBusy(true);
    try {
      const updated = await api.setAvatar(cfg);
      updateUser(updated);
      // Аватар едет во всех пользовательских списках — обновляем их кэш.
      ["profile", "students", "admin-users", "class-detail", "class-overview"].forEach(invalidate);
      showToast("Аватар сохранён");
      onClose();
    } catch (e) { showToast(String((e as Error).message)); }
    finally { setBusy(false); }
  };

  return (
    <Modal onClose={onClose} maxWidth={520} title={<><Icon name="user" size={20} /> Мой аватар</>}>
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 16, flexWrap: "wrap" }}>
          <Avatar config={cfg} size={104} />
          <Button variant="soft" onClick={() => setCfg(randomAvatar())}>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 7 }}>
              <Icon name="dice" size={16} /> Случайный
            </span>
          </Button>
        </div>

        <div><Label style={{ marginBottom: 7 }}>Фон</Label>
          <ColorDots colors={BG_COLORS} value={cfg.bg || ""} onPick={(bg) => set({ bg })} /></div>
        <div><Label style={{ marginBottom: 7 }}>Кожа</Label>
          <ColorDots colors={SKIN_COLORS} value={cfg.skin || ""} onPick={(skin) => set({ skin })} /></div>
        <div><Label style={{ marginBottom: 7 }}>Причёска</Label>
          <VariantRow cfg={cfg} part="hair" count={HAIR_NAMES.length} names={HAIR_NAMES} onPick={(hair) => set({ hair })} /></div>
        <div><Label style={{ marginBottom: 7 }}>Цвет волос</Label>
          <ColorDots colors={HAIR_COLORS} value={cfg.hair_color || ""} onPick={(hair_color) => set({ hair_color })} /></div>
        <div><Label style={{ marginBottom: 7 }}>Глаза</Label>
          <VariantRow cfg={cfg} part="eyes" count={EYES_NAMES.length} names={EYES_NAMES} onPick={(eyes) => set({ eyes })} /></div>
        <div><Label style={{ marginBottom: 7 }}>Рот</Label>
          <VariantRow cfg={cfg} part="mouth" count={MOUTH_NAMES.length} names={MOUTH_NAMES} onPick={(mouth) => set({ mouth })} /></div>
        <div><Label style={{ marginBottom: 7 }}>Дополнительно</Label>
          <VariantRow cfg={cfg} part="accessory" count={ACC_NAMES.length} names={ACC_NAMES} onPick={(accessory) => set({ accessory })} /></div>

        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8, marginTop: 2 }}>
          <Button variant="ghost" onClick={onClose}>Отмена</Button>
          <Button onClick={save} disabled={busy}>{busy ? "Сохраняем…" : "Сохранить"}</Button>
        </div>
      </div>
    </Modal>
  );
}
