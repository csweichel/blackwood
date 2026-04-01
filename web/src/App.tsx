import { useState, useCallback, useEffect, useRef } from "react";
import {
  BrowserRouter,
  Routes,
  Route,
  Navigate,
  useNavigate,
  useParams,
  useLocation,
} from "react-router-dom";
import Calendar from "./components/Calendar";
import DailyNoteView from "./components/DailyNote";
import ChatView from "./components/ChatView";
import ClipPage from "./components/ClipPage";
import WeekView from "./components/WeekView";
import MonthView from "./components/MonthView";
import SearchPage from "./components/SearchPage";
import ImportModal from "./components/ImportModal";
import ImportBanner from "./components/ImportBanner";
import OfflineBanner from "./components/OfflineBanner";
import SettingsPanel from "./components/SettingsPanel";
import Logo from "./components/Logo";
import TimezoneBanner from "./components/TimezoneBanner";
import AuthLogin from "./components/AuthLogin";
import AuthSetup from "./components/AuthSetup";
import { PreferencesProvider, usePreferences } from "./hooks/usePreferences";
import { useImportJobs } from "./hooks/useImportJobs";
import { jobToFileResult } from "./components/importUtils";
import { getCurrentWeekId, getCurrentMonthId, todayInTimezone } from "./lib/dateUtils";
import { ColorTheme } from "./api/types";

/** Hook that returns today's date string in the user's configured timezone. */
function useTodayStr(): string {
  const { preferences, detectedTimezone } = usePreferences();
  const tz = preferences.timezone || detectedTimezone;
  return todayInTimezone(tz);
}

function DailyNotePage() {
  const { date } = useParams<{ date: string }>();
  const navigate = useNavigate();
  const today = useTodayStr();
  const selectedDate = date || today;

  function handleSelectDate(d: string) {
    navigate(`/day/${d}`);
  }

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      <Calendar selectedDate={selectedDate} onSelectDate={handleSelectDate} />
      <main className="flex-1 overflow-y-auto">
        <div className="max-w-4xl mx-auto px-4 md:px-6 py-6">
          <DailyNoteView key={selectedDate} date={selectedDate} />
        </div>
      </main>
    </div>
  );
}

function ChatPage() {
  const { slug } = useParams<{ slug: string }>();
  const navigate = useNavigate();

  const handleNavigateToDate = useCallback(
    (date: string) => {
      navigate(`/day/${date}`);
    },
    [navigate]
  );

  return <ChatView slug={slug} onNavigateToDate={handleNavigateToDate} />;
}

