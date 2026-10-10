import { useEffect, useState } from "react";

// Wait for completion before scheduling another request. Hidden tabs pause
// polling, and becoming visible starts a fresh request immediately.
export function useSerialPolling(
  refresh: (signal: AbortSignal) => Promise<void>,
  interval: number | null,
) {
  const [visible, setVisible] = useState(!document.hidden);
  useEffect(() => {
    const update = () => setVisible(!document.hidden);
    document.addEventListener("visibilitychange", update);
    return () => document.removeEventListener("visibilitychange", update);
  }, []);

  useEffect(() => {
    if (!visible || interval === null) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      await refresh(controller.signal);
      if (!controller.signal.aborted)
        timer = setTimeout(() => void poll(), interval);
    };
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [refresh, interval, visible]);
}
