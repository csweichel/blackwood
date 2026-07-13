import Foundation

public struct MarkdownEditingSession: Equatable, Sendable {
    public var blocks: [MarkdownEditorBlock]
    public var activeBlockID: UUID?
    public var focusedBlockID: UUID?
    public var selectedRange: NSRange

    public init(markdown: String) {
        let parsedBlocks = MarkdownDocument(markdown: markdown).blocks
        blocks = parsedBlocks
        activeBlockID = parsedBlocks.first?.id
        focusedBlockID = nil
        selectedRange = NSRange(location: 0, length: 0)
    }

    public var markdown: String {
        MarkdownDocument(blocks: blocks).markdown
    }

    public var activeIndex: Int? {
        let selectedID = focusedBlockID ?? activeBlockID
        guard let selectedID else { return blocks.indices.first }
        return blocks.firstIndex { $0.id == selectedID } ?? blocks.indices.first
    }

    public var activeKind: MarkdownBlockKind {
        guard let activeIndex else { return .paragraph }
        return blocks[activeIndex].kind
    }

    public var canIndent: Bool {
        activeKind.isList && activeKind.indentation < 12
    }

    public var canOutdent: Bool {
        activeKind.isList && activeKind.indentation > 0
    }

    public var canMoveUp: Bool {
        guard let activeIndex else { return false }
        return activeIndex > blocks.startIndex
    }

    public var canMoveDown: Bool {
        guard let activeIndex else { return false }
        return activeIndex < blocks.index(before: blocks.endIndex)
    }

    public var canInsertLineBreak: Bool {
        activeKind.supportsLineBreaks
    }

    public mutating func replaceDocument(with markdown: String) {
        let document = MarkdownDocument(markdown: markdown)
        guard document.markdown != self.markdown else { return }
        blocks = document.blocks
        activeBlockID = blocks.first?.id
        focusedBlockID = nil
        selectedRange = NSRange(location: 0, length: 0)
    }

    public mutating func select(_ id: UUID, atEnd: Bool = true) {
        guard let index = blockIndex(id) else { return }
        activeBlockID = id
        focusedBlockID = id
        guard atEnd else { return }
        selectedRange = NSRange(location: utf16Length(blocks[index].text), length: 0)
    }

    public mutating func updateFocus(for id: UUID, isFocused: Bool) {
        guard blockIndex(id) != nil else { return }
        if isFocused {
            activeBlockID = id
            focusedBlockID = id
        } else if focusedBlockID == id {
            focusedBlockID = nil
        }
    }

    public mutating func activate(_ id: UUID) {
        guard let index = blockIndex(id) else { return }
        activeBlockID = id
        focusedBlockID = nil
        selectedRange = NSRange(location: utf16Length(blocks[index].text), length: 0)
    }

    public mutating func appendBlock() {
        let block = MarkdownEditorBlock(kind: .paragraph, text: "")
        blocks.append(block)
        focus(block, selection: NSRange(location: 0, length: 0))
    }

    public mutating func appendBlockAfterActive() {
        guard let activeIndex else {
            appendBlock()
            return
        }
        let block = MarkdownEditorBlock(kind: nextKind(after: blocks[activeIndex]), text: "")
        blocks.insert(block, at: blocks.index(after: activeIndex))
        focus(block, selection: NSRange(location: 0, length: 0))
    }

    public mutating func splitBlock(_ id: UUID, at range: NSRange) {
        guard let index = blockIndex(id) else { return }
        let block = blocks[index]
        let text = block.text as NSString
        let safeRange = clamped(range, toLength: text.length)

        if block.kind.isList, text.length == 0 {
            blocks[index].kind = .paragraph
            focus(blocks[index], selection: NSRange(location: 0, length: 0))
            return
        }

        blocks[index].text = text.substring(to: safeRange.location)
        let right = text.substring(from: safeRange.location + safeRange.length)
        let newBlock = MarkdownEditorBlock(kind: nextKind(after: block), text: right)
        blocks.insert(newBlock, at: blocks.index(after: index))
        focus(newBlock, selection: NSRange(location: 0, length: 0))
    }

    /// Ends a touch-edited multiline paragraph or quote after the user presses
    /// Return on the empty trailing line. The first Return is stored in the
    /// block; the second consumes it and creates a fresh text block.
    public mutating func endMultilineBlock(_ id: UUID, at range: NSRange) {
        guard let index = blockIndex(id), blocks[index].kind.supportsLineBreaks else { return }
        let source = blocks[index].text as NSString
        let safeRange = clamped(range, toLength: source.length)
        var left = source.substring(to: safeRange.location)
        let right = source.substring(from: safeRange.location + safeRange.length)

        if left.hasSuffix("\n") {
            left.removeLast()
        }

        blocks[index].text = left
        let newBlock = MarkdownEditorBlock(kind: .paragraph, text: right)
        blocks.insert(newBlock, at: blocks.index(after: index))
        focus(newBlock, selection: NSRange(location: 0, length: 0))
    }

