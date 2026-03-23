import { useState, useRef } from "react";
import { EntryType, EntrySource } from "../api/types";
import { createEntry } from "../api/client";

interface EntryFormProps {
  date: string;
  onCreated: () => void;
}

export default function EntryForm({ date, onCreated }: EntryFormProps) {
  const [content, setContent] = useState("");
  const [submitting, setSubmitting] = useState(false);
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

  return (
    <form onSubmit={handleSubmit} className="flex items-center gap-2 bg-card border border-border rounded-full px-3 py-1.5 shadow-sm">
      <input
        ref={inputRef}
        type="text"
        value={content}
        onChange={(e) => setContent(e.target.value)}
        placeholder="Add an entry..."
        className="flex-1 text-sm bg-transparent border-none outline-none placeholder:text-muted-foreground py-1 text-foreground"
      />
      <button
        type="submit"
        disabled={submitting || !content.trim()}
        className="w-8 h-8 flex items-center justify-center rounded-full bg-primary text-primary-foreground hover:opacity-90 disabled:opacity-30 disabled:cursor-not-allowed transition-colors shrink-0"
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
  );
}
