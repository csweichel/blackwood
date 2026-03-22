import { useEffect, useState } from "react";
import type { DailyNote } from "../api/types";
import { listDailyNotes } from "../api/client";

interface TimelineProps {
  selectedDate: string;
  onSelectDate: (date: string) => void;
}

function todayStr(): string {
  return new Date().toISOString().slice(0, 10);
}

function formatDateShort(dateStr: string): string {
  const d = new Date(dateStr + "T00:00:00");
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    weekday: "short",
  });
}

export default function Timeline({ selectedDate, onSelectDate }: TimelineProps) {
  const [dates, setDates] = useState<DailyNote[]>([]);
  const [loading, setLoading] = useState(true);
  const [dateInput, setDateInput] = useState("");

  useEffect(() => {
    async function load() {
      setLoading(true);
      try {
        const resp = await listDailyNotes({ limit: 90 });
        setDates(resp.dailyNotes ?? []);
      } catch (err) {
        console.error("Failed to list daily notes:", err);
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  function handleDatePick(e: React.FormEvent) {
    e.preventDefault();
    if (dateInput) {
      onSelectDate(dateInput);
    }
  }

  return (
    <div className="flex flex-col h-full">
      <div className="p-3 border-b border-gray-200 space-y-2">
        <button
          onClick={() => onSelectDate(todayStr())}
          className="w-full px-3 py-2 bg-blue-600 text-white text-sm font-medium rounded-md hover:bg-blue-700 transition-colors"
        >
          Today
        </button>
        <form onSubmit={handleDatePick} className="flex gap-1">
          <input
            type="date"
            value={dateInput}
            onChange={(e) => setDateInput(e.target.value)}
            className="flex-1 min-w-0 border border-gray-300 rounded-md px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          <button
            type="submit"
            disabled={!dateInput}
            className="px-2 py-1.5 bg-gray-100 text-gray-700 text-sm rounded-md hover:bg-gray-200 disabled:opacity-50 transition-colors"
          >
            Go
          </button>
        </form>
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="p-3 text-gray-400 text-sm">Loading...</div>
        ) : dates.length === 0 ? (
          <div className="p-3 text-gray-400 text-sm">No notes yet.</div>
        ) : (
          <ul className="divide-y divide-gray-100">
            {dates.map((note) => (
              <li key={note.id || note.date}>
                <button
                  onClick={() => onSelectDate(note.date)}
                  className={`w-full text-left px-3 py-2.5 text-sm transition-colors ${
                    note.date === selectedDate
                      ? "bg-blue-50 text-blue-700 font-medium"
                      : "text-gray-700 hover:bg-gray-50"
                  }`}
                >
                  <div>{formatDateShort(note.date)}</div>
                  <div className="text-xs text-gray-400 mt-0.5">
                    {note.entries?.length ?? 0} {(note.entries?.length ?? 0) === 1 ? "entry" : "entries"}
                  </div>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
