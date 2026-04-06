import { useRef, useEffect } from "react";
import { EditorView, keymap, placeholder as phPlugin } from "@codemirror/view";
import { EditorState, EditorSelection } from "@codemirror/state";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { defaultKeymap, indentWithTab, history, historyKeymap } from "@codemirror/commands";
import { syntaxHighlighting, HighlightStyle } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { searchKeymap } from "@codemirror/search";

/** Toggle a markdown inline marker (e.g. `**` for bold, `*` for italic) around the selection. */
function toggleMarkdownMarker(view: EditorView, marker: string): boolean {
  const { state } = view;
  const changes = state.changeByRange((range) => {
    const selected = state.sliceDoc(range.from, range.to);
    if (
      range.from >= marker.length &&
      range.to + marker.length <= state.doc.length &&
      state.sliceDoc(range.from - marker.length, range.from) === marker &&
      state.sliceDoc(range.to, range.to + marker.length) === marker
    ) {
      // Already wrapped — remove markers
      return {
        changes: [
          { from: range.from - marker.length, to: range.from, insert: "" },
          { from: range.to, to: range.to + marker.length, insert: "" },
        ],
        range: EditorSelection.range(
          range.from - marker.length,
          range.to - marker.length,
        ),
      };
    }
    // Wrap selection (or insert empty markers when nothing is selected)
    const insert = `${marker}${selected}${marker}`;
    return {
      changes: { from: range.from, to: range.to, insert },
      range: selected
        ? EditorSelection.range(range.from, range.from + insert.length)
        : EditorSelection.cursor(range.from + marker.length),
    };
  });
  view.dispatch(state.update(changes, { userEvent: "input.format" }));
  return true;
}

// Minimal highlight style that makes markdown structure visible while editing
const markdownHighlight = HighlightStyle.define([
  { tag: tags.heading1, fontWeight: "600", fontSize: "1.375rem" },
  { tag: tags.heading2, fontWeight: "600", fontSize: "1.175rem" },
  { tag: tags.heading3, fontWeight: "600", fontSize: "1.05rem" },
  { tag: tags.heading4, fontWeight: "600" },
  { tag: tags.strong, fontWeight: "600" },
  { tag: tags.emphasis, fontStyle: "italic" },
  { tag: tags.strikethrough, textDecoration: "line-through" },
  { tag: tags.link, class: "cm-link-colored" },
  { tag: tags.url, class: "cm-link-colored", textDecoration: "underline" },
  { tag: tags.monospace, fontFamily: "'JetBrains Mono', Menlo, monospace", fontSize: "0.85em", class: "cm-code-bg" },
  { tag: tags.quote, class: "cm-quote-colored", fontStyle: "italic" },
  { tag: tags.processingInstruction, class: "cm-quote-colored" },
]);

// Theme that matches the note-prose reading view
const noteEditorTheme = EditorView.theme({
  "&": {
    fontSize: "0.9375rem",
    lineHeight: "1.75",
    fontFamily: "'JetBrains Mono', Menlo, Consolas, monospace",
  },
  "&.cm-focused": {
    outline: "none",
  },
  ".cm-content": {
    padding: "0",
    caretColor: "var(--accent)",
    color: "var(--foreground)",
  },
  ".cm-line": {
    padding: "0",
  },
  ".cm-cursor": {
    borderLeftColor: "var(--accent)",
    borderLeftWidth: "1.5px",
  },
  ".cm-selectionBackground": {
    backgroundColor: "var(--muted) !important",
  },
  "&.cm-focused .cm-selectionBackground": {
    backgroundColor: "var(--muted) !important",
  },
  ".cm-activeLine": {
    backgroundColor: "transparent",
  },
  ".cm-gutters": {
    display: "none",
  },
  ".cm-placeholder": {
    color: "var(--muted-foreground)",
    fontStyle: "normal",
  },
  ".cm-link-colored": {
    color: "var(--accent)",
  },
  ".cm-code-bg": {
    backgroundColor: "var(--muted)",
    borderRadius: "3px",
    padding: "1px 4px",
  },
  ".cm-quote-colored": {
    color: "var(--muted-foreground)",
  },
  ".cm-scroller": {
    overflow: "auto",
  },
});

interface MarkdownEditorProps {
  value: string;
  onChange: (value: string) => void;
  onSubmit?: () => void;
  placeholder?: string;
  autoFocus?: boolean;
}

export default function MarkdownEditor({
  value,
  onChange,
  onSubmit,
  placeholder = "Start writing...",
  autoFocus = false,
}: MarkdownEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;
  const onSubmitRef = useRef(onSubmit);
  onSubmitRef.current = onSubmit;

  useEffect(() => {
    if (!containerRef.current) return;

    const state = EditorState.create({
      doc: value,
      extensions: [
        noteEditorTheme,
        syntaxHighlighting(markdownHighlight),
        markdown({ base: markdownLanguage, codeLanguages: languages }),
        phPlugin(placeholder),
        history(),
        keymap.of([
          {
            key: "Mod-Enter",
            run: () => {
              onSubmitRef.current?.();
              return true;
            },
          },
          {
            key: "Mod-b",
            run: (view) => toggleMarkdownMarker(view, "**"),
          },
          {
            key: "Mod-i",
            run: (view) => toggleMarkdownMarker(view, "*"),
          },
          ...defaultKeymap,
          ...historyKeymap,
          ...searchKeymap,
          indentWithTab,
        ]),
        EditorView.lineWrapping,
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChangeRef.current(update.state.doc.toString());
          }
        }),
      ],
    });

    const view = new EditorView({
      state,
      parent: containerRef.current,
    });

    viewRef.current = view;

    if (autoFocus) {
      // Small delay to ensure DOM is ready
      requestAnimationFrame(() => {
        view.focus();
        // Place cursor at end
        view.dispatch({
          selection: { anchor: view.state.doc.length },
        });
      });
    }

    return () => {
      view.destroy();
      viewRef.current = null;
    };
    // Only create editor once on mount
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Sync external value changes (e.g. if content reloads)
  useEffect(() => {
    const view = viewRef.current;
    if (!view) return;
    const current = view.state.doc.toString();
    if (current !== value) {
      view.dispatch({
        changes: { from: 0, to: current.length, insert: value },
      });
    }
  }, [value]);

  return (
    <div
      ref={containerRef}
      className="note-editor"
    />
  );
}
