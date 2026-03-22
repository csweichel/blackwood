import { useEffect, useState, useCallback } from "react";
import { listDatesWithContent } from "../api/client";

interface CalendarProps {
  selectedDate: string;
  onSelectDate: (date: string) => void;
}

function todayStr(): string {
  return new Date().toISOString().slice(0, 10);
}

function pad(n: number): string {
  return n.toString().padStart(2, "0");
}

function formatMonth(year: number, month: number): string {
  const d = new Date(year, month, 1);
  return d.toLocaleDateString(undefined, { month: "long", year: "numeric" });
}

function daysInMonth(year: number, month: number): number {
  return new Date(year, month + 1, 0).getDate();
}

// Returns 0=Sun..6=Sat for the first day of the month
function firstDayOfWeek(year: number, month: number): number {
  return new Date(year, month, 1).getDay();
}

function dateStr(year: number, month: number, day: number): string {
  return `${year}-${pad(month + 1)}-${pad(day)}`;
}

interface DayCell {
  date: string;
  day: number;
  inMonth: boolean;
}

function buildGrid(year: number, month: number): DayCell[] {
  const cells: DayCell[] = [];
  const startDow = firstDayOfWeek(year, month);
  const totalDays = daysInMonth(year, month);

  // Previous month fill
  const prevMonth = month === 0 ? 11 : month - 1;
  const prevYear = month === 0 ? year - 1 : year;
  const prevDays = daysInMonth(prevYear, prevMonth);
  for (let i = startDow - 1; i >= 0; i--) {
    const d = prevDays - i;
    cells.push({ date: dateStr(prevYear, prevMonth, d), day: d, inMonth: false });
  }

  // Current month
  for (let d = 1; d <= totalDays; d++) {
    cells.push({ date: dateStr(year, month, d), day: d, inMonth: true });
  }

  // Next month fill (up to 42 cells = 6 rows)
  const nextMonth = month === 11 ? 0 : month + 1;
  const nextYear = month === 11 ? year + 1 : year;
  let nextDay = 1;
  while (cells.length < 42) {
    cells.push({ date: dateStr(nextYear, nextMonth, nextDay), day: nextDay, inMonth: false });
    nextDay++;
  }

  return cells;
}

const WEEKDAYS = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];

export default function Calendar({ selectedDate, onSelectDate }: CalendarProps) {
  const today = todayStr();
  const [year, setYear] = useState(() => {
    const d = new Date(selectedDate + "T00:00:00");
    return d.getFullYear();
  });
  const [month, setMonth] = useState(() => {
    const d = new Date(selectedDate + "T00:00:00");
    return d.getMonth();
  });
  const [contentDates, setContentDates] = useState<Set<string>>(new Set());

  const fetchContentDates = useCallback(async (y: number, m: number) => {
    // Fetch a range covering the visible grid (prev month tail to next month head)
    const start = dateStr(y, m, 1);
    const endDays = daysInMonth(y, m);
    const end = dateStr(y, m, endDays);
    try {
      const resp = await listDatesWithContent(start, end);
      setContentDates(new Set(resp.dates ?? []));
    } catch (err) {
      console.error("Failed to fetch content dates:", err);
    }
  }, []);

  useEffect(() => {
    fetchContentDates(year, month);
  }, [year, month, fetchContentDates]);

  function prevMonth() {
    if (month === 0) {
      setYear(year - 1);
      setMonth(11);
    } else {
      setMonth(month - 1);
    }
  }

  function nextMonth() {
    if (month === 11) {
      setYear(year + 1);
      setMonth(0);
    } else {
      setMonth(month + 1);
    }
  }

  function goToday() {
    const now = new Date();
    setYear(now.getFullYear());
    setMonth(now.getMonth());
    onSelectDate(todayStr());
  }

  const grid = buildGrid(year, month);

  return (
    <div className="flex flex-col h-full">
      {/* Month navigation */}
      <div className="p-3 border-b border-gray-200">
        <div className="flex items-center justify-between mb-2">
          <button
            onClick={prevMonth}
            className="w-7 h-7 flex items-center justify-center rounded-md text-gray-500 hover:bg-gray-100 hover:text-gray-700 transition-colors"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
            </svg>
          </button>
          <span className="text-sm font-semibold text-gray-900">
            {formatMonth(year, month)}
          </span>
          <button
            onClick={nextMonth}
            className="w-7 h-7 flex items-center justify-center rounded-md text-gray-500 hover:bg-gray-100 hover:text-gray-700 transition-colors"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
            </svg>
          </button>
        </div>
        <button
          onClick={goToday}
          className="w-full px-3 py-1.5 text-xs font-medium text-blue-600 hover:bg-blue-50 rounded-md transition-colors"
        >
          Today
        </button>
      </div>

      {/* Calendar grid */}
      <div className="p-3">
        {/* Weekday headers */}
        <div className="grid grid-cols-7 mb-1">
          {WEEKDAYS.map((d) => (
            <div key={d} className="text-center text-xs font-medium text-gray-400 py-1">
              {d}
            </div>
          ))}
        </div>

        {/* Day cells */}
        <div className="grid grid-cols-7">
          {grid.map((cell) => {
            const isToday = cell.date === today;
            const isSelected = cell.date === selectedDate;
            const hasContent = contentDates.has(cell.date);

            return (
              <button
                key={cell.date}
                onClick={() => onSelectDate(cell.date)}
                className={`
                  relative w-full aspect-square flex flex-col items-center justify-center rounded-md text-xs transition-colors
                  ${!cell.inMonth ? "text-gray-300" : "text-gray-700"}
                  ${isSelected ? "bg-blue-600 text-white" : ""}
                  ${isToday && !isSelected ? "ring-1 ring-blue-500 text-blue-600 font-semibold" : ""}
                  ${!isSelected ? "hover:bg-gray-100" : ""}
                `}
              >
                <span>{cell.day}</span>
                {hasContent && (
                  <span
                    className={`absolute bottom-0.5 w-1 h-1 rounded-full ${
                      isSelected ? "bg-white" : "bg-blue-500"
                    }`}
                  />
                )}
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
