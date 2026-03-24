import { useEffect, useState, useRef, useCallback } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";

interface SearchResult {
  entry_id: string;
  date: string;
  snippet: string;
  score: number;
}

async function fetchSearch(query: string, limit = 20): Promise<SearchResult[]> {
  const resp = await fetch(`/api/search?q=${encodeURIComponent(query)}&limit=${limit}`);
  if (!resp.ok) throw new Error("Search failed");
  const data = await resp.json();
  return data.results || [];
}

function highlightSnippet(snippet: string, query: string): string {
  if (!query.trim()) return snippet;
  const words = query.trim().split(/\s+/).filter(Boolean);
  const pattern = words.map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")).join("|");
  return snippet.replace(new RegExp(`(${pattern})`, "gi"), "<mark>$1</mark>");
}

export default function SearchPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState(searchParams.get("q") || "");
  const [results, setResults] = useState<SearchResult[]>([]);
  const [loading, setLoading] = useState(false);
  const [searched, setSearched] = useState(false);

  const doSearch = useCallback(async (q: string) => {
    if (!q.trim()) {
      setResults([]);
      setSearched(false);
      return;
    }
    setLoading(true);
    setSearched(true);
    try {
      const data = await fetchSearch(q);
      setResults(data);
    } catch {
      setResults([]);
    } finally {
      setLoading(false);
    }
  }, []);

  // Search on mount if query param present.
  useEffect(() => {
    const q = searchParams.get("q");
    if (q) {
      setQuery(q);
      doSearch(q);
    }
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Listen for focus-search custom event.
  useEffect(() => {
    function handleFocus() {
      inputRef.current?.focus();
    }
    window.addEventListener("focus-search", handleFocus);
    return () => window.removeEventListener("focus-search", handleFocus);
  }, []);

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSearchParams(query.trim() ? { q: query.trim() } : {});
    doSearch(query.trim());
  }

  // Group results by date.
  const grouped = results.reduce<Record<string, SearchResult[]>>((acc, r) => {
    const key = r.date || "Unknown";
    if (!acc[key]) acc[key] = [];
    acc[key].push(r);
    return acc;
  }, {});

  const sortedDates = Object.keys(grouped).sort().reverse();

  return (
    <div className="flex flex-col flex-1 overflow-hidden">
      <div className="max-w-4xl mx-auto px-4 md:px-6 py-6 w-full flex-1 overflow-y-auto">
        <form onSubmit={handleSubmit} className="mb-6">
          <div className="relative">
            <svg
              className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
              xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"
            >
              <circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>
            </svg>
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search your notes..."
              className="w-full pl-10 pr-4 py-2.5 bg-muted border border-border rounded-lg text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-accent/50"
              autoFocus
            />
          </div>
        </form>

        {loading && (
          <div className="flex items-center justify-center py-12">
            <div className="text-muted-foreground text-sm">Searching...</div>
          </div>
        )}

        {!loading && searched && results.length === 0 && (
          <div className="text-center py-12">
            <p className="text-muted-foreground text-sm">No results found for "{query}"</p>
          </div>
        )}

        {!loading && results.length > 0 && (
          <div>
            <p className="text-xs text-muted-foreground mb-4">
              {results.length} result{results.length !== 1 ? "s" : ""}
            </p>
            <div className="space-y-6">
              {sortedDates.map((date) => (
                <div key={date}>
                  <button
                    onClick={() => navigate(`/day/${date}`)}
                    className="text-sm font-medium text-accent hover:underline mb-2 block"
                  >
                    {new Date(date + "T00:00:00").toLocaleDateString(undefined, {
                      weekday: "long",
                      year: "numeric",
                      month: "long",
                      day: "numeric",
                    })}
                  </button>
                  <div className="space-y-2">
                    {grouped[date].map((r) => (
                      <div
                        key={r.entry_id}
                        className="p-3 border border-border rounded-lg hover:bg-muted/50 transition-colors cursor-pointer"
                        onClick={() => navigate(`/day/${date}`)}
                      >
                        <p
                          className="text-sm text-foreground [&_mark]:bg-accent/30 [&_mark]:text-foreground [&_mark]:rounded-sm [&_mark]:px-0.5"
                          dangerouslySetInnerHTML={{
                            __html: highlightSnippet(r.snippet, query),
                          }}
                        />
                        <div className="flex items-center gap-2 mt-1">
                          <span
                            className={`inline-block w-2 h-2 rounded-full ${
                              r.score > 0.8
                                ? "bg-green-500"
                                : r.score > 0.6
                                ? "bg-yellow-500"
                                : "bg-muted-foreground"
                            }`}
                          />
                          <span className="text-xs text-muted-foreground">
                            {(r.score * 100).toFixed(0)}% match
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {!loading && !searched && (
          <div className="text-center py-12">
            <p className="text-muted-foreground text-sm">
              Search across all your notes using semantic search.
            </p>
            <p className="text-muted-foreground text-xs mt-2">
              <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+K</kbd> to focus search from anywhere
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
