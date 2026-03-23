import { useEffect, useState, useCallback, useRef } from "react";
import Markdown from "react-markdown";
import rehypeRaw from "rehype-raw";
import { visit } from "unist-util-visit";
import { getDailyNote, updateDailyNoteContent } from "../api/client";
import EntryForm from "./EntryForm";
import MarkdownEditor from "./MarkdownEditor";
import AudioRecorder from "./AudioRecorder";
import PhotoCapture from "./PhotoCapture";

/**
 * Rehype plugin that converts standalone YouTube URLs in paragraphs
 * into responsive embedded iframes using youtube-nocookie.com.
 */
function rehypeYoutubeEmbed() {
  const YT_REGEX =
    /^https?:\/\/(?:www\.)?(?:youtube\.com\/watch\?v=|youtu\.be\/)([\w-]+)(?:[&?].*)?$/;

  function extractVideoId(url: string): { id: string; start?: string } | null {
    const match = url.match(YT_REGEX);
    if (!match) return null;
    const id = match[1];
    // Extract t= or start= parameter
    try {
      const parsed = new URL(url);
      const t = parsed.searchParams.get("t") ?? parsed.searchParams.get("start");
      return { id, start: t ?? undefined };
    } catch {
      return { id };
    }
  }

  function isSoleYoutubeUrl(paragraph: any): { id: string; start?: string } | null {
    const children = paragraph.children?.filter(
      (c: any) => !(c.type === "text" && c.value.trim() === "")
    );
    if (!children || children.length !== 1) return null;
    const child = children[0];

    // Text node containing a bare URL
    if (child.type === "text") {
      return extractVideoId(child.value.trim());
    }
    // <a> element wrapping the URL (markdown autolink)
    if (child.type === "element" && child.tagName === "a") {
      const href = child.properties?.href ?? "";
      return extractVideoId(href.trim());
    }
    return null;
  }

  return (tree: any) => {
    visit(tree, "element", (node: any, index: number | undefined, parent: any) => {
      if (index === undefined || !parent) return;
      if (node.tagName !== "p") return;

      const result = isSoleYoutubeUrl(node);
      if (!result) return;

      let src = `https://www.youtube-nocookie.com/embed/${result.id}`;
      if (result.start) src += `?start=${result.start}`;

      const embedNode = {
        type: "element",
        tagName: "div",
        properties: {
          style:
            "position:relative;padding-bottom:56.25%;height:0;overflow:hidden;border-radius:0.5rem;margin:0.75em 0",
        },
        children: [
          {
            type: "element",
            tagName: "iframe",
            properties: {
              src,
              style: "position:absolute;top:0;left:0;width:100%;height:100%",
              frameBorder: "0",
              allow:
                "accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture",
              allowFullScreen: true,
            },
            children: [],
          },
        ],
      };

      parent.children.splice(index, 1, embedNode);
    });
  };
}

/**
 * Rehype plugin that makes headings and nested list items collapsible
 * by wrapping them in <details open> / <summary> elements.
 */
function rehypeCollapsible() {
  const HEADING_TAGS = new Set(["h1", "h2", "h3", "h4", "h5", "h6"]);

  function headingLevel(tagName: string): number {
    return parseInt(tagName.charAt(1), 10);
  }

  function isHeading(node: any): boolean {
    return node.type === "element" && HEADING_TAGS.has(node.tagName);
  }

  function wrapHeadingSections(parent: any) {
    const newChildren: any[] = [];
    let i = 0;

    while (i < parent.children.length) {
      const node = parent.children[i];

      if (!isHeading(node)) {
        newChildren.push(node);
        i++;
        continue;
      }

      const level = headingLevel(node.tagName);
      const sectionContent: any[] = [];
      let j = i + 1;

      // Collect siblings until the next heading of equal or higher level
      while (j < parent.children.length) {
        const sibling = parent.children[j];
        if (isHeading(sibling) && headingLevel(sibling.tagName) <= level) {
          break;
        }
        sectionContent.push(sibling);
        j++;
      }

      const detailsNode = {
        type: "element",
        tagName: "details",
        properties: { open: true },
        children: [
          {
            type: "element",
            tagName: "summary",
            properties: {},
            children: [
              {
                type: "element",
                tagName: node.tagName,
                properties: { ...(node.properties || {}) },
                children: [...(node.children || [])],
              },
            ],
          },
          ...sectionContent,
        ],
      };

      newChildren.push(detailsNode);
      i = j;
    }

    parent.children = newChildren;
  }

  function wrapListItems(tree: any) {
    visit(tree, "element", (node: any) => {
      if (node.tagName !== "li") return;

      const hasNestedList = node.children?.some(
        (c: any) =>
          c.type === "element" && (c.tagName === "ul" || c.tagName === "ol")
      );
      if (!hasNestedList) return;

      const summaryContent: any[] = [];
      const detailsBody: any[] = [];

      for (const child of node.children) {
        if (
          child.type === "element" &&
          (child.tagName === "ul" || child.tagName === "ol")
        ) {
          detailsBody.push(child);
        } else {
          summaryContent.push(child);
        }
      }

      node.children = [
        {
          type: "element",
          tagName: "details",
          properties: { open: true },
          children: [
            {
              type: "element",
              tagName: "summary",
              properties: {},
              children: summaryContent,
            },
            ...detailsBody,
          ],
        },
      ];
    });
  }

  return (tree: any) => {
    // Process list items first (deeper in the tree), then headings at the top level
    wrapListItems(tree);
    wrapHeadingSections(tree);
  };
}

