import { useEffect, useState } from "react";
import { useSyncStatus } from "../lib/syncEngine";

export default function OfflineBanner() {
  const { isOnline, pendingCount, syncing } = useSyncStatus();
  const [showSynced, setShowSynced] = useState(false);
  const [prevSyncing, setPrevSyncing] = useState(false);

  // Show "All changes synced" briefly after sync completes.
  useEffect(() => {
    if (prevSyncing && !syncing && pendingCount === 0) {
      setShowSynced(true);
      const timer = setTimeout(() => setShowSynced(false), 3000);
      return () => clearTimeout(timer);
    }
    setPrevSyncing(syncing);
  }, [syncing, pendingCount, prevSyncing]);

  // Offline banner
  if (!isOnline) {
    return (
      <div className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-amber-700 bg-amber-100 rounded-full">
        <svg
          className="w-3.5 h-3.5"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <line x1="1" y1="1" x2="23" y2="23" />
          <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55" />
          <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39" />
          <path d="M10.71 5.05A16 16 0 0 1 22.56 9" />
          <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88" />
          <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
          <line x1="12" y1="20" x2="12.01" y2="20" />
        </svg>
        <span>
          Offline
          {pendingCount > 0
            ? ` — ${pendingCount} change${pendingCount !== 1 ? "s" : ""} pending`
            : " — changes will sync when reconnected"}
        </span>
      </div>
    );
  }

  // Syncing indicator
  if (syncing) {
    return (
      <div className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-accent bg-muted rounded-full">
        <svg
          className="w-3.5 h-3.5 animate-spin text-accent"
          viewBox="0 0 24 24"
          fill="none"
        >
          <circle
            className="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeWidth="4"
          />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"
          />
        </svg>
        <span>
          Syncing {pendingCount} change{pendingCount !== 1 ? "s" : ""}...
        </span>
      </div>
    );
  }

  // Pending changes (online but not yet synced)
  if (pendingCount > 0) {
    return (
      <div className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-amber-700 bg-amber-100 rounded-full">
        <span>
          {pendingCount} unsynced change{pendingCount !== 1 ? "s" : ""}
        </span>
      </div>
    );
  }

  // Briefly show sync success
  if (showSynced) {
    return (
      <div className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-green-700 bg-green-100 rounded-full">
        <svg
          className="w-3.5 h-3.5"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <polyline points="20 6 9 17 4 12" />
        </svg>
        <span>All changes synced</span>
      </div>
    );
  }

  return null;
}
