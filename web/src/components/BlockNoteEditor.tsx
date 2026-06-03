import { useEffect, useMemo, useRef, useCallback, useState } from "react";
import { useCreateBlockNote } from "@blocknote/react";
import { BlockNoteView } from "@blocknote/mantine";
import { BlockNoteSchema, defaultBlockSpecs } from "@blocknote/core";
import { filterSuggestionItems } from "@blocknote/core/extensions";
import {
  SuggestionMenuController,
  getDefaultReactSlashMenuItems,
} from "@blocknote/react";
import "@blocknote/core/fonts/inter.css";
import "@blocknote/mantine/style.css";
import "./BlockNoteEditor.css";

import { YouTubeBlock, getYouTubeSlashMenuItem } from "./blocknote/YouTubeBlock";
import {
  preprocessMarkdown,
  postprocessMarkdown,
  splitMarkdownStorage,
  appendBlockState,
  rewriteAttachmentUrls,
  resolveAttachmentUrl,
  promoteImageLinks,
  convertYouTubeBlocks,
  nestBlocksUnderHeadings,
  flattenBlockHierarchy,
  expandAllToggleBlocks,
} from "../lib/markdownTransforms";
import { uploadDailyNoteAttachment } from "../api/client";

/** Detect dark mode from the document root class or system preference. */
function useColorScheme(): "light" | "dark" {
  const [scheme, setScheme] = useState<"light" | "dark">(() => {
    if (typeof document === "undefined") return "light";
    if (document.documentElement.classList.contains("dark")) return "dark";
    if (
      !document.documentElement.classList.contains("light") &&
      window.matchMedia("(prefers-color-scheme: dark)").matches
    ) {
      return "dark";
    }
    return "light";
  });

  useEffect(() => {
    // Watch for class changes on <html> (user toggles theme in settings)
    const observer = new MutationObserver(() => {
      if (document.documentElement.classList.contains("dark")) {
        setScheme("dark");
      } else if (document.documentElement.classList.contains("light")) {
        setScheme("light");
      } else {
        // System preference
        setScheme(
          window.matchMedia("(prefers-color-scheme: dark)").matches
            ? "dark"
            : "light",
        );
      }
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });

    // Also watch system preference changes
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const handleChange = () => {
      if (
        !document.documentElement.classList.contains("dark") &&
        !document.documentElement.classList.contains("light")
      ) {
        setScheme(mq.matches ? "dark" : "light");
      }
    };
    mq.addEventListener("change", handleChange);

    return () => {
      observer.disconnect();
      mq.removeEventListener("change", handleChange);
    };
  }, []);

  return scheme;
}

// Custom schema with YouTube block
const schema = BlockNoteSchema.create({
  blockSpecs: {
    ...defaultBlockSpecs,
    youtube: YouTubeBlock,
  },
});

interface BlockNoteEditorProps {
  content: string;
  onChange: (markdown: string) => void;
  date: string;
  existingSubpages: Set<string>;
  placeholder?: string;
}

