import { useRef, useEffect } from "react";
import { EditorView, keymap, placeholder as phPlugin } from "@codemirror/view";
import { EditorState } from "@codemirror/state";
import { markdown, markdownLanguage } from "@codemirror/lang-markdown";
import { languages } from "@codemirror/language-data";
import { defaultKeymap, indentWithTab, history, historyKeymap } from "@codemirror/commands";
import { syntaxHighlighting, HighlightStyle } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { searchKeymap } from "@codemirror/search";

// Minimal highlight style that makes markdown structure visible while editing
const markdownHighlight = HighlightStyle.define([
  { tag: tags.heading1, fontWeight: "600", fontSize: "1.375rem", color: "#2C2416" },
  { tag: tags.heading2, fontWeight: "600", fontSize: "1.175rem", color: "#2C2416" },
  { tag: tags.heading3, fontWeight: "600", fontSize: "1.05rem", color: "#2C2416" },
  { tag: tags.heading4, fontWeight: "600", color: "#2C2416" },
  { tag: tags.strong, fontWeight: "600" },
  { tag: tags.emphasis, fontStyle: "italic" },
  { tag: tags.strikethrough, textDecoration: "line-through" },
  { tag: tags.link, color: "#B8860B" },
  { tag: tags.url, color: "#B8860B", textDecoration: "underline" },
  { tag: tags.monospace, fontFamily: "'JetBrains Mono', Menlo, monospace", fontSize: "0.85em", backgroundColor: "#F0EBE3", borderRadius: "3px", padding: "1px 4px" },
  { tag: tags.quote, color: "#6B5D4D", fontStyle: "italic" },
  { tag: tags.processingInstruction, color: "#6B5D4D" }, // markdown markers like #, *, etc.
]);

// Theme that matches the note-prose reading view
const noteEditorTheme = EditorView.theme({
  "&": {
    fontSize: "0.9375rem",
    lineHeight: "1.75",
    fontFamily: "'Source Serif 4', Georgia, serif",
  },
  "&.cm-focused": {
    outline: "none",
  },
  ".cm-content": {
    padding: "0",
    caretColor: "#2C2416",
  },
  ".cm-line": {
    padding: "0",
  },
  ".cm-cursor": {
    borderLeftColor: "#2C2416",
    borderLeftWidth: "1.5px",
  },
  ".cm-selectionBackground": {
    backgroundColor: "#E5DFD5 !important",
  },
  "&.cm-focused .cm-selectionBackground": {
    backgroundColor: "#E5DFD5 !important",
  },
  ".cm-activeLine": {
    backgroundColor: "transparent",
  },
  ".cm-gutters": {
    display: "none",
  },
  ".cm-placeholder": {
    color: "#6B5D4D",
    fontStyle: "normal",
  },
  ".cm-scroller": {
    overflow: "auto",
  },
});

interface MarkdownEditorProps {
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  autoFocus?: boolean;
}

export default function MarkdownEditor({
  value,
  onChange,
  placeholder = "Start writing...",
  autoFocus = false,
}: MarkdownEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  onChangeRef.current = onChange;

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
        keymap.of([...defaultKeymap, ...historyKeymap, ...searchKeymap, indentWithTab]),
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
