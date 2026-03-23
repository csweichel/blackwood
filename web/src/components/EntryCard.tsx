import { useState } from "react";
import type { Entry } from "../api/types";
import { EntryType, entryTypeLabel, entrySourceLabel } from "../api/types";
import { deleteEntry } from "../api/client";

interface EntryCardProps {
  entry: Entry;
  onDeleted: () => void;
}

function formatTime(ts: string): string {
  if (!ts) return "";
  const d = new Date(ts);
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
}

export default function EntryCard({ entry, onDeleted }: EntryCardProps) {
  const [deleting, setDeleting] = useState(false);

  async function handleDelete() {
    if (!confirm("Delete this entry?")) return;
    setDeleting(true);
    try {
      await deleteEntry({ id: entry.id });
      onDeleted();
    } catch (err) {
      console.error("Failed to delete entry:", err);
      alert("Failed to delete entry");
    } finally {
      setDeleting(false);
    }
  }

  function renderContent() {
    const textBlock = (text: string, color = "text-foreground") => (
      <div className={`whitespace-pre-wrap font-sans text-sm ${color}`} style={{ lineHeight: "1.7" }}>
        {text}
      </div>
    );

    switch (entry.type) {
      case EntryType.TEXT:
        return textBlock(entry.content);

      case EntryType.AUDIO:
        return (
          <div className="space-y-2">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
              </svg>
              <span>Audio transcription</span>
            </div>
            {textBlock(entry.content)}
          </div>
        );

      case EntryType.PHOTO:
        return (
          <div className="space-y-2">
            {entry.attachments?.map((att) => (
              <img
                key={att.id}
                src={att.url}
                alt={att.filename}
                className="max-w-full rounded-lg max-h-64 object-contain"
              />
            ))}
            {entry.content && textBlock(entry.content)}
          </div>
        );

      case EntryType.VIWOODS:
        return (
          <div className="space-y-2">
            {entry.attachments?.map((att) => (
              <img
                key={att.id}
                src={att.url}
                alt={att.filename}
                className="max-w-full rounded-lg max-h-64 object-contain"
              />
            ))}
            {entry.content && textBlock(entry.content, "text-muted-foreground")}
          </div>
        );

      case EntryType.WEBCLIP: {
        let url = "";
        try {
          const meta = JSON.parse(entry.metadata || "{}");
          url = meta.url || "";
        } catch {
          // ignore
        }
        return (
          <div className="space-y-2">
            {url && (
              <a
                href={url}
                target="_blank"
                rel="noopener noreferrer"
                className="text-accent hover:underline text-sm break-all"
              >
                {url}
              </a>
            )}
            {textBlock(entry.content)}
          </div>
        );
      }

      default:
        return textBlock(entry.content);
    }
  }

  return (
    <div className="bg-card border border-border rounded-lg p-4 shadow-sm">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-muted text-foreground">
            {entryTypeLabel[entry.type] ?? "Unknown"}
          </span>
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-muted text-accent">
            {entrySourceLabel[entry.source] ?? "Unknown"}
          </span>
          <span className="text-xs text-muted-foreground">
            {formatTime(entry.createdAt)}
          </span>
        </div>
        <button
          onClick={handleDelete}
          disabled={deleting}
          className="text-muted-foreground hover:text-destructive transition-colors disabled:opacity-50"
          title="Delete entry"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
          </svg>
        </button>
      </div>
      {renderContent()}
    </div>
  );
}
