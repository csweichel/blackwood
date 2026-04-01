/** Returns today's date as YYYY-MM-DD in the given IANA timezone (or local if omitted). */
export function todayInTimezone(tz?: string): string {
  const now = new Date();
  if (!tz) {
    const y = now.getFullYear();
    const m = String(now.getMonth() + 1).padStart(2, "0");
    const d = String(now.getDate()).padStart(2, "0");
    return `${y}-${m}-${d}`;
  }
  // Use Intl to get the date parts in the target timezone.
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: tz,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
  }).format(now);
  // en-CA formats as YYYY-MM-DD.
  return parts;
}

/** ISO week number for a date. */
export function getISOWeek(d: Date): number {
  const tmp = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
  tmp.setUTCDate(tmp.getUTCDate() + 4 - (tmp.getUTCDay() || 7));
  const yearStart = new Date(Date.UTC(tmp.getUTCFullYear(), 0, 1));
  return Math.ceil(((tmp.getTime() - yearStart.getTime()) / 86400000 + 1) / 7);
}

/** Returns the ISO week ID for a date, e.g. "2025-W03". */
export function getWeekId(d: Date): string {
  const tmp = new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
  tmp.setUTCDate(tmp.getUTCDate() + 4 - (tmp.getUTCDay() || 7));
  const year = tmp.getUTCFullYear();
  const week = getISOWeek(d);
  return `${year}-W${String(week).padStart(2, "0")}`;
}

/** Returns the month ID for a date, e.g. "2025-01". */
export function getMonthId(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}

export function getCurrentWeekId(): string {
  return getWeekId(new Date());
}

export function getCurrentMonthId(): string {
  return getMonthId(new Date());
}

/** Parse "2025-W03" → { year, week }. */
function parseWeekId(weekId: string): { year: number; week: number } {
  const [y, w] = weekId.split("-W");
  return { year: parseInt(y, 10), week: parseInt(w, 10) };
}

/** Get the Monday of an ISO week. */
function mondayOfWeek(year: number, week: number): Date {
  const jan4 = new Date(Date.UTC(year, 0, 4));
  const dayOfWeek = jan4.getUTCDay() || 7;
  const monday = new Date(jan4);
  monday.setUTCDate(jan4.getUTCDate() - dayOfWeek + 1 + (week - 1) * 7);
  return monday;
}

/** Returns [monday, sunday] for a week ID like "2025-W03". */
export function getWeekRange(weekId: string): { start: string; end: string } {
  const { year, week } = parseWeekId(weekId);
  const monday = mondayOfWeek(year, week);
  const sunday = new Date(monday);
  sunday.setUTCDate(monday.getUTCDate() + 6);
  return { start: fmtDate(monday), end: fmtDate(sunday) };
}

/** Returns [first, last] day of a month ID like "2025-01". */
export function getMonthRange(monthId: string): { start: string; end: string } {
  const [y, m] = monthId.split("-").map(Number);
  const first = new Date(Date.UTC(y, m - 1, 1));
  const last = new Date(Date.UTC(y, m, 0));
  return { start: fmtDate(first), end: fmtDate(last) };
}

export function prevWeekId(weekId: string): string {
  const { year, week } = parseWeekId(weekId);
  const monday = mondayOfWeek(year, week);
  monday.setUTCDate(monday.getUTCDate() - 7);
  return getWeekId(monday);
}

export function nextWeekId(weekId: string): string {
  const { year, week } = parseWeekId(weekId);
  const monday = mondayOfWeek(year, week);
  monday.setUTCDate(monday.getUTCDate() + 7);
  return getWeekId(monday);
}

export function prevMonthId(monthId: string): string {
  const [y, m] = monthId.split("-").map(Number);
  const d = new Date(Date.UTC(y, m - 2, 1));
  return getMonthId(d);
}

export function nextMonthId(monthId: string): string {
  const [y, m] = monthId.split("-").map(Number);
  const d = new Date(Date.UTC(y, m, 1));
  return getMonthId(d);
}

/** Format a Date as "YYYY-MM-DD". */
export function fmtDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** Get all dates (as Date objects) in a week range, Mon-Sun. */
export function getWeekDates(weekId: string): Date[] {
  const { year, week } = parseWeekId(weekId);
  const monday = mondayOfWeek(year, week);
  const dates: Date[] = [];
  for (let i = 0; i < 7; i++) {
    const d = new Date(monday);
    d.setUTCDate(monday.getUTCDate() + i);
    dates.push(d);
  }
  return dates;
}

/** Get calendar grid dates for a month (Mon-start, includes leading/trailing days). */
export function getMonthCalendarDates(monthId: string): Date[] {
  const [y, m] = monthId.split("-").map(Number);
  const first = new Date(Date.UTC(y, m - 1, 1));
  const last = new Date(Date.UTC(y, m, 0));

  // Find the Monday on or before the first day.
  const startDay = first.getUTCDay() || 7; // 1=Mon, 7=Sun
  const gridStart = new Date(first);
  gridStart.setUTCDate(first.getUTCDate() - (startDay - 1));

  // Find the Sunday on or after the last day.
  const endDay = last.getUTCDay() || 7;
  const gridEnd = new Date(last);
  gridEnd.setUTCDate(last.getUTCDate() + (7 - endDay));

  const dates: Date[] = [];
  const current = new Date(gridStart);
  while (current <= gridEnd) {
    dates.push(new Date(current));
    current.setUTCDate(current.getUTCDate() + 1);
  }
  return dates;
}

/** Format month ID as display string, e.g. "January 2025". */
export function formatMonthDisplay(monthId: string): string {
  const [y, m] = monthId.split("-").map(Number);
  const d = new Date(Date.UTC(y, m - 1, 1));
  return d.toLocaleDateString(undefined, { month: "long", year: "numeric" });
}

/** Format week ID as display string, e.g. "Jan 6 – 12, 2025". */
export function formatWeekDisplay(weekId: string): string {
  const dates = getWeekDates(weekId);
  const mon = dates[0];
  const sun = dates[6];
  const monStr = mon.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  const sunStr = sun.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
  return `${monStr} – ${sunStr}`;
}
