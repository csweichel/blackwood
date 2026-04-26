import { describe, it, expect } from "vitest";
import {
  preprocessMarkdown,
  postprocessMarkdown,
  flattenBlockHierarchy,
  nestBlocksUnderHeadings,
  extractYouTubeUrl,
  promoteImageLinks,
  postprocessAttachmentUrls,
  resolveAttachmentUrl,
} from "./markdownTransforms";

describe("wikilink round-trip", () => {
  const date = "2025-01-15";
  const existingSubpages = new Set(["Meeting Notes", "Ideas"]);

  it("preprocess converts wikilinks to markdown links", () => {
    const md = "See [[Meeting Notes]] for details.";
    const result = preprocessMarkdown(md, date, existingSubpages);
    expect(result).toBe(
      "See [Meeting Notes](/day/2025-01-15/Meeting%20Notes) for details."
    );
  });

  it("preprocess marks new subpages with ?new=1", () => {
    const md = "Create [[New Page]] here.";
    const result = preprocessMarkdown(md, date, existingSubpages);
    expect(result).toContain("?new=1");
  });

  it("postprocess converts markdown links back to wikilinks", () => {
    const md =
      "See [Meeting Notes](/day/2025-01-15/Meeting%20Notes) for details.";
    const result = postprocessMarkdown(md, date);
    expect(result).toBe("See [[Meeting Notes]] for details.");
  });

  it("postprocess strips ?new=1 marker", () => {
    const md =
      "Create [New Page](/day/2025-01-15/New%20Page?new=1) here.";
    const result = postprocessMarkdown(md, date);
    expect(result).toBe("Create [[New Page]] here.");
  });

  it("round-trip is idempotent for existing subpages", () => {
    const original = "Check [[Meeting Notes]] and [[Ideas]].";
    const preprocessed = preprocessMarkdown(original, date, existingSubpages);
    const postprocessed = postprocessMarkdown(preprocessed, date);
    expect(postprocessed).toBe(original);
  });

  it("round-trip is idempotent for new subpages", () => {
    const original = "Check [[Brand New Page]].";
    const preprocessed = preprocessMarkdown(original, date, existingSubpages);
    const postprocessed = postprocessMarkdown(preprocessed, date);
    expect(postprocessed).toBe(original);
  });

  it("does not touch links to other dates", () => {
    const md = "See [Other](/day/2025-01-16/Other) link.";
    const result = postprocessMarkdown(md, date);
    expect(result).toBe(md);
  });

  it("handles multiple wikilinks in one line", () => {
    const original = "See [[Meeting Notes]] and [[Ideas]] today.";
    const preprocessed = preprocessMarkdown(original, date, existingSubpages);
    const postprocessed = postprocessMarkdown(preprocessed, date);
    expect(postprocessed).toBe(original);
  });
});

describe("nestBlocksUnderHeadings / flattenBlockHierarchy round-trip", () => {
  it("flatten(nest(blocks)) preserves block order", () => {
    const blocks = [
      { id: "1", type: "heading", props: { level: 1 }, content: [{ type: "text", text: "Summary" }], children: [] },
      { id: "2", type: "paragraph", props: {}, content: [{ type: "text", text: "Some text" }], children: [] },
      { id: "3", type: "heading", props: { level: 1 }, content: [{ type: "text", text: "Notes" }], children: [] },
      { id: "4", type: "paragraph", props: {}, content: [{ type: "text", text: "More text" }], children: [] },
    ];

    const nested = nestBlocksUnderHeadings(blocks);
    const flat = flattenBlockHierarchy(nested);

    // Should have same number of blocks
    expect(flat.length).toBe(blocks.length);

    // Block IDs should be in the same order
    expect(flat.map((b) => b.id)).toEqual(["1", "2", "3", "4"]);

    // Block types should be preserved
    expect(flat.map((b) => b.type)).toEqual([
      "heading",
      "paragraph",
      "heading",
      "paragraph",
    ]);
  });

  it("flatten strips isToggleable from headings", () => {
    const blocks = [
      { id: "1", type: "heading", props: { level: 1 }, content: [], children: [] },
      { id: "2", type: "paragraph", props: {}, content: [], children: [] },
    ];

    const nested = nestBlocksUnderHeadings(blocks);
    // After nesting, heading should have isToggleable
    expect((nested[0].props as Record<string, unknown>).isToggleable).toBe(true);

    const flat = flattenBlockHierarchy(nested);
    // After flattening, isToggleable should be stripped
    expect((flat[0].props as Record<string, unknown>).isToggleable).toBeUndefined();
  });

  it("handles nested headings (h1 > h2 > content)", () => {
    const blocks = [
      { id: "1", type: "heading", props: { level: 1 }, content: [], children: [] },
      { id: "2", type: "heading", props: { level: 2 }, content: [], children: [] },
      { id: "3", type: "paragraph", props: {}, content: [], children: [] },
      { id: "4", type: "heading", props: { level: 1 }, content: [], children: [] },
    ];

    const nested = nestBlocksUnderHeadings(blocks);
    const flat = flattenBlockHierarchy(nested);

    expect(flat.map((b) => b.id)).toEqual(["1", "2", "3", "4"]);
  });

  it("round-trip is idempotent (nest then flatten twice)", () => {
    const blocks = [
      { id: "1", type: "heading", props: { level: 1 }, content: [], children: [] },
      { id: "2", type: "paragraph", props: {}, content: [], children: [] },
      { id: "3", type: "heading", props: { level: 2 }, content: [], children: [] },
      { id: "4", type: "paragraph", props: {}, content: [], children: [] },
    ];

    const flat1 = flattenBlockHierarchy(nestBlocksUnderHeadings(blocks));
    const flat2 = flattenBlockHierarchy(nestBlocksUnderHeadings(flat1));

    expect(flat2.map((b) => b.id)).toEqual(flat1.map((b) => b.id));
    expect(flat2.map((b) => b.type)).toEqual(flat1.map((b) => b.type));
  });
});

