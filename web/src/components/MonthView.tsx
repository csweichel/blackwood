import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { fetchSummariesInRange } from "../api/client";
import { subscribeToChanges } from "../lib/changeEvents";
import {
  getCurrentMonthId,
  getMonthCalendarDates,
  prevMonthId,
  nextMonthId,
  fmtDate,
  formatMonthDisplay,
} from "../lib/dateUtils";

const DAY_LABELS = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];

export default function MonthView() {
  const { monthId } = useParams<{ monthId: string }>();
  const navigate = useNavigate();
  const currentMonth = monthId || getCurrentMonthId();
  const today = fmtDate(new Date());

  const [summaries, setSummaries] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    const dates = getMonthCalendarDates(currentMonth);
    const start = fmtDate(dates[0]);
    const end = fmtDate(dates[dates.length - 1]);
    fetchSummariesInRange(start, end)
      .then((data) => {
        const map: Record<string, string> = {};
        for (const s of data) map[s.date] = s.summary;
        setSummaries(map);
      })
      .catch(() => setSummaries({}))
      .finally(() => setLoading(false));
  }, [currentMonth]);

  useEffect(() => {
    const dates = getMonthCalendarDates(currentMonth);
    const start = fmtDate(dates[0]);
    const end = fmtDate(dates[dates.length - 1]);
    return subscribeToChanges((event) => {
      if (event.kind !== "CHANGE_EVENT_KIND_DAILY_NOTE_UPDATED") return;
      if (event.date < start || event.date > end) return;
      setLoading(true);
      fetchSummariesInRange(start, end)
        .then((data) => {
          const map: Record<string, string> = {};
          for (const s of data) map[s.date] = s.summary;
          setSummaries(map);
        })
        .catch(() => setSummaries({}))
        .finally(() => setLoading(false));
    });
  }, [currentMonth]);

  const calendarDates = getMonthCalendarDates(currentMonth);
  const [y, m] = currentMonth.split("-").map(Number);

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      <div className="max-w-4xl mx-auto px-4 md:px-6 py-6 w-full flex-1 overflow-y-auto">
        {/* Navigation */}
        <div className="flex items-center justify-between mb-6">
          <button
            onClick={() => navigate(`/month/${prevMonthId(currentMonth)}`)}
            className="p-1.5 text-muted-foreground hover:text-foreground rounded-md hover:bg-muted transition-colors"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
          </button>
          <div className="text-center">
            <h2 className="text-lg font-semibold text-foreground">{formatMonthDisplay(currentMonth)}</h2>
            {currentMonth !== getCurrentMonthId() && (
              <button
                onClick={() => navigate(`/month/${getCurrentMonthId()}`)}
                className="text-xs text-muted-foreground hover:text-foreground mt-1"
              >
                This month
              </button>
            )}
          </div>
          <button
            onClick={() => navigate(`/month/${nextMonthId(currentMonth)}`)}
            className="p-1.5 text-muted-foreground hover:text-foreground rounded-md hover:bg-muted transition-colors"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="9 18 15 12 9 6"/></svg>
          </button>
        </div>

        {loading ? (
          <div className="flex items-center justify-center py-12">
            <div className="text-muted-foreground text-sm">Loading...</div>
          </div>
        ) : (
          <div>
            {/* Day headers */}
            <div className="grid grid-cols-7 gap-1 mb-1">
              {DAY_LABELS.map((label) => (
                <div key={label} className="text-center text-xs font-medium text-muted-foreground py-1">
                  {label}
                </div>
              ))}
            </div>

            {/* Calendar grid */}
            <div className="grid grid-cols-7 gap-1">
              {calendarDates.map((d) => {
                const dateStr = fmtDate(d);
                const isToday = dateStr === today;
                const isCurrentMonth = d.getUTCMonth() + 1 === m && d.getUTCFullYear() === y;
                const summary = summaries[dateStr];

                return (
                  <button
                    key={dateStr}
                    onClick={() => navigate(`/day/${dateStr}`)}
                    className={`text-left p-2 rounded-lg border min-h-[5rem] transition-colors hover:bg-muted/50 ${
                      isToday
                        ? "border-accent bg-accent/5"
                        : isCurrentMonth
                        ? "border-border"
                        : "border-border/50 opacity-50"
                    }`}
                  >
                    <div className={`text-xs font-medium mb-1 ${isToday ? "text-accent" : isCurrentMonth ? "text-foreground" : "text-muted-foreground"}`}>
                      {d.getUTCDate()}
                    </div>
                    {summary && (
                      <p className="text-[10px] leading-tight text-muted-foreground line-clamp-3">
                        {summary}
                      </p>
                    )}
                  </button>
                );
              })}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
