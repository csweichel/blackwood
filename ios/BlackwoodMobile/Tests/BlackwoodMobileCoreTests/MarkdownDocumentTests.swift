import Testing
@testable import BlackwoodMobileCore

@Test func markdownDocumentParsesAndSerializesWebCompatibleBlocks() {
    let markdown = """
    # Summary

    A **useful** summary with [[Project Atlas]].

    ## Notes

    - First item
      - Nested item
    - [x] Shipped
    1. Numbered item

    > A quoted thought
    > on two lines.

    ```swift
    let answer = 42
    ```

    ![Sketch](sketch.png)

    ---
    """

    let document = MarkdownDocument(markdown: markdown)

    #expect(document.blocks.map(\.kind) == [
        .heading(level: 1),
        .paragraph,
        .heading(level: 2),
        .bullet(indentation: 0),
        .bullet(indentation: 2),
        .task(isChecked: true, indentation: 0),
        .numbered(ordinal: 1, indentation: 0),
        .quote,
        .code(language: "swift"),
        .media,
        .divider,
    ])
    #expect(document.markdown == markdown)
}

@Test func markdownDocumentStripsWebBlockStateTrailer() {
    let markdown = "# Notes\n\nKeep this visible."
    let stored = """
    \(markdown)

    <!-- blackwood:block-state:v1
    {"version":1,"markdownHash":"deadbeef","blocks":[]}
    -->
    """

    let document = MarkdownDocument(markdown: stored)

    #expect(document.markdown == markdown)
}

@Test func markdownDocumentKeepsOneEditableBlockForEmptyNotes() {
    let document = MarkdownDocument(markdown: "")

    #expect(document.blocks.count == 1)
    #expect(document.blocks[0].kind == .paragraph)
    #expect(document.blocks[0].text.isEmpty)
    #expect(document.markdown.isEmpty)
}

@Test func markdownDocumentPreservesEmptyListBlocks() {
    let bullet = MarkdownDocument(markdown: "- ")
    let numbered = MarkdownDocument(markdown: "1. ")

    #expect(bullet.blocks[0].kind == .bullet(indentation: 0))
    #expect(bullet.blocks[0].text.isEmpty)
    #expect(bullet.markdown == "-")
    #expect(numbered.blocks[0].kind == .numbered(ordinal: 1, indentation: 0))
    #expect(numbered.blocks[0].text.isEmpty)
    #expect(numbered.markdown == "1.")
}

@Test func markdownDocumentKeepsMultilineParagraphsAndQuotesAsSingleBlocks() {
    let markdown = """
    First line
    second line

    > Quoted first line
    > quoted second line
    """

    let document = MarkdownDocument(markdown: markdown)

    #expect(document.blocks.count == 2)
    #expect(document.blocks[0].kind == .paragraph)
    #expect(document.blocks[0].text == "First line\nsecond line")
    #expect(document.blocks[1].kind == .quote)
    #expect(document.blocks[1].text == "Quoted first line\nquoted second line")
    #expect(document.markdown == markdown)
}
