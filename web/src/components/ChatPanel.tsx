import { useState, useRef, useEffect } from "react";
import type { ChatMessage, SourceReference } from "../api/types";
import { streamChat } from "../api/client";

interface ChatPanelProps {
  conversationId: string;
  messages: ChatMessage[];
  onMessagesUpdate: (messages: ChatMessage[], conversationId: string) => void;
  onSourceClick: (date: string) => void;
}

function TypingIndicator() {
  return (
    <div className="flex items-center gap-1 px-4 py-3">
      <div className="w-2 h-2 bg-gray-400 rounded-full animate-[pulse_1s_ease-in-out_0s_infinite]" />
      <div className="w-2 h-2 bg-gray-400 rounded-full animate-[pulse_1s_ease-in-out_0.2s_infinite]" />
      <div className="w-2 h-2 bg-gray-400 rounded-full animate-[pulse_1s_ease-in-out_0.4s_infinite]" />
    </div>
  );
}

function SourceChips({ sources, onSourceClick }: { sources: SourceReference[]; onSourceClick: (date: string) => void }) {
  if (!sources || sources.length === 0) return null;

  // Deduplicate by date
  const seen = new Set<string>();
  const unique = sources.filter((s) => {
    if (seen.has(s.dailyNoteDate)) return false;
    seen.add(s.dailyNoteDate);
    return true;
  });

  return (
    <div className="flex flex-wrap gap-1.5 mt-2">
      {unique.map((source) => (
        <button
          key={source.entryId}
          onClick={() => onSourceClick(source.dailyNoteDate)}
          className="inline-flex items-center gap-1 px-2 py-0.5 text-xs bg-blue-50 text-blue-700 rounded-full hover:bg-blue-100 transition-colors border border-blue-200"
          title={source.snippet}
        >
          <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
          </svg>
          {source.dailyNoteDate}
        </button>
      ))}
    </div>
  );
}

function MessageBubble({ message, onSourceClick }: { message: ChatMessage; onSourceClick: (date: string) => void }) {
  const isUser = message.role === "user";

  return (
    <div className={`flex ${isUser ? "justify-end" : "justify-start"} mb-3`}>
      <div
        className={`max-w-[80%] rounded-2xl px-4 py-2.5 ${
          isUser
            ? "bg-blue-600 text-white rounded-br-md"
            : "bg-white text-gray-900 border border-gray-200 rounded-bl-md"
        }`}
      >
        <p className="text-sm whitespace-pre-wrap break-words">{message.content}</p>
        {!isUser && <SourceChips sources={message.sources} onSourceClick={onSourceClick} />}
      </div>
    </div>
  );
}

export default function ChatPanel({ conversationId, messages, onMessagesUpdate, onSourceClick }: ChatPanelProps) {
  const [input, setInput] = useState("");
  const [isStreaming, setIsStreaming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, isStreaming]);

  useEffect(() => {
    inputRef.current?.focus();
  }, [conversationId]);

  async function handleSend() {
    const text = input.trim();
    if (!text || isStreaming) return;

    setInput("");
    setError(null);

    const userMessage: ChatMessage = {
      id: crypto.randomUUID(),
      role: "user",
      content: text,
      sources: [],
      createdAt: new Date().toISOString(),
    };

    const updatedMessages = [...messages, userMessage];
    onMessagesUpdate(updatedMessages, conversationId);

    setIsStreaming(true);

    let assistantContent = "";
    let assistantSources: SourceReference[] = [];
    let currentConvId = conversationId;

    try {
      for await (const chunk of streamChat(conversationId, text)) {
        if (chunk.conversationId) {
          currentConvId = chunk.conversationId;
        }
        assistantContent += chunk.content;
        if (chunk.done && chunk.sources) {
          assistantSources = chunk.sources;
        }

        const assistantMessage: ChatMessage = {
          id: "streaming",
          role: "assistant",
          content: assistantContent,
          sources: chunk.done ? assistantSources : [],
          createdAt: new Date().toISOString(),
        };

        onMessagesUpdate([...updatedMessages, assistantMessage], currentConvId);
      }

      // Finalize with a stable ID
      const finalMessage: ChatMessage = {
        id: crypto.randomUUID(),
        role: "assistant",
        content: assistantContent,
        sources: assistantSources,
        createdAt: new Date().toISOString(),
      };
      onMessagesUpdate([...updatedMessages, finalMessage], currentConvId);
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to send message";
      if (msg.includes("not available") || msg.includes("not configured")) {
        setError("Chat is not available. Configure an OpenAI API key to enable it.");
      } else {
        setError(msg);
      }
    } finally {
      setIsStreaming(false);
    }
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Messages */}
      <div className="flex-1 overflow-y-auto px-4 py-4">
        {messages.length === 0 && !isStreaming && (
          <div className="flex flex-col items-center justify-center h-full text-gray-400">
            <svg className="w-12 h-12 mb-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
            </svg>
            <p className="text-sm">Ask a question about your notes</p>
          </div>
        )}

        {messages.map((msg) => (
          <MessageBubble key={msg.id} message={msg} onSourceClick={onSourceClick} />
        ))}

        {isStreaming && messages[messages.length - 1]?.role !== "assistant" && <TypingIndicator />}

        {error && (
          <div className="mx-4 mb-3 px-3 py-2 bg-red-50 text-red-700 text-sm rounded-lg border border-red-200">
            {error}
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t border-gray-200 bg-white px-4 py-3">
        <div className="flex items-end gap-2 max-w-2xl mx-auto">
          <textarea
            ref={inputRef}
            value={input}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="Ask about your notes..."
            rows={1}
            disabled={isStreaming}
            className="flex-1 resize-none rounded-xl border border-gray-300 px-4 py-2.5 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent disabled:opacity-50 disabled:bg-gray-50"
            style={{ maxHeight: "120px" }}
            onInput={(e) => {
              const target = e.target as HTMLTextAreaElement;
              target.style.height = "auto";
              target.style.height = Math.min(target.scrollHeight, 120) + "px";
            }}
          />
          <button
            onClick={handleSend}
            disabled={!input.trim() || isStreaming}
            className="shrink-0 w-10 h-10 flex items-center justify-center rounded-xl bg-blue-600 text-white hover:bg-blue-700 disabled:opacity-40 disabled:hover:bg-blue-600 transition-colors"
          >
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 19V5m0 0l-7 7m7-7l7 7" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  );
}
