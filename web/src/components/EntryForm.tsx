import { useState } from "react";
import { EntryType, EntrySource } from "../api/types";
import { createEntry } from "../api/client";

interface EntryFormProps {
  date: string;
  onCreated: () => void;
}

export default function EntryForm({ date, onCreated }: EntryFormProps) {
  const [content, setContent] = useState("");
  const [submitting, setSubmitting] = useState(false);

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

  return (
    <form onSubmit={handleSubmit} className="bg-white border border-gray-200 rounded-lg p-4 shadow-sm">
      <textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="Write a new entry..."
        rows={3}
        className="w-full border border-gray-300 rounded-md p-3 text-sm resize-y focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
      />
      <div className="flex justify-end mt-2">
        <button
          type="submit"
          disabled={submitting || !content.trim()}
          className="px-4 py-2 bg-blue-600 text-white text-sm font-medium rounded-md hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
        >
          {submitting ? "Saving..." : "Add Entry"}
        </button>
      </div>
    </form>
  );
}
