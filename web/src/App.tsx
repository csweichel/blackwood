import { useState, useCallback, useRef } from "react";
import Calendar from "./components/Calendar";
import DailyNoteView from "./components/DailyNote";
import ChatView from "./components/ChatView";
import ImportModal, { type ImportFileResult } from "./components/ImportModal";
import { importObsidian, importViwoods } from "./api/client";

type View = "notes" | "chat";

function todayStr(): string {
  return new Date().toISOString().slice(0, 10);
}

export default function App() {
  const [selectedDate, setSelectedDate] = useState(todayStr());
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [activeView, setActiveView] = useState<View>("notes");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [importModalOpen, setImportModalOpen] = useState(false);
  const [importFiles, setImportFiles] = useState<ImportFileResult[]>([]);

  async function handleImportFiles(e: React.ChangeEvent<HTMLInputElement>) {
    const files = e.target.files;
    if (!files || files.length === 0) return;

    const fileList = Array.from(files);

    // Initialize all files as pending
    const initial: ImportFileResult[] = fileList.map((f) => ({
      filename: f.name,
      status: "pending" as const,
    }));
    setImportFiles(initial);
    setImportModalOpen(true);

    // Process each file sequentially
    for (let i = 0; i < fileList.length; i++) {
      const f = fileList[i];

      // Mark as processing
      setImportFiles((prev) =>
        prev.map((r, j) => (j === i ? { ...r, status: "processing" } : r))
      );

      try {
        if (f.name.endsWith(".note")) {
          const result = await importViwoods(f);
          // Extract date from dailyNoteId (format: "daily/<YYYY-MM-DD>")
          const dateMatch = result.dailyNoteId?.match(/\d{4}-\d{2}-\d{2}/);
          setImportFiles((prev) =>
            prev.map((r, j) =>
              j === i
                ? {
                    ...r,
                    status: "done",
                    message: `${result.pagesProcessed} pages processed`,
                    pagesProcessed: result.pagesProcessed,
                    date: dateMatch ? dateMatch[0] : undefined,
                  }
                : r
            )
          );
        } else if (f.name.endsWith(".md")) {
          const result = await importObsidian([f]);
          const parts: string[] = [];
          if (result.imported) parts.push(`${result.imported} imported`);
          if (result.skipped) parts.push(`${result.skipped} skipped`);
          if (result.errors?.length)
            parts.push(`${result.errors.length} errors`);
          setImportFiles((prev) =>
            prev.map((r, j) =>
              j === i
                ? {
                    ...r,
                    status: "done",
                    message: parts.join(", ") || "Imported",
                    date: result.dates?.[0],
                  }
                : r
            )
          );
        } else {
          setImportFiles((prev) =>
            prev.map((r, j) =>
              j === i
                ? { ...r, status: "error", message: "Unsupported file type" }
                : r
            )
          );
        }
      } catch (err) {
        setImportFiles((prev) =>
          prev.map((r, j) =>
            j === i
              ? {
                  ...r,
                  status: "error",
                  message: err instanceof Error ? err.message : String(err),
                }
              : r
          )
        );
      }
    }

    e.target.value = "";
  }

  function handleSelectDate(date: string) {
    setSelectedDate(date);
    setSidebarOpen(false);
  }

  const handleNavigateToDate = useCallback((date: string) => {
    setSelectedDate(date);
    setActiveView("notes");
  }, []);

  return (
    <div className="h-screen flex flex-col bg-gray-50">
      {/* Header */}
      <header className="bg-white border-b border-gray-200 px-4 py-3 flex items-center justify-between shrink-0">
        <div className="flex items-center gap-3">
          {activeView === "notes" && (
            <button
              onClick={() => setSidebarOpen(!sidebarOpen)}
              className="md:hidden text-gray-500 hover:text-gray-700"
            >
              <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
              </svg>
            </button>
          )}
          <h1 className="text-lg font-semibold text-gray-900">Blackwood</h1>
        </div>

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
            className="px-3 py-1.5 text-sm font-medium text-gray-600 hover:text-gray-900 bg-gray-100 hover:bg-gray-200 rounded-lg transition-colors"
          >
            Import
          </button>

        {/* View toggle */}
        <div className="flex items-center bg-gray-100 rounded-lg p-0.5">
          <button
            onClick={() => setActiveView("notes")}
            className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
              activeView === "notes"
                ? "bg-white text-gray-900 shadow-sm"
                : "text-gray-500 hover:text-gray-700"
            }`}
          >
            Notes
          </button>
          <button
            onClick={() => setActiveView("chat")}
            className={`px-3 py-1.5 text-sm font-medium rounded-md transition-colors ${
              activeView === "chat"
                ? "bg-white text-gray-900 shadow-sm"
                : "text-gray-500 hover:text-gray-700"
            }`}
          >
            Chat
          </button>
        </div>
        </div>
      </header>

      {/* Body */}
      {activeView === "notes" ? (
        <div className="flex flex-1 overflow-hidden relative">
          {/* Sidebar overlay on mobile */}
          {sidebarOpen && (
            <div
              className="fixed inset-0 bg-black/30 z-10 md:hidden"
              onClick={() => setSidebarOpen(false)}
            />
          )}

          {/* Sidebar */}
          <aside
            className={`
              bg-white border-r border-gray-200 w-64 shrink-0 flex flex-col
              fixed inset-y-0 left-0 z-20 pt-14 transition-transform md:relative md:pt-0 md:translate-x-0
              ${sidebarOpen ? "translate-x-0" : "-translate-x-full"}
            `}
          >
            <Calendar selectedDate={selectedDate} onSelectDate={handleSelectDate} />
          </aside>

          {/* Main content */}
          <main className="flex-1 overflow-y-auto p-4 md:p-6">
            <div className="max-w-2xl mx-auto">
              <DailyNoteView key={selectedDate} date={selectedDate} />
            </div>
          </main>
        </div>
      ) : (
        <ChatView onNavigateToDate={handleNavigateToDate} />
      )}

      <ImportModal
        open={importModalOpen}
        files={importFiles}
        onClose={() => setImportModalOpen(false)}
        onNavigateToDate={(date) => {
          setSelectedDate(date);
          setActiveView("notes");
          setImportModalOpen(false);
        }}
      />
    </div>
  );
}
