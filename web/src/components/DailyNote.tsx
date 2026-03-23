import { useEffect, useState, useCallback, useRef } from "react";
import Markdown from "react-markdown";
import rehypeRaw from "rehype-raw";
import { visit } from "unist-util-visit";
import { getDailyNote, updateDailyNoteContent } from "../api/client";
import EntryForm from "./EntryForm";
import MarkdownEditor from "./MarkdownEditor";

/**
 * Remark plugin that converts Obsidian-style [[wikilinks]] into
 * <span class="wikilink"> elements for styled rendering.
 */
function remarkWikilinks() {
  return (tree: any) => {
    visit(tree, "text", (node: any, index: number | undefined, parent: any) => {
      if (index === undefined || !parent) return;
      const regex = /\[\[([^\]]+)\]\]/g;
      const value: string = node.value;
      if (!regex.test(value)) return;

      // Reset regex after test
      regex.lastIndex = 0;
      const children: any[] = [];
      let lastIndex = 0;
      let match: RegExpExecArray | null;

      while ((match = regex.exec(value)) !== null) {
        // Text before the match
        if (match.index > lastIndex) {
          children.push({ type: "text", value: value.slice(lastIndex, match.index) });
        }
        // The wikilink as an inline HTML node
        children.push({
          type: "html",
          value: `<span class="wikilink">${match[1]}</span>`,
        });
        lastIndex = regex.lastIndex;
      }

      // Remaining text after last match
      if (lastIndex < value.length) {
        children.push({ type: "text", value: value.slice(lastIndex) });
      }

      parent.children.splice(index, 1, ...children);
    });
  };
}

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

type SaveStatus = "idle" | "saving" | "saved" | "error";

export default function DailyNoteView({ date }: DailyNoteViewProps) {
  const [content, setContent] = useState("");
  const [editContent, setEditContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getDailyNote({ date });
      setContent(data.content ?? "");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load";
      if (msg.includes("404") || msg.includes("not found") || msg.includes("not_found")) {
        setContent("");
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }, [date]);

  useEffect(() => {
    load();
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    };
  }, [load]);

  // Reset editing state when date changes
  useEffect(() => {
    setEditing(false);
  }, [date]);


  const doSave = useCallback(
    async (text: string) => {
      setSaveStatus("saving");
      try {
        await updateDailyNoteContent(date, text);
        setSaveStatus("saved");
        setTimeout(() => setSaveStatus((s) => (s === "saved" ? "idle" : s)), 2000);
      } catch {
        setSaveStatus("error");
      }
    },
    [date]
  );

  function handleEditChange(text: string) {
    setEditContent(text);
    setSaveStatus("idle");

    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      doSave(text);
    }, 1000);
  }

  function startEditing() {
    setEditContent(content);
    setEditing(true);
  }

  function handleSave() {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    setContent(editContent);
    setEditing(false);
    doSave(editContent);
  }

  function handleCancel() {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    setEditing(false);
    setSaveStatus("idle");
  }

  async function handleEntryCreated() {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    await load();
  }

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
      <div className="flex items-center justify-between mb-3 md:mb-4">
        <h2 className="text-lg md:text-xl font-semibold text-foreground">
          {formatDateHeading(date)}
        </h2>
        <div className="flex items-center gap-3">
          <span
            className={`text-xs transition-opacity ${
              saveStatus === "idle" ? "opacity-0" : "opacity-100"
            } ${
              saveStatus === "saving"
                ? "text-muted-foreground"
                : saveStatus === "saved"
                ? "text-accent"
                : saveStatus === "error"
                ? "text-destructive"
                : ""
            }`}
          >
            {saveStatus === "saving"
              ? "Saving..."
              : saveStatus === "saved"
              ? "Saved"
              : saveStatus === "error"
              ? "Save failed"
              : ""}
          </span>
          {!editing ? (
            <button
              onClick={startEditing}
              className="px-3 py-1.5 text-xs font-medium text-muted-foreground bg-muted rounded-md hover:bg-border transition-colors"
            >
              Edit
            </button>
          ) : (
            <div className="flex items-center gap-2">
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
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto mb-4">
        {editing ? (
          <MarkdownEditor
            value={editContent}
            onChange={handleEditChange}
            placeholder="Start writing..."
            autoFocus
          />
        ) : content.trim() ? (
          <div
            className="prose prose-sm max-w-none note-prose note-container"
            onClick={startEditing}
          >
            <Markdown remarkPlugins={[remarkWikilinks]} rehypePlugins={[rehypeRaw]}>
              {content}
            </Markdown>
          </div>
        ) : (
          <div className="note-empty" onClick={startEditing}>
            <p className="text-muted-foreground text-sm">
              No entries yet. Click to start writing, or add an entry below.
            </p>
          </div>
        )}
      </div>

      <EntryForm date={date} onCreated={handleEntryCreated} />

      <div className="mt-12 pt-6 border-t border-border">
        <p className="text-xs text-muted-foreground">
          <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+T</kbd> insert time
          <span className="mx-2">&middot;</span>
          <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Esc</kbd> exit edit
        </p>
      </div>
    </div>
  );
}
