import { useState, useRef } from "react";
import { EntryType, EntrySource } from "../api/types";
import { createEntry } from "../api/client";
import AudioRecorder from "./AudioRecorder";
import PhotoCapture from "./PhotoCapture";

interface EntryFormProps {
  date: string;
  onCreated: () => void;
}

type ActivePanel = "none" | "audio" | "photo";

export default function EntryForm({ date, onCreated }: EntryFormProps) {
  const [content, setContent] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [activePanel, setActivePanel] = useState<ActivePanel>("none");
  const inputRef = useRef<HTMLInputElement>(null);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!content.trim()) return;

    setSubmitting(true);
    try {
      await createEntry({
        date,
        type: EntryType.TEXT,
        content: content.trim(),
        source: EntrySource.WEB,
      });
      setContent("");
      inputRef.current?.focus();
      onCreated();
    } catch (err) {
      console.error("Failed to create entry:", err);
      alert("Failed to create entry");
    } finally {
      setSubmitting(false);
    }
  }

  function togglePanel(panel: "audio" | "photo") {
    setActivePanel((prev) => (prev === panel ? "none" : panel));
  }

  return (
    <div className="space-y-2">
      <form onSubmit={handleSubmit} className="flex items-center gap-2 bg-white border border-gray-200 rounded-full px-2 py-1.5 shadow-sm">
        <div className="flex items-center gap-1 pl-1">
          <button
            type="button"
            onClick={() => togglePanel("audio")}
            title="Record audio"
            className={`
              w-8 h-8 flex items-center justify-center rounded-full transition-colors
              ${activePanel === "audio"
                ? "bg-red-100 text-red-600"
                : "text-gray-400 hover:bg-gray-100 hover:text-gray-600"}
            `}
          >
            <svg className="w-4 h-4" fill="currentColor" viewBox="0 0 24 24">
              <path d="M12 14c1.66 0 3-1.34 3-3V5c0-1.66-1.34-3-3-3S9 3.34 9 5v6c0 1.66 1.34 3 3 3z" />
              <path d="M17 11c0 2.76-2.24 5-5 5s-5-2.24-5-5H5c0 3.53 2.61 6.43 6 6.92V21h2v-3.08c3.39-.49 6-3.39 6-6.92h-2z" />
            </svg>
          </button>
          <button
            type="button"
            onClick={() => togglePanel("photo")}
            title="Upload photo"
            className={`
              w-8 h-8 flex items-center justify-center rounded-full transition-colors
              ${activePanel === "photo"
                ? "bg-blue-100 text-blue-600"
                : "text-gray-400 hover:bg-gray-100 hover:text-gray-600"}
            `}
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 13a3 3 0 11-6 0 3 3 0 016 0z" />
            </svg>
          </button>
        </div>
        <input
          ref={inputRef}
          type="text"
          value={content}
          onChange={(e) => setContent(e.target.value)}
          placeholder="Add an entry..."
          className="flex-1 text-sm bg-transparent border-none outline-none placeholder:text-gray-400 py-1"
        />
        <button
          type="submit"
          disabled={submitting || !content.trim()}
          className="w-8 h-8 flex items-center justify-center rounded-full bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-30 disabled:cursor-not-allowed transition-colors shrink-0"
        >
          {submitting ? (
            <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
            </svg>
          ) : (
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 12h14M12 5l7 7-7 7" />
            </svg>
          )}
        </button>
      </form>

      {activePanel === "audio" && (
        <AudioRecorder
          date={date}
          onCreated={onCreated}
          onClose={() => setActivePanel("none")}
        />
      )}

      {activePanel === "photo" && (
        <PhotoCapture
          date={date}
          onCreated={onCreated}
          onClose={() => setActivePanel("none")}
        />
      )}
    </div>
  );
}
