import { useEffect, useState, useCallback } from "react";
import { Link } from "react-router-dom";
import {
  getSubpage,
  RPCError,
  updateSubpageContent,
  listSubpages,
} from "../api/client";
import { subscribeToChanges } from "../lib/changeEvents";
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
  const [revision, setRevision] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [autoEdit, setAutoEdit] = useState(false);
  const [isEditing, setIsEditing] = useState(false);
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
      setRevision(data.revision ?? "");
      setExistingSubpages(new Set(subpagesResp.names ?? []));
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load";
      if (
        msg.includes("not_found") ||
        msg.includes("not found") ||
        msg.includes("404")
      ) {
        try {
          const created = await updateSubpageContent(date, name, "", "");
          setContent("");
          setRevision(created.revision ?? "");
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

  useEffect(() => {
    return subscribeToChanges((event) => {
      if (event.date !== date || event.subpageName !== name) return;
      if (event.kind === "CHANGE_EVENT_KIND_SUBPAGE_UPDATED" && !isEditing && event.revision !== revision) {
        void load();
      }
    });
  }, [date, isEditing, load, name, revision]);

  const handleSave = useCallback(
    async (text: string) => {
      try {
        const updated = await updateSubpageContent(date, name, text, revision);
        setContent(updated.content ?? text);
        setRevision(updated.revision ?? revision);
        setError(null);
      } catch (err) {
        if (err instanceof RPCError && err.code === "failed_precondition") {
          setError("This subpage changed on another client. The latest version has been reloaded.");
          await load();
        }
        throw err;
      }
    },
    [date, load, name, revision]
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
        onEditingChange={setIsEditing}
        date={date}
        existingSubpages={existingSubpages}
        emptyMessage="No content yet. Click to start writing."
        startInEditMode={autoEdit}
        showAttach={false}
      />
    </div>
  );
}
