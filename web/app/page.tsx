"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Activity, Cpu, Radar as RadarIcon, Rss, Mic, LayoutGrid } from "lucide-react";

import { PipelinePanel } from "@/components/pipeline-panel";
import { RadarPanel } from "@/components/radar-panel";
import { FeedPanel } from "@/components/feed-panel";
import { VoiceInterface } from "@/components/voice-interface";
import { api, connect } from "@/lib/ws";
import type { Match, Metrics, RadarState, ScrubEvent, ServerMsg } from "@/lib/types";

export default function Dashboard() {
  const [activeTab, setActiveTab] = useState<"ops" | "voice">("ops");
  const [wsOn, setWsOn] = useState(false);
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [scrubs, setScrubs] = useState<ScrubEvent[]>([]);
  const [radar, setRadar] = useState<RadarState | null>(null);
  const [matches, setMatches] = useState<Match[]>([]);
  const [lastHeartbeat, setLastHeartbeat] = useState<string>("—");
  const bootRef = useRef(false);

  useEffect(() => {
    if (bootRef.current) return;
    bootRef.current = true;

    api<Metrics>("/metrics").then(setMetrics).catch(() => undefined);
    api<{ events: ScrubEvent[] }>("/v1/pipeline/events")
      .then((r) => {
        const tail = r.events.filter((e) => e.type === "scrub.complete");
        setScrubs(tail.slice(-30));
      })
      .catch(() => undefined);

    const interval = window.setInterval(
      () => fetchMetrics(setMetrics),
      5000,
    );

    const close = connect(
      (m: ServerMsg) => {
        switch (m.type) {
          case "hello":
            setWsOn(true);
            break;
          case "pipeline":
            if (m.scrub && m.scrub.type === "scrub.complete") {
              setScrubs((prev) => [...prev.slice(-29), m.scrub!]);
            }
            break;
          case "radar":
            if (m.radar) setRadar(m.radar);
            break;
          case "match": {
            const mt = m.match!;
            setLastHeartbeat(new Date().toLocaleTimeString());
            setMatches((prev) => {
              const i = prev.findIndex((x) => x.id === mt.id);
              if (i >= 0) {
                const next = [...prev];
                next[i] = { ...next[i], ...mt };
                return next;
              }
              return [mt, ...prev].slice(0, 30);
            });
            break;
          }
          case "match_ttl":
            if (m.message && m.ttl_seconds !== undefined) {
              setMatches((prev) =>
                prev.map((x) =>
                  x.id === m.message! ? { ...x, ttl_seconds: m.ttl_seconds! } : x,
                ),
              );
            }
            break;
          default:
            break;
        }
      },
      () => setWsOn(true),
    );

    return () => {
      window.clearInterval(interval);
      close();
    };
  }, []);

  const pipelineUp = useMemo(
    () => metrics && metrics.scrubs_run > 0,
    [metrics],
  );
  const radarLock = !!radar && radar.targets.length > 0;

  return (
    <div className="flex min-h-screen flex-col bg-[#0b1220]">
      <Header
        wsOn={wsOn}
        metrics={metrics}
        activeTab={activeTab}
        setActiveTab={setActiveTab}
      />

      {activeTab === "voice" ? (
        <main className="flex flex-1 flex-col p-4 md:p-6">
          <VoiceInterface />
        </main>
      ) : (
        <main className="grid flex-1 grid-cols-1 gap-px bg-line/40 md:grid-cols-3">
          <section className="flex min-h-[420px] flex-col bg-ink-900">
            <PanelTitle
              icon={<Cpu size={13} />}
              title="PII Scrubbing Pipeline"
              sub={pipelineUp ? `${metrics!.scrubs_run} transcripts scrubbed` : "waiting for transcripts…"}
              accent={pipelineUp ? "#E7839A" : "#7C8797"}
              code={metrics?.pipeline?.toUpperCase()}
            />
            <div className="flex-1 overflow-hidden p-4">
              <PipelinePanel events={scrubs} />
            </div>
          </section>

          <section className="relative flex min-h-[420px] flex-col bg-ink-900">
            <PanelTitle
              icon={<RadarIcon size={13} />}
              title="Proximity Match Radar"
              sub={radarLock ? `${radar!.targets.length} targets / ${radar!.radius_m}m` : "no targets · scanning"}
              accent="#E7839A"
              code={metrics ? `${metrics!.active_users} users` : "…"}
            />
            <div className="flex flex-1 items-center justify-center p-4">
              <RadarPanel state={radar} />
            </div>
          </section>

          <section className="flex min-h-[420px] flex-col bg-ink-900">
            <PanelTitle
              icon={<Rss size={13} />}
              title="Ephemeral Match Feed"
              sub={lastHeartbeat === "—" ? "awaiting proximity signal" : `last signal ${lastHeartbeat}`}
              accent="#E7839A"
              code={metrics ? `TTL ${metrics!.match_ttl_seconds}s` : "…"}
            />
            <div className="flex-1 overflow-hidden p-4">
              <FeedPanel matches={matches} />
            </div>
          </section>
        </main>
      )}

      <footer className="flex items-center gap-3 border-t border-line/50 px-5 py-2 text-[10px] text-muted">
        <Activity size={11} className={wsOn ? "text-pulse" : ""} />
        <span>{wsOn ? "WS · live" : "WS · connecting…"}</span>
        <span className="ml-auto">
          {metrics
            ? `p95 ${metrics.p95_match_latency_ms.toFixed(1)}ms · radius ${metrics.match_radius_m}m · ${metrics.ollama_active ? `edge ${metrics.ollama_model}` : "edge model offline"}` 
            : "loading metrics…"}
        </span>
      </footer>
    </div>
  );
}

