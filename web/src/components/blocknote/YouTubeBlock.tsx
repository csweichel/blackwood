import { createReactBlockSpec } from "@blocknote/react";
import { insertOrUpdateBlockForSlashMenu } from "@blocknote/core/extensions";
import type { DefaultReactSuggestionItem } from "@blocknote/react";
import type { BlockNoteEditor, BlockSchema, InlineContentSchema, StyleSchema } from "@blocknote/core";

/**
 * Extract a YouTube video ID from various URL formats.
 */
function parseYouTubeVideoId(url: string): string | null {
  try {
    const u = new URL(url);
    if (u.hostname === "youtu.be") {
      return u.pathname.slice(1) || null;
    }
    if (
      u.hostname === "www.youtube.com" ||
      u.hostname === "youtube.com" ||
      u.hostname === "www.youtube-nocookie.com" ||
      u.hostname === "youtube-nocookie.com"
    ) {
      // /watch?v=ID
      const v = u.searchParams.get("v");
      if (v) return v;
      // /embed/ID
      const embedMatch = u.pathname.match(/^\/embed\/([\w-]+)/);
      if (embedMatch) return embedMatch[1];
    }
  } catch {
    // not a valid URL
  }
  return null;
}

/**
 * Build a youtube-nocookie embed URL preserving start time if present.
 */
function buildEmbedUrl(url: string): string {
  const videoId = parseYouTubeVideoId(url);
  if (!videoId) return "";

  let startSeconds: string | null = null;
  try {
    const u = new URL(url);
    startSeconds = u.searchParams.get("t") || u.searchParams.get("start");
  } catch {
    // ignore
  }

  const embedUrl = `https://www.youtube-nocookie.com/embed/${videoId}`;
  if (startSeconds) {
    // Strip trailing 's' if present (e.g. "120s" → "120")
    const seconds = startSeconds.replace(/s$/, "");
    return `${embedUrl}?start=${seconds}`;
  }
  return embedUrl;
}

/**
 * Custom YouTube embed block for BlockNote.
 */
export const YouTubeBlock = createReactBlockSpec(
  {
    type: "youtube" as const,
    propSchema: {
      url: { default: "" },
    },
    content: "none" as const,
  },
  {
    render: ({ block }) => {
      const embedUrl = buildEmbedUrl(block.props.url);

      if (!embedUrl) {
        return (
          <div
            style={{
              padding: "12px 16px",
              background: "var(--muted, #f5f5f5)",
              borderRadius: "6px",
              color: "var(--muted-foreground, #888)",
              fontSize: "14px",
            }}
          >
            Invalid YouTube URL
          </div>
        );
      }

      return (
        <div
          style={{
            position: "relative",
            width: "100%",
            paddingBottom: "56.25%", // 16:9 aspect ratio
            borderRadius: "8px",
            overflow: "hidden",
          }}
        >
          <iframe
            src={embedUrl}
            title="YouTube video"
            style={{
              position: "absolute",
              top: 0,
              left: 0,
              width: "100%",
              height: "100%",
              border: "none",
            }}
            allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
            allowFullScreen
          />
        </div>
      );
    },
    toExternalHTML: ({ block }) => {
      return (
        <a href={block.props.url}>{block.props.url}</a>
      );
    },
  },
)();

/**
 * Slash menu item for inserting a YouTube embed.
 */
export function getYouTubeSlashMenuItem<
  BSchema extends BlockSchema,
  I extends InlineContentSchema,
  S extends StyleSchema,
>(
  editor: BlockNoteEditor<BSchema, I, S>,
): DefaultReactSuggestionItem {
  return {
    title: "YouTube",
    onItemClick: () => {
      const url = prompt("Enter YouTube URL:");
      if (url) {
        insertOrUpdateBlockForSlashMenu(editor, {
          type: "youtube" as keyof BSchema & string,
          props: { url } as Record<string, unknown>,
        } as Parameters<typeof insertOrUpdateBlockForSlashMenu<BSchema, I, S>>[1]);
      }
    },
    aliases: ["youtube", "video", "embed"],
    group: "Media",
    subtext: "Embed a YouTube video",
  };
}
