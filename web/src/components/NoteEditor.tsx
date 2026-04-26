import { useState, useCallback, useRef, useEffect } from "react";
import BlockNoteEditor from "./BlockNoteEditor";
import AudioRecorder from "./AudioRecorder";
import PhotoCapture from "./PhotoCapture";

export type SaveStatus = "idle" | "saving" | "saved" | "error";

interface NoteEditorProps {
  content: string;
  onContentChange: (content: string) => void;
  onSave: (content: string) => Promise<void>;
  /** Called after an attachment (voice/photo/clip) is created. */
  onEntryCreated?: () => void;
  date: string;
  existingSubpages: Set<string>;
  emptyMessage?: string;
  /** Extra toolbar buttons rendered before the attach button. */
  toolbarExtra?: React.ReactNode;
  /** Extra items rendered inside the attach dropdown (e.g. location). */
  attachMenuExtra?: React.ReactNode;
  afterContent?: React.ReactNode;
  emptyTemplate?: string;
  title?: React.ReactNode;
  /** Show the attach button and panels (voice/photo/clip). Defaults to true. */
  showAttach?: boolean;
  /** Ref that receives a function to cancel any pending auto-save debounce.
   *  Call before reloading content from the server to prevent a stale save
   *  from overwriting the freshly loaded data. */
  cancelPendingSaveRef?: React.MutableRefObject<(() => void) | null>;
}

function SaveStatusIndicator({ status }: { status: SaveStatus }) {
  return (
    <span
      className={`text-xs transition-opacity ${
        status === "idle" ? "opacity-0" : "opacity-100"
      } ${
        status === "saving"
          ? "text-muted-foreground"
          : status === "saved"
            ? "text-accent"
            : status === "error"
              ? "text-destructive"
              : ""
      }`}
    >
      {status === "saving"
        ? "Saving..."
        : status === "saved"
          ? "Saved"
          : status === "error"
            ? "Save failed"
            : ""}
    </span>
  );
}