function fetchMetrics(set: (m: Metrics) => void) {
  api<Metrics>("/metrics").then(set).catch(() => undefined);
}

function Header({
  wsOn,
  metrics,
  activeTab,
  setActiveTab,
}: {
  wsOn: boolean;
  metrics: Metrics | null;
  activeTab: "ops" | "voice";
  setActiveTab: (tab: "ops" | "voice") => void;
}) {
  return (
    <header className="flex items-center gap-4 border-b border-line/60 px-5 py-3">
      <div>
        <h1 className="text-sm font-bold tracking-wide text-paper">EchoSync</h1>
        <p className="text-[10px] text-muted">Command Center · ops view</p>
      </div>

      {/* Center Navigation Tabs */}
      <div className="mx-auto flex items-center gap-1 rounded-lg border border-line/30 bg-ink-850 p-1 text-xs">
        <button
          onClick={() => setActiveTab("ops")}
          className={`flex items-center gap-1.5 rounded px-3 py-1 text-[11px] font-medium transition-colors ${
            activeTab === "ops"
              ? "bg-line/30 text-white shadow-sm"
              : "text-muted hover:text-paper"
          }`}
        >
          <LayoutGrid size={13} />
          Ops Grid
        </button>
        <button
          onClick={() => setActiveTab("voice")}
          className={`flex items-center gap-1.5 rounded px-3 py-1 text-[11px] font-medium transition-all ${
            activeTab === "voice"
              ? "bg-cyan-500/20 text-cyan-300 shadow-[0_0_12px_rgba(0,242,254,0.3)]"
              : "text-muted hover:text-cyan-300"
          }`}
        >
          <Mic size={13} />
          EchoSync Voice
          <span className="rounded bg-cyan-400/20 px-1 py-0.2 text-[8px] font-mono text-cyan-300">
            NEW
          </span>
        </button>
      </div>

      <div className="flex items-center gap-4 text-[11px]">
        <StatusDot live={wsOn} label={wsOn ? "live" : "reconnecting"} />
        <span className="hidden font-mono text-muted sm:inline">
          {metrics ? new Date(metrics.uptime_seconds * 1000).toISOString().substr(11, 8) : "00:00:00"} uptime
        </span>
      </div>
    </header>
  );
}

function StatusDot({ live, label }: { live: boolean; label: string }) {
  return (
    <span className="flex items-center gap-2">
      <span
        className={`inline-block h-2 w-2 rounded-full ${live ? "animate-blink bg-pulse" : "bg-muted"}`}
      />
      {label}
    </span>
  );
}

function PanelTitle({
  icon, title, sub, accent, code,
}: { icon: React.ReactNode; title: string; sub: string; accent: string; code?: string }) {
  return (
    <div className="flex items-center gap-3 border-b border-line/60 px-4 py-3">
      <span style={{ color: accent }}>{icon}</span>
      <div className="min-w-0">
        <h2 className="text-[12px] font-semibold text-paper">{title}</h2>
        <p className="truncate text-[10px] text-muted">{sub}</p>
      </div>
      {code && (
        <span className="ml-auto rounded border border-line/50 px-1.5 py-0.5 font-mono text-[9px] text-muted">
          {code}
        </span>
      )}
    </div>
  );
}