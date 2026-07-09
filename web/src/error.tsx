import { Component, ErrorInfo, ReactNode } from "react";

// ErrorBoundary — the app had NONE, so any render-time throw (a Recharts
// remount race, an unexpected API shape, …) unmounted the whole React tree and
// left a blank white page (the "переключение предмета → пустая страница" bug).
// This catches the throw, keeps everything AROUND it alive (wrap it inside the
// Shell so the nav survives and the user can switch away), and shows a
// recoverable card WITH the real error text — so an otherwise-undiagnosable
// blank page becomes something a screenshot can pinpoint.
//
// resetKey: when it changes (we pass `view:subject`), a boundary that is
// currently showing an error clears itself — so navigating to another tab or
// switching subject re-attempts the render instead of staying stuck.
// bare: the per-screen boundary sits INSIDE the Shell's content div, which
// already applies --main-pad — without it the fallback double-padded (64px of
// dead space per side on a phone).
export class ErrorBoundary extends Component<
  { children: ReactNode; resetKey?: string; theme?: "light" | "dark"; bare?: boolean },
  { error: Error | null }
> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Surface it to the console so prod devtools / logs capture the stack.
    console.error("[ErrorBoundary]", error, info.componentStack);
  }

  componentDidUpdate(prev: { resetKey?: string }) {
    if (this.state.error && prev.resetKey !== this.props.resetKey) {
      this.setState({ error: null });
    }
  }

  render() {
    if (!this.state.error) return this.props.children;
    // The design tokens live on `.app` + data-theme; re-establish that scope so
    // the fallback is styled even when the boundary sits above the app shell.
    const theme = this.props.theme
      ?? ((localStorage.getItem("egeism.theme") as "light" | "dark") || "light");
    return (
      <div className="app" data-theme={theme} style={{ padding: this.props.bare ? 0 : "var(--main-pad)" }}>
        <div className="card" style={{ padding: "clamp(16px, 4vw, 24px)", maxWidth: 560, margin: "24px auto" }}>
          <div style={{ fontWeight: 700, fontSize: 18, letterSpacing: "-0.01em", marginBottom: 8 }}>
            Что-то пошло не так
          </div>
          <div style={{ color: "var(--text-2)", fontSize: 14, lineHeight: 1.5, marginBottom: 14 }}>
            Этот экран не удалось отобразить. Обнови страницу — а если повторяется,
            пришли, пожалуйста, текст ошибки ниже.
          </div>
          <div className="mono" style={{
            background: "var(--surface-2)", border: "1px solid var(--border)", borderRadius: 10,
            padding: "10px 12px", fontSize: 12.5, color: "var(--bad)",
            whiteSpace: "pre-wrap", wordBreak: "break-word", maxHeight: 180, overflow: "auto",
          }}>
            {this.state.error.message || String(this.state.error)}
          </div>
          <div style={{ marginTop: 16, display: "flex", gap: 10, flexWrap: "wrap" }}>
            <button className="btn btn-primary" onClick={() => window.location.reload()}>
              Обновить страницу
            </button>
            <button className="btn btn-ghost" onClick={() => this.setState({ error: null })}>
              Попробовать снова
            </button>
          </div>
        </div>
      </div>
    );
  }
}
