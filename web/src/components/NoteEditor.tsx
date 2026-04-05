import { useState, useCallback, useRef, type RefObject } from "react";
import { useNavigate } from "react-router-dom";
import Markdown from "react-markdown";
import rehypeRaw from "rehype-raw";
import remarkGfm from "remark-gfm";
import MarkdownEditor from "./MarkdownEditor";
import {
  rehypeYoutubeEmbed,
  rehypeCollapsible,
  rehypeSectionLabels,
  rehypeCheckboxIndex,
  rehypeAttachmentUrls,
  remarkWikilinks,
  SECTION_ICONS_JSX,
} from "./DailyNote";

export type SaveStatus = "idle" | "saving" | "saved" | "error";

interface NoteEditorProps {
  /** Markdown content to display/edit. */
  content: string;
  /** Called when content changes (debounced saves are handled internally). */
  onContentChange: (content: string) => void;
  /** Persist content to the backend. */
  onSave: (content: string) => Promise<void>;
  /** Date for resolving attachment URLs and wikilinks. */
  date: string;
  /** Set of subpage names that exist for this date. */
  existingSubpages: Set<string>;
  /** Placeholder for the editor when content is empty. */
  emptyMessage?: string;
  /** If true, start in edit mode immediately. */
  startInEditMode?: boolean;
  /** Extra toolbar elements rendered before the edit/cancel/done buttons. */
  toolbarExtra?: React.ReactNode;
  /** Extra content rendered between the toolbar and the editor/viewer. */
  beforeContent?: React.ReactNode;
  /** Extra content rendered after the editor/viewer. */
  afterContent?: React.ReactNode;
  /** Optional ref forwarded to the prose container div for DOM operations. */
  proseRef?: RefObject<HTMLDivElement | null>;
  /** Template content used when starting to edit an empty note. */
  emptyTemplate?: string;
  /** Title rendered on the left side of the toolbar row. */
  title?: React.ReactNode;
}

