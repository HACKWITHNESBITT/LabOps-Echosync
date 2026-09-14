"use client";

import type { ServerMsg } from "./types";

export const API_BASE =
  process.env.NEXT_PUBLIC_API_URL?.replace(/\/+$/, "") || "http://127.0.0.1:8080";

export function wsUrl(): string {
  const base = API_BASE.replace(/^http/, "ws");
  return `${base}/ws?user_id=dashboard`;
}

export function connect(onMsg: (m: ServerMsg) => void, onOpen?: () => void) {
  let ws: WebSocket | null = null;
  let closedByUs = false;

  const open = () => {
    ws = new WebSocket(wsUrl());
    ws.onopen = () => onOpen?.();
    ws.onmessage = (ev) => {
      try {
        onMsg(JSON.parse(ev.data) as ServerMsg);
      } catch {
        /* malformed frame — ignore */
      }
    };
    ws.onclose = () => {
      if (!closedByUs) setTimeout(open, 2000);
    };
    ws.onerror = () => ws?.close();
  };

  open();
  return () => {
    closedByUs = true;
    ws?.close();
  };
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}