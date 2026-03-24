import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";

function todayStr(): string {
  return new Date().toISOString().slice(0, 10);
}

export default function ClipPage() {
  const navigate = useNavigate();
  const [status, setStatus] = useState<"loading" | "error">("loading");
  const [errorMsg, setErrorMsg] = useState("");

  useEffect(() => {
    const raw = window.location.hash.slice(1);
    if (!raw) {
      setStatus("error");
      setErrorMsg("No URL provided. Use the bookmarklet to clip a page.");
      return;
    }

    let targetURL: string;
    try {
      targetURL = decodeURIComponent(raw);
    } catch {
      setStatus("error");
      setErrorMsg("Invalid URL in hash fragment.");
      return;
    }

    let cancelled = false;

    async function clip() {
      try {
        const resp = await fetch("/api/clip", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ url: targetURL }),
        });

        if (cancelled) return;

        if (!resp.ok) {
          const text = await resp.text();
          setStatus("error");
          setErrorMsg(text || `Server error (${resp.status})`);
          return;
        }

        const data = await resp.json();
        navigate(`/day/${data.date}`, { replace: true });
      } catch (err) {
        if (cancelled) return;
        setStatus("error");
        setErrorMsg(err instanceof Error ? err.message : "Network error");
      }
    }

    clip();
    return () => {
      cancelled = true;
    };
  }, [navigate]);

  if (status === "error") {
    return (
      <div className="flex flex-col items-center justify-center flex-1 px-4 py-12">
        <div className="max-w-md w-full text-center space-y-4">
          <div className="text-4xl">⚠️</div>
          <h2 className="text-lg font-medium text-foreground">
            Clip failed
          </h2>
          <p className="text-sm text-muted-foreground">{errorMsg}</p>
          <button
            onClick={() => navigate(`/day/${todayStr()}`)}
            className="px-4 py-2 text-sm font-medium text-foreground bg-muted hover:bg-muted/80 rounded-lg transition-colors"
          >
            Go to today's note
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex flex-col items-center justify-center flex-1 px-4 py-12">
      <div className="max-w-md w-full text-center space-y-4">
        <svg
          className="w-6 h-6 mx-auto animate-spin text-muted-foreground"
          viewBox="0 0 24 24"
          fill="none"
        >
          <circle
            className="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeWidth="4"
          />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8v4a4 4 0 00-4 4H4z"
          />
        </svg>
        <p className="text-sm text-muted-foreground">Clipping…</p>
      </div>
    </div>
  );
}
