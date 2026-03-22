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
    switch (entry.type) {
      case EntryType.TEXT:
        return (
          <pre className="whitespace-pre-wrap font-sans text-sm text-gray-800 leading-relaxed">
            {entry.content}
          </pre>
        );

      case EntryType.AUDIO:
        return (
          <div className="space-y-2">
            <div className="flex items-center gap-2 text-sm text-gray-500">
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
              </svg>
              <span>Audio transcription</span>
            </div>
            <pre className="whitespace-pre-wrap font-sans text-sm text-gray-800 leading-relaxed">
              {entry.content}
            </pre>
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
            {entry.content && (
              <pre className="whitespace-pre-wrap font-sans text-sm text-gray-800 leading-relaxed">
                {entry.content}
              </pre>
            )}
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
            {entry.content && (
              <pre className="whitespace-pre-wrap font-sans text-sm text-gray-700 leading-relaxed">
                {entry.content}
              </pre>
            )}
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
                className="text-blue-600 hover:underline text-sm break-all"
              >
                {url}
              </a>
            )}
            <pre className="whitespace-pre-wrap font-sans text-sm text-gray-800 leading-relaxed">
              {entry.content}
            </pre>
          </div>
        );
      }

      default:
        return (
          <pre className="whitespace-pre-wrap font-sans text-sm text-gray-800 leading-relaxed">
            {entry.content}
          </pre>
        );
    }
  }

  return (
    <div className="bg-white border border-gray-200 rounded-lg p-4 shadow-sm">
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100 text-gray-700">
            {entryTypeLabel[entry.type] ?? "Unknown"}
          </span>
          <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-blue-50 text-blue-700">
            {entrySourceLabel[entry.source] ?? "Unknown"}
          </span>
          <span className="text-xs text-gray-400">
            {formatTime(entry.createdAt)}
          </span>
        </div>
        <button
          onClick={handleDelete}
          disabled={deleting}
          className="text-gray-400 hover:text-red-500 transition-colors disabled:opacity-50"
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
