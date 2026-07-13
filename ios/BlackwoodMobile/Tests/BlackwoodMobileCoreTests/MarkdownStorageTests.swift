import Testing
@testable import BlackwoodMobileCore

@Test
func visibleMarkdownStripsBlockStateTrailer() {
    let stored = """
    # Summary

    # Notes

    <!-- blackwood:block-state:v1
    {"version":1,"blocks":[]}
    -->
    """

    #expect(MarkdownStorage.visibleMarkdown(from: stored) == "# Summary\n\n# Notes")
}

@Test
func visibleMarkdownKeepsMalformedBlockStateText() {
    let stored = """
    # Notes

    <!-- blackwood:block-state:v1
    {"version":1}

    Still user-visible because the trailer never closed.
    """

    #expect(MarkdownStorage.visibleMarkdown(from: stored).contains("Still user-visible"))
}
