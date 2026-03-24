import { useState, useCallback, useEffect, useRef } from "react";
import Calendar from "./components/Calendar";
import DailyNoteView from "./components/DailyNote";
import ChatView from "./components/ChatView";
import ImportModal from "./components/ImportModal";
import ImportBanner from "./components/ImportBanner";
import { useImportJobs } from "./hooks/useImportJobs";
import { jobToFileResult } from "./components/importUtils";

type View = "notes" | "chat";

function todayStr(): string {
  return new Date().toISOString().slice(0, 10);
}

export default function App() {
  const [selectedDate, setSelectedDate] = useState(todayStr());
  const [activeView, setActiveView] = useState<View>("notes");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [importModalOpen, setImportModalOpen] = useState(false);

  const { jobs, activeCount, submit, deleteJob } = useImportJobs();

  async function handleImportFiles(e: React.ChangeEvent<HTMLInputElement>) {
    const files = e.target.files;
    if (!files || files.length === 0) return;
    setImportModalOpen(true);
    await submit(Array.from(files));
    e.target.value = "";
  }

  function handleSelectDate(date: string) {
    setSelectedDate(date);
  }

  const handleNavigateToDate = useCallback((date: string) => {
    setSelectedDate(date);
    setActiveView("notes");
  }, []);

  // Cmd+D / Ctrl+D jumps to today
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === "d") {
        e.preventDefault();
        setSelectedDate(todayStr());
        setActiveView("notes");
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  return (
    <div className="h-screen flex flex-col bg-background">
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
              className="px-3 py-1.5 text-sm font-medium text-muted-foreground hover:text-foreground bg-muted hover:bg-border rounded-lg transition-colors"
            >
              Import
            </button>

            <ImportBanner
              activeCount={activeCount}
              onClick={() => setImportModalOpen(true)}
            />

            {/* View toggle */}
            <div className="flex items-center bg-muted rounded-lg p-0.5">
              <button
                onClick={() => setActiveView("notes")}
                className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
                  activeView === "notes"
                    ? "bg-card text-foreground shadow-sm"
                    : "text-muted-foreground hover:text-foreground"
                }`}
              >
                Notes
              </button>
              <button
                onClick={() => setActiveView("chat")}
                className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
                  activeView === "chat"
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
      {activeView === "notes" ? (
        <div className="flex flex-col flex-1 overflow-hidden">
          {/* Horizontal calendar timeline */}
          <Calendar selectedDate={selectedDate} onSelectDate={handleSelectDate} />

          {/* Main content */}
          <main className="flex-1 overflow-y-auto">
            <div className="max-w-4xl mx-auto px-4 md:px-6 py-6">
              <DailyNoteView key={selectedDate} date={selectedDate} />
            </div>
          </main>
        </div>
      ) : (
        <ChatView onNavigateToDate={handleNavigateToDate} />
      )}

      <ImportModal
        open={importModalOpen}
        files={jobs.map(jobToFileResult)}
        onClose={() => setImportModalOpen(false)}
        onNavigateToDate={(date) => {
          setSelectedDate(date);
          setActiveView("notes");
          setImportModalOpen(false);
        }}
        onDeleteJob={deleteJob}
      />
    </div>
  );
}
