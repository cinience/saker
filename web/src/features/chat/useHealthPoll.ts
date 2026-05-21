import { useEffect, useRef } from "react";
import { useChatStore } from "./useChatStore";

const POLL_INTERVAL = 15_000;
const RETRY_INTERVAL = 5_000;
const TIMEOUT = 5_000;

function resolveHealthUrl(): string {
  if (typeof window === "undefined") return "/health";
  const loc = window.location;
  if (loc.port === "10111") {
    return `${loc.protocol}//${loc.hostname}:17000/health`;
  }
  return "/health";
}

export function useHealthPoll() {
  const setServerReachable = useChatStore((s) => s.setServerReachable);
  const timerRef = useRef<ReturnType<typeof setTimeout>>();
  const mountedRef = useRef(true);

  useEffect(() => {
    mountedRef.current = true;
    const url = resolveHealthUrl();
    let consecutiveFailures = 0;

    const check = async () => {
      try {
        const ctrl = new AbortController();
        const id = setTimeout(() => ctrl.abort(), TIMEOUT);
        const res = await fetch(url, {
          method: "GET",
          signal: ctrl.signal,
          credentials: "omit",
        });
        clearTimeout(id);

        if (!mountedRef.current) return;
        if (res.ok) {
          consecutiveFailures = 0;
          setServerReachable(true);
          timerRef.current = setTimeout(check, POLL_INTERVAL);
        } else {
          throw new Error(`status ${res.status}`);
        }
      } catch {
        if (!mountedRef.current) return;
        consecutiveFailures++;
        if (consecutiveFailures >= 2) {
          setServerReachable(false);
        }
        timerRef.current = setTimeout(check, RETRY_INTERVAL);
      }
    };

    check();

    const onVisibility = () => {
      if (!document.hidden) {
        clearTimeout(timerRef.current);
        check();
      }
    };
    document.addEventListener("visibilitychange", onVisibility);

    return () => {
      mountedRef.current = false;
      clearTimeout(timerRef.current);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, [setServerReachable]);
}
