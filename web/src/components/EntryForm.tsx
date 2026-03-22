import { useState } from "react";
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
    <div className="space-y-3">
      <form onSubmit={handleSubmit} className="bg-white border border-gray-200 rounded-lg p-4 shadow-sm">
        <textarea
          value={content}
          onChange={(e) => setContent(e.target.value)}
          placeholder="Write a new entry..."
          rows={3}
          className="w-full border border-gray-300 rounded-md p-3 text-sm resize-y focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
        />
        <div className="flex items-center justify-between mt-2">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => togglePanel("audio")}
              title="Record audio"
              className={`
                w-9 h-9 flex items-center justify-center rounded-full transition-colors
                ${activePanel === "audio"
                  ? "bg-red-100 text-red-600"
                  : "bg-gray-100 text-gray-500 hover:bg-gray-200 hover:text-gray-700"}
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
                w-9 h-9 flex items-center justify-center rounded-full transition-colors
                ${activePanel === "photo"
                  ? "bg-blue-100 text-blue-600"
                  : "bg-gray-100 text-gray-500 hover:bg-gray-200 hover:text-gray-700"}
              `}
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 9a2 2 0 012-2h.93a2 2 0 001.664-.89l.812-1.22A2 2 0 0110.07 4h3.86a2 2 0 011.664.89l.812 1.22A2 2 0 0018.07 7H19a2 2 0 012 2v9a2 2 0 01-2 2H5a2 2 0 01-2-2V9z" />
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 13a3 3 0 11-6 0 3 3 0 016 0z" />
              </svg>
            </button>
          </div>
          <button
            type="submit"
            disabled={submitting || !content.trim()}
            className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {submitting ? "Saving..." : "Add Entry"}
          </button>
        </div>
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
