import { describe, it, expect } from "vitest";
import { mergeContent } from "./mergeContent";

describe("mergeContent", () => {
  describe("trivial cases", () => {
    it("takes remote when local is unchanged", () => {
      const base = "# Summary\n\nold\n\n# Notes\n\n";
      const local = base;
      const remote = "# Summary\n\nold\n\n# Notes\n\nnew entry\n";
      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toBe(remote);
    });

    it("keeps local when remote is unchanged", () => {
      const base = "# Summary\n\nold\n\n# Notes\n\n";
      const local = "# Summary\n\nedited\n\n# Notes\n\n";
      const remote = base;
      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toBe(local);
    });

    it("keeps local when both made the same change", () => {
      const base = "# Summary\n\nold\n\n# Notes\n\n";
      const local = "# Summary\n\nnew\n\n# Notes\n\n";
      const remote = local;
      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toBe(local);
    });
  });

  describe("non-conflicting section changes", () => {
    it("merges when user edits Summary and server appends to Notes", () => {
      const base = "# Summary\n\nold summary\n\n# Notes\n\nexisting note\n\n# Links\n\n";
      const local = "# Summary\n\nmy new summary\n\n# Notes\n\nexisting note\n\n# Links\n\n";
      const remote = "# Summary\n\nold summary\n\n# Notes\n\nexisting note\n\n---\n*10:30 — Voice memo*\n\ntranscription text\n\n# Links\n\n";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toContain("my new summary");
      expect(result.merged).toContain("transcription text");
      expect(result.conflicts).toEqual([]);
    });

    it("merges when user edits Notes and server adds Links", () => {
      const base = "# Summary\n\n\n\n# Notes\n\n\n\n# Links\n\n";
      const local = "# Summary\n\n\n\n# Notes\n\nmy note\n\n# Links\n\n";
      const remote = "# Summary\n\n\n\n# Notes\n\n\n\n# Links\n\nhttps://example.com\n";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toContain("my note");
      expect(result.merged).toContain("https://example.com");
    });

    it("handles server adding a new section", () => {
      const base = "# Summary\n\n\n\n# Notes\n\n";
      const local = "# Summary\n\nmy summary\n\n# Notes\n\n";
      const remote = "# Summary\n\n\n\n# Notes\n\n\n\n# Links\n\nhttp://example.com\n";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toContain("my summary");
      expect(result.merged).toContain("http://example.com");
      expect(result.merged).toContain("# Links");
    });
  });

  describe("conflicts", () => {
    it("detects conflict when both edit the same section", () => {
      const base = "# Summary\n\nold\n\n# Notes\n\n";
      const local = "# Summary\n\nuser version\n\n# Notes\n\n";
      const remote = "# Summary\n\nserver version\n\n# Notes\n\n";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(false);
      expect(result.merged).toBeNull();
      expect(result.conflicts).toContain("# Summary");
    });

    it("detects conflict in Notes section", () => {
      const base = "# Summary\n\n\n\n# Notes\n\noriginal\n";
      const local = "# Summary\n\n\n\n# Notes\n\nuser edited\n";
      const remote = "# Summary\n\n\n\n# Notes\n\nserver edited\n";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(false);
      expect(result.conflicts).toContain("# Notes");
    });

    it("reports multiple conflicts", () => {
      const base = "# Summary\n\na\n\n# Notes\n\nb\n";
      const local = "# Summary\n\nlocal-a\n\n# Notes\n\nlocal-b\n";
      const remote = "# Summary\n\nremote-a\n\n# Notes\n\nremote-b\n";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(false);
      expect(result.conflicts).toHaveLength(2);
    });

    it("no conflict when both make identical changes to same section", () => {
      const base = "# Summary\n\nold\n\n# Notes\n\n";
      const local = "# Summary\n\nsame new\n\n# Notes\n\n";
      const remote = "# Summary\n\nsame new\n\n# Notes\n\n";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toBe(local);
    });
  });

  describe("content without headings (append heuristic)", () => {
    it("merges when server appends to base", () => {
      const base = "some content";
      const local = "some content edited";
      const remote = "some content\n\nappended by server";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toBe("some content edited\n\nappended by server");
    });

    it("merges when local appends to base", () => {
      const base = "some content";
      const local = "some content\n\nuser appended";
      const remote = "some content modified by server";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toBe("some content modified by server\n\nuser appended");
    });

    it("conflicts when both modify the body", () => {
      const base = "original";
      const local = "user version";
      const remote = "server version";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(false);
      expect(result.conflicts).toContain("(entire note)");
    });
  });

  describe("real-world scenarios", () => {
    it("voice memo appended while user edits summary", () => {
      const base = [
        "# Summary",
        "",
        "",
        "",
        "# Notes",
        "",
        "",
        "",
        "# Links",
        "",
      ].join("\n");

      const local = [
        "# Summary",
        "",
        "Today I worked on the merge algorithm.",
        "",
        "# Notes",
        "",
        "",
        "",
        "# Links",
        "",
      ].join("\n");

      const remote = [
        "# Summary",
        "",
        "",
        "",
        "# Notes",
        "",
        "",
        "",
        "---",
        "*10:30 — Voice memo*",
        "",
        "Meeting with the team about Q3 planning.",
        "",
        "# Links",
        "",
      ].join("\n");

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toContain("Today I worked on the merge algorithm.");
      expect(result.merged).toContain("Meeting with the team about Q3 planning.");
    });

    it("web clip added while user types in Notes", () => {
      const base = [
        "# Summary",
        "",
        "",
        "",
        "# Notes",
        "",
        "Started the day with coffee.",
        "",
        "# Links",
        "",
      ].join("\n");

      const local = [
        "# Summary",
        "",
        "",
        "",
        "# Notes",
        "",
        "Started the day with coffee.",
        "",
        "Then had a meeting.",
        "",
        "# Links",
        "",
      ].join("\n");

      const remote = [
        "# Summary",
        "",
        "",
        "",
        "# Notes",
        "",
        "Started the day with coffee.",
        "",
        "# Links",
        "",
        "[Interesting article](https://example.com)",
        "",
      ].join("\n");

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(true);
      expect(result.merged).toContain("Then had a meeting.");
      expect(result.merged).toContain("Interesting article");
    });

    it("user and server both edit Notes — conflict", () => {
      const base = "# Summary\n\n\n\n# Notes\n\noriginal note\n\n# Links\n\n";
      const local = "# Summary\n\n\n\n# Notes\n\nuser rewrote this\n\n# Links\n\n";
      const remote = "# Summary\n\n\n\n# Notes\n\noriginal note\n\n---\n*10:30 — Voice memo*\n\ntranscription\n\n# Links\n\n";

      const result = mergeContent(base, local, remote);
      expect(result.ok).toBe(false);
      expect(result.conflicts).toContain("# Notes");
      // Summary and Links should not be in conflicts
      expect(result.conflicts).not.toContain("# Summary");
      expect(result.conflicts).not.toContain("# Links");
    });
  });
});
