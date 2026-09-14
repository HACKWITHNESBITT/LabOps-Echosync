"use client";

import { useEffect, useRef, useState } from "react";
import type { RadarState, RadarTarget } from "@/lib/types";

// Center panel: radial sonar radar with real-time vector links between self
// and every geo candidate inside the 15m geofence.
export function RadarPanel({ state }: { state: RadarState | null }) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const angleRef = useRef(0);
  const [hovered, setHovered] = useState<RadarTarget | null>(null);

  useEffect(() => {
    let raf: number;
    const canvas = canvasRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    let last = performance.now();
    const draw = (now: number) => {
      const dt = (now - last) / 1000;
      last = now;
      angleRef.current = (angleRef.current + dt * 0.6) % (Math.PI * 2);
      render(ctx, canvas, state, angleRef.current);
      raf = requestAnimationFrame(draw);
    };
    raf = requestAnimationFrame(draw);
    return () => cancelAnimationFrame(raf);
  }, [state]);

  if (!state) return <EmptyRadar />;

  const handleMove = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const el = e.currentTarget;
    const r = el.getBoundingClientRect();
    // same transform as render(): canvas is CSS-scaled to fit, so scale by ratios
    const x = ((e.clientX - r.left) / r.width) * el.width - 6;
    const y = ((e.clientY - r.top) / r.height) * el.height - 4;
    const scale = 138 / 120;
    const hit = state.targets.slice(0, 6).find((t) => {
      const end = t.link?.[1];
      if (!end) return false;
      const dx = x - (6 + end[0] * scale);
      const dy = y - (4 + end[1] * scale);
      return dx * dx + dy * dy < 15 * 15;
    });
    setHovered((prev) => (prev?.id === hit?.id ? prev : hit ?? null));
  };

  return (
    <div className="relative flex items-center justify-center">
      <canvas
        ref={canvasRef}
        width={360}
        height={360}
        onMouseMove={handleMove}
        onMouseLeave={() => setHovered(null)}
        className="block h-auto max-w-[min(100%,360px)] select-none"
      />

      <div className="pointer-events-none absolute top-2">
        <Tooltip target={hovered} />
      </div>
    </div>
  );
}

function render(
  ctx: CanvasRenderingContext2D,
  canvas: HTMLCanvasElement,
  state: RadarState | null,
  angle: number,
) {
  const W = canvas.width;
  const H = canvas.height;
  const R = 138;
  const cx = W / 2 + 6;
  const cy = H / 2 + 4;

  ctx.clearRect(0, 0, W, H);

  if (!state) {
    // idle rings
    drawRings(ctx, cx, cy, R, "#9BAAC1", 0.25);
    return;
  }

  drawRings(ctx, cx, cy, R, "#9BAAC1", 0.4);

  // scale link coords into canvas space (x is fwd +x, y is +y)
  const scale = R / 120;
  const px = (x: number) => cx + x * scale;
  const py = (y: number) => cy + y * scale;

  // vector links (dashed, pulse-colored) drawn beneath the targets
  state.targets.forEach((t) => {
    const end = t.link?.[1];
    if (!end) return;
    const x = px(end[0]);
    const y = py(end[1]);
    ctx.beginPath();
    ctx.setLineDash([4, 5]);
    ctx.strokeStyle = "rgba(231,131,154,0.5)";
    ctx.lineWidth = 1;
    ctx.moveTo(cx, cy);
    ctx.lineTo(x, y);
    ctx.stroke();
    ctx.setLineDash([]);
  });

  // self
  ctx.beginPath();
  ctx.fillStyle = "#E78A9F";
  ctx.arc(cx, cy, 6, 0, Math.PI * 2);
  ctx.fill();
  ctx.beginPath();
  ctx.fillStyle = "#182334";
  ctx.arc(cx, cy, 2.6, 0, Math.PI * 2);
  ctx.fill();

  // sweep beam
  ctx.beginPath();
  ctx.moveTo(cx, cy);
  ctx.lineTo(cx + Math.cos(angle) * R, cy + Math.sin(angle) * R);
  ctx.strokeStyle = "rgba(233,144,165,0.85)";
  ctx.lineWidth = 1.2;
  ctx.stroke();
  const grad = ctx.createRadialGradient(cx, cy, 0, cx, cy, R);
  grad.addColorStop(0, "rgba(231,132,155,0.10)");
  grad.addColorStop(1, "rgba(231,132,155,0)");
  ctx.beginPath();
  ctx.moveTo(cx, cy);
  ctx.arc(cx, cy, R, angle - 0.6, angle);
  ctx.closePath();
  ctx.fillStyle = grad;
  ctx.fill();

  // targets
  state.targets.slice(0, 6).forEach((t) => {
    const end = t.link?.[1] ?? t.link?.[0];
    if (!end) return;
    const x = px(end[0]);
    const y = py(end[1]);
    const r = t.similarity !== undefined && t.similarity >= 0.6 ? 15 : 10;
    ctx.beginPath();
    ctx.strokeStyle = "#E2E8F0";
    ctx.lineWidth = 1.2;
    ctx.arc(x, y, r, 0, Math.PI * 2);
    ctx.stroke();
    ctx.beginPath();
    ctx.fillStyle = t.similarity !== undefined && t.similarity >= 0.6 ? "#E78A9F" : "#E2E8F0";
    ctx.arc(x, y, 5, 0, Math.PI * 2);
    ctx.fill();
  });
}

function drawRings(
  ctx: CanvasRenderingContext2D,
  cx: number, cy: number, R: number, color: string, alpha: number,
) {
  ctx.strokeStyle = color;
  ctx.globalAlpha = alpha;
  for (let i = 1; i <= 4; i++) {
    ctx.beginPath();
    ctx.arc(cx, cy, (R / 4) * i, 0, Math.PI * 2);
    ctx.lineWidth = 1;
    ctx.stroke();
  }
  ctx.beginPath();
  ctx.moveTo(cx - R, cy);
  ctx.lineTo(cx + R, cy);
  ctx.moveTo(cx, cy - R);
  ctx.lineTo(cx, cy + R);
  ctx.globalAlpha = alpha * 0.8;
  ctx.stroke();
  ctx.globalAlpha = 1;
}

function Tooltip({ target }: { target: RadarTarget | null }) {
  if (!target) return null;
  return (
    <div className="pointer-events-none absolute top-2 animate-fadeup rounded border border-line/50 bg-ink-850 px-2 py-1 text-[9px] shadow-lg">
      <p className="font-mono text-pulse">{target.id}</p>
      <p className="text-muted">
        ~{target.distance_m} m · {(target.shared_tokens ?? []).slice(0, 2).join(", ")}
      </p>
    </div>
  );
}

function EmptyRadar() {
  return (
    <div className="relative flex h-[360px] w-[360px] max-w-full items-center justify-center">
      <div className="absolute inset-1/2 h-1/2 w-1/2 -translate-x-1/2 -translate-y-1/2 rounded-full border border-line/30" />
      <div className="rounded border border-line/20 bg-ink-850/50 px-4 py-3 text-center">
        <p className="text-[10px] text-muted">proximity radar idling</p>
        <p className="mt-1 font-mono text-[9px] text-pulse/70">awaiting POST /v1/presence</p>
      </div>
    </div>
  );
}