import { useState, useEffect, useCallback, useRef } from "react";
import {
  getPendingEntries,
  getPendingContentUpdates,
  removePendingEntry,
  removePendingContentUpdate,
  getPendingCount,
} from "./offlineStore";
import type { Entry, DailyNote } from "../api/types";

const DAILY_NOTES_SERVICE = "/blackwood.v1.DailyNotesService";

// Direct RPC call that bypasses the offline-aware client wrappers.
// Used only by the sync engine to replay queued mutations.
async function rawRpc<Req, Res>(method: string, request: Req): Promise<Res> {
  const url = `${DAILY_NOTES_SERVICE}/${method}`;
  const resp = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request),
  });
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(`RPC ${method} failed (${resp.status}): ${text}`);
  }
  return resp.json();
}

// --- Sync state (module-level singleton) ---

type SyncListener = () => void;

let _isOnline = typeof navigator !== "undefined" ? navigator.onLine : true;
let _syncing = false;
let _pendingCount = 0;
const _listeners = new Set<SyncListener>();

function notify() {
  for (const fn of _listeners) fn();
}

function subscribe(fn: SyncListener): () => void {
  _listeners.add(fn);
  return () => _listeners.delete(fn);
}

// --- Online/offline detection ---

function handleOnline() {
  _isOnline = true;
  notify();
  // Auto-flush when coming back online.
  startSync();
}

function handleOffline() {
  _isOnline = false;
  notify();
}

let _eventsRegistered = false;

function ensureEvents() {
  if (_eventsRegistered || typeof window === "undefined") return;
  _eventsRegistered = true;
  window.addEventListener("online", handleOnline);
  window.addEventListener("offline", handleOffline);
  // Refresh pending count on load.
  refreshPendingCount();
}

async function refreshPendingCount() {
  _pendingCount = await getPendingCount();
  notify();
}

// --- Sync logic ---

export async function startSync(): Promise<void> {
  if (_syncing || !_isOnline) return;
  _syncing = true;
  notify();

  try {
    // 1. Flush pending content updates (last-write-wins per date).
    const contentUpdates = await getPendingContentUpdates();
    for (const update of contentUpdates) {
      try {
        await rawRpc<{ date: string; content: string }, DailyNote>(
          "UpdateDailyNoteContent",
          { date: update.date, content: update.content }
        );
        await removePendingContentUpdate(update.date);
      } catch (err) {
        console.error(`Sync: failed to update content for ${update.date}:`, err);
        // Stop syncing on network error to avoid hammering a down server.
        if (!navigator.onLine) break;
      }
    }

    // 2. Flush pending entries in order.
    const entries = await getPendingEntries();
    for (const entry of entries) {
      try {
        const req: Record<string, unknown> = {
          date: entry.date,
          type: entry.type,
          content: entry.content,
          source: entry.source,
        };
        if (entry.attachmentData) req.attachmentData = entry.attachmentData;
        if (entry.attachmentFilenames) req.attachmentFilenames = entry.attachmentFilenames;
        if (entry.attachmentContentTypes) req.attachmentContentTypes = entry.attachmentContentTypes;

        await rawRpc<Record<string, unknown>, Entry>("CreateEntry", req);
        await removePendingEntry(entry.id!);
      } catch (err) {
        console.error(`Sync: failed to create entry ${entry.id}:`, err);
        if (!navigator.onLine) break;
      }
    }
  } finally {
    _syncing = false;
    await refreshPendingCount();
  }
}

// Notify the sync engine that a new item was queued so the count updates.
export async function notifyPendingChange(): Promise<void> {
  await refreshPendingCount();
}

// --- React hook ---

export function useSyncStatus(): {
  isOnline: boolean;
  pendingCount: number;
  syncing: boolean;
} {
  const [, forceUpdate] = useState(0);
  const mountedRef = useRef(true);

  useEffect(() => {
    ensureEvents();
    mountedRef.current = true;
    const unsub = subscribe(() => {
      if (mountedRef.current) forceUpdate((n) => n + 1);
    });
    return () => {
      mountedRef.current = false;
      unsub();
    };
  }, []);

  // Auto-start sync on mount if there are pending items and we're online.
  const didAutoSync = useRef(false);
  const autoSync = useCallback(async () => {
    if (didAutoSync.current) return;
    didAutoSync.current = true;
    await refreshPendingCount();
    if (_isOnline && _pendingCount > 0) {
      startSync();
    }
  }, []);

  useEffect(() => {
    autoSync();
  }, [autoSync]);

  return {
    isOnline: _isOnline,
    pendingCount: _pendingCount,
    syncing: _syncing,
  };
}
