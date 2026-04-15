/**
 * Markdown pre/post-processing for BlockNote editor.
 *
 * Handles wikilink ↔ markdown link conversion and attachment URL rewriting
 * so that BlockNote's standard markdown parser/serializer can round-trip
 * Blackwood-specific syntax without custom inline content types.
 */

// ── Wikilinks ────────────────────────────────────────────────────────

const WIKILINK_RE = /\[\[([^\]]+)\]\]/g;

/**
 * Convert `[[Page Name]]` to standard markdown links before BlockNote parsing.
 * Existing subpages get a data attribute via a class marker so CSS can style them differently.
 */
export function preprocessWikilinks(
  markdown: string,
  date: string,
  existingSubpages: Set<string>,
): string {
  return markdown.replace(WIKILINK_RE, (_match, name: string) => {
    const slug = encodeURIComponent(name);
    const href = `/day/${date}/${slug}`;
    const exists = existingSubpages.has(name);
    // Encode existence as a query param so we can style differently in CSS.
    // BlockNote preserves link hrefs through round-trips.
    const marker = exists ? "" : "?new=1";
    return `[${name}](${href}${marker})`;
  });
}

/**
 * Convert wikilink-pattern links back to `[[...]]` after BlockNote serialization.
 * Matches links like `[Page Name](/day/2024-01-15/Page%20Name)` or with `?new=1`.
 */
export function postprocessWikilinks(markdown: string, date: string): string {
  // Match markdown links whose href starts with /day/{date}/
  const linkPattern = new RegExp(
    `\\[([^\\]]+)\\]\\(/day/${escapeRegExp(date)}/[^)]+\\)`,
    "g",
  );
  return markdown.replace(linkPattern, (_match, text: string) => {
    return `[[${text}]]`;
  });
}

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// ── Attachment URL rewriting ─────────────────────────────────────────

/**
 * Walk a BlockNote block tree and rewrite relative image/audio URLs to
 * the daily-notes attachment API path.
 *
 * Mutates blocks in place for efficiency.
 */
export function rewriteAttachmentUrls(
  blocks: Array<Record<string, unknown>>,
  date: string,
): void {
  for (const block of blocks) {
    const type = block.type as string | undefined;
    const props = block.props as Record<string, unknown> | undefined;

    if (
      (type === "image" || type === "audio" || type === "video" ||
        type === "file") &&
      props?.url &&
      typeof props.url === "string"
    ) {
      const url = props.url;
      // Rewrite relative paths (no protocol, no leading slash)
      if (url && !url.startsWith("http") && !url.startsWith("/") && !url.startsWith("data:")) {
        props.url = `/api/daily-notes/${date}/attachments/${encodeURIComponent(url)}`;
      }
    }

    // Recurse into children
    const children = block.children as Array<Record<string, unknown>> | undefined;
    if (Array.isArray(children) && children.length > 0) {
      rewriteAttachmentUrls(children, date);
    }
  }
}

// ── YouTube URL detection ────────────────────────────────────────────

const YOUTUBE_PATTERNS = [
  // youtube.com/watch?v=ID or youtube.com/watch?v=ID&t=123
  /(?:https?:\/\/)?(?:www\.)?youtube\.com\/watch\?v=([\w-]+)(?:&[^)\s]*)?/,
  // youtu.be/ID or youtu.be/ID?t=123
  /(?:https?:\/\/)?youtu\.be\/([\w-]+)(?:\?[^)\s]*)?/,
  // youtube-nocookie.com/embed/ID
  /(?:https?:\/\/)?(?:www\.)?youtube-nocookie\.com\/embed\/([\w-]+)(?:\?[^)\s]*)?/,
];

/**
 * Check if a string is a YouTube URL. Returns the full URL if it is, null otherwise.
 */
export function extractYouTubeUrl(text: string): string | null {
  const trimmed = text.trim();
  for (const pattern of YOUTUBE_PATTERNS) {
    if (pattern.test(trimmed)) {
      // Ensure it has a protocol
      if (trimmed.startsWith("http")) return trimmed;
      return `https://${trimmed}`;
    }
  }
  return null;
}

/**
 * Walk parsed blocks and convert paragraph blocks whose only content is a
 * YouTube URL (plain text or a single link) into youtube embed blocks.
 *
 * Returns a new array (does not mutate input).
 */
export function convertYouTubeBlocks(
  blocks: Array<Record<string, unknown>>,
): Array<Record<string, unknown>> {
  return blocks.map((block) => {
    if (block.type !== "paragraph") return block;

    const content = block.content as Array<Record<string, unknown>> | undefined;
    if (!content || content.length === 0) return block;

    // Single text node with just a URL
    if (content.length === 1) {
      const item = content[0];
      if (item.type === "text") {
        const url = extractYouTubeUrl(item.text as string);
        if (url) {
          return {
            id: block.id,
            type: "youtube",
            props: { url },
            content: undefined,
            children: [],
          };
        }
      }
      // Single link whose text is the URL itself
      if (item.type === "link") {
        const href = item.href as string;
        const url = extractYouTubeUrl(href);
        if (url) {
          return {
            id: block.id,
            type: "youtube",
            props: { url },
            content: undefined,
            children: [],
          };
        }
      }
    }

    return block;
  });
}

