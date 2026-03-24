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
import ImportModal from "./components/ImportModal";
import ImportBanner from "./components/ImportBanner";
import OfflineBanner from "./components/OfflineBanner";
import { useImportJobs } from "./hooks/useImportJobs";
import { jobToFileResult } from "./components/importUtils";

function todayStr(): string {
  return new Date().toISOString().slice(0, 10);
}

function DailyNotePage() {
  const { date } = useParams<{ date: string }>();
  const navigate = useNavigate();
  const selectedDate = date || todayStr();

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
  const navigate = useNavigate();
  const location = useLocation();
  const [importModalOpen, setImportModalOpen] = useState(false);
  const { jobs, activeCount, submit, deleteJob } = useImportJobs();

  async function handleImportFiles(e: React.ChangeEvent<HTMLInputElement>) {
    const files = e.target.files;
    if (!files || files.length === 0) return;
    setImportModalOpen(true);
    await submit(Array.from(files));
    e.target.value = "";
  }

  const isChat = location.pathname.startsWith("/chat");

  // Cmd+D → today, Cmd+/ → toggle chat/notes
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === "d") {
        e.preventDefault();
        navigate(`/day/${todayStr()}`);
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "/") {
        e.preventDefault();
        navigate(isChat ? `/day/${todayStr()}` : "/chat");
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [navigate, isChat]);

  return (
    <div className="flex flex-col bg-background" style={{ height: "100dvh" }}>
      {/* Header */}
      <header className="border-b border-border bg-card shrink-0">
        <div className="max-w-4xl mx-auto px-4 md:px-6 py-3 flex items-center justify-between">
          <h1 className="text-lg font-medium tracking-tight text-foreground">Blackwood</h1>

          {/* Actions */}
          <div className="flex items-center gap-2">
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
              className="px-3 py-1.5 text-sm font-medium text-muted-foreground hover:text-foreground border border-border rounded-lg transition-colors"
            >
              Import
            </button>

            <OfflineBanner />

            <ImportBanner
              activeCount={activeCount}
              onClick={() => setImportModalOpen(true)}
            />

            {/* View toggle */}
            <div className="flex items-center bg-muted rounded-lg p-0.5">
              <button
                onClick={() => navigate(`/day/${todayStr()}`)}
                className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
                  !isChat
                    ? "bg-card text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Notes
              </button>
              <button
                onClick={() => navigate("/chat")}
                className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
                  isChat
                    ? "bg-card text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Chat
              </button>
            </div>
          </div>
        </div>
      </header>

      {/* Body */}
      <Routes>
        <Route path="/" element={<Navigate to={`/day/${todayStr()}`} replace />} />
        <Route path="/day/:date" element={<DailyNotePage />} />
        <Route path="/chat" element={<ChatPage />} />
        <Route path="/chat/:slug" element={<ChatPage />} />
        <Route path="/clip" element={<ClipPage />} />
        <Route path="*" element={<Navigate to={`/day/${todayStr()}`} replace />} />
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
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AppLayout />
    </BrowserRouter>
  );
}
