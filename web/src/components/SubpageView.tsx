import { useEffect, useState, useCallback, useRef, useMemo } from "react";
import { Link } from "react-router-dom";
import {
  getSubpage,
  RPCError,
  updateSubpageContent,
  listSubpages,
} from "../api/client";
import { subscribeToChanges } from "../lib/changeEvents";
import { stableSortedArray } from "../lib/stableArray";
import { mergeContent } from "../lib/mergeContent";
import NoteEditor from "./NoteEditor";
import ConflictBanner from "./ConflictBanner";

interface SubpageViewProps {
  date: string;
  name: string;
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

export default function SubpageView({ date, name }: SubpageViewProps) {
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [subpageNames, setSubpageNames] = useState<string[]>([]);
  const existingSubpages = useMemo(() => new Set(subpageNames), [subpageNames]);
  const cancelPendingSaveRef = useRef<(() => void) | null>(null);
  const contentRef = useRef("");
  const revisionRef = useRef("");
  const saveInFlightRef = useRef(false);
  const deferredChangeRevisionRef = useRef<string | null>(null);

  const updateContent = useCallback((nextContent: string) => {
    contentRef.current = nextContent;
    setContent(nextContent);
  }, []);

  const updateRevision = useCallback((nextRevision: string) => {
    revisionRef.current = nextRevision;
  }, []);

  const baseContentRef = useRef("");
  const [conflicts, setConflicts] = useState<string[]>([]);
  const serverContentRef = useRef("");
  const [showOverflowMenu, setShowOverflowMenu] = useState(false);
  const [copied, setCopied] = useState(false);
  const overflowRef = useRef<HTMLDivElement>(null);

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

  const load = useCallback(async () => {
    cancelPendingSaveRef.current?.();
    setLoading(true);
    setError(null);
    setConflicts([]);
    try {
      const [data, subpagesResp] = await Promise.all([
        getSubpage(date, name),
        listSubpages(date).catch(() => ({ names: [] as string[] })),
      ]);
      const serverContent = data.content ?? "";
      updateContent(serverContent);
      updateRevision(data.revision ?? "");
      baseContentRef.current = serverContent;
      const names = subpagesResp.names ?? [];
      setSubpageNames((prev) => stableSortedArray(prev, names));
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load";
      if (
        msg.includes("not_found") ||
        msg.includes("not found") ||
        msg.includes("404")
      ) {
        try {
          const created = await updateSubpageContent(date, name, "", "");
          updateContent("");
          updateRevision(created.revision ?? "");
          baseContentRef.current = "";
        } catch (createErr) {
          setError(
            createErr instanceof Error
              ? createErr.message
              : "Failed to create subpage"
          );
        }
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }, [date, name, updateContent, updateRevision]);

  const mergeLoad = useCallback(async () => {
    try {
      const [data, subpagesResp] = await Promise.all([
        getSubpage(date, name),
        listSubpages(date).catch(() => ({ names: [] as string[] })),
      ]);
      const remoteContent = data.content ?? "";
      const remoteRevision = data.revision ?? "";
      const names = subpagesResp.names ?? [];
      setSubpageNames((prev) => stableSortedArray(prev, names));

      setContent((localContent) => {
        const base = baseContentRef.current;
        const result = mergeContent(base, localContent, remoteContent);

        if (result.ok) {
          cancelPendingSaveRef.current?.();
          baseContentRef.current = remoteContent;
          updateRevision(remoteRevision);
          setConflicts([]);
          const mergedContent = result.merged ?? localContent;
          contentRef.current = mergedContent;
          return mergedContent;
        }

        serverContentRef.current = remoteContent;
        setConflicts(result.conflicts);
        contentRef.current = localContent;
        return localContent;
      });
    } catch (err) {
      console.warn("Merge load failed, falling back to full load", err);
      void load();
    }
  }, [date, name, load, updateRevision]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    return subscribeToChanges((event) => {
      if (event.date !== date || event.subpageName !== name) return;
      if (event.kind === "CHANGE_EVENT_KIND_SUBPAGE_UPDATED" && event.revision !== revisionRef.current) {
        if (saveInFlightRef.current) {
          deferredChangeRevisionRef.current = event.revision;
          return;
        }
        void mergeLoad();
      }
    });
  }, [date, mergeLoad, name]);

  const handleConflictKeepLocal = useCallback(() => {
    setConflicts([]);
    setContent((c) => {
      contentRef.current = c;
      baseContentRef.current = c;
      return c;
    });
  }, []);

  const handleConflictUseServer = useCallback(() => {
    cancelPendingSaveRef.current?.();
    const server = serverContentRef.current;
    updateContent(server);
    baseContentRef.current = server;
    setConflicts([]);
  }, [updateContent]);

  const handleSave = useCallback(
    async (text: string) => {
      const baseRevision = revisionRef.current;
      let handledPrecondition = false;
      saveInFlightRef.current = true;
      try {
        const updated = await updateSubpageContent(date, name, text, baseRevision);
        const savedContent = updated.content ?? text;
        updateRevision(updated.revision ?? baseRevision);
        baseContentRef.current = savedContent;
        if (contentRef.current === text) {
          updateContent(savedContent);
        }
        setError(null);
      } catch (err) {
        if (err instanceof RPCError && err.code === "failed_precondition") {
          handledPrecondition = true;
          deferredChangeRevisionRef.current = null;
          setError("This subpage changed on another client. I kept your edits and merged in the latest version.");
          await mergeLoad();
        }
        throw err;
      } finally {
        saveInFlightRef.current = false;
        const deferredRevision = deferredChangeRevisionRef.current;
        deferredChangeRevisionRef.current = null;
        if (!handledPrecondition && deferredRevision && deferredRevision !== revisionRef.current) {
          void mergeLoad();
        }
      }
    },
    [date, mergeLoad, name, updateContent, updateRevision]
  );

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

  return (
    <div className="flex flex-col h-full">
      <nav className="flex items-center gap-1.5 text-sm text-muted-foreground mb-3">
        <Link
          to={`/day/${date}`}
          className="hover:text-foreground transition-colors"
        >
          {formatDateHeading(date)}
        </Link>
        <span>›</span>
        <span className="text-foreground font-medium">{name}</span>
      </nav>

      <ConflictBanner
        conflicts={conflicts}
        onKeepLocal={handleConflictKeepLocal}
        onUseServer={handleConflictUseServer}
      />

      <NoteEditor
        title={<h2 className="text-lg md:text-xl font-semibold text-foreground truncate">{name}</h2>}
        content={content}
        onContentChange={updateContent}
        onSave={handleSave}
        onEntryCreated={load}
        cancelPendingSaveRef={cancelPendingSaveRef}
        date={date}
        existingSubpages={existingSubpages}
        emptyMessage="No content yet. Click to start writing."
        showAttach={false}
        toolbarExtra={
          content.trim() ? (
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
                </div>
              )}
            </div>
          ) : undefined
        }
      />
    </div>
  );
}
