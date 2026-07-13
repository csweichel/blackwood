import Foundation
import Testing
@testable import BlackwoodMobileCore

@Test func markdownEditingSessionSplitsAndContinuesListBlocks() {
    var session = MarkdownEditingSession(markdown: "1. First item")
    let firstID = session.blocks[0].id

    session.splitBlock(firstID, at: NSRange(location: 13, length: 0))

    #expect(session.blocks.count == 2)
    #expect(session.blocks[0].kind == .numbered(ordinal: 1, indentation: 0))
    #expect(session.blocks[1].kind == .numbered(ordinal: 2, indentation: 0))
    #expect(session.blocks[1].text.isEmpty)
    #expect(session.focusedBlockID == session.blocks[1].id)
}

@Test func markdownEditingSessionLeavesAnEmptyListOnReturn() {
    var session = MarkdownEditingSession(markdown: "")
    session.blocks[0].kind = .bullet(indentation: 0)
    let blockID = session.blocks[0].id

    session.splitBlock(blockID, at: NSRange(location: 0, length: 0))

    #expect(session.blocks.count == 1)
    #expect(session.blocks[0].kind == .paragraph)
}

@Test func markdownEditingSessionAppliesShortcutsAndIndentation() {
    var session = MarkdownEditingSession(markdown: "")
    let blockID = session.blocks[0].id
    session.select(blockID)
    session.blocks[0].text = "- "

    let didApplyShortcut = session.applyMarkdownShortcutIfNeeded(to: blockID)
    #expect(didApplyShortcut)
    #expect(session.blocks[0].kind == .bullet(indentation: 0))
    #expect(session.focusedBlockID == blockID)
    session.indentActiveBlock()
    #expect(session.blocks[0].kind == .bullet(indentation: 2))
    session.outdentActiveBlock()
    #expect(session.blocks[0].kind == .bullet(indentation: 0))
}

@Test func markdownEditingSessionWrapsUTF16Selection() {
    var session = MarkdownEditingSession(markdown: "Hello 👋 world")
    session.select(session.blocks[0].id, atEnd: false)
    session.selectedRange = NSRange(location: 6, length: 2)

    session.wrapSelection(prefix: "**", suffix: "**")

    #expect(session.blocks[0].text == "Hello **👋** world")
    #expect(session.selectedRange == NSRange(location: 8, length: 2))
}

@Test func markdownEditingSessionBackspaceMergesParagraphBlocks() {
    var session = MarkdownEditingSession(markdown: "First\n\nSecond")
    let secondID = session.blocks[1].id

    session.handleBackspaceAtStart(in: secondID)

    #expect(session.blocks.count == 1)
    #expect(session.blocks[0].text == "FirstSecond")
    #expect(session.selectedRange == NSRange(location: 5, length: 0))
}

@Test func markdownEditingSessionEndsMultilineParagraphWithoutKeepingBlankLine() {
    var session = MarkdownEditingSession(markdown: "First line\nSecond line")
    let blockID = session.blocks[0].id
    session.blocks[0].text += "\n"

    session.endMultilineBlock(
        blockID,
        at: NSRange(location: (session.blocks[0].text as NSString).length, length: 0)
    )

    #expect(session.blocks.count == 2)
    #expect(session.blocks[0].text == "First line\nSecond line")
    #expect(session.blocks[1].kind == .paragraph)
    #expect(session.blocks[1].text.isEmpty)
    #expect(session.markdown == "First line\nSecond line")
    #expect(session.focusedBlockID == session.blocks[1].id)
}

@Test func markdownEditingSessionInsertsLineBreakAtUTF16Selection() {
    var session = MarkdownEditingSession(markdown: "Hello 👋world")
    session.select(session.blocks[0].id, atEnd: false)
    session.selectedRange = NSRange(location: 8, length: 0)

    session.insertLineBreakInActiveBlock()

    #expect(session.blocks[0].text == "Hello 👋\nworld")
    #expect(session.selectedRange == NSRange(location: 9, length: 0))
}
