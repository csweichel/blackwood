import SwiftUI
import UIKit

struct MarkdownCellEditor: View {
    fileprivate enum SaveState: Equatable {
        case idle
        case pending
        case saving
        case saved
        case failed
    }

    @Binding private var markdown: String
    private let placeholder: String
    private let onSave: (String) async -> Bool

    @State private var session: MarkdownEditingSession
    @State private var saveState: SaveState = .idle
    @State private var saveTask: Task<Void, Never>?

    init(
        markdown: Binding<String>,
        placeholder: String = "Write something…",
        onSave: @escaping (String) async -> Bool
    ) {
        _markdown = markdown
        self.placeholder = placeholder
        self.onSave = onSave
        _session = State(initialValue: MarkdownEditingSession(markdown: markdown.wrappedValue))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            ForEach($session.blocks) { $block in
                MarkdownEditorRow(
                    block: $block,
                    placeholder: rowPlaceholder(for: block),
                    isFocused: focusBinding(for: block.id),
                    selectedRange: selectionBinding(for: block.id),
                    onTextChanged: { session.applyMarkdownShortcutIfNeeded(to: block.id) },
                    onReturn: { range in session.splitBlock(block.id, at: range) },
                    onEndMultilineBlock: { range in session.endMultilineBlock(block.id, at: range) },
                    onBackspaceAtStart: { session.handleBackspaceAtStart(in: block.id) },
                    onToggleTask: { session.toggleTask(block.id) },
                    onSelect: { session.activate(block.id) }
                )
            }

            editorFooter
        }
        .toolbar {
            ToolbarItemGroup(placement: .keyboard) {
                editorActionsMenu(iconOnly: true)

                Button {
                    session.insertLineBreakInActiveBlock()
                } label: {
                    Image(systemName: "arrow.turn.down.left")
                }
                .disabled(!session.canInsertLineBreak)
                .accessibilityLabel("Insert line break")

                Button {
                    session.outdentActiveBlock()
                } label: {
                    Image(systemName: "decrease.indent")
                }
                .disabled(!session.canOutdent)
                .accessibilityLabel("Outdent block")

                Button {
                    session.indentActiveBlock()
                } label: {
                    Image(systemName: "increase.indent")
                }
                .disabled(!session.canIndent)
                .accessibilityLabel("Indent block")

                Button {
                    session.appendBlockAfterActive()
                } label: {
                    Image(systemName: "plus")
                }
                .accessibilityLabel("Add block")
            }
        }
        .onChange(of: session.blocks) { _, newBlocks in
            synchronizeMarkdown(from: newBlocks)
        }
        .onDisappear {
            saveTask?.cancel()
        }
    }

    private var editorFooter: some View {
        HStack(spacing: 10) {
            editorActionsMenu(iconOnly: true)

            Button {
                session.appendBlock()
            } label: {
                Label("New block", systemImage: "plus")
                    .font(.system(size: 13, weight: .medium))
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("markdown-add-block")

            Spacer(minLength: 8)

            SaveStateLabel(state: saveState)
        }
        .foregroundStyle(BlackwoodPalette.mutedForeground)
        .padding(.horizontal, 4)
        .padding(.top, 8)
        .padding(.bottom, 4)
    }

    private func editorActionsMenu(iconOnly: Bool) -> some View {
        Menu {
            Menu("Block type", systemImage: "textformat.size") {
                Button("Text") { session.changeActiveBlockKind(to: .paragraph) }
                Button("Heading 1") { session.changeActiveBlockKind(to: .heading(level: 1)) }
                Button("Heading 2") { session.changeActiveBlockKind(to: .heading(level: 2)) }
                Button("Heading 3") { session.changeActiveBlockKind(to: .heading(level: 3)) }
                Button("Bulleted list") { session.changeActiveBlockKind(to: .bullet(indentation: session.activeKind.indentation)) }
                Button("Numbered list") { session.changeActiveBlockKind(to: .numbered(ordinal: 1, indentation: session.activeKind.indentation)) }
                Button("Checklist") { session.changeActiveBlockKind(to: .task(isChecked: false, indentation: session.activeKind.indentation)) }
                Button("Quote") { session.changeActiveBlockKind(to: .quote) }
                Button("Code") { session.changeActiveBlockKind(to: .code(language: nil)) }
                Button("Divider") { session.changeActiveBlockKind(to: .divider) }
            }

            Menu("Inline Markdown", systemImage: "bold.italic.underline") {
                Button("Bold") { session.wrapSelection(prefix: "**", suffix: "**") }
                Button("Italic") { session.wrapSelection(prefix: "_", suffix: "_") }
                Button("Inline code") { session.wrapSelection(prefix: "`", suffix: "`") }
                Button("Wikilink") { session.wrapSelection(prefix: "[[", suffix: "]]" ) }
            }

            Divider()
            Button("Insert line break", systemImage: "arrow.turn.down.left") {
                session.insertLineBreakInActiveBlock()
            }
            .disabled(!session.canInsertLineBreak)
            Button("Add block below", systemImage: "plus") { session.appendBlockAfterActive() }
            Button("Outdent", systemImage: "decrease.indent") { session.outdentActiveBlock() }
                .disabled(!session.canOutdent)
            Button("Indent", systemImage: "increase.indent") { session.indentActiveBlock() }
                .disabled(!session.canIndent)
            Button("Move up", systemImage: "arrow.up") { session.moveActiveBlock(by: -1) }
                .disabled(!session.canMoveUp)
            Button("Move down", systemImage: "arrow.down") { session.moveActiveBlock(by: 1) }
                .disabled(!session.canMoveDown)
            Button("Delete block", systemImage: "trash", role: .destructive) { session.deleteActiveBlock() }
        } label: {
            if iconOnly {
                Image(systemName: "textformat")
            } else {
                Label(activeKindLabel, systemImage: activeKindIcon)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(BlackwoodPalette.foreground)
            }
        }
        .accessibilityLabel("Markdown block and formatting menu")
        .accessibilityValue(activeKindLabel)
    }

    private var activeKind: MarkdownBlockKind {
        session.activeKind
    }

    private var activeKindLabel: String {
        switch activeKind {
        case .paragraph: return "Text"
        case .heading(let level): return "Heading \(level)"
        case .bullet: return "Bulleted list"
        case .numbered: return "Numbered list"
        case .task: return "Checklist"
        case .quote: return "Quote"
        case .code: return "Code"
        case .divider: return "Divider"
        case .media: return "Attachment"
        }
    }

    private var activeKindIcon: String {
        switch activeKind {
        case .paragraph: return "text.alignleft"
        case .heading: return "textformat.size"
        case .bullet: return "list.bullet"
        case .numbered: return "list.number"
        case .task: return "checklist"
        case .quote: return "text.quote"
        case .code: return "chevron.left.forwardslash.chevron.right"
        case .divider: return "minus"
        case .media: return "paperclip"
        }
    }

    private func rowPlaceholder(for block: MarkdownEditorBlock) -> String {
        if session.blocks.count == 1, block.text.isEmpty {
            return placeholder
        }
        switch block.kind {
        case .heading: return "Heading"
        case .code: return "Code"
        case .quote: return "Quote"
        case .media: return "Attachment Markdown"
        default: return "Type something"
        }
    }

    private func focusBinding(for id: UUID) -> Binding<Bool> {
        Binding(
            get: { session.focusedBlockID == id },
            set: { focused in
                session.updateFocus(for: id, isFocused: focused)
            }
        )
    }

    private func selectionBinding(for id: UUID) -> Binding<NSRange> {
        Binding(
            get: { session.focusedBlockID == id ? session.selectedRange : NSRange(location: 0, length: 0) },
            set: { range in
                guard session.focusedBlockID == id else { return }
                session.selectedRange = range
            }
        )
    }

    private func synchronizeMarkdown(from blocks: [MarkdownEditorBlock]) {
        let updatedMarkdown = MarkdownDocument(blocks: blocks).markdown
        guard updatedMarkdown != markdown else { return }
        markdown = updatedMarkdown
        scheduleSave(updatedMarkdown)
    }

    private func scheduleSave(_ content: String) {
        saveTask?.cancel()
        saveState = .pending
        saveTask = Task { @MainActor in
            do {
                try await Task.sleep(for: .milliseconds(900))
            } catch {
                return
            }
            guard !Task.isCancelled else { return }
            saveState = .saving
            let didSave = await onSave(content)
            guard !Task.isCancelled else { return }
            guard session.markdown == content else { return }
            saveState = didSave ? .saved : .failed
            guard didSave else { return }
            do {
                try await Task.sleep(for: .seconds(2))
            } catch {
                return
            }
            if saveState == .saved {
                saveState = .idle
            }
        }
    }

}

