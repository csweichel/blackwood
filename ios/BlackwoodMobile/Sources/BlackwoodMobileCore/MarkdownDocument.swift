import Foundation

public enum MarkdownBlockKind: Equatable, Sendable {
    case paragraph
    case heading(level: Int)
    case bullet(indentation: Int)
    case numbered(ordinal: Int, indentation: Int)
    case task(isChecked: Bool, indentation: Int)
    case quote
    case code(language: String?)
    case divider
    case media

    public var isList: Bool {
        switch self {
        case .bullet, .numbered, .task:
            return true
        default:
            return false
        }
    }

    public var supportsLineBreaks: Bool {
        switch self {
        case .paragraph, .quote, .code:
            return true
        default:
            return false
        }
    }

    public var indentation: Int {
        switch self {
        case .bullet(let indentation), .numbered(_, let indentation), .task(_, let indentation):
            return indentation
        default:
            return 0
        }
    }

    public func withIndentation(_ indentation: Int) -> MarkdownBlockKind {
        let normalized = max(0, min(indentation, 12))
        switch self {
        case .bullet:
            return .bullet(indentation: normalized)
        case .numbered(let ordinal, _):
            return .numbered(ordinal: ordinal, indentation: normalized)
        case .task(let isChecked, _):
            return .task(isChecked: isChecked, indentation: normalized)
        default:
            return self
        }
    }
}

public struct MarkdownEditorBlock: Identifiable, Equatable, Sendable {
    public let id: UUID
    public var kind: MarkdownBlockKind
    public var text: String

    public init(id: UUID = UUID(), kind: MarkdownBlockKind, text: String) {
        self.id = id
        self.kind = kind
        self.text = text
    }
}

public struct MarkdownDocument: Equatable, Sendable {
    public var blocks: [MarkdownEditorBlock]

    public init(blocks: [MarkdownEditorBlock]) {
        self.blocks = blocks.isEmpty
            ? [MarkdownEditorBlock(kind: .paragraph, text: "")]
            : blocks
    }

    public init(markdown: String) {
        let visibleMarkdown = MarkdownStorage.visibleMarkdown(from: markdown)
            .replacingOccurrences(of: "\r\n", with: "\n")
            .replacingOccurrences(of: "\r", with: "\n")
        self.init(blocks: Self.parse(lines: visibleMarkdown.components(separatedBy: "\n")))
    }

    public var markdown: String {
        guard !blocks.isEmpty else { return "" }

        var result = ""
        for index in blocks.indices {
            if index > blocks.startIndex {
                let previous = blocks[blocks.index(before: index)]
                let current = blocks[index]
                result += previous.kind.isList && current.kind.isList ? "\n" : "\n\n"
            }
            result += Self.markdown(for: blocks[index])
        }
        return result.trimmingCharacters(in: .newlines)
    }

    private static func parse(lines: [String]) -> [MarkdownEditorBlock] {
        var result: [MarkdownEditorBlock] = []
        var paragraph: [String] = []
        var index = 0

        func flushParagraph() {
            guard !paragraph.isEmpty else { return }
            let text = paragraph.joined(separator: "\n")
            if isStandaloneMedia(text) {
                result.append(MarkdownEditorBlock(kind: .media, text: text))
            } else {
                result.append(MarkdownEditorBlock(kind: .paragraph, text: text))
            }
            paragraph.removeAll()
        }

        while index < lines.count {
            let rawLine = lines[index]
            let trimmed = rawLine.trimmingCharacters(in: .whitespaces)

            if trimmed.isEmpty {
                flushParagraph()
                index += 1
                continue
            }

            if let fence = codeFence(from: trimmed) {
                flushParagraph()
                var codeLines: [String] = []
                index += 1
                while index < lines.count {
                    if lines[index].trimmingCharacters(in: .whitespaces).hasPrefix(fence.marker) {
                        index += 1
                        break
                    }
                    codeLines.append(lines[index])
                    index += 1
                }
                result.append(
                    MarkdownEditorBlock(
                        kind: .code(language: fence.language),
                        text: codeLines.joined(separator: "\n")
                    )
                )
                continue
            }

            if let heading = heading(from: trimmed) {
                flushParagraph()
                result.append(MarkdownEditorBlock(kind: .heading(level: heading.level), text: heading.text))
                index += 1
                continue
            }

            if trimmed == "---" || trimmed == "***" || trimmed == "___" {
                flushParagraph()
                result.append(MarkdownEditorBlock(kind: .divider, text: ""))
                index += 1
                continue
            }

            if let list = listItem(from: rawLine) {
                flushParagraph()
                result.append(MarkdownEditorBlock(kind: list.kind, text: list.text))
                index += 1
                continue
            }

            if trimmed.hasPrefix(">") {
                flushParagraph()
                var quoteLines: [String] = []
                while index < lines.count {
                    let quoteLine = lines[index].trimmingCharacters(in: .whitespaces)
                    guard quoteLine.hasPrefix(">") else { break }
                    var body = String(quoteLine.dropFirst())
                    if body.hasPrefix(" ") {
                        body.removeFirst()
                    }
                    quoteLines.append(body)
                    index += 1
                }
                result.append(MarkdownEditorBlock(kind: .quote, text: quoteLines.joined(separator: "\n")))
                continue
            }

            paragraph.append(rawLine)
            index += 1
        }

        flushParagraph()
        return result
    }

