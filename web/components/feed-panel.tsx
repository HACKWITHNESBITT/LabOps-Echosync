"use client";

import { useEffect, useState } from "react";
import { Loader2, Send, Sparkles } from "lucide-react";

import { api } from "@/lib/ws";
import type { IcebreakerResponse, Match } from "@/lib/types";

// Right panel: self-destructing match cards (Redis-enforced 15-min TTL) with
// AI icebreaker generation triggers.
export function FeedPanel({ matches }: { matches: Match[] }) {
  if (matches.length === 0) return <EmptyFeed />;

  return (
    <div className="flex h-full flex-col gap-2 overflow-y-auto pr-1">
      {matches.map((m) => (
        <FeedCard key={m.id} match={m} />
      ))}
    </div>
  );
}

function FeedCard({ match }: { match: Match }) {
  const [ice, setIce] = useState<{ text: string; source: string } | null>(match.icebreaker
    ? { text: match.icebreaker, source: "match" }
    : null);
  const [busy, setBusy] = useState(false);
  const [expired, setExpired] = useState(match.ttl_seconds <= 0);

  useEffect(() => {
    setExpired(match.ttl_seconds <= 0);
  }, [match.ttl_seconds]);

  const generate = async () => {
    setBusy(true);
    try {
      const res = await api<IcebreakerResponse>("/v1/icebreaker", {
        method: "POST",
        body: JSON.stringify({
          user_id: match.peer_user_id,
          match_id: match.id,
        }),
      });
      setIce({ text: res.icebreaker, source: res.source });
    } catch {
      setIce({ text: match.icebreaker ?? "match expired — try again at closer range.", source: "expired" });
    } finally {
      setBusy(false);
    }
  };

  if (expired) {
    return (
      <div className="animate-fadeup rounded border border-line/30 bg-ink-850/40 p-3 opacity-60">
        <p className="font-mono text-[9px] text-line">expired · auto-purged by Redis TTL</p>
        <p className="mt-1 truncate text-[10px] text-muted">{match.id}</p>
      </div>
    );
  }

  const pct = Math.max(0, Math.min(100, Math.round(match.similarity * 100)));

  return (
    <div className="animate-fadeup rounded border border-pulse/30 bg-ink-850/70 p-3 shadow-[0_6px_20px_rgba(0,0,0,0.35)]">
      <div className="flex items-center gap-2">
        <span className="font-mono text-[11px] text-paper">{match.peer_user_id}</span>
        <span className="ml-auto rounded border border-line/40 px-1 py-0.5 font-mono text-[8px] text-muted">
          ~{match.distance_m}m
        </span>
      </div>

      <div className="mt-1.5 flex flex-wrap gap-1">
        {match.shared_tokens.map((t, i) => (
          <span key={i} className="rounded-sm bg-pulse/10 px-1.5 py-0.5 font-mono text-[8px] text-pulse">
            {t}
          </span>
        ))}
      </div>

      <div className="mt-2 flex items-center gap-2 text-[9px] text-muted">
        <span className="shrink-0">similarity</span>
        <div className="h-1 min-w-0 flex-1 overflow-hidden rounded bg-line/30">
          <div className="h-full rounded bg-pulse/80 transition-all" style={{ width: `${pct}%` }} />
        </div>
        <span className="font-mono">{match.similarity.toFixed(3)}</span>
      </div>

      {ice ? (
        <p className="mt-2 rounded border border-line/30 bg-ink-900/70 p-2 text-[10px] leading-relaxed text-paper"
          onClick={generate}
          title="click to regenerate">
          “{ice.text}”
          <span className="mt-1 block font-mono text-[8px] text-muted">source: {ice.source}</span>
        </p>
      ) : (
        <button
          onClick={generate}
          disabled={busy}
          className="mt-2 flex w-full items-center justify-center gap-1.5 rounded border border-pulse/50 py-1.5 text-[10px] font-semibold text-pulse transition-colors hover:bg-pulse/10 disabled:opacity-50"
        >
          {busy ? <Loader2 size={11} className="animate-spin" /> : <Sparkles size={11} />}
          generate icebreaker
        </button>
      )}

      <div className="mt-2 flex items-center justify-between border-t border-line/30 pt-1.5 font-mono text-[9px]">
        <span className="flex items-center gap-1 text-pulse">
          <Send size={9} /> self-destructs in
        </span>
        <Countdown ttl={match.ttl_seconds} expiresAt={match.expires_at} />
      </div>
    </div>
  );
}

function Countdown({ ttl, expiresAt }: { ttl: number; expiresAt?: string }) {
  const [, tick] = useState(0);
  useEffect(() => {
    const i = window.setInterval(() => tick((t) => t + 1), 1000);
    return () => window.clearInterval(i);
  }, []);
  const remaining =
    expiresAt && !expired(expiresAt)
      ? Math.max(0, Math.floor((new Date(expiresAt).getTime() - Date.now()) / 1000))
      : Math.max(0, ttl);
  return (
    <span className="text-pulse">
      {mmss(remaining)} <span className="text-muted">left</span>
    </span>
  );
}

function expired(iso: string) {
  return new Date(iso).getTime() <= Date.now();
}

function mmss(s: number) {
  const m = Math.floor(s / 60);
  const ss = s % 60;
  return `${String(m).padStart(2, "0")}:${String(ss).padStart(2, "0")}`;
}

function EmptyFeed() {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
      <Sparkles size={18} className="text-line" />
      <p className="text-[10px] text-muted">no ephemeral matches</p>
      <p className="font-mono text-[9px] text-line">cards here self-destruct after 15:00 TTL</p>
    </div>
  );
}