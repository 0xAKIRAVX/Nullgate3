"use client";

import { useEffect, useRef, useState } from "react";
import type { LiveMsg, PanelState } from "./types";

export interface LiveState {
  up: number;
  down: number;
  xray: { running: boolean; restarts: number };
  mode: "ws" | "poll" | "init";
}

/**
 * Live traffic feed: tries the admin WebSocket (/api/ws) first — the same
 * stream the Go hub broadcasts — and falls back to polling /api/state when a
 * WebSocket is unavailable (dev demo mode, strict proxies, …).
 */
export function useLive(initial: PanelState | null): LiveState {
  const [live, setLive] = useState<LiveState>({
    up: initial?.traffic.up ?? 0,
    down: initial?.traffic.down ?? 0,
    xray: {
      running: initial?.xray.running ?? false,
      restarts: initial?.xray.restarts ?? 0,
    },
    mode: "init",
  });
  const wsFail = useRef(0);

  // seed from the first /api/state payload while WS/poll is still connecting
  useEffect(() => {
    if (!initial) return;
    setLive((s) =>
      s.mode === "init"
        ? {
            up: initial.traffic.up,
            down: initial.traffic.down,
            xray: { running: !!initial.xray.running, restarts: initial.xray.restarts ?? 0 },
            mode: "init",
          }
        : s,
    );
  }, [initial]);

  useEffect(() => {
    if (!initial) return;
    let ws: WebSocket | null = null;
    let poll: ReturnType<typeof setInterval> | null = null;
    let alive = true;

    const startPolling = () => {
      if (poll || !alive) return;
      setLive((s) => ({ ...s, mode: "poll" }));
      const tick = async () => {
        try {
          const res = await fetch("/api/state", { credentials: "include" });
          if (!res.ok) return;
          const st = (await res.json()) as PanelState;
          setLive({
            up: st.traffic.up,
            down: st.traffic.down,
            xray: { running: !!st.xray.running, restarts: st.xray.restarts ?? 0 },
            mode: "poll",
          });
        } catch {
          /* transient */
        }
      };
      void tick();
      poll = setInterval(tick, 5000);
    };

    const tryWS = () => {
      if (!alive) return;
      const proto = location.protocol === "https:" ? "wss:" : "ws:";
      try {
        ws = new WebSocket(`${proto}//${location.host}/api/ws`);
      } catch {
        startPolling();
        return;
      }
      const failTimer = setTimeout(() => {
        // no message within 4s → server likely has no WS (demo mode)
        if (alive && ws && ws.readyState !== WebSocket.OPEN) {
          ws.close();
          startPolling();
        }
      }, 4000);
      ws.onopen = () => {
        wsFail.current = 0;
        clearTimeout(failTimer);
        setLive((s) => ({ ...s, mode: "ws" }));
      };
      ws.onmessage = (ev) => {
        try {
          const m = JSON.parse(ev.data) as LiveMsg;
          if (m.type === "traffic") {
            setLive({
              up: m.up, down: m.down,
              xray: { running: !!m.xray.running, restarts: m.xray.restarts ?? 0 },
              mode: "ws",
            });
          }
        } catch { /* ignore */ }
      };
      ws.onerror = () => clearTimeout(failTimer);
      ws.onclose = () => {
        clearTimeout(failTimer);
        if (!alive) return;
        wsFail.current += 1;
        if (wsFail.current >= 2) startPolling();
        else setTimeout(tryWS, 2500);
      };
    };

    tryWS();
    return () => {
      alive = false;
      if (poll) clearInterval(poll);
      if (ws) { ws.onclose = null; ws.close(); }
    };
  }, [initial]);

  return live;
}
