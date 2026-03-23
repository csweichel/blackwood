import { useEffect, useState } from "react";
import type { Conversation } from "../api/types";
import { listConversations } from "../api/client";

interface ConversationListProps {
  activeId: string;
  onSelect: (conversation: Conversation) => void;
  onNewConversation: () => void;
  refreshKey: number;
}

function formatDate(dateStr: string): string {
  if (!dateStr) return "";
  const d = new Date(dateStr);
  const now = new Date();
  const diff = now.getTime() - d.getTime();
  const days = Math.floor(diff / (1000 * 60 * 60 * 24));

  if (days === 0) return "Today";
  if (days === 1) return "Yesterday";
  if (days < 7) return `${days}d ago`;
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

export default function ConversationList({ activeId, onSelect, onNewConversation, refreshKey }: ConversationListProps) {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    listConversations(50, 0)
      .then((res) => {
        if (!cancelled) {
          setConversations(res.conversations || []);
        }
      })
      .catch(() => {
        if (!cancelled) setConversations([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [refreshKey]);

  return (
    <div className="flex flex-col h-full">
      <div className="p-3 border-b border-border">
        <button
          onClick={onNewConversation}
          className="w-full flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-accent bg-muted rounded-lg hover:bg-border transition-colors"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New conversation
        </button>
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading && (
          <div className="p-4 text-center text-sm text-muted-foreground">Loading...</div>
        )}

        {!loading && conversations.length === 0 && (
          <div className="p-4 text-center text-sm text-muted-foreground">No conversations yet</div>
        )}

        {conversations.map((conv) => (
          <button
            key={conv.id}
            onClick={() => onSelect(conv)}
            className={`w-full text-left px-3 py-3 border-b border-border hover:bg-muted transition-colors ${
              conv.id === activeId ? "bg-muted border-l-2 border-l-accent" : ""
            }`}
          >
            <p className="text-sm font-medium text-foreground truncate">
              {conv.title || "Untitled"}
            </p>
            <p className="text-xs text-muted-foreground mt-0.5">
              {formatDate(conv.updatedAt || conv.createdAt)}
            </p>
          </button>
        ))}
      </div>
    </div>
  );
}