// ── Block hierarchy ──────────────────────────────────────────────────

type Block = Record<string, unknown>;

/**
 * Infer a nested block hierarchy from a flat list of blocks.
 *
 * BlockNote's `tryParseMarkdownToBlocks` produces a flat list. For collapsible
 * headings to work, content blocks must be nested as children of their heading.
 *
 * Algorithm:
 * - Walk the flat list. When a heading is encountered, all subsequent blocks
 *   until the next heading of equal or higher level become its children.
 * - List items that already have children get converted to toggleListItem.
 * - All headings are marked `isToggleable: true`.
 * - All toggle blocks are pre-set to expanded in localStorage.
 *
 * Returns a new array with the nested structure.
 */
export function nestBlocksUnderHeadings(
  blocks: Array<Block>,
): Array<Block> {
  const result: Array<Block> = [];
  let i = 0;

  while (i < blocks.length) {
    const block = blocks[i];

    if (block.type === "heading") {
      const props = block.props as Record<string, unknown>;
      const level = (props?.level as number) ?? 1;
      props.isToggleable = true;

      // Collect all subsequent blocks that belong under this heading
      const children: Array<Block> = [];
      i++;

      while (i < blocks.length) {
        const next = blocks[i];

        // Stop at a heading of equal or higher level (lower number)
        if (next.type === "heading") {
          const nextProps = next.props as Record<string, unknown>;
          const nextLevel = (nextProps?.level as number) ?? 1;
          if (nextLevel <= level) break;
        }

        children.push(next);
        i++;
      }

      // Recursively nest sub-headings within the collected children
      const nestedChildren = nestBlocksUnderHeadings(children);

      // Mark list items with children as toggleListItem
      markListToggles(nestedChildren);

      // Merge with any existing children the block may have
      const existing = block.children as Array<Block> | undefined;
      block.children = [...(existing ?? []), ...nestedChildren];

      result.push(block);
    } else {
      // Non-heading block at top level — keep as-is
      markListToggles([block]);
      result.push(block);
      i++;
    }
  }

  return result;
}

/**
 * Pre-expand all toggle blocks by setting their localStorage state to "true".
 * BlockNote uses `localStorage.getItem("toggle-${id}")` to determine initial
 * toggle state — "true" = expanded, anything else = collapsed.
 */
export function expandAllToggleBlocks(blocks: Array<Block>): void {
  for (const block of blocks) {
    const id = block.id as string | undefined;
    const props = block.props as Record<string, unknown> | undefined;

    if (id && block.type === "heading" && props?.isToggleable) {
      window.localStorage.setItem(`toggle-${id}`, "true");
    }

    if (id && block.type === "toggleListItem") {
      window.localStorage.setItem(`toggle-${id}`, "true");
    }

    const children = block.children as Array<Block> | undefined;
    if (Array.isArray(children) && children.length > 0) {
      expandAllToggleBlocks(children);
    }
  }
}

/**
 * Convert list items that have children to toggleListItem.
 * Recurses into children.
 */
function markListToggles(blocks: Array<Block>): void {
  for (const block of blocks) {
    const children = block.children as Array<Block> | undefined;

    if (
      (block.type === "bulletListItem" || block.type === "numberedListItem") &&
      children &&
      children.length > 0
    ) {
      block.type = "toggleListItem";
    }

    if (Array.isArray(children) && children.length > 0) {
      markListToggles(children);
    }
  }
}

/**
 * Flatten a nested block hierarchy back to a flat list.
 *
 * This is the inverse of `nestBlocksUnderHeadings`. Before serializing
 * blocks to markdown, we need to un-nest heading children so that
 * `blocksToMarkdownLossy` produces standard flat markdown (not indented
 * content under headings, which would be lossy).
 *
 * - Heading children are pulled out and placed after the heading.
 * - toggleListItem blocks are converted back to bulletListItem.
 * - `isToggleable` is stripped from headings.
 *
 * Returns a new flat array.
 */
export function flattenBlockHierarchy(
  blocks: Array<Block>,
): Array<Block> {
  const result: Array<Block> = [];

  for (const block of blocks) {
    const children = block.children as Array<Block> | undefined;

    if (block.type === "heading") {
      // Strip toggle props for serialization
      const props = block.props as Record<string, unknown>;
      if (props) {
        delete props.isToggleable;
      }

      // Push the heading itself with empty children
      result.push({ ...block, children: [] });

      // Flatten and append its children after it
      if (children && children.length > 0) {
        result.push(...flattenBlockHierarchy(children));
      }
    } else if (block.type === "toggleListItem") {
      // Convert back to bulletListItem for markdown
      result.push({ ...block, type: "bulletListItem" });
    } else {
      result.push(block);
    }
  }

  return result;
}

// ── Combined transforms ──────────────────────────────────────────────

/**
 * Pre-process raw markdown before passing to BlockNote's parser.
 */
export function preprocessMarkdown(
  markdown: string,
  date: string,
  existingSubpages: Set<string>,
): string {
  return preprocessWikilinks(markdown, date, existingSubpages);
}

/**
 * Post-process markdown output from BlockNote's serializer.
 */
export function postprocessMarkdown(markdown: string, date: string): string {
  return postprocessWikilinks(markdown, date);
}
