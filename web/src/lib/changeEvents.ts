import type { ChangeEvent } from "../api/types";
import { streamChanges } from "../api/client";

type ChangeListener = (event: ChangeEvent) => void;

const listeners = new Set<ChangeListener>();
let streamPromise: Promise<void> | null = null;
let controller: AbortController | null = null;

function notify(event: ChangeEvent) {
  for (const listener of listeners) {
    listener(event);
  }
}

async function runStreamLoop(signal: AbortSignal): Promise<void> {
  let retryDelay = 1000;

  while (!signal.aborted && listeners.size > 0) {
    try {
      for await (const event of streamChanges(signal)) {
        if (signal.aborted) {
          return;
        }
        notify(event);
      }
      retryDelay = 1000;
    } catch (err) {
      if (signal.aborted) {
        return;
      }
      console.warn("Change stream disconnected", err);
      await new Promise((resolve) => window.setTimeout(resolve, retryDelay));
      retryDelay = Math.min(retryDelay * 2, 15000);
    }
  }
}

function ensureStream() {
  if (streamPromise || listeners.size === 0 || typeof window === "undefined") return;
  controller = new AbortController();
  streamPromise = runStreamLoop(controller.signal).finally(() => {
    streamPromise = null;
    controller = null;
    if (listeners.size > 0) {
      ensureStream();
    }
  });
}

export function subscribeToChanges(listener: ChangeListener): () => void {
  listeners.add(listener);
  ensureStream();

  return () => {
    listeners.delete(listener);
    if (listeners.size === 0 && controller) {
      controller.abort();
    }
  };
}
