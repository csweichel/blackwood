import { useEffect, useState, useCallback, useRef } from "react";
import { listDatesWithContent } from "../api/client";
import { usePreferences } from "../hooks/usePreferences";
import { todayInTimezone } from "../lib/dateUtils";

interface CalendarProps {
  selectedDate: string;
  onSelectDate: (date: string) => void;
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

function dateStr(year: number, month: number, day: number): string {
  return `${year}-${pad(month + 1)}-${pad(day)}`;
}

const WEEKDAY_LETTERS = ["S", "M", "T", "W", "T", "F", "S"];

interface DayItem {
  date: string;
  day: number;
  weekday: number; // 0=Sun..6=Sat
}

function buildDays(year: number, month: number): DayItem[] {
  const total = daysInMonth(year, month);
  const days: DayItem[] = [];
  for (let d = 1; d <= total; d++) {
    const dt = new Date(year, month, d);
    days.push({
      date: dateStr(year, month, d),
      day: d,
      weekday: dt.getDay(),
    });
  }
  return days;
}

export default function Calendar({ selectedDate, onSelectDate }: CalendarProps) {
  const { preferences, detectedTimezone } = usePreferences();
  const tz = preferences.timezone || detectedTimezone;
  const today = todayInTimezone(tz);
  const scrollRef = useRef<HTMLDivElement>(null);
  const selectedRef = useRef<HTMLButtonElement>(null);

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

  // Scroll selected day into view
  useEffect(() => {
    if (selectedRef.current && scrollRef.current) {
      selectedRef.current.scrollIntoView({
        behavior: "smooth",
        block: "nearest",
        inline: "center",
      });
    }
  }, [selectedDate, month, year]);

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
    const t = todayInTimezone(tz);
    const [y, m] = t.split("-").map(Number);
    setYear(y);
    setMonth(m - 1);
    onSelectDate(t);
  }

  const days = buildDays(year, month);

  return (
    <div className="border-b border-border bg-card">
      <div className="max-w-4xl mx-auto px-4 md:px-6 py-3">
        {/* Month nav row */}
        <div className="flex items-center justify-between mb-3">
          <div className="flex items-center gap-2">
            <button
              onClick={prevMonth}
              className="w-7 h-7 flex items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
              </svg>
            </button>
            <span className="text-sm font-medium min-w-[140px] text-center text-foreground">
              {formatMonth(year, month)}
            </span>
            <button
              onClick={nextMonth}
              className="w-7 h-7 flex items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
              </svg>
            </button>
          </div>
          <button
            onClick={goToday}
            className="text-xs text-muted-foreground hover:text-accent transition-colors"
          >
            Today
          </button>
        </div>

        {/* Days row */}
        <div
          ref={scrollRef}
          className="flex items-end gap-1 overflow-x-auto scrollbar-hide pb-1"
        >
          {days.map((day) => {
            const isToday = day.date === today;
            const isSelected = day.date === selectedDate;
            const hasContent = contentDates.has(day.date);

            let btnClass =
              "flex flex-col items-center min-w-[32px] py-1.5 px-1 rounded-md transition-colors cursor-pointer ";
            if (isSelected) {
              btnClass += "bg-primary text-primary-foreground";
            } else if (isToday) {
              btnClass += "text-accent hover:bg-muted";
            } else {
              btnClass += "text-foreground hover:bg-muted";
            }

            return (
              <button
                key={day.date}
                ref={isSelected ? selectedRef : undefined}
                onClick={() => onSelectDate(day.date)}
                className={btnClass}
              >
                <span className="text-[10px] uppercase leading-none mb-0.5">
                  {WEEKDAY_LETTERS[day.weekday]}
                </span>
                <span className="text-sm font-medium leading-none">
                  {day.day}
                </span>
                <div className="h-1.5 mt-0.5 flex items-center justify-center">
                  {hasContent && (
                    <div
                      className={`w-1 h-1 rounded-full ${
                        isSelected ? "bg-primary-foreground" : "bg-accent"
                      }`}
                    />
                  )}
                </div>
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