describe("extractYouTubeUrl", () => {
  it("extracts standard youtube.com URL", () => {
    expect(extractYouTubeUrl("https://www.youtube.com/watch?v=dQw4w9WgXcQ")).toBe(
      "https://www.youtube.com/watch?v=dQw4w9WgXcQ"
    );
  });

  it("extracts youtu.be short URL", () => {
    expect(extractYouTubeUrl("https://youtu.be/dQw4w9WgXcQ")).toBe(
      "https://youtu.be/dQw4w9WgXcQ"
    );
  });

  it("returns null for non-YouTube URLs", () => {
    expect(extractYouTubeUrl("https://example.com")).toBeNull();
  });

  it("adds protocol if missing", () => {
    expect(extractYouTubeUrl("youtu.be/dQw4w9WgXcQ")).toBe(
      "https://youtu.be/dQw4w9WgXcQ"
    );
  });
});

describe("attachment image handling", () => {
  const date = "2025-01-15";

  it("resolves day-relative attachment URLs through the attachment endpoint", () => {
    expect(resolveAttachmentUrl("photo 1.jpg", date)).toBe(
      "/api/daily-notes/2025-01-15/attachments/photo%201.jpg"
    );
    expect(resolveAttachmentUrl("photo%201.jpg", date)).toBe(
      "/api/daily-notes/2025-01-15/attachments/photo%201.jpg"
    );
    expect(resolveAttachmentUrl("/api/daily-notes/2025-01-15/attachments/photo.jpg", date)).toBe(
      "/api/daily-notes/2025-01-15/attachments/photo.jpg"
    );
    expect(resolveAttachmentUrl("https://example.com/photo.jpg", date)).toBe(
      "https://example.com/photo.jpg"
    );
  });

  it("promotes standalone markdown image text to image blocks", () => {
    const blocks = [
      {
        id: "p1",
        type: "paragraph",
        props: {},
        content: [{ type: "text", text: "![photo](photo.jpg)", styles: {} }],
        children: [],
      },
    ];

    const promoted = promoteImageLinks(blocks, date);

    expect(promoted[0].type).toBe("image");
    expect((promoted[0].props as Record<string, unknown>).url).toBe(
      "/api/daily-notes/2025-01-15/attachments/photo.jpg"
    );
    expect((promoted[0].props as Record<string, unknown>).name).toBe("photo");
  });

  it("promotes standalone image links to image blocks", () => {
    const blocks = [
      {
        id: "p1",
        type: "paragraph",
        props: {},
        content: [
          {
            type: "link",
            href: "photo.png",
            content: [{ type: "text", text: "Photo", styles: {} }],
          },
        ],
        children: [],
      },
    ];

    const promoted = promoteImageLinks(blocks, date);

    expect(promoted[0].type).toBe("image");
    expect((promoted[0].props as Record<string, unknown>).url).toBe(
      "/api/daily-notes/2025-01-15/attachments/photo.png"
    );
    expect((promoted[0].props as Record<string, unknown>).name).toBe("Photo");
  });

  it("does not promote non-image links", () => {
    const blocks = [
      {
        id: "p1",
        type: "paragraph",
        props: {},
        content: [
          {
            type: "link",
            href: "notes.md",
            content: [{ type: "text", text: "Notes", styles: {} }],
          },
        ],
        children: [],
      },
    ];

    const promoted = promoteImageLinks(blocks, date);

    expect(promoted[0].type).toBe("paragraph");
  });

  it("saves same-day attachment API URLs back as filenames", () => {
    const md = "![photo](/api/daily-notes/2025-01-15/attachments/photo%201.jpg)";

    expect(postprocessAttachmentUrls(md, date)).toBe("![photo](photo%201.jpg)");
  });
});
