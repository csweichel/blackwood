import { useEffect, useState, useCallback } from "react";
import type { DailyNote as DailyNoteType } from "../api/types";
import { getDailyNote } from "../api/client";
import EntryCard from "./EntryCard";
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

export default function DailyNoteView({ date }: DailyNoteViewProps) {
  const [note, setNote] = useState<DailyNoteType | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getDailyNote({ date });
      setNote(data);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load";
      // A "not found" style error just means no entries yet
      if (msg.includes("404") || msg.includes("not found") || msg.includes("not_found")) {
        setNote({ id: "", date, entries: [], createdAt: "", updatedAt: "" });
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }, [date]);

  useEffect(() => {
    load();
  }, [load]);

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

  const entries = note?.entries ?? [];

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold text-gray-900">
        {formatDateHeading(date)}
      </h2>

      <EntryForm date={date} onCreated={load} />

      {entries.length === 0 ? (
        <div className="text-center py-8 text-gray-400 text-sm">
          No entries for this date.
        </div>
      ) : (
        <div className="space-y-3">
          {entries.map((entry) => (
            <EntryCard key={entry.id} entry={entry} onDeleted={load} />
          ))}
        </div>
      )}
    </div>
  );
}