    private static func markdown(for block: MarkdownEditorBlock) -> String {
        switch block.kind {
        case .paragraph, .media:
            return block.text
        case .heading(let level):
            return "\(String(repeating: "#", count: max(1, min(level, 3)))) \(block.text)"
        case .bullet(let indentation):
            return "\(spaces(indentation))-\(block.text.isEmpty ? "" : " \(block.text)")"
        case .numbered(let ordinal, let indentation):
            return "\(spaces(indentation))\(max(ordinal, 1)).\(block.text.isEmpty ? "" : " \(block.text)")"
        case .task(let isChecked, let indentation):
            return "\(spaces(indentation))- [\(isChecked ? "x" : " ")]\(block.text.isEmpty ? "" : " \(block.text)")"
        case .quote:
            return block.text
                .components(separatedBy: .newlines)
                .map { $0.isEmpty ? ">" : "> \($0)" }
                .joined(separator: "\n")
        case .code(let language):
            let fence = block.text.contains("```") ? "~~~" : "```"
            let languageSuffix = language?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            return "\(fence)\(languageSuffix)\n\(block.text)\n\(fence)"
        case .divider:
            return "---"
        }
    }

    private static func heading(from line: String) -> (level: Int, text: String)? {
        for level in stride(from: 3, through: 1, by: -1) {
            let prefix = String(repeating: "#", count: level) + " "
            if line.hasPrefix(prefix) {
                return (level, String(line.dropFirst(prefix.count)))
            }
        }
        return nil
    }

    private static func listItem(from rawLine: String) -> (kind: MarkdownBlockKind, text: String)? {
        let indentation = rawLine.prefix { $0 == " " || $0 == "\t" }.reduce(into: 0) { count, character in
            count += character == "\t" ? 4 : 1
        }
        let line = String(rawLine.drop { $0 == " " || $0 == "\t" })

        for marker in ["-", "*", "+"] where line == marker {
            return (.bullet(indentation: indentation), "")
        }

        for marker in ["- ", "* ", "+ "] where line.hasPrefix(marker) {
            let text = String(line.dropFirst(marker.count))
            if text == "[ ]" {
                return (.task(isChecked: false, indentation: indentation), "")
            }
            if text == "[x]" || text == "[X]" {
                return (.task(isChecked: true, indentation: indentation), "")
            }
            if text.hasPrefix("[ ] ") {
                return (.task(isChecked: false, indentation: indentation), String(text.dropFirst(4)))
            }
            if text.hasPrefix("[x] ") || text.hasPrefix("[X] ") {
                return (.task(isChecked: true, indentation: indentation), String(text.dropFirst(4)))
            }
            return (.bullet(indentation: indentation), text)
        }

        guard let period = line.firstIndex(of: ".") else { return nil }
        let ordinalText = line[..<period]
        guard let ordinal = Int(ordinalText), ordinal > 0 else { return nil }
        let bodyStart = line.index(after: period)
        if bodyStart == line.endIndex {
            return (.numbered(ordinal: ordinal, indentation: indentation), "")
        }
        guard line[bodyStart] == " " else { return nil }
        let textStart = line.index(after: bodyStart)
        return (.numbered(ordinal: ordinal, indentation: indentation), String(line[textStart...]))
    }

    private static func codeFence(from line: String) -> (marker: String, language: String?)? {
        guard line.hasPrefix("```") || line.hasPrefix("~~~") else { return nil }
        let marker = String(line.prefix(3))
        let language = String(line.dropFirst(3)).trimmingCharacters(in: .whitespacesAndNewlines)
        return (marker, language.isEmpty ? nil : language)
    }

    private static func isStandaloneMedia(_ text: String) -> Bool {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let patterns = [
            #"^!\[[^\]]*\]\([^)]+\)$"#,
            #"^\[[^\]]+\]\([^\s)]+\.(?:apng|avif|bmp|gif|heic|heif|jpe?g|png|svg|tiff?|webp)(?:[?#][^)]*)?\)$"#,
            #"^<img\b[^>]*>$"#,
        ]
        return patterns.contains { trimmed.range(of: $0, options: [.regularExpression, .caseInsensitive]) != nil }
    }

    private static func spaces(_ count: Int) -> String {
        String(repeating: " ", count: max(0, min(count, 12)))
    }
}