/**
 * Remark plugin that converts Obsidian-style [[wikilinks]] into
 * <span class="wikilink"> elements for styled rendering.
 */
function remarkWikilinks() {
  return (tree: any) => {
    visit(tree, "text", (node: any, index: number | undefined, parent: any) => {
      if (index === undefined || !parent) return;
      const regex = /\[\[([^\]]+)\]\]/g;
      const value: string = node.value;
      if (!regex.test(value)) return;

      // Reset regex after test
      regex.lastIndex = 0;
      const children: any[] = [];
      let lastIndex = 0;
      let match: RegExpExecArray | null;

      while ((match = regex.exec(value)) !== null) {
        // Text before the match
        if (match.index > lastIndex) {
          children.push({ type: "text", value: value.slice(lastIndex, match.index) });
        }
        // The wikilink as an inline HTML node
        children.push({
          type: "html",
          value: `<span class="wikilink">${match[1]}</span>`,
        });
        lastIndex = regex.lastIndex;
      }

      // Remaining text after last match
      if (lastIndex < value.length) {
        children.push({ type: "text", value: value.slice(lastIndex) });
      }

      parent.children.splice(index, 1, ...children);
    });
  };
}

interface DailyNoteViewProps {
  date: string;
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

type SaveStatus = "idle" | "saving" | "saved" | "error";

export default function DailyNoteView({ date }: DailyNoteViewProps) {
  const [content, setContent] = useState("");
  const [editContent, setEditContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [saveStatus, setSaveStatus] = useState<SaveStatus>("idle");
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await getDailyNote({ date });
      setContent(data.content ?? "");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Failed to load";
      if (msg.includes("404") || msg.includes("not found") || msg.includes("not_found")) {
        setContent("");
      } else {
        setError(msg);
      }
    } finally {
      setLoading(false);
    }
  }, [date]);

  useEffect(() => {
    load();
    return () => {
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    };
  }, [load]);

  const [showRecorder, setShowRecorder] = useState(false);
  const [showCamera, setShowCamera] = useState(false);
  const [pdfLoading, setPdfLoading] = useState(false);

  // Reset editing state when date changes
  useEffect(() => {
    setEditing(false);
    setShowRecorder(false);
    setShowCamera(false);
  }, [date]);


  const doSave = useCallback(
    async (text: string) => {
      setSaveStatus("saving");
      try {
        await updateDailyNoteContent(date, text);
        setSaveStatus("saved");
        setTimeout(() => setSaveStatus((s) => (s === "saved" ? "idle" : s)), 2000);
      } catch {
        setSaveStatus("error");
      }
    },
    [date]
  );

  const downloadPdf = useCallback(async () => {
    setPdfLoading(true);
    try {
      const resp = await fetch(`/api/daily-notes/${date}/pdf`);
      if (!resp.ok) throw new Error("PDF generation failed");
      const blob = await resp.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `${date}.pdf`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      console.error("PDF download failed:", err);
    } finally {
      setPdfLoading(false);
    }
  }, [date]);

  function handleEditChange(text: string) {
    setEditContent(text);
    setSaveStatus("idle");

    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      doSave(text);
    }, 1000);
  }

  function startEditing() {
    setEditContent(content);
    setEditing(true);
  }

  function handleSave() {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    setContent(editContent);
    setEditing(false);
    doSave(editContent);
  }

  function handleCancel() {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    setEditing(false);
    setSaveStatus("idle");
  }

  async function handleEntryCreated() {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    await load();
  }

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
      <div className="flex items-center justify-between mb-3 md:mb-4">
        <h2 className="text-lg md:text-xl font-semibold text-foreground">
          {formatDateHeading(date)}
        </h2>
        <div className="flex items-center gap-3">
          <span
            className={`text-xs transition-opacity ${
              saveStatus === "idle" ? "opacity-0" : "opacity-100"
            } ${
              saveStatus === "saving"
                ? "text-muted-foreground"
                : saveStatus === "saved"
                ? "text-accent"
                : saveStatus === "error"
                ? "text-destructive"
                : ""
            }`}
          >
            {saveStatus === "saving"
              ? "Saving..."
              : saveStatus === "saved"
              ? "Saved"
              : saveStatus === "error"
              ? "Save failed"
              : ""}
          </span>
          {content.trim() && (
            <button
              onClick={downloadPdf}
              disabled={pdfLoading}
              className={`p-1.5 rounded-md transition-colors ${pdfLoading ? "text-muted-foreground opacity-50 cursor-wait" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
              title="Download as PDF"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="12" x2="12" y1="18" y2="12"/><polyline points="9 15 12 18 15 15"/></svg>
            </button>
          )}
          <button
            onClick={() => { setShowRecorder((v) => !v); setShowCamera(false); }}
            className={`p-1.5 rounded-md transition-colors ${showRecorder ? "text-accent bg-muted" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
            title="Record audio"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" x2="12" y1="19" y2="22"/></svg>
          </button>
          <button
            onClick={() => { setShowCamera((v) => !v); setShowRecorder(false); }}
            className={`p-1.5 rounded-md transition-colors ${showCamera ? "text-accent bg-muted" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
            title="Take photo"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z"/><circle cx="12" cy="13" r="3"/></svg>
          </button>
          {!editing ? (
            <button
              onClick={startEditing}
              className="px-3 py-1.5 text-xs font-medium text-muted-foreground bg-muted rounded-md hover:bg-border transition-colors"
            >
              Edit
            </button>
          ) : (
            <div className="flex items-center gap-2">
              <button
                onClick={handleCancel}
                className="px-3 py-1.5 text-xs font-medium text-muted-foreground bg-muted rounded-md hover:bg-border transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleSave}
                className="px-3 py-1.5 text-xs font-medium text-primary-foreground bg-primary rounded-md hover:opacity-90 transition-colors"
              >
                Done
              </button>
            </div>
          )}
        </div>
      </div>

      {showRecorder && (
        <div className="mb-4">
          <AudioRecorder
            date={date}
            onCreated={() => { setShowRecorder(false); handleEntryCreated(); }}
            onClose={() => setShowRecorder(false)}
            autoStart
          />
        </div>
      )}

      {showCamera && (
        <div className="mb-4">
          <PhotoCapture
            date={date}
            onCreated={() => { setShowCamera(false); handleEntryCreated(); }}
            onClose={() => setShowCamera(false)}
          />
        </div>
      )}

      <div className="flex-1 min-h-0 overflow-y-auto mb-4">
        {editing ? (
          <MarkdownEditor
            value={editContent}
            onChange={handleEditChange}
            onSubmit={handleSave}
            placeholder="Start writing..."
            autoFocus
          />
        ) : content.trim() ? (
          <div
            className="prose prose-sm max-w-none note-prose note-container"
            onClick={startEditing}
          >
            <Markdown remarkPlugins={[remarkWikilinks]} rehypePlugins={[rehypeRaw, rehypeYoutubeEmbed, rehypeCollapsible]}>
              {content}
            </Markdown>
          </div>
        ) : (
          <div className="note-empty" onClick={startEditing}>
            <p className="text-muted-foreground text-sm">
              No entries yet. Click to start writing, or add an entry below.
            </p>
          </div>
        )}
      </div>

      <EntryForm date={date} onCreated={handleEntryCreated} />

      <div className="mt-12 pt-6 border-t border-border">
        <p className="text-xs text-muted-foreground">
          <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+T</kbd> insert time
          <span className="mx-2">&middot;</span>
          <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+Enter</kbd> done editing
          <span className="mx-2">&middot;</span>
          <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Esc</kbd> exit edit
        </p>
      </div>
    </div>
  );
}
