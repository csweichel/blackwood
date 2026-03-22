import { useEffect, useState, useCallback, useRef } from "react";
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
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
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

  function handleContentChange(e: React.ChangeEvent<HTMLTextAreaElement>) {
    const text = e.target.value;
    setContent(text);
    setSaveStatus("idle");

    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      doSave(text);
    }, 1000);
  }

  // After an entry is created via EntryForm, reload to pick up appended content
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
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold text-gray-900">
          {formatDateHeading(date)}
        </h2>
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
      </div>

      <textarea
        value={content}
        onChange={handleContentChange}
        placeholder="Start writing..."
        className="w-full min-h-[300px] border border-gray-200 rounded-lg p-4 text-sm font-mono leading-relaxed resize-y focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent bg-white"
      />

      <EntryForm date={date} onCreated={handleEntryCreated} />
    </div>
  );
}
