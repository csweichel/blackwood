import { useState, useCallback, useEffect, useRef } from "react";
import { useNavigate } from "react-router-dom";
import type { ChatMessage, Conversation } from "../api/types";
import { getConversation, listConversations } from "../api/client";
import { conversationSlug } from "../lib/slugify";
import ChatPanel from "./ChatPanel";
import ConversationList from "./ConversationList";

interface ChatViewProps {
  slug?: string;
  onNavigateToDate: (date: string) => void;
}

export default function ChatView({ slug, onNavigateToDate }: ChatViewProps) {
  const navigate = useNavigate();
  const [conversationId, setConversationId] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [refreshKey, setRefreshKey] = useState(0);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const initialLoadDone = useRef(false);

  // Resolve slug to a conversation on mount or when slug changes
  useEffect(() => {
    let cancelled = false;

    async function resolve() {
      const convs = (await listConversations(50, 0)).conversations || [];

      if (cancelled) return;

      if (slug) {
        // Find conversation matching the slug
        const match = convs.find((c) => conversationSlug(c) === slug);
        if (match) {
          const full = await getConversation(match.id);
          if (!cancelled) {
            setConversationId(full.id);
            setMessages(full.messages || []);
          }
          return;
        }
      }

      // No slug or no match: load most recent conversation (only on first load)
      if (!initialLoadDone.current && convs.length > 0) {
        const full = await getConversation(convs[0].id);
        if (!cancelled) {
          setConversationId(full.id);
          setMessages(full.messages || []);
          navigate(`/chat/${conversationSlug(convs[0])}`, { replace: true });
        }
      }
    }

    initialLoadDone.current = true;
    resolve().catch(() => {});

    return () => {
      cancelled = true;
    };
  }, [slug, navigate]);

  const handleNewConversation = useCallback(() => {
    setConversationId("");
    setMessages([]);
    setSidebarOpen(false);
    navigate("/chat");
  }, [navigate]);

  const handleSelectConversation = useCallback(
    async (conv: Conversation) => {
      setSidebarOpen(false);
      try {
        const full = await getConversation(conv.id);
        setConversationId(full.id);
        setMessages(full.messages || []);
      } catch {
        setConversationId(conv.id);
        setMessages([]);
      }
      navigate(`/chat/${conversationSlug(conv)}`);
    },
    [navigate]
  );

  const handleMessagesUpdate = useCallback(
    (updatedMessages: ChatMessage[], newConversationId: string) => {
      setMessages(updatedMessages);
      if (newConversationId && newConversationId !== conversationId) {
        setConversationId(newConversationId);
        // Refresh conversation list when a new conversation is created
        setRefreshKey((k) => k + 1);
      }
    },
    [conversationId]
  );

  const handleSourceClick = useCallback(
    (date: string) => {
      onNavigateToDate(date);
    },
    [onNavigateToDate]
  );

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
          bg-card border-r border-border w-64 shrink-0 flex flex-col
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
        <div className="md:hidden border-b border-border px-3 py-2">
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="text-sm text-muted-foreground hover:text-foreground flex items-center gap-1"
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