function SaveStatusIndicator({ status, editing, className }: { status: SaveStatus; editing: boolean; className?: string }) {
  return (
    <span
      className={`text-xs transition-opacity ${editing ? "hidden md:inline" : ""} ${
        status === "idle" ? "opacity-0" : "opacity-100"
      } ${
        status === "saving"
          ? "text-muted-foreground"
          : status === "saved"
            ? "text-accent"
            : status === "error"
              ? "text-destructive"
              : ""
      } ${className ?? ""}`}
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

/**
 * Shared markdown view/edit component used by DailyNoteView and SubpageView.
 * Handles the edit toggle, CodeMirror editor, markdown rendering with all
 * plugins, checkbox toggling, auto-save with debounce, and the mobile
 * editing bar.
 */
export default function NoteEditor({
  content,
  onContentChange,
  onSave,
  date,
  existingSubpages,
  emptyMessage = "No content yet. Click to start writing.",
  startInEditMode = false,
  toolbarExtra,
  beforeContent,
  afterContent,
  proseRef: externalProseRef,
  emptyTemplate,
  title,
}: NoteEditorProps) {
  const navigate = useNavigate();
  const [editContent, setEditContent] = useState("");
  const [editing, setEditing] = useState(startInEditMode);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const internalProseRef = useRef<HTMLDivElement>(null);
  const proseRef = externalProseRef ?? internalProseRef;

  const doSave = useCallback(
    async (text: string) => {
      setSaveStatus("saving");
      try {
        await onSave(text);
        setSaveStatus("saved");
        setTimeout(() => setSaveStatus((s) => (s === "saved" ? "idle" : s)), 2000);
      } catch {
        setSaveStatus("error");
      }
    },
    [onSave]
  );

  function handleEditChange(text: string) {
    setEditContent(text);
    setSaveStatus("idle");
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      doSave(text);
    }, 1000);
  }

  function toggleCheckbox(index: number) {
    const checkboxRe = /- \[([ xX])\]/g;
    let count = 0;
    const newContent = content.replace(checkboxRe, (match, mark) => {
      if (count++ === index) {
        return mark.trim() ? "- [ ]" : "- [x]";
      }
      return match;
    });
    onContentChange(newContent);
    doSave(newContent);
  }

  function startEditing() {
    setEditContent(content.trim() ? content : (emptyTemplate ?? content));
    setEditing(true);
  }

  function handleSave() {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    onContentChange(editContent);
    setEditing(false);
    doSave(editContent);
  }

  function handleCancel() {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    setEditing(false);
    setSaveStatus("idle");
  }

  return (
    <>
      {/* Toolbar row: title + save status + extra buttons + edit/cancel/done */}
      <div className="flex items-center gap-3 mb-3 md:mb-4">
        {title && <div className="flex-1 min-w-0">{title}</div>}
        {!title && <div className="flex-1" />}
        <SaveStatusIndicator status={saveStatus} editing={editing} />
        {toolbarExtra}
        {!editing ? (
          <button
            onClick={startEditing}
            className="px-3 py-1.5 text-xs font-medium text-muted-foreground bg-muted rounded-md hover:bg-border transition-colors"
          >
            Edit
          </button>
        ) : (
          <div className="hidden md:flex items-center gap-2">
            <button
              onClick={handleCancel}
              className="px-3 py-1.5 text-xs font-medium text-muted-foreground bg-muted rounded-md hover:bg-border transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              className="px-3 py-1.5 text-xs font-medium text-primary-foreground bg-primary rounded-md hover:opacity-90 transition-colors"
            >
              Done
            </button>
          </div>
        )}
      </div>

      {beforeContent}

      {/* Content area */}
      <div className="flex-1 min-h-0 overflow-y-auto mb-4">
        {editing ? (
          <MarkdownEditor
            value={editContent}
            onChange={handleEditChange}
            onSubmit={handleSave}
            placeholder="Start writing..."
            autoFocus
          />
        ) : content.trim() ? (
          <div
            ref={proseRef}
            className="prose prose-sm max-w-none note-prose note-container"
            onClick={(e) => {
              const target = e.target as HTMLElement;
              if (target.tagName === "INPUT" && (target as HTMLInputElement).type === "checkbox") {
                e.preventDefault();
                const idx = target.getAttribute("data-checkbox-index");
                if (idx != null) toggleCheckbox(parseInt(idx, 10));
                return;
              }
              const wikilinkEl = target.closest("a[data-subpage]") as HTMLAnchorElement | null;
              if (wikilinkEl) {
                e.preventDefault();
                const href = wikilinkEl.getAttribute("href");
                if (href) navigate(href);
                return;
              }
              if (target.closest("summary, details, a, audio, button, video, iframe, input")) return;
              startEditing();
            }}
          >
            <Markdown
              remarkPlugins={[remarkGfm, remarkWikilinks(date, existingSubpages)]}
              rehypePlugins={[rehypeRaw, rehypeYoutubeEmbed, rehypeCollapsible, rehypeSectionLabels, rehypeCheckboxIndex, rehypeAttachmentUrls(date)]}
              components={{
                h1: ({ className, children, ...props }) => {
                  const cls = typeof className === "string" ? className : "";
                  if (cls.includes("note-section-label")) {
                    const text = typeof children === "string" ? children : String(children ?? "");
                    const icon = SECTION_ICONS_JSX[text.trim()];
                    return (
                      <h1 className={cls} {...props}>
                        {icon && <span className="note-section-icon">{icon}</span>}
                        {children}
                      </h1>
                    );
                  }
                  return <h1 className={className} {...props}>{children}</h1>;
                },
                input: ({ type, checked, disabled: _disabled, ...props }) => {
                  if (type === "checkbox") {
                    return <input type="checkbox" checked={checked} readOnly {...props} />;
                  }
                  return <input type={type} checked={checked} {...props} />;
                },
              }}
            >
              {content}
            </Markdown>
          </div>
        ) : (
          <div className="note-empty" onClick={startEditing}>
            <p className="text-muted-foreground text-sm">{emptyMessage}</p>
          </div>
        )}
      </div>

      {afterContent}

      {/* Mobile bottom bar when editing */}
      {editing && (
        <div className="md:hidden fixed bottom-0 left-0 right-0 bg-card border-t border-border px-4 py-3 flex items-center justify-between z-40">
          <SaveStatusIndicator status={saveStatus} editing={false} />
          <div className="flex items-center gap-2">
            <button
              onClick={handleCancel}
              className="px-4 py-2 text-sm font-medium text-muted-foreground bg-muted rounded-md hover:bg-border transition-colors"
            >
              Cancel
            </button>
            <button
              onClick={handleSave}
              className="px-4 py-2 text-sm font-medium text-primary-foreground bg-primary rounded-md hover:opacity-90 transition-colors"
            >
              Done
            </button>
          </div>
        </div>
      )}
    </>
  );
}
