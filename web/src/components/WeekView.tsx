import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { fetchSummariesInRange } from "../api/client";
import { subscribeToChanges } from "../lib/changeEvents";
import {
  getCurrentWeekId,
  getWeekRange,
  getWeekDates,
  prevWeekId,
  nextWeekId,
  fmtDate,
  formatWeekDisplay,
} from "../lib/dateUtils";

export default function WeekView() {
  const { weekId } = useParams<{ weekId: string }>();
  const navigate = useNavigate();
  const currentWeek = weekId || getCurrentWeekId();
  const today = fmtDate(new Date());

  const [summaries, setSummaries] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    setLoading(true);
    const { start, end } = getWeekRange(currentWeek);
    fetchSummariesInRange(start, end)
      .then((data) => {
        const map: Record<string, string> = {};
        for (const s of data) map[s.date] = s.summary;
        setSummaries(map);
      })
      .catch(() => setSummaries({}))
      .finally(() => setLoading(false));
  }, [currentWeek]);

  useEffect(() => {
    const { start, end } = getWeekRange(currentWeek);
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
  }, [currentWeek]);

  const dates = getWeekDates(currentWeek);

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      <div className="max-w-4xl mx-auto px-4 md:px-6 py-6 w-full flex-1 overflow-y-auto">
        {/* Navigation */}
        <div className="flex items-center justify-between mb-6">
          <button
            onClick={() => navigate(`/week/${prevWeekId(currentWeek)}`)}
            className="p-1.5 text-muted-foreground hover:text-foreground rounded-md hover:bg-muted transition-colors"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
          </button>
          <div className="text-center">
            <h2 className="text-lg font-semibold text-foreground">{formatWeekDisplay(currentWeek)}</h2>
            {currentWeek !== getCurrentWeekId() && (
              <button
                onClick={() => navigate(`/week/${getCurrentWeekId()}`)}
                className="text-xs text-muted-foreground hover:text-foreground mt-1"
              >
                This week
              </button>
            )}
          </div>
          <button
            onClick={() => navigate(`/week/${nextWeekId(currentWeek)}`)}
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
          <div className="space-y-2">
            {dates.map((d) => {
              const dateStr = fmtDate(d);
              const isToday = dateStr === today;
              const summary = summaries[dateStr];
              const dayName = d.toLocaleDateString(undefined, { weekday: "short" });
              const dayNum = d.getUTCDate();

              return (
                <button
                  key={dateStr}
                  onClick={() => navigate(`/day/${dateStr}`)}
                  className={`w-full text-left p-3 rounded-lg border transition-colors hover:bg-muted/50 ${
                    isToday ? "border-accent bg-accent/5" : "border-border"
                  }`}
                >
                  <div className="flex items-start gap-3">
                    <div className={`text-center min-w-[3rem] ${isToday ? "text-accent" : "text-muted-foreground"}`}>
                      <div className="text-xs font-medium uppercase">{dayName}</div>
                      <div className="text-lg font-semibold">{dayNum}</div>
                    </div>
                    <div className="flex-1 min-w-0">
                      {summary ? (
                        <p className="text-sm text-foreground line-clamp-3">{summary}</p>
                      ) : (
                        <p className="text-sm text-muted-foreground italic">No summary</p>
                      )}
                    </div>
                  </div>
                </button>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}