export default function NoteEditor({
  content,
  onContentChange,
  onSave,
  onEntryCreated,
  date,
  existingSubpages,
  emptyMessage = "No content yet. Click to start writing.",
  toolbarExtra,
  attachMenuExtra,
  afterContent,
  emptyTemplate,
  title,
  showAttach = true,
  cancelPendingSaveRef,
}: NoteEditorProps) {
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const latestMarkdownRef = useRef(content);
  const savingRef = useRef(false);
  const queuedSaveRef = useRef<string | null>(null);

  // Attach state
  const [showAttachMenu, setShowAttachMenu] = useState(false);
  const [showRecorder, setShowRecorder] = useState(false);
  const [showCamera, setShowCamera] = useState(false);
  const [showClipForm, setShowClipForm] = useState(false);
  const [clipUrl, setClipUrl] = useState("");
  const [clipLoading, setClipLoading] = useState(false);
  const attachRef = useRef<HTMLDivElement>(null);

  // Expose cancel function so parent can cancel pending saves before reloading
  useEffect(() => {
    if (cancelPendingSaveRef) {
      cancelPendingSaveRef.current = () => {
        if (saveTimerRef.current) {
          clearTimeout(saveTimerRef.current);
          saveTimerRef.current = null;
        }
        queuedSaveRef.current = null;
      };
    }
    return () => {
      if (cancelPendingSaveRef) cancelPendingSaveRef.current = null;
    };
  }, [cancelPendingSaveRef]);

  useEffect(() => {
    latestMarkdownRef.current = content;
  }, [content]);

  // Close attach menu on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (attachRef.current && !attachRef.current.contains(e.target as Node)) {
        setShowAttachMenu(false);
      }
    }
    if (showAttachMenu) {
      document.addEventListener("mousedown", handleClickOutside);
      return () => document.removeEventListener("mousedown", handleClickOutside);
    }
  }, [showAttachMenu]);

  const doSave = useCallback(
    async (text: string) => {
      if (savingRef.current) {
        queuedSaveRef.current = text;
        return;
      }

      savingRef.current = true;
      let nextText = text;

      try {
        while (true) {
          queuedSaveRef.current = null;
          setSaveStatus("saving");

          try {
            await onSave(nextText);
          } catch {
            setSaveStatus("error");
            return;
          }

          const latestText = latestMarkdownRef.current;
          if (latestText === nextText) {
            if (saveTimerRef.current) {
              clearTimeout(saveTimerRef.current);
              saveTimerRef.current = null;
            }
            setSaveStatus("saved");
            setTimeout(() => setSaveStatus((s) => (s === "saved" ? "idle" : s)), 2000);
            return;
          }

          if (saveTimerRef.current) {
            clearTimeout(saveTimerRef.current);
            saveTimerRef.current = null;
          }
          nextText = latestText;
        }
      } finally {
        savingRef.current = false;
      }
    },
    [onSave]
  );

  function handleEditorChange(markdown: string) {
    latestMarkdownRef.current = markdown;
    onContentChange(markdown);
    setSaveStatus("idle");
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => doSave(markdown), 1000);
  }

  function handleAttachmentCreated() {
    setShowRecorder(false);
    setShowCamera(false);
    onEntryCreated?.();
  }

  const effectiveContent = content.trim() ? content : (emptyTemplate ?? "");

  return (
    <>
      {/* Toolbar */}
      <div className="flex items-center gap-3 mb-3 md:mb-4">
        {title ? <div className="flex-1 min-w-0">{title}</div> : <div className="flex-1" />}
        <SaveStatusIndicator status={saveStatus} />
        {toolbarExtra}
        {showAttach && (
          <div className="relative" ref={attachRef}>
            <button
              onClick={() => setShowAttachMenu((v) => !v)}
              className={`p-1.5 rounded-md transition-colors ${showAttachMenu ? "text-accent bg-muted" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
              title="Attach"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48"/></svg>
            </button>
            {showAttachMenu && (
              <div className="absolute right-0 top-full mt-1 bg-card border border-border rounded-lg shadow-lg z-50 min-w-[160px]">
                <button
                  onClick={() => { setShowAttachMenu(false); setShowCamera(false); setShowRecorder(true); }}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" x2="12" y1="19" y2="22"/></svg>
                  Voice memo
                </button>
                <button
                  onClick={() => { setShowAttachMenu(false); setShowRecorder(false); setShowCamera(true); }}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z"/><circle cx="12" cy="13" r="3"/></svg>
                  Photo
                </button>
                {attachMenuExtra}
                <button
                  onClick={() => { setShowAttachMenu(false); setShowClipForm(true); }}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
                  Clip page
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Attachment panels */}
      {showRecorder && (
        <div className="mb-4">
          <AudioRecorder
            date={date}
            onCreated={handleAttachmentCreated}
            onClose={() => setShowRecorder(false)}
            autoStart
          />
        </div>
      )}
      {showCamera && (
        <div className="mb-4">
          <PhotoCapture
            date={date}
            onCreated={handleAttachmentCreated}
            onClose={() => setShowCamera(false)}
          />
        </div>
      )}
      {showClipForm && (
        <div className="mb-4 bg-card border border-border rounded-lg p-3">
          <form
            className="flex items-center gap-2"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!clipUrl.trim()) return;
              setClipLoading(true);
              try {
                const resp = await fetch("/api/clip", {
                  method: "POST",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify({ url: clipUrl.trim() }),
                });
                if (!resp.ok) throw new Error("Clip failed");
                setClipUrl("");
                setShowClipForm(false);
                onEntryCreated?.();
              } catch (err) {
                console.error("Clip page failed:", err);
              } finally {
                setClipLoading(false);
              }
            }}
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-muted-foreground shrink-0"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
            <input
              type="url"
              value={clipUrl}
              onChange={(e) => setClipUrl(e.target.value)}
              placeholder="Paste URL to clip..."
              className="flex-1 bg-transparent border border-border rounded-md px-2 py-1 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-accent"
              autoFocus
              disabled={clipLoading}
            />
            <button
              type="submit"
              disabled={clipLoading || !clipUrl.trim()}
              className="px-3 py-1 text-xs font-medium text-primary-foreground bg-primary rounded-md hover:opacity-90 transition-colors disabled:opacity-50"
            >
              {clipLoading ? "Clipping..." : "Clip"}
            </button>
            <button
              type="button"
              onClick={() => { setShowClipForm(false); setClipUrl(""); }}
              className="p-1 text-muted-foreground hover:text-foreground transition-colors"
              title="Cancel"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" x2="6" y1="6" y2="18"/><line x1="6" x2="18" y1="6" y2="18"/></svg>
            </button>
          </form>
        </div>
      )}

      {/* Content area */}
      <div className="flex-1 min-h-0 overflow-y-auto mb-4">
        <BlockNoteEditor
          content={effectiveContent}
          onChange={handleEditorChange}
          date={date}
          existingSubpages={existingSubpages}
          placeholder={emptyMessage}
        />
      </div>

      {afterContent}
    </>
  );
}
