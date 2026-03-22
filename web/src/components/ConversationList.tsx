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
      <div className="p-3 border-b border-gray-200">
        <button
          onClick={onNewConversation}
          className="w-full flex items-center justify-center gap-2 px-3 py-2 text-sm font-medium text-blue-600 bg-blue-50 rounded-lg hover:bg-blue-100 transition-colors"
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          New conversation
        </button>
      </div>

      <div className="flex-1 overflow-y-auto">
        {loading && (
          <div className="p-4 text-center text-sm text-gray-400">Loading...</div>
        )}

        {!loading && conversations.length === 0 && (
          <div className="p-4 text-center text-sm text-gray-400">No conversations yet</div>
        )}

        {conversations.map((conv) => (
          <button
            key={conv.id}
            onClick={() => onSelect(conv)}
            className={`w-full text-left px-3 py-3 border-b border-gray-100 hover:bg-gray-50 transition-colors ${
              conv.id === activeId ? "bg-blue-50 border-l-2 border-l-blue-500" : ""
            }`}
          >
            <p className="text-sm font-medium text-gray-900 truncate">
              {conv.title || "Untitled"}
            </p>
            <p className="text-xs text-gray-400 mt-0.5">
              {formatDate(conv.updatedAt || conv.createdAt)}
            </p>
          </button>
        ))}
      </div>
    </div>
  );
}
