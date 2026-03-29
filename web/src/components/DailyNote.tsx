import { useEffect, useState, useCallback, useRef } from "react";
import Markdown from "react-markdown";
import rehypeRaw from "rehype-raw";
import remarkGfm from "remark-gfm";
import { visit } from "unist-util-visit";
import { getDailyNote, updateDailyNoteContent } from "../api/client";
import { useGeolocation } from "../hooks/useGeolocation";

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

const SECTION_HEADINGS = new Set(["Summary", "Notes", "Links"]);

/**
 * React SVG icons for section headings, injected via component overrides.
 */
const SECTION_ICONS_JSX: Record<string, React.ReactNode> = {
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
 * Rehype plugin that styles known section headings (# Summary, # Notes,
 * # Links) with an icon and bold title. Works on bare h1 elements and
 * h1 elements inside <summary> (collapsible).
 * Also marks the paragraph after # Summary with a special class.
 */
function rehypeSectionLabels() {
  return (tree: any) => {
    visit(tree, "element", (node: any, index: number | undefined, parent: any) => {
      if (index === undefined || !parent) return;

      // Case 1: bare h1 (not wrapped by collapsible — e.g. empty section)
      if (node.tagName === "h1" && parent.tagName !== "summary") {
        const text = getTextContent(node).trim();
        if (!SECTION_HEADINGS.has(text)) return;
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
      if (!SECTION_HEADINGS.has(text)) return;

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
 * Rehype plugin that rewrites relative src/href attributes on img and audio
 * elements to point to the date-based attachment API route. This makes
 * relative filenames (e.g. "photo-abc12345.jpg") written into the markdown
 * resolve correctly in the web UI.
 */
function rehypeAttachmentUrls(date: string) {
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
  const [showAttachMenu, setShowAttachMenu] = useState(false);
  const [showClipForm, setShowClipForm] = useState(false);
  const [clipUrl, setClipUrl] = useState("");
  const [clipLoading, setClipLoading] = useState(false);
  const attachRef = useRef<HTMLDivElement>(null);
  const [pdfLoading, setPdfLoading] = useState(false);
  const [summarizing, setSummarizing] = useState(false);
  const { position: geoPosition, loading: geoLoading, error: geoError, requestLocation } = useGeolocation();
  const [locationTagged, setLocationTagged] = useState(false);

  // Close attach menu on outside click
  useEffect(() => {
    function handleClickOutside(e: MouseEvent) {
      if (attachRef.current && !attachRef.current.contains(e.target as Node)) {
        setShowAttachMenu(false);
      }
    }
    if (showAttachMenu) {
      document.addEventListener("mousedown", handleClickOutside);
      return () => document.removeEventListener("mousedown", handleClickOutside);
    }
  }, [showAttachMenu]);

  // Reset editing state when date changes
  useEffect(() => {
    setEditing(false);
    setShowRecorder(false);
    setShowCamera(false);
    setShowAttachMenu(false);
    setShowClipForm(false);
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
    doSave(newContent);
  }, [geoPosition, locationTagged, content, doSave]);

  function handleEditChange(text: string) {
    setEditContent(text);
    setSaveStatus("idle");

    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      doSave(text);
    }, 1000);
  }

  function startEditing() {
    const template = "# Summary\n\n# Notes\n\n# Links\n";
    setEditContent(content.trim() ? content : template);
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
              onClick={generateSummary}
              disabled={summarizing}
              className={`p-1.5 rounded-md transition-colors ${summarizing ? "text-muted-foreground opacity-50 cursor-wait" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
              title="Generate summary"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 3l1.912 5.813a2 2 0 0 0 1.275 1.275L21 12l-5.813 1.912a2 2 0 0 0-1.275 1.275L12 21l-1.912-5.813a2 2 0 0 0-1.275-1.275L3 12l5.813-1.912a2 2 0 0 0 1.275-1.275L12 3z"/></svg>
            </button>
          )}
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
          <div className="relative" ref={attachRef}>
            <button
              onClick={() => setShowAttachMenu((v) => !v)}
              className={`p-1.5 rounded-md transition-colors ${showAttachMenu ? "text-accent bg-muted" : "text-muted-foreground hover:text-foreground hover:bg-muted"}`}
              title="Attach"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48"/></svg>
            </button>
            {showAttachMenu && (
              <div className="absolute right-0 top-full mt-1 bg-card border border-border rounded-lg shadow-lg z-50 min-w-[160px]">
                <button
                  onClick={() => { setShowAttachMenu(false); setShowCamera(false); setShowRecorder(true); }}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2"/><line x1="12" x2="12" y1="19" y2="22"/></svg>
                  Voice memo
                </button>
                <button
                  onClick={() => { setShowAttachMenu(false); setShowRecorder(false); setShowCamera(true); }}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z"/><circle cx="12" cy="13" r="3"/></svg>
                  Photo
                </button>
                <button
                  onClick={() => { setShowAttachMenu(false); requestLocation(); }}
                  disabled={geoLoading || locationTagged}
                  className={`flex items-center gap-2 px-3 py-2 text-sm w-full text-left ${locationTagged ? "text-accent" : geoLoading ? "text-muted-foreground opacity-50 cursor-wait" : geoError ? "text-destructive" : "text-foreground hover:bg-muted"}`}
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/></svg>
                  {locationTagged ? "Location tagged" : geoError ? geoError : "Location"}
                </button>
                <button
                  onClick={() => { setShowAttachMenu(false); setShowClipForm(true); }}
                  className="flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-muted w-full text-left"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
                  Clip page
                </button>
              </div>
            )}
          </div>
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

      {showClipForm && (
        <div className="mb-4 bg-card border border-border rounded-lg p-3">
          <form
            className="flex items-center gap-2"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!clipUrl.trim()) return;
              setClipLoading(true);
              try {
                const resp = await fetch("/api/clip", {
                  method: "POST",
                  headers: { "Content-Type": "application/json" },
                  body: JSON.stringify({ url: clipUrl.trim() }),
                });
                if (!resp.ok) throw new Error("Clip failed");
                setClipUrl("");
                setShowClipForm(false);
                await load();
              } catch (err) {
                console.error("Clip page failed:", err);
              } finally {
                setClipLoading(false);
              }
            }}
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-muted-foreground shrink-0"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>
            <input
              type="url"
              value={clipUrl}
              onChange={(e) => setClipUrl(e.target.value)}
              placeholder="Paste URL to clip..."
              className="flex-1 bg-transparent border border-border rounded-md px-2 py-1 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-accent"
              autoFocus
              disabled={clipLoading}
            />
            <button
              type="submit"
              disabled={clipLoading || !clipUrl.trim()}
              className="px-3 py-1 text-xs font-medium text-primary-foreground bg-primary rounded-md hover:opacity-90 transition-colors disabled:opacity-50"
            >
              {clipLoading ? "Clipping..." : "Clip"}
            </button>
            <button
              type="button"
              onClick={() => { setShowClipForm(false); setClipUrl(""); }}
              className="p-1 text-muted-foreground hover:text-foreground transition-colors"
              title="Cancel"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><line x1="18" x2="6" y1="6" y2="18"/><line x1="6" x2="18" y1="6" y2="18"/></svg>
            </button>
          </form>
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
            onClick={(e) => {
              // Don't enter edit mode when clicking interactive elements
              const target = e.target as HTMLElement;
              if (target.closest("summary, details, a, audio, button, video, iframe")) return;
              startEditing();
            }}
          >
            <Markdown
              remarkPlugins={[remarkGfm, remarkWikilinks]}
              rehypePlugins={[rehypeRaw, rehypeYoutubeEmbed, rehypeCollapsible, rehypeSectionLabels, rehypeAttachmentUrls(date)]}
              components={{
                h1: ({ className, children, ...props }) => {
                  const cls = typeof className === "string" ? className : "";
                  if (cls.includes("note-section-label")) {
                    const text = typeof children === "string" ? children : String(children ?? "");
                    const icon = SECTION_ICONS_JSX[text.trim()];
                    return (
                      <h1 className={cls} {...props}>
                        {icon && <span className="note-section-icon">{icon}</span>}
                        {children}
                      </h1>
                    );
                  }
                  return <h1 className={className} {...props}>{children}</h1>;
                },
              }}
            >
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
    </div>
  );
}