    public mutating func handleBackspaceAtStart(in id: UUID) {
        guard let index = blockIndex(id) else { return }
        let block = blocks[index]

        if block.kind.isList, block.kind.indentation > 0 {
            blocks[index].kind = block.kind.withIndentation(block.kind.indentation - 2)
            focus(blocks[index], selection: NSRange(location: 0, length: 0))
            return
        }

        switch block.kind {
        case .paragraph, .media:
            break
        default:
            blocks[index].kind = .paragraph
            focus(blocks[index], selection: NSRange(location: 0, length: 0))
            return
        }

        guard index > blocks.startIndex else { return }
        let previousIndex = blocks.index(before: index)
        if blocks[previousIndex].kind == .divider {
            blocks.remove(at: previousIndex)
            focus(blocks[previousIndex], selection: NSRange(location: 0, length: 0))
            return
        }

        let insertionPoint = utf16Length(blocks[previousIndex].text)
        blocks[previousIndex].text += block.text
        let previousBlock = blocks[previousIndex]
        blocks.remove(at: index)
        focus(previousBlock, selection: NSRange(location: insertionPoint, length: 0))
    }

    public mutating func changeActiveBlockKind(to kind: MarkdownBlockKind) {
        guard let activeIndex else { return }
        blocks[activeIndex].kind = kind
        if kind == .divider {
            blocks[activeIndex].text = ""
            activeBlockID = blocks[activeIndex].id
            focusedBlockID = nil
        } else {
            focus(blocks[activeIndex], selection: selectedRange)
        }
    }

    @discardableResult
    public mutating func applyMarkdownShortcutIfNeeded(to id: UUID) -> Bool {
        guard let index = blockIndex(id), blocks[index].kind == .paragraph else { return false }

        let kind: MarkdownBlockKind?
        switch blocks[index].text {
        case "# ": kind = .heading(level: 1)
        case "## ": kind = .heading(level: 2)
        case "### ": kind = .heading(level: 3)
        case "- ", "* ": kind = .bullet(indentation: 0)
        case "1. ": kind = .numbered(ordinal: 1, indentation: 0)
        case "- [ ] ", "[] ": kind = .task(isChecked: false, indentation: 0)
        case "> ": kind = .quote
        case "``` ": kind = .code(language: nil)
        default: kind = nil
        }

        guard let kind else { return false }
        blocks[index].kind = kind
        blocks[index].text = ""
        focus(blocks[index], selection: NSRange(location: 0, length: 0))
        return true
    }

    public mutating func wrapSelection(prefix: String, suffix: String) {
        guard let activeIndex, blocks[activeIndex].kind != .divider else { return }
        let source = blocks[activeIndex].text as NSString
        let range = clamped(selectedRange, toLength: source.length)
        let replacement = prefix + source.substring(with: range) + suffix
        blocks[activeIndex].text = source.replacingCharacters(in: range, with: replacement)
        selectedRange = NSRange(location: range.location + utf16Length(prefix), length: range.length)
        focus(blocks[activeIndex], selection: selectedRange)
    }

    public mutating func insertLineBreakInActiveBlock() {
        guard let activeIndex, canInsertLineBreak else { return }
        let source = blocks[activeIndex].text as NSString
        let range = clamped(selectedRange, toLength: source.length)
        blocks[activeIndex].text = source.replacingCharacters(in: range, with: "\n")
        let cursor = NSRange(location: range.location + 1, length: 0)
        focus(blocks[activeIndex], selection: cursor)
    }

    public mutating func toggleTask(_ id: UUID) {
        guard let index = blockIndex(id) else { return }
        guard case .task(let isChecked, let indentation) = blocks[index].kind else { return }
        blocks[index].kind = .task(isChecked: !isChecked, indentation: indentation)
    }

    public mutating func indentActiveBlock() {
        guard let activeIndex, canIndent else { return }
        blocks[activeIndex].kind = activeKind.withIndentation(activeKind.indentation + 2)
    }

    public mutating func outdentActiveBlock() {
        guard let activeIndex, canOutdent else { return }
        blocks[activeIndex].kind = activeKind.withIndentation(activeKind.indentation - 2)
    }

    public mutating func moveActiveBlock(by offset: Int) {
        guard let activeIndex else { return }
        let destination = activeIndex + offset
        guard blocks.indices.contains(destination) else { return }
        blocks.swapAt(activeIndex, destination)
    }

    public mutating func deleteActiveBlock() {
        guard let activeIndex else { return }
        if blocks.count == 1 {
            blocks[0].kind = .paragraph
            blocks[0].text = ""
            focus(blocks[0], selection: NSRange(location: 0, length: 0))
            return
        }

        blocks.remove(at: activeIndex)
        let nextIndex = min(activeIndex, blocks.index(before: blocks.endIndex))
        focus(
            blocks[nextIndex],
            selection: NSRange(location: utf16Length(blocks[nextIndex].text), length: 0)
        )
    }

    private func blockIndex(_ id: UUID) -> Int? {
        blocks.firstIndex { $0.id == id }
    }

    private func nextKind(after block: MarkdownEditorBlock) -> MarkdownBlockKind {
        switch block.kind {
        case .bullet(let indentation):
            return .bullet(indentation: indentation)
        case .numbered(let ordinal, let indentation):
            return .numbered(ordinal: ordinal + 1, indentation: indentation)
        case .task(_, let indentation):
            return .task(isChecked: false, indentation: indentation)
        case .quote where !block.text.isEmpty:
            return .quote
        case .code:
            return block.kind
        default:
            return .paragraph
        }
    }

    private mutating func focus(_ block: MarkdownEditorBlock, selection: NSRange) {
        activeBlockID = block.id
        focusedBlockID = block.id
        selectedRange = selection
    }

    private func clamped(_ range: NSRange, toLength length: Int) -> NSRange {
        let location = min(max(range.location, 0), length)
        return NSRange(location: location, length: min(max(range.length, 0), length - location))
    }

    private func utf16Length(_ text: String) -> Int {
        (text as NSString).length
    }
}
