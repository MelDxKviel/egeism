import { AnswerKind } from "./api";
import { Icon } from "./icons";

// AnswerInput adapts to the task's answer_kind (design §3.3):
// number → numeric field · string → text · sequence → digit keypad + chips ·
// set → space-separated numbers with a chip preview.
export function AnswerInput({ kind, value, onChange, disabled }:
  { kind: AnswerKind; value: string; onChange: (v: string) => void; disabled?: boolean }) {

  if (kind === "sequence") {
    const chips = value.split("").filter(Boolean);
    return (
      <div>
        <div style={{ display: "flex", gap: 6, minHeight: 40, flexWrap: "wrap", marginBottom: 12 }}>
          {chips.length === 0 && <span style={{ color: "var(--text-3)" }}>Нажимай цифры по порядку…</span>}
          {chips.map((c, i) => (
            <span key={i} className="mono" style={{ background: "var(--accent-soft)", color: "var(--accent-2)", borderRadius: 999, padding: "8px 12px", fontWeight: 700 }}>{c}</span>
          ))}
        </div>
        <Keypad disabled={disabled} onKey={(k) => onChange(value + k)} onBack={() => onChange(value.slice(0, -1))} />
      </div>
    );
  }

  if (kind === "set") {
    const chips = value.split(/[\s,;]+/).filter(Boolean);
    return (
      <div>
        <input disabled={disabled} value={value} onChange={(e) => onChange(e.target.value)}
          placeholder="числа через пробел, порядок не важен" style={{ width: "100%", fontSize: 18 }} className="mono" />
        <div style={{ display: "flex", gap: 6, flexWrap: "wrap", marginTop: 10 }}>
          {chips.map((c, i) => (
            <span key={i} className="mono" style={{ background: "color-mix(in srgb, var(--text) 8%, transparent)", color: "var(--text-2)", borderRadius: 999, padding: "4px 10px", fontSize: 13 }}>{c}</span>
          ))}
        </div>
      </div>
    );
  }

  return (
    <input disabled={disabled} value={value} onChange={(e) => onChange(e.target.value)}
      inputMode={kind === "number" ? "decimal" : "text"}
      placeholder={kind === "number" ? "число" : "ответ"}
      style={{ width: "100%", fontSize: 20 }} className="mono" />
  );
}

function Keypad({ onKey, onBack, disabled }: { onKey: (k: string) => void; onBack: () => void; disabled?: boolean }) {
  const keys = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "0"];
  return (
    // auto-fit keeps every key at ≥52px (above the 44px touch-target minimum) —
    // the fixed 5 columns gave ~36px keys on narrow phones. 52px still yields
    // exactly 5 columns in the full 320px-wide grid, so desktop is unchanged;
    // «стереть» spans whatever column count the width produced.
    <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(52px, 1fr))", gap: 8, maxWidth: 320 }}>
      {keys.map((k) => (
        <button key={k} disabled={disabled} onClick={() => onKey(k)} className="btn btn-ghost mono" style={{
          padding: "12px 0", fontSize: 18, fontWeight: 700,
        }}>{k}</button>
      ))}
      <button disabled={disabled} onClick={onBack} className="btn btn-ghost" style={{
        gridColumn: "1 / -1", padding: "10px 0",
      }}><Icon name="arrowLeft" size={15} /> стереть</button>
    </div>
  );
}
