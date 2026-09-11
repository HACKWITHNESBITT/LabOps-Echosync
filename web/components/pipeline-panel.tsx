"use client";

import { useEffect, useRef } from "react";
import { ArrowDown } from "lucide-react";

import type { ScrubEvent } from "@/lib/types";

// Left panel: live PII scrubbing pipeline.
// raw (RAM) -> firewall scrub -> tokens -> vector embed. Never persisted.
export function PipelinePanel({ events }: { events: ScrubEvent[] }) {
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTop = 0;
  }, [events.length]);

  if (events.length === 0) {
    return <EmptyPipeline />;
  }

  const latest = events[0];

  return (
    <div className="flex h-full flex-col gap-3">
      <StageBar event={latest} />

      <ArrowDown size={14} className="mx-auto text-muted" />

      <div ref={scrollRef} className="flex-1 space-y-1.5 overflow-y-auto pr-1">
        {events.map((e) => (
          <ScrubRow key={e.id} e={e} />
        ))}
      </div>
    </div>
  );
}

function StageBar({ event }: { event: ScrubEvent }) {
  const tokens = event.tokens ?? [];
  const blocked = (event.blocked ?? []).length;
  return (
    <div className="rounded border border-line/40 bg-ink-850 p-3 text-[10px] leading-relaxed">
      <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1">
        <Stage label="raw · RAM" active={true} />
        <Stage label="firewall" active={true} danger={blocked > 0} />
        <Stage label="tokens" active={true} />
        <Stage label="embed" active={false} />
      </div>
      <div className="flex flex-wrap gap-1">
        {tokens.map((t) => (
          <span
            key={t}
            className="rounded-sm border border-pulse/40 bg-pulse/10 px-1.5 py-0.5 font-mono text-[9px] text-pulse"
          >
            {t}
          </span>
        ))}
        {tokens.length === 0 && (
          <span className="text-muted">no interest tokens extracted</span>
        )}
      </div>
      <div className="mt-2 space-y-1 font-mono text-muted">
        <Dot>blocked {blocked} PII categories · {event.pipeline} engine</Dot>
        <Dot>raw {event.raw_length}B in-memory → {event.redacted_len ?? "–"}B kept</Dot>
        {event.raw && (
          <p className="truncate text-[9px] text-line" title={event.raw}>
            source: “{truncate(event.raw, 72)}”
          </p>
        )}
      </div>
    </div>
  );
}

function ScrubRow({ e }: { e: ScrubEvent }) {
  const isFallback = e.pipeline === "rule" && (e.blocked ?? []).length === 0;
  return (
    <div className="animate-fadeup rounded border border-line/30 bg-ink-850/60 px-2.5 py-2 text-[10px]">
      <div className="flex items-center gap-2">
        <span className="font-mono text-pulse/70">{e.id}</span>
        <span className="font-mono text-muted">{span2(e.at)}</span>
        <span
          className={`ml-auto rounded-sm px-1 py-0.5 font-mono text-[8px] ${
            isFallback ? "bg-amber-500/15 text-amber-300" : "bg-pulse/10 text-pulse"
          }`}
        >
          {e.pipeline}
        </span>
      </div>
      <div className="mt-1 truncate font-mono text-muted">
        {(e.tokens ?? []).join(" · ") || "no tokens"}
      </div>
    </div>
  );
}

function EmptyPipeline() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-3 text-muted">
      <div className="flex gap-1.5">
        {[...Array(12)].map((_, i) => (
          <span
            key={i}
            className="inline-block h-4 w-[3px] animate-pulse"
            style={{
              background: "#7C8797",
              opacity: 0.25 + ((i % 5) / 5) * 0.6,
              animationDelay: `${i * 90}ms`,
            }}
          />
        ))}
      </div>
      <p className="text-[10px]">
        send <span className="font-mono text-pulse">POST /v1/scrub</span> to feed the firewall
      </p>
    </div>
  );
}

function Stage({ label, active, danger }: { label: string; active: boolean; danger?: boolean }) {
  return (
    <span className="flex items-center gap-1">
      <span
        className={`h-1.5 w-1.5 rounded-full ${
          active ? (danger ? "bg-red-400" : "bg-pulse") : "bg-line"
        }`}
      />
      {label}
    </span>
  );
}

function Dot({ children }: { children: React.ReactNode }) {
  return (
    <p className="flex items-center gap-1">
      <span className="h-1 w-1 rounded-full bg-line/60" />
      {children}
    </p>
  );
}

function span2(iso: string) {
  return iso ? new Date(iso).toLocaleTimeString() : "—";
}

function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, n) + "…" : s;
}