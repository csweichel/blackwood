import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { getDailyNote, listSubpages, RPCError, updateDailyNoteContent } from "../api/client";
import { useGeolocation } from "../hooks/useGeolocation";
import { subscribeToChanges } from "../lib/changeEvents";
import { stableSortedArray } from "../lib/stableArray";
import { mergeContent } from "../lib/mergeContent";

import NoteEditor from "./NoteEditor";
import ConflictBanner from "./ConflictBanner";

interface DailyNoteViewProps {
  date: string;
}

function formatDateHeading(dateStr: string): string {
  const d = new Date(dateStr + "T00:00:00");
  return d.toLocaleDateString(undefined, {
    weekday: "long",
    year: "numeric",
    month: "long",
    day: "numeric",
  });
}

export default function DailyNoteView({ date }: DailyNoteViewProps) {
  const [content, setContent] = useState("");
  const [revision, setRevision] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [subpageNames, setSubpageNames] = useState<string[]>([]);
  const existingSubpages = useMemo(() => new Set(subpageNames), [subpageNames]);
  const cancelPendingSaveRef = useRef<(() => void) | null>(null);

  // Base content: the last version received from the server (after load or save).
  // Used as the common ancestor for three-way merge.
  const baseContentRef = useRef("");

  // Conflict state
  const [conflicts, setConflicts] = useState<string[]>([]);
  const serverContentRef = useRef("");

  // Full load — used for initial load and explicit reloads (e.g. after failed_precondition).
  const load = useCallback(async () => {
    cancelPendingSaveRef.current?.();
    setLoading(true);
    setError(null);
    setConflicts([]);
    try {
      const [data, subpagesResp] = await Promise.all([
        getDailyNote({ date }),
        listSubpages(date).catch(() => ({ names: [] as string[] })),
      ]);
      const serverContent = data.content ?? "";
      setContent(serverContent);
      setRevision(data.revision ?? "");
      baseContentRef.current = serverContent;
      const names = subpagesResp.names ?? [];
      setSubpageNames((prev) => stableSortedArray(prev, names));
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load";
      if (msg.includes("404") || msg.includes("not found") || msg.includes("not_found")) {
        setContent("");
        setRevision("");
        baseContentRef.current = "";
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }, [date]);

  // Merge load — used when a change event arrives while the editor may have unsaved edits.
  // Fetches the new server content and three-way merges with the current editor state.
  const mergeLoad = useCallback(async () => {
    try {
      const [data, subpagesResp] = await Promise.all([
        getDailyNote({ date }),
        listSubpages(date).catch(() => ({ names: [] as string[] })),
      ]);
      const remoteContent = data.content ?? "";
      const remoteRevision = data.revision ?? "";
      const names = subpagesResp.names ?? [];
      setSubpageNames((prev) => stableSortedArray(prev, names));

      // Read the current editor content via the ref-backed state.
      // We need the latest value, so we use a functional update to read it.
      setContent((localContent) => {
        const base = baseContentRef.current;
        const result = mergeContent(base, localContent, remoteContent);

        if (result.ok) {
          // Merge succeeded — cancel pending save (merged content will be saved
          // on next keystroke or we can trigger a save explicitly).
          cancelPendingSaveRef.current?.();
          baseContentRef.current = remoteContent;
          setRevision(remoteRevision);
          setConflicts([]);
          return result.merged ?? localContent;
        }

        // Conflict — stash server content, show banner, keep local in editor
        serverContentRef.current = remoteContent;
        setConflicts(result.conflicts);
        // Don't update revision yet — user hasn't resolved the conflict
        return localContent;
      });
    } catch (err) {
      console.warn("Merge load failed, falling back to full load", err);
      void load();
    }
  }, [date, load]);

  useEffect(() => {
    load();
  }, [load]);

  // Use a ref for revision so the change-event listener always sees the latest
  // value without needing to re-subscribe on every save.
  const revisionRef = useRef(revision);
  useEffect(() => {
    revisionRef.current = revision;
  }, [revision]);

  useEffect(() => {
    return subscribeToChanges((event) => {
      if (event.date !== date) return;
      if (event.kind === "CHANGE_EVENT_KIND_DAILY_NOTE_UPDATED" && event.revision !== revisionRef.current) {
        void mergeLoad();
      }
    });
  }, [date, mergeLoad]);

  const handleConflictKeepLocal = useCallback(() => {
    // User chose their version — save it to establish as the new base
    setConflicts([]);
    // The current content is already the local version; trigger a save
    setContent((c) => {
      baseContentRef.current = c;
      return c;
    });
  }, []);

  const handleConflictUseServer = useCallback(() => {
    // User chose server version — replace editor content
    cancelPendingSaveRef.current?.();
    const server = serverContentRef.current;
    setContent(server);
    baseContentRef.current = server;
    setConflicts([]);
  }, []);

  const [showOverflowMenu, setShowOverflowMenu] = useState(false);
  const overflowRef = useRef<HTMLDivElement>(null);
  const [pdfLoading, setPdfLoading] = useState(false);
  const [summarizing, setSummarizing] = useState(false);
  const [copied, setCopied] = useState(false);
  const { position: geoPosition, loading: geoLoading, error: geoError, requestLocation } = useGeolocation();
  const [locationTagged, setLocationTagged] = useState(false);

  // Close overflow menu on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (overflowRef.current && !overflowRef.current.contains(e.target as Node)) {
        setShowOverflowMenu(false);
      }
    }
    if (showOverflowMenu) {
      document.addEventListener("mousedown", handleClickOutside);
      return () => document.removeEventListener("mousedown", handleClickOutside);
    }
  }, [showOverflowMenu]);

  // Reset UI state when date changes
  useEffect(() => {
    setShowOverflowMenu(false);
  }, [date]);

  const handleSave = useCallback(
    async (text: string) => {
      try {
        const updated = await updateDailyNoteContent(date, text, revision);
        const savedContent = updated.content ?? text;
        setContent(savedContent);
        setRevision(updated.revision ?? revision);
        baseContentRef.current = savedContent;
        setError(null);
      } catch (err) {
        if (err instanceof RPCError && err.code === "failed_precondition") {
          setError("This note changed on another client. The latest version has been reloaded.");
          await load();
        }
        throw err;
      }
    },
    [date, load, revision]
  );

  const downloadPdf = useCallback(async () => {
    setPdfLoading(true);
    try {
      const resp = await fetch(`/api/daily-notes/${date}/pdf`);
      if (!resp.ok) throw new Error("PDF generation failed");
      const blob = await resp.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${date}.pdf`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      console.error("PDF download failed:", err);
    } finally {
      setPdfLoading(false);
    }
  }, [date]);

  const generateSummary = useCallback(async () => {
    setSummarizing(true);
    try {
      const resp = await fetch(`/api/daily-notes/${date}/summarize`, {
        method: "POST",
      });
      if (!resp.ok) throw new Error("Summarize failed");
      await load();
    } catch (err) {
      console.error("Summary generation failed:", err);
    } finally {
      setSummarizing(false);
    }
  }, [date, load]);

  // Tag the note with the current location when geolocation resolves.
  useEffect(() => {
    if (!geoPosition || locationTagged) return;
    setLocationTagged(true);
    const { latitude, longitude, address } = geoPosition;
    const ts = new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    const mapUrl = `https://www.openstreetmap.org/?mlat=${latitude}&mlon=${longitude}#map=15/${latitude}/${longitude}`;
    const locationLabel = address || `${latitude.toFixed(4)}, ${longitude.toFixed(4)}`;
    const snippet = `\n\n---\n*${ts} — 📍 [${locationLabel}](${mapUrl})*\n`;
    const newContent = content + snippet;
    setContent(newContent);
    handleSave(newContent);
  }, [geoPosition, locationTagged, content, handleSave]);

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-muted-foreground text-sm">Loading...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-muted border border-destructive/30 rounded-lg p-4 text-destructive text-sm">
        {error}
      </div>
    );
  }

  const toolbarExtra = (
    <>
      {content.trim() && (
        <div className="relative" ref={overflowRef}>
          <button
            onClick={() => setShowOverflowMenu((v) => !v)}
            className={`p-1.5 rounded-md transition-colors ${showOverflowMenu ? "text-accent bg-muted" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
            title="More actions"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="1"/><circle cx="12" cy="5" r="1"/><circle cx="12" cy="19" r="1"/></svg>
          </button>
          {showOverflowMenu && (
            <div className="absolute right-0 top-full mt-1 bg-card border border-border rounded-lg shadow-lg z-50 min-w-[180px]">
              <button
                onClick={async () => {
                  setShowOverflowMenu(false);
                  await navigator.clipboard.writeText(content);
                  setCopied(true);
                  setTimeout(() => setCopied(false), 2000);
                }}
                className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>
                {copied ? "Copied!" : "Copy as Markdown"}
              </button>
              <button
                onClick={() => { setShowOverflowMenu(false); generateSummary(); }}
                disabled={summarizing}
                className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left disabled:opacity-50"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3l1.912 5.813a2 2 0 0 0 1.275 1.275L21 12l-5.813 1.912a2 2 0 0 0-1.275 1.275L12 21l-1.912-5.813a2 2 0 0 0-1.275-1.275L3 12l5.813-1.912a2 2 0 0 0 1.275-1.275L12 3z"/></svg>
                {summarizing ? "Summarising…" : "Generate summary"}
              </button>
              <button
                onClick={() => { setShowOverflowMenu(false); downloadPdf(); }}
                disabled={pdfLoading}
                className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left disabled:opacity-50"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="12" x2="12" y1="18" y2="12"/><polyline points="9 15 12 18 15 15"/></svg>
                {pdfLoading ? "Exporting…" : "Export as PDF"}
              </button>
            </div>
          )}
        </div>
      )}
    </>
  );

  const attachMenuExtra = (
    <button
      onClick={() => requestLocation()}
      disabled={geoLoading || locationTagged}
      className={`flex items-center gap-2 px-3 py-2 text-sm w-full text-left ${locationTagged ? "text-accent" : geoLoading ? "text-muted-foreground opacity-50 cursor-wait" : geoError ? "text-destructive" : "text-foreground hover:bg-muted"}`}
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/></svg>
      {locationTagged ? "Location tagged" : geoError ? geoError : "Location"}
    </button>
  );

  const afterContent = (
    <div className="hidden md:block mt-12 pt-6 border-t border-border">
      <p className="text-xs text-muted-foreground">
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+D</kbd> today
        <span className="mx-2">&middot;</span>
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+/</kbd> chat
        <span className="mx-2">&middot;</span>
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+T</kbd> insert time
        <span className="mx-2">&middot;</span>
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+Enter</kbd> done editing
        <span className="mx-2">&middot;</span>
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Esc</kbd> exit edit
      </p>
    </div>
  );

  return (
    <div className="flex flex-col h-full">
      <ConflictBanner
        conflicts={conflicts}
        onKeepLocal={handleConflictKeepLocal}
        onUseServer={handleConflictUseServer}
      />
      <NoteEditor
        title={<h2 className="text-lg md:text-xl font-semibold text-foreground truncate">{formatDateHeading(date)}</h2>}
        content={content}
        onContentChange={setContent}
        onSave={handleSave}
        onEntryCreated={load}
        cancelPendingSaveRef={cancelPendingSaveRef}
        date={date}
        existingSubpages={existingSubpages}
        emptyMessage="No entries yet. Click to start writing, or add an entry below."
        emptyTemplate={"# Summary\n\n# Notes\n\n# Links\n"}
        toolbarExtra={toolbarExtra}
        attachMenuExtra={attachMenuExtra}
        afterContent={afterContent}
      />
    </div>
  );
}