function AppLayout() {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const location = useLocation();
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const { jobs, activeCount, submit, deleteJob } = useImportJobs();
  const { preferences } = usePreferences();
  const today = useTodayStr();

  // Apply color theme to <html> element.
  useEffect(() => {
    const root = document.documentElement;
    const theme = preferences.colorTheme;
    if (theme === ColorTheme.DARK) {
      root.classList.add("dark");
      root.classList.remove("light");
    } else if (theme === ColorTheme.LIGHT) {
      root.classList.add("light");
      root.classList.remove("dark");
    } else {
      // System: remove overrides, let prefers-color-scheme handle it.
      root.classList.remove("dark", "light");
    }
  }, [preferences.colorTheme]);

  async function handleImportFiles(e: React.ChangeEvent<HTMLInputElement>) {
    const files = e.target.files;
    if (!files || files.length === 0) return;
    setImportModalOpen(true);
    await submit(Array.from(files));
    e.target.value = "";
  }

  const isChat = location.pathname.startsWith("/chat");
  const isWeek = location.pathname.startsWith("/week");
  const isMonth = location.pathname.startsWith("/month");
  const isDay = !isChat && !isWeek && !isMonth;

  // Cmd+D → today, Cmd+/ → toggle chat/notes
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === "d") {
        e.preventDefault();
        navigate(`/day/${today}`);
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "/") {
        e.preventDefault();
        navigate(isChat ? `/day/${today}` : "/chat");
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        if (location.pathname.startsWith("/search")) {
          window.dispatchEvent(new CustomEvent("focus-search"));
        } else if (searchInputRef.current) {
          searchInputRef.current.focus();
        } else {
          navigate("/search");
        }
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [navigate, isChat, location.pathname, today]);

  return (
    <div className="flex flex-col bg-background" style={{ height: "100dvh" }}>
      {/* Header */}
      <header className="border-b border-border bg-card shrink-0">
        <div className="max-w-4xl mx-auto px-4 md:px-6 py-3 flex items-center justify-between">
          <Logo height={36} width={128} />

          {/* Actions */}
          <div className="flex items-center gap-2">
            <button
              onClick={() => navigate("/search")}
              className="p-1.5 text-muted-foreground hover:text-foreground rounded-lg transition-colors"
              title="Search (Cmd+K)"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>
            </button>
            <input
              ref={fileInputRef}
              type="file"
              accept=".md,.note"
              multiple
              className="hidden"
              onChange={handleImportFiles}
            />
            <button
              onClick={() => fileInputRef.current?.click()}
              className="p-1.5 text-muted-foreground hover:text-foreground rounded-lg transition-colors"
              title="Import files"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                <polyline points="7 10 12 15 17 10" />
                <line x1="12" y1="15" x2="12" y2="3" />
              </svg>
            </button>

            <button
              onClick={() => setSettingsOpen(true)}
              className="p-1.5 text-muted-foreground hover:text-foreground rounded-lg transition-colors"
              title="Settings"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
                <circle cx="12" cy="12" r="3" />
              </svg>
            </button>

            <OfflineBanner />

            <ImportBanner
              activeCount={activeCount}
              onClick={() => setImportModalOpen(true)}
            />

            {/* View toggle */}
            <div className="flex items-center bg-muted rounded-lg p-0.5">
              <button
                onClick={() => navigate(`/day/${today}`)}
                className={`px-2 sm:px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
                  isDay
                    ? "bg-card text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                <span className="sm:hidden">D</span>
                <span className="hidden sm:inline">Day</span>
              </button>
              <button
                onClick={() => navigate(`/week/${getCurrentWeekId()}`)}
                className={`px-2 sm:px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
                  isWeek
                    ? "bg-card text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                <span className="sm:hidden">W</span>
                <span className="hidden sm:inline">Week</span>
              </button>
              <button
                onClick={() => navigate(`/month/${getCurrentMonthId()}`)}
                className={`px-2 sm:px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
                  isMonth
                    ? "bg-card text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                <span className="sm:hidden">M</span>
                <span className="hidden sm:inline">Month</span>
              </button>
              <button
                onClick={() => navigate("/chat")}
                className={`px-2 sm:px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
                  isChat
                    ? "bg-card text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                <span className="sm:hidden">C</span>
                <span className="hidden sm:inline">Chat</span>
              </button>
            </div>
          </div>
        </div>
      </header>

      <TimezoneBanner />

      {/* Body */}
      <Routes>
        <Route path="/" element={<Navigate to={`/day/${today}`} replace />} />
        <Route path="/day/:date" element={<DailyNotePage />} />
        <Route path="/week" element={<WeekView />} />
        <Route path="/week/:weekId" element={<WeekView />} />
        <Route path="/month" element={<MonthView />} />
        <Route path="/month/:monthId" element={<MonthView />} />
        <Route path="/search" element={<SearchPage />} />
        <Route path="/chat" element={<ChatPage />} />
        <Route path="/chat/:slug" element={<ChatPage />} />
        <Route path="/clip" element={<ClipPage />} />
        <Route path="*" element={<Navigate to={`/day/${today}`} replace />} />
      </Routes>

      <ImportModal
        open={importModalOpen}
        files={jobs.map(jobToFileResult)}
        onClose={() => setImportModalOpen(false)}
        onNavigateToDate={(date) => {
          navigate(`/day/${date}`);
          setImportModalOpen(false);
        }}
        onDeleteJob={deleteJob}
      />

      <SettingsPanel open={settingsOpen} onClose={() => setSettingsOpen(false)} />
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/auth/login" element={<AuthLogin />} />
        <Route path="/auth/setup" element={<AuthSetup />} />
        <Route
          path="*"
          element={
            <PreferencesProvider>
              <AppLayout />
            </PreferencesProvider>
          }
        />
      </Routes>
    </BrowserRouter>
  );
}