export default function BlockNoteEditor({
  content,
  onChange,
  date,
  existingSubpages,
  placeholder,
}: BlockNoteEditorProps) {
  const colorScheme = useColorScheme();

  // Keep refs for values used in callbacks to avoid stale closures
  const dateRef = useRef(date);
  useEffect(() => {
    dateRef.current = date;
  }, [date]);

  const editor = useCreateBlockNote({
    schema,
    defaultStyles: true,
    resolveFileUrl: async (url) => resolveAttachmentUrl(url, dateRef.current),
    uploadFile: async (file) => {
      const uploaded = await uploadDailyNoteAttachment(dateRef.current, file);
      const props: Record<string, unknown> = {
        url: uploaded.filename,
        name: file.name || uploaded.filename,
        caption: "",
      };
      if (file.type.startsWith("image/")) {
        props.showPreview = true;
      }
      return {
        props,
      };
    },
    ...(placeholder ? { placeholders: { default: placeholder } } : {}),
  });

  const serializeEditorContent = useCallback((): string => {
    const documentBlocks = editor.document as Array<Record<string, unknown>>;
    const flat = flattenBlockHierarchy(documentBlocks);
    const markdown = editor.blocksToMarkdownLossy(flat as typeof editor.document);
    const processed = postprocessMarkdown(markdown, dateRef.current);
    return appendBlockState(processed, documentBlocks);
  }, [editor]);

  const insertUploadedFiles = useCallback(
    async (files: File[]) => {
      if (!editor || files.length === 0) return;

      const insertedBlocks: Array<Record<string, unknown>> = [];
      for (const file of files) {
        const uploaded = await uploadDailyNoteAttachment(dateRef.current, file);
        const name = file.name || uploaded.filename;
        if (file.type.startsWith("image/")) {
          insertedBlocks.push({
            type: "image",
            props: {
              url: uploaded.filename,
              name,
              caption: "",
              showPreview: true,
            },
          });
        } else {
          insertedBlocks.push({
            type: "file",
            props: {
              url: uploaded.filename,
              name,
              caption: "",
            },
          });
        }
      }

      const cursor = editor.getTextCursorPosition();
      const inserted = editor.insertBlocks(
        insertedBlocks as Parameters<typeof editor.insertBlocks>[0],
        cursor.block,
        "after",
      );
      const lastInserted = inserted[inserted.length - 1];
      if (lastInserted) {
        editor.setTextCursorPosition(lastInserted, "end");
      }
    },
    [editor],
  );

  // Track the last markdown we set into the editor to avoid echo loops
  const lastSetContent = useRef<string>("");
  // Track whether initial content has been loaded
  const initialLoaded = useRef(false);

  // Use a ref for existingSubpages so it doesn't trigger the content-sync effect.
  const existingSubpagesForSync = useRef(existingSubpages);
  useEffect(() => {
    existingSubpagesForSync.current = existingSubpages;
  }, [existingSubpages]);

  // Load initial content and handle external content changes.
  // This effect runs when `content` changes (from parent state). It compares
  // against what the editor already contains and only calls replaceBlocks when
  // the content genuinely differs — preventing flicker from echo loops where
  // the editor's own onChange output flows back as a prop change.
  useEffect(() => {
    if (!editor) return;

    // Skip if the content matches what we last set (avoids clobbering edits)
    if (initialLoaded.current && content === lastSetContent.current) return;

    const { markdown: visibleMarkdown, blockState } = splitMarkdownStorage(content);

    // If already loaded, check if editor content actually differs
    if (initialLoaded.current) {
      const currentStoredContent = serializeEditorContent();
      if (currentStoredContent.trim() === content.trim()) {
        lastSetContent.current = content;
        return;
      }
    }

    let nextBlocks: Array<Record<string, unknown>>;
    if (blockState) {
      nextBlocks = JSON.parse(JSON.stringify(blockState.blocks)) as Array<
        Record<string, unknown>
      >;
      rewriteAttachmentUrls(nextBlocks, date);
    } else {
      // Pre-process: convert wikilinks to standard markdown links
      const preprocessed = preprocessMarkdown(
        visibleMarkdown,
        date,
        existingSubpagesForSync.current,
      );

      // Parse markdown to blocks
      const blocks = editor.tryParseMarkdownToBlocks(preprocessed);

      // Post-process blocks: rewrite attachment URLs, convert YouTube URLs,
      // and nest blocks under headings for collapsible sections
      const rawBlocks = blocks as Array<Record<string, unknown>>;
      rewriteAttachmentUrls(rawBlocks, date);
      const withImages = promoteImageLinks(rawBlocks, date);
      const withYouTube = convertYouTubeBlocks(withImages);
      nextBlocks = nestBlocksUnderHeadings(withYouTube);
    }

    // Pre-expand all toggle blocks so sections start open
    expandAllToggleBlocks(nextBlocks);

    editor.replaceBlocks(editor.document, nextBlocks as typeof editor.document);
    lastSetContent.current = content;
    initialLoaded.current = true;
  }, [editor, content, date, serializeEditorContent]);

  const handleChange = useCallback(() => {
    if (!editor) return;
    const processed = serializeEditorContent();
    lastSetContent.current = processed;
    onChange(processed);
  }, [editor, onChange, serializeEditorContent]);

  // Build slash menu items with YouTube added
  const getSlashMenuItems = useMemo(
    () => async (query: string) =>
      filterSuggestionItems(
        [...getDefaultReactSlashMenuItems(editor), getYouTubeSlashMenuItem(editor)],
        query,
      ),
    [editor],
  );

  // Build wikilink suggestion items from existing subpages
  const getWikilinkItems = useCallback(
    async (query: string) => {
      const pages = Array.from(existingSubpagesForSync.current);
      const lowerQuery = query.toLowerCase();

      const makeItem = (name: string, isNew: boolean) => ({
        title: isNew ? `Create "${name}"` : name,
        onItemClick: () => {
          // clearQuery is called by the SuggestionMenuController's onItemClick
          // before this runs — see the onItemClick prop below
          const slug = encodeURIComponent(name);
          const href = `/day/${dateRef.current}/${slug}${isNew ? "?new=1" : ""}`;
          editor.createLink(href, name);
        },
        group: isNew ? "New" : "Subpages",
      });

      // Filter existing subpages by query
      const matches = pages
        .filter((name) => name.toLowerCase().includes(lowerQuery))
        .sort()
        .map((name) => makeItem(name, false));

      // Always offer "Create new page" if query is non-empty and doesn't
      // exactly match an existing subpage
      if (
        query.trim() &&
        !pages.some((p) => p.toLowerCase() === lowerQuery)
      ) {
        matches.push(makeItem(query.trim(), true));
      }

      return matches;
    },
    [editor],
  );

  return (
    <div
      className="bn-blackwood-wrapper"
      onDragOver={(event) => {
        if (event.dataTransfer.types.includes("Files")) {
          event.preventDefault();
        }
      }}
      onDrop={(event) => {
        const files = Array.from(event.dataTransfer.files);
        if (files.length === 0) return;
        event.preventDefault();
        void insertUploadedFiles(files);
      }}
    >
      <BlockNoteView
        editor={editor}
        onChange={handleChange}
        theme={colorScheme}
        slashMenu={false}
      >
        <SuggestionMenuController
          triggerCharacter="/"
          getItems={getSlashMenuItems}
        />
        <SuggestionMenuController
          triggerCharacter="@"
          getItems={getWikilinkItems}
          onItemClick={(item) => {
            // Clear the @query text before inserting the link
            const suggestionMenu = editor.extensions.get("suggestionMenu") as
              | { clearQuery: () => void }
              | undefined;
            suggestionMenu?.clearQuery();
            item.onItemClick();
          }}
        />
      </BlockNoteView>
    </div>
  );
}

// TODO: Feature 4 — Section label icons (Summary, Notes, Links headings with icon prefixes).
//       Requires custom heading block or component override. Deferred as cosmetic.
