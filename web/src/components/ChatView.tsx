import { useState, useCallback } from "react";
import type { ChatMessage, Conversation } from "../api/types";
import { getConversation } from "../api/client";
import ChatPanel from "./ChatPanel";
import ConversationList from "./ConversationList";

interface ChatViewProps {
  onNavigateToDate: (date: string) => void;
}

export default function ChatView({ onNavigateToDate }: ChatViewProps) {
  const [conversationId, setConversationId] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [refreshKey, setRefreshKey] = useState(0);
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const handleNewConversation = useCallback(() => {
    setConversationId("");
    setMessages([]);
    setSidebarOpen(false);
  }, []);

  const handleSelectConversation = useCallback(async (conv: Conversation) => {
    setSidebarOpen(false);
    try {
      const full = await getConversation(conv.id);
      setConversationId(full.id);
      setMessages(full.messages || []);
    } catch {
      // If fetching fails, just set the ID and show empty
      setConversationId(conv.id);
      setMessages([]);
    }
  }, []);

  const handleMessagesUpdate = useCallback((updatedMessages: ChatMessage[], newConversationId: string) => {
    setMessages(updatedMessages);
    if (newConversationId && newConversationId !== conversationId) {
      setConversationId(newConversationId);
      // Refresh conversation list when a new conversation is created
      setRefreshKey((k) => k + 1);
    }
  }, [conversationId]);

  const handleSourceClick = useCallback((date: string) => {
    onNavigateToDate(date);
  }, [onNavigateToDate]);

  return (
    <div className="flex flex-1 overflow-hidden relative">
      {/* Mobile sidebar overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 bg-black/30 z-10 md:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Conversation sidebar */}
      <aside
        className={`
          bg-white border-r border-gray-200 w-64 shrink-0 flex flex-col
          fixed inset-y-0 left-0 z-20 pt-14 transition-transform md:relative md:pt-0 md:translate-x-0
          ${sidebarOpen ? "translate-x-0" : "-translate-x-full"}
        `}
      >
        <ConversationList
          activeId={conversationId}
          onSelect={handleSelectConversation}
          onNewConversation={handleNewConversation}
          refreshKey={refreshKey}
        />
      </aside>

      {/* Chat area */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Mobile toggle for conversation list */}
        <div className="md:hidden border-b border-gray-200 px-3 py-2">
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="text-sm text-gray-500 hover:text-gray-700 flex items-center gap-1"
          >
            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 6h16M4 12h16M4 18h16" />
            </svg>
            Conversations
          </button>
        </div>

        <ChatPanel
          conversationId={conversationId}
          messages={messages}
          onMessagesUpdate={handleMessagesUpdate}
          onSourceClick={handleSourceClick}
        />
      </div>
    </div>
  );
}
