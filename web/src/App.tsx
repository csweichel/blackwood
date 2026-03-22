import { useState, useCallback } from "react";
import Timeline from "./components/Timeline";
import DailyNoteView from "./components/DailyNote";
import ChatView from "./components/ChatView";

type View = "notes" | "chat";

function todayStr(): string {
  return new Date().toISOString().slice(0, 10);
}

export default function App() {
  const [selectedDate, setSelectedDate] = useState(todayStr());
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [activeView, setActiveView] = useState<View>("notes");

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
            <Timeline selectedDate={selectedDate} onSelectDate={handleSelectDate} />
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
    </div>
  );
}
