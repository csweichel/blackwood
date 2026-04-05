import { useEffect, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import {
  getSubpage,
  updateSubpageContent,
  listSubpages,
} from "../api/client";
import NoteEditor from "./NoteEditor";

interface SubpageViewProps {
  date: string;
  name: string;
}

function formatDateHeading(dateStr: string): string {
  const d = new Date(dateStr + "T00:00:00");
  return d.toLocaleDateString(undefined, {
    weekday: "long",
    year: "numeric",
    month: "long",
    day: "numeric",
  });
}

export default function SubpageView({ date, name }: SubpageViewProps) {
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [autoEdit, setAutoEdit] = useState(false);
  const [existingSubpages, setExistingSubpages] = useState<Set<string>>(
    new Set()
  );

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    setAutoEdit(false);
    try {
      const [data, subpagesResp] = await Promise.all([
        getSubpage(date, name),
        listSubpages(date).catch(() => ({ names: [] as string[] })),
      ]);
      setContent(data.content ?? "");
      setExistingSubpages(new Set(subpagesResp.names ?? []));
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load";
      if (
        msg.includes("not_found") ||
        msg.includes("not found") ||
        msg.includes("404")
      ) {
        try {
          await updateSubpageContent(date, name, "");
          setContent("");
          setAutoEdit(true);
        } catch (createErr) {
          setError(
            createErr instanceof Error
              ? createErr.message
              : "Failed to create subpage"
          );
        }
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }, [date, name]);

  useEffect(() => {
    load();
  }, [load]);

  const handleSave = useCallback(
    async (text: string) => {
      await updateSubpageContent(date, name, text);
    },
    [date, name]
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <div className="text-muted-foreground text-sm">Loading...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-muted border border-destructive/30 rounded-lg p-4 text-destructive text-sm">
        {error}
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <nav className="flex items-center gap-1.5 text-sm text-muted-foreground mb-3">
        <Link
          to={`/day/${date}`}
          className="hover:text-foreground transition-colors"
        >
          {formatDateHeading(date)}
        </Link>
        <span>›</span>
        <span className="text-foreground font-medium">{name}</span>
      </nav>

      <NoteEditor
        title={<h2 className="text-lg md:text-xl font-semibold text-foreground truncate">{name}</h2>}
        content={content}
        onContentChange={setContent}
        onSave={handleSave}
        onEntryCreated={load}
        date={date}
        existingSubpages={existingSubpages}
        emptyMessage="No content yet. Click to start writing."
        startInEditMode={autoEdit}
        showAttach={false}
      />
    </div>
  );
}
