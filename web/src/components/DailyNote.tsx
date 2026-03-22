import { useEffect, useState, useCallback, useRef } from "react";
import Markdown from "react-markdown";
import rehypeRaw from "rehype-raw";
import { getDailyNote, updateDailyNoteContent } from "../api/client";
import EntryForm from "./EntryForm";

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

  function handleEditChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const text = e.target.value;
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
        <div className="text-gray-400 text-sm">Loading...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 rounded-lg p-4 text-red-700 text-sm">
        {error}
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-semibold text-gray-900">
          {formatDateHeading(date)}
        </h2>
        <div className="flex items-center gap-3">
          <span
            className={`text-xs transition-opacity ${
              saveStatus === "idle" ? "opacity-0" : "opacity-100"
            } ${
              saveStatus === "saving"
                ? "text-gray-400"
                : saveStatus === "saved"
                ? "text-green-500"
                : saveStatus === "error"
                ? "text-red-500"
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
              className="px-3 py-1.5 text-xs font-medium text-gray-600 bg-gray-100 rounded-md hover:bg-gray-200 transition-colors"
            >
              Edit
            </button>
          ) : (
            <div className="flex items-center gap-2">
              <button
                onClick={handleCancel}
                className="px-3 py-1.5 text-xs font-medium text-gray-600 bg-gray-100 rounded-md hover:bg-gray-200 transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleSave}
                className="px-3 py-1.5 text-xs font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700 transition-colors"
              >
                Done
              </button>
            </div>
          )}
        </div>
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto mb-4">
        {editing ? (
          <textarea
            value={editContent}
            onChange={handleEditChange}
            placeholder="Start writing..."
            className="w-full h-full min-h-[300px] border border-gray-200 rounded-lg p-4 text-sm font-mono leading-relaxed resize-y focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent bg-white"
            autoFocus
          />
        ) : content.trim() ? (
          <div
            className="prose prose-sm max-w-none prose-headings:text-gray-900 prose-p:text-gray-700 prose-hr:border-gray-200 prose-strong:text-gray-900 cursor-text rounded-lg p-4 bg-white border border-gray-100 min-h-[300px]"
            onClick={startEditing}
          >
            <Markdown rehypePlugins={[rehypeRaw]}>{content}</Markdown>
          </div>
        ) : (
          <div
            className="rounded-lg p-4 bg-white border border-gray-100 min-h-[300px] flex items-center justify-center cursor-text"
            onClick={startEditing}
          >
            <p className="text-gray-400 text-sm">No entries yet. Click to start writing, or add an entry below.</p>
          </div>
        )}
      </div>

      <EntryForm date={date} onCreated={handleEntryCreated} />
    </div>
  );
}
