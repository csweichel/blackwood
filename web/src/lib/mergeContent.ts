/**
 * Three-way merge for markdown note content.
 *
 * Splits content into sections (delimited by top-level headings `# ...`)
 * and merges at the section level. If both local and remote changed the
 * same section relative to the base, that's a conflict.
 *
 * For content without headings, falls back to a simple append-detection
 * heuristic: if the remote only appended to the base, the append is
 * grafted onto the local version.
 */

export interface MergeResult {
  /** The merged content, or null if there are conflicts. */
  merged: string | null;
  /** True if the merge succeeded without conflicts. */
  ok: boolean;
  /** Conflicting section names (empty if ok). */
  conflicts: string[];
}

interface Section {
  /** The heading line (e.g. "# Notes") or "" for the preamble before any heading. */
  heading: string;
  /** The full text of the section including the heading line. */
  text: string;
}

const HEADING_RE = /^# /m;

/**
 * Split markdown into sections by top-level headings.
 * Content before the first heading becomes a section with heading "".
 */
function splitSections(content: string): Section[] {
  const lines = content.split("\n");
  const sections: Section[] = [];
  let currentHeading = "";
  let currentLines: string[] = [];

  for (const line of lines) {
    if (line.startsWith("# ")) {
      // Flush previous section
      sections.push({
        heading: currentHeading,
        text: currentLines.join("\n"),
      });
      currentHeading = line;
      currentLines = [line];
    } else {
      currentLines.push(line);
    }
  }
  // Flush last section
  sections.push({
    heading: currentHeading,
    text: currentLines.join("\n"),
  });

  return sections;
}

/**
 * Build a map from heading → section text for quick lookup.
 */
function sectionMap(sections: Section[]): Map<string, string> {
  const map = new Map<string, string>();
  for (const s of sections) {
    map.set(s.heading, s.text);
  }
  return map;
}

/**
 * Three-way merge of markdown content.
 *
 * @param base    The last version both sides agree on (last load or last save).
 * @param local   The current editor content (may have unsaved user edits).
 * @param remote  The new version from the server.
 * @returns       MergeResult with merged content or conflict info.
 */
export function mergeContent(
  base: string,
  local: string,
  remote: string,
): MergeResult {
  // Trivial cases
  if (base === local) {
    // User hasn't changed anything — take remote wholesale
    return { merged: remote, ok: true, conflicts: [] };
  }
  if (base === remote || local === remote) {
    // Server hasn't changed, or both made the same change
    return { merged: local, ok: true, conflicts: [] };
  }

  // If content has no headings, use append heuristic
  if (!HEADING_RE.test(base)) {
    return mergeAppend(base, local, remote);
  }

  const baseSections = splitSections(base);
  const localMap = sectionMap(splitSections(local));
  const remoteMap = sectionMap(splitSections(remote));
  const baseMap = sectionMap(baseSections);

  // Collect all heading keys in order (preserving base order, then any new ones)
  const allHeadings = new Set<string>();
  for (const s of baseSections) allHeadings.add(s.heading);
  // Add any new sections from remote (e.g. server added a new heading)
  for (const key of remoteMap.keys()) allHeadings.add(key);
  // Add any new sections from local
  for (const key of localMap.keys()) allHeadings.add(key);

  const mergedSections: string[] = [];
  const conflicts: string[] = [];

  for (const heading of allHeadings) {
    const baseText = baseMap.get(heading);
    const localText = localMap.get(heading);
    const remoteText = remoteMap.get(heading);

    const localChanged = localText !== baseText;
    const remoteChanged = remoteText !== baseText;

    if (!localChanged && !remoteChanged) {
      // Neither changed this section
      mergedSections.push(baseText ?? "");
    } else if (localChanged && !remoteChanged) {
      // Only local changed — keep local
      if (localText != null) mergedSections.push(localText);
      // If localText is undefined, user deleted this section — omit it
    } else if (!localChanged && remoteChanged) {
      // Only remote changed — take remote
      if (remoteText != null) mergedSections.push(remoteText);
    } else {
      // Both changed the same section
      if (localText === remoteText) {
        // Same change — no conflict
        mergedSections.push(localText ?? "");
      } else {
        // Real conflict
        const label = heading || "(preamble)";
        conflicts.push(label);
        // For the merged output, keep local version (will be discarded if conflicts)
        mergedSections.push(localText ?? remoteText ?? "");
      }
    }
  }

  if (conflicts.length > 0) {
    return { merged: null, ok: false, conflicts };
  }

  return { merged: mergedSections.join("\n"), ok: true, conflicts: [] };
}

/**
 * Check if `text` is `prefix` followed by content starting at a newline boundary.
 * Returns the appended portion (starting with \n) or null.
 */
function extractAppend(prefix: string, text: string): string | null {
  if (!text.startsWith(prefix)) return null;
  const rest = text.slice(prefix.length);
  // Must be empty (identical) or start with a newline — not a partial line match
  if (rest === "") return rest;
  if (rest.startsWith("\n")) return rest;
  return null;
}

/**
 * Fallback merge for content without headings.
 * Detects if remote only appended to base, and if so, grafts the
 * appended content onto local.
 */
function mergeAppend(
  base: string,
  local: string,
  remote: string,
): MergeResult {
  const remoteAppended = extractAppend(base, remote);
  const localAppended = extractAppend(base, local);

  // If remote is a pure append to base, graft it onto local
  if (remoteAppended != null) {
    return { merged: local + remoteAppended, ok: true, conflicts: [] };
  }

  // If local is a pure append to base, graft it onto remote
  if (localAppended != null) {
    return { merged: remote + localAppended, ok: true, conflicts: [] };
  }

  // Both modified the body — conflict
  return { merged: null, ok: false, conflicts: ["(entire note)"] };
}
