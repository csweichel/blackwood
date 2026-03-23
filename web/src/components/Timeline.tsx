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
      <div className="p-3 border-b border-border space-y-2">
        <button
          onClick={() => onSelectDate(todayStr())}
          className="w-full px-3 py-2 bg-accent text-accent-foreground text-sm font-medium rounded-md hover:bg-accent-hover transition-colors"
        >
          Today
        </button>
        <form onSubmit={handleDatePick} className="flex gap-1">
          <input
            type="date"
            value={dateInput}
            onChange={(e) => setDateInput(e.target.value)}
            className="flex-1 min-w-0 border border-border rounded-md px-2 py-1.5 text-sm bg-background text-foreground focus:outline-none focus:ring-2 focus:ring-accent"
          />
          <button
            type="submit"
            disabled={!dateInput}
            className="px-2 py-1.5 bg-muted text-foreground text-sm rounded-md hover:bg-border disabled:opacity-50 transition-colors"
          >
            Go
          </button>
        </form>
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading ? (
          <div className="p-3 text-muted-foreground text-sm">Loading...</div>
        ) : dates.length === 0 ? (
          <div className="p-3 text-muted-foreground text-sm">No notes yet.</div>
        ) : (
          <ul className="divide-y divide-border">
            {dates.map((note) => (
              <li key={note.id || note.date}>
                <button
                  onClick={() => onSelectDate(note.date)}
                  className={`w-full text-left px-3 py-2.5 text-sm transition-colors ${
                    note.date === selectedDate
                      ? "bg-accent-subtle text-accent font-medium"
                      : "text-foreground hover:bg-muted"
                  }`}
                >
                  <div>{formatDateShort(note.date)}</div>
                  <div className="text-xs text-muted-foreground mt-0.5">
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