private struct SaveStateLabel: View {
    let state: MarkdownCellEditor.SaveState

    var body: some View {
        HStack(spacing: 5) {
            if state == .saving {
                ProgressView()
                    .controlSize(.mini)
            } else if let icon {
                Image(systemName: icon)
                    .font(.system(size: 10, weight: .semibold))
            }

            Text(label)
                .font(.system(size: 11, weight: .medium))
        }
        .foregroundStyle(tint)
        .opacity(state == .idle ? 0 : 1)
        .animation(.easeInOut(duration: 0.18), value: state)
        .accessibilityLabel(label)
        .accessibilityHidden(state == .idle)
    }

    private var label: String {
        switch state {
        case .idle: return "Saved"
        case .pending: return "Edited"
        case .saving: return "Saving"
        case .saved: return "Saved"
        case .failed: return "Save failed"
        }
    }

    private var icon: String? {
        switch state {
        case .idle, .saving: return nil
        case .pending: return "circle.fill"
        case .saved: return "checkmark.circle.fill"
        case .failed: return "exclamationmark.triangle.fill"
        }
    }

    private var tint: Color {
        switch state {
        case .idle, .pending, .saving: return BlackwoodPalette.mutedForeground
        case .saved: return BlackwoodPalette.success
        case .failed: return BlackwoodPalette.destructive
        }
    }
}
