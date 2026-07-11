// Confetti — a tiny dependency-free canvas burst for the delight moments:
// a correct answer, a perfect variant, a streak milestone, the closed day goal.
// One fixed full-viewport canvas (pointer-events: none) is created on demand,
// runs a rAF particle loop and removes itself when the last piece settles.
// Colors are read from the live theme tokens, so the salute matches light and
// dark; prefers-reduced-motion turns the whole thing into a silent no-op.

interface Particle {
  x: number; y: number;       // px, viewport coords
  vx: number; vy: number;     // px/s
  w: number; h: number;       // piece size, px
  rot: number; vr: number;    // rotation, rad / rad per s
  age: number; ttl: number;   // s
  color: string;
}

export interface BurstOptions {
  /** Piece count; scale it with the moment (combo, идеальный вариант). */
  count?: number;
  /** Burst origin, viewport px. Defaults to the upper-center of the screen. */
  x?: number;
  y?: number;
}

const GRAVITY = 1500;   // px/s²
const DRAG = 0.55;      // horizontal air drag, 1/s

let canvas: HTMLCanvasElement | null = null;
let ctx: CanvasRenderingContext2D | null = null;
let parts: Particle[] = [];
let raf = 0;
let lastT = 0;

export const prefersReducedMotion = () =>
  window.matchMedia("(prefers-reduced-motion: reduce)").matches;

// The design tokens live on the .app scope (see theme.css), so resolve them
// there — a bare documentElement lookup would come back empty.
function tokenColors(): string[] {
  const app = document.querySelector(".app");
  if (!app) return ["#0071E3", "#34C759", "#FF9F0A"];
  const cs = getComputedStyle(app);
  const colors = ["--accent", "--ok", "--warn", "--hm3", "--accent-2"]
    .map((t) => cs.getPropertyValue(t).trim())
    .filter(Boolean);
  return colors.length > 0 ? colors : ["#0071E3", "#34C759", "#FF9F0A"];
}

function ensureCanvas(): CanvasRenderingContext2D | null {
  if (!canvas) {
    canvas = document.createElement("canvas");
    canvas.setAttribute("aria-hidden", "true");
    Object.assign(canvas.style, {
      position: "fixed", inset: "0", width: "100%", height: "100%",
      pointerEvents: "none", zIndex: "80",
    } as CSSStyleDeclaration);
    document.body.appendChild(canvas);
    ctx = canvas.getContext("2d");
  }
  // Re-fit every burst — cheap, and it tracks window resizes between bursts.
  const dpr = window.devicePixelRatio || 1;
  canvas.width = Math.round(window.innerWidth * dpr);
  canvas.height = Math.round(window.innerHeight * dpr);
  ctx?.setTransform(dpr, 0, 0, dpr, 0, 0);
  return ctx;
}

function teardown() {
  cancelAnimationFrame(raf);
  raf = 0;
  parts = [];
  canvas?.remove();
  canvas = null;
  ctx = null;
}

function tick(t: number) {
  if (!ctx || !canvas) { teardown(); return; }
  const dt = Math.min(0.05, (t - lastT) / 1000 || 0.016);
  lastT = t;
  ctx.clearRect(0, 0, window.innerWidth, window.innerHeight);
  const alive: Particle[] = [];
  for (const p of parts) {
    p.age += dt;
    if (p.age >= p.ttl || p.y > window.innerHeight + 40) continue;
    p.vy += GRAVITY * dt;
    p.vx *= Math.max(0, 1 - DRAG * dt);
    p.x += p.vx * dt;
    p.y += p.vy * dt;
    p.rot += p.vr * dt;
    // Fade out over the last third of the piece's life.
    const left = 1 - p.age / p.ttl;
    ctx.globalAlpha = Math.min(1, left / 0.33);
    ctx.fillStyle = p.color;
    ctx.save();
    ctx.translate(p.x, p.y);
    ctx.rotate(p.rot);
    // A cheap 3D tumble: the piece's width breathes as it flips.
    ctx.scale(1, 0.35 + 0.65 * Math.abs(Math.sin(p.rot * 1.7 + p.ttl)));
    ctx.fillRect(-p.w / 2, -p.h / 2, p.w, p.h);
    ctx.restore();
    alive.push(p);
  }
  ctx.globalAlpha = 1;
  parts = alive;
  if (parts.length > 0) raf = requestAnimationFrame(tick);
  else teardown();
}

/** Fire a confetti burst. Silently does nothing under prefers-reduced-motion. */
export function confettiBurst(opts: BurstOptions = {}) {
  if (prefersReducedMotion()) return;
  const c = ensureCanvas();
  if (!c) return;
  const colors = tokenColors();
  const count = opts.count ?? 28;
  const ox = opts.x ?? window.innerWidth / 2;
  const oy = opts.y ?? window.innerHeight * 0.38;
  for (let i = 0; i < count; i++) {
    // Fan upward: -90° ± 65°, faster pieces fly further before gravity wins.
    const angle = (-90 + (Math.random() * 130 - 65)) * (Math.PI / 180);
    const speed = 380 + Math.random() * 520;
    parts.push({
      x: ox + (Math.random() - 0.5) * 24,
      y: oy + (Math.random() - 0.5) * 12,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed,
      w: 5 + Math.random() * 5,
      h: 8 + Math.random() * 6,
      rot: Math.random() * Math.PI * 2,
      vr: (Math.random() - 0.5) * 14,
      age: 0,
      ttl: 1 + Math.random() * 0.8,
      color: colors[i % colors.length],
    });
  }
  if (!raf) {
    lastT = performance.now();
    raf = requestAnimationFrame(tick);
  }
}
