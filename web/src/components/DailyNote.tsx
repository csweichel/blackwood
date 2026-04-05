import { useEffect, useState, useCallback, useRef } from "react";
import { visit } from "unist-util-visit";
import { getDailyNote, updateDailyNoteContent, listSubpages } from "../api/client";
import { useGeolocation } from "../hooks/useGeolocation";

import NoteEditor from "./NoteEditor";

/**
 * Rehype plugin that converts standalone YouTube URLs in paragraphs
 * into responsive embedded iframes using youtube-nocookie.com.
 */
export function rehypeYoutubeEmbed() {
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
export function rehypeCollapsible() {
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

      // Collect siblings until the next heading of equal or higher level.
      // An h2 section stops at the next h1 or h2 — it never swallows
      // a heading at a higher (lower-numbered) level.
      while (j < parent.children.length) {
        const sibling = parent.children[j];
        if (isHeading(sibling) && headingLevel(sibling.tagName) <= level) {
          break;
        }
        sectionContent.push(sibling);
        j++;
      }

      // Only wrap if there's content to fold.
      if (sectionContent.length === 0) {
        newChildren.push(node);
        i = j;
        continue;
      }

      // Recursively wrap nested headings within this section's content.
      const wrapper = { type: "root", children: sectionContent };
      wrapHeadingSections(wrapper);

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
          ...wrapper.children,
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
          properties: { open: true, className: "list-collapse" },
          children: [
            {
              type: "element",
              tagName: "summary",
              properties: { className: "list-collapse-summary" },
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
 * clickable <a> elements that navigate to subpages within the same day.
 * Links to existing subpages get class "wikilink"; missing ones get
 * "wikilink wikilink-missing".
 */
export function remarkWikilinks(date: string, existingSubpages: Set<string>) {
  return () => (tree: any) => {
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
        const name = match[1];
        const exists = existingSubpages.has(name);
        const cls = exists ? "wikilink" : "wikilink wikilink-missing";
        const href = `/day/${date}/${encodeURIComponent(name)}`;
        children.push({
          type: "html",
          value: `<a class="${cls}" href="${href}" data-subpage="${name}">${name}</a>`,
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

// All root-level h1 headings are treated as section labels.

/**
 * React SVG icons for section headings, injected via component overrides.
 */
export const SECTION_ICONS_JSX: Record<string, React.ReactNode> = {
  Summary: (
    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/>
    </svg>
  ),
  Notes: (
    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/>
    </svg>
  ),
  Links: (
    <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>
    </svg>
  ),
};

/**
 * Rehype plugin that styles all root-level h1 headings as section labels
 * with an icon and bold title. Works on bare h1 elements and h1 elements
 * inside <summary> (collapsible).
 * Also marks the paragraph after # Summary with a special class.
 */
export function rehypeSectionLabels() {
  return (tree: any) => {
    visit(tree, "element", (node: any, index: number | undefined, parent: any) => {
      if (index === undefined || !parent) return;

      // Case 1: bare h1 (not wrapped by collapsible — e.g. empty section)
      if (node.tagName === "h1" && parent.tagName !== "summary") {
        const text = getTextContent(node).trim();
        const cls = text === "Summary" ? "note-section-label note-section-summary-label" : "note-section-label";
        node.properties = { ...(node.properties || {}), className: cls };
        node.children = [{ type: "text", value: text }];
        // Mark next sibling paragraph for Summary.
        if (text === "Summary" && parent.children[index + 1]?.tagName === "p") {
          const p = parent.children[index + 1];
          const existing = p.properties?.className || "";
          p.properties = { ...p.properties, className: (existing + " note-summary").trim() };
        }
        return;
      }

      // Case 2: h1 inside <summary> (wrapped by collapsible)
      if (node.tagName !== "summary") return;
      const h1 = node.children?.find((c: any) => c.type === "element" && c.tagName === "h1");
      if (!h1) return;
      const text = getTextContent(h1).trim();

      // Add the label class to the h1 but keep it as h1 so collapsible CSS works.
      const cls = text === "Summary" ? "note-section-label note-section-summary-label" : "note-section-label";
      h1.properties = { ...(h1.properties || {}), className: cls };
      h1.children = [{ type: "text", value: text }];

      // Mark the first paragraph inside the <details> as summary text.
      if (text === "Summary" && parent.tagName === "details") {
        const p = parent.children.find(
          (c: any) => c.type === "element" && c.tagName === "p"
        );
        if (p) {
          const existing = p.properties?.className || "";
          p.properties = { ...p.properties, className: (existing + " note-summary").trim() };
        }
      }
    });
  };
}

function getTextContent(node: any): string {
  if (node.type === "text") return node.value || "";
  if (node.children) return node.children.map(getTextContent).join("");
  return "";
}

/**
 * Rehype plugin that assigns a sequential data-checkbox-index to each
 * checkbox input so we can map a click back to the raw markdown.
 */
export function rehypeCheckboxIndex() {
  return (tree: any) => {
    let idx = 0;
    visit(tree, "element", (node: any) => {
      if (
        node.tagName === "input" &&
        node.properties?.type === "checkbox"
      ) {
        node.properties["data-checkbox-index"] = idx;
        idx++;
      }
    });
  };
}

/**
 * Rehype plugin that rewrites relative src/href attributes on img and audio
 * elements to point to the date-based attachment API route. This makes
 * relative filenames (e.g. "photo-abc12345.jpg") written into the markdown
 * resolve correctly in the web UI.
 */
export function rehypeAttachmentUrls(date: string) {
  return () => (tree: any) => {
    visit(tree, "element", (node: any) => {
      if (node.tagName !== "img" && node.tagName !== "audio") return;
      const src = node.properties?.src;
      if (!src || typeof src !== "string") return;
      // Skip absolute URLs and existing API paths.
      if (src.startsWith("/") || src.startsWith("http://") || src.startsWith("https://") || src.startsWith("data:")) return;
      node.properties.src = `/api/daily-notes/${date}/attachments/${encodeURIComponent(src)}`;
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

export default function DailyNoteView({ date }: DailyNoteViewProps) {
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [existingSubpages, setExistingSubpages] = useState<Set<string>>(new Set());

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [data, subpagesResp] = await Promise.all([
        getDailyNote({ date }),
        listSubpages(date).catch(() => ({ names: [] as string[] })),
      ]);
      setContent(data.content ?? "");
      setExistingSubpages(new Set(subpagesResp.names ?? []));
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
  }, [load]);

  const [showOverflowMenu, setShowOverflowMenu] = useState(false);
  const overflowRef = useRef<HTMLDivElement>(null);
  const [pdfLoading, setPdfLoading] = useState(false);
  const [summarizing, setSummarizing] = useState(false);
  const { position: geoPosition, loading: geoLoading, error: geoError, requestLocation } = useGeolocation();
  const [locationTagged, setLocationTagged] = useState(false);

  // Close overflow menu on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (overflowRef.current && !overflowRef.current.contains(e.target as Node)) {
        setShowOverflowMenu(false);
      }
    }
    if (showOverflowMenu) {
      document.addEventListener("mousedown", handleClickOutside);
      return () => document.removeEventListener("mousedown", handleClickOutside);
    }
  }, [showOverflowMenu]);

  // Reset UI state when date changes
  useEffect(() => {
    setShowOverflowMenu(false);
  }, [date]);

  const handleSave = useCallback(
    async (text: string) => {
      await updateDailyNoteContent(date, text);
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

  const generateSummary = useCallback(async () => {
    setSummarizing(true);
    try {
      const resp = await fetch(`/api/daily-notes/${date}/summarize`, {
        method: "POST",
      });
      if (!resp.ok) throw new Error("Summarize failed");
      await load();
    } catch (err) {
      console.error("Summary generation failed:", err);
    } finally {
      setSummarizing(false);
    }
  }, [date, load]);

  // Tag the note with the current location when geolocation resolves.
  useEffect(() => {
    if (!geoPosition || locationTagged) return;
    setLocationTagged(true);
    const { latitude, longitude, address } = geoPosition;
    const ts = new Date().toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
    const mapUrl = `https://www.openstreetmap.org/?mlat=${latitude}&mlon=${longitude}#map=15/${latitude}/${longitude}`;
    const locationLabel = address || `${latitude.toFixed(4)}, ${longitude.toFixed(4)}`;
    const snippet = `\n\n---\n*${ts} — 📍 [${locationLabel}](${mapUrl})*\n`;
    const newContent = content + snippet;
    setContent(newContent);
    handleSave(newContent);
  }, [geoPosition, locationTagged, content, handleSave]);



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

  const toolbarExtra = (
    <>
      {content.trim() && (
        <div className="relative" ref={overflowRef}>
          <button
            onClick={() => setShowOverflowMenu((v) => !v)}
            className={`p-1.5 rounded-md transition-colors ${showOverflowMenu ? "text-accent bg-muted" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
            title="More actions"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="1"/><circle cx="12" cy="5" r="1"/><circle cx="12" cy="19" r="1"/></svg>
          </button>
          {showOverflowMenu && (
            <div className="absolute right-0 top-full mt-1 bg-card border border-border rounded-lg shadow-lg z-50 min-w-[180px]">
              <button
                onClick={() => { setShowOverflowMenu(false); generateSummary(); }}
                disabled={summarizing}
                className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left disabled:opacity-50"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3l1.912 5.813a2 2 0 0 0 1.275 1.275L21 12l-5.813 1.912a2 2 0 0 0-1.275 1.275L12 21l-1.912-5.813a2 2 0 0 0-1.275-1.275L3 12l5.813-1.912a2 2 0 0 0 1.275-1.275L12 3z"/></svg>
                {summarizing ? "Summarising…" : "Generate summary"}
              </button>
              <button
                onClick={() => { setShowOverflowMenu(false); downloadPdf(); }}
                disabled={pdfLoading}
                className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left disabled:opacity-50"
              >
                <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="12" x2="12" y1="18" y2="12"/><polyline points="9 15 12 18 15 15"/></svg>
                {pdfLoading ? "Exporting…" : "Export as PDF"}
              </button>
            </div>
          )}
        </div>
      )}
    </>
  );

  const attachMenuExtra = (
    <button
      onClick={() => requestLocation()}
      disabled={geoLoading || locationTagged}
      className={`flex items-center gap-2 px-3 py-2 text-sm w-full text-left ${locationTagged ? "text-accent" : geoLoading ? "text-muted-foreground opacity-50 cursor-wait" : geoError ? "text-destructive" : "text-foreground hover:bg-muted"}`}
    >
      <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/></svg>
      {locationTagged ? "Location tagged" : geoError ? geoError : "Location"}
    </button>
  );

  const afterContent = (
    <div className="hidden md:block mt-12 pt-6 border-t border-border">
      <p className="text-xs text-muted-foreground">
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+D</kbd> today
        <span className="mx-2">&middot;</span>
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+/</kbd> chat
        <span className="mx-2">&middot;</span>
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+T</kbd> insert time
        <span className="mx-2">&middot;</span>
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Cmd+Enter</kbd> done editing
        <span className="mx-2">&middot;</span>
        <kbd className="px-1.5 py-0.5 bg-muted rounded text-[10px]">Esc</kbd> exit edit
      </p>
    </div>
  );

  return (
    <div className="flex flex-col h-full">
      <NoteEditor
        title={<h2 className="text-lg md:text-xl font-semibold text-foreground truncate">{formatDateHeading(date)}</h2>}
        content={content}
        onContentChange={setContent}
        onSave={handleSave}
        onEntryCreated={load}
        date={date}
        existingSubpages={existingSubpages}
        emptyMessage="No entries yet. Click to start writing, or add an entry below."
        emptyTemplate={"# Summary\n\n# Notes\n\n# Links\n"}
        toolbarExtra={toolbarExtra}
        attachMenuExtra={attachMenuExtra}
        afterContent={afterContent}
      />
    </div>
  );
}
