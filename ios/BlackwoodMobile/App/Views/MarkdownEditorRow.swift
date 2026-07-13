import SwiftUI
import UIKit

struct MarkdownEditorRow: View {
    @Binding var block: MarkdownEditorBlock
    let placeholder: String
    @Binding var isFocused: Bool
    @Binding var selectedRange: NSRange
    let onTextChanged: () -> Void
    let onReturn: (NSRange) -> Void
    let onEndMultilineBlock: (NSRange) -> Void
    let onBackspaceAtStart: () -> Void
    let onToggleTask: () -> Void
    let onSelect: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            if showsMarker {
                marker
                    .frame(width: 24, alignment: .center)
                    .accessibilityHidden(!isTask)
            }

            if block.kind == .divider {
                Button(action: onSelect) {
                    Rectangle()
                        .fill(BlackwoodPalette.border)
                        .frame(height: 1)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 20)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Divider block")
            } else if block.kind == .media {
                Button(action: onSelect) {
                    VStack(alignment: .leading, spacing: 3) {
                        Text(mediaTitle)
                            .font(.system(size: 16, weight: .semibold))
                            .foregroundStyle(BlackwoodPalette.foreground)
                        Text(mediaDetail)
                            .font(.system(size: 12))
                            .foregroundStyle(BlackwoodPalette.mutedForeground)
                            .lineLimit(1)
                    }
                    .frame(maxWidth: .infinity, minHeight: 42, alignment: .leading)
                    .padding(.horizontal, 5)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("\(mediaTitle), \(mediaDetail)")
            } else {
                ZStack(alignment: .topLeading) {
                    if block.text.isEmpty {
                        Text(placeholder)
                            .font(swiftUIFont)
                            .foregroundStyle(BlackwoodPalette.mutedForeground.opacity(0.72))
                            .padding(.horizontal, 5)
                            .padding(.vertical, 9)
                            .allowsHitTesting(false)
                            .accessibilityHidden(true)
                    }

                    ExpandingMarkdownTextView(
                        text: $block.text,
                        isFocused: $isFocused,
                        selectedRange: $selectedRange,
                        baseFont: uiFont,
                        isCode: isCode,
                        returnBehavior: returnBehavior,
                        accessibilityLabel: accessibilityLabel,
                        onTextChanged: onTextChanged,
                        onReturn: onReturn,
                        onEndMultilineBlock: onEndMultilineBlock,
                        onBackspaceAtStart: onBackspaceAtStart
                    )
                    .frame(minHeight: minimumHeight)
                }
            }
        }
        .padding(.leading, indentationPadding)
        .padding(.horizontal, 2)
        .padding(.vertical, 2)
        .contentShape(Rectangle())
    }

    @ViewBuilder
    private var marker: some View {
        switch block.kind {
        case .paragraph:
            EmptyView()
        case .heading:
            EmptyView()
        case .bullet:
            Text("•")
                .font(.system(size: 18, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.foreground)
                .padding(.top, 6)
        case .numbered(let ordinal, _):
            Text("\(ordinal).")
                .font(.system(size: 14, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.foreground)
                .padding(.top, 9)
        case .task(let isChecked, _):
            Button(action: onToggleTask) {
                Image(systemName: isChecked ? "checkmark.square.fill" : "square")
                    .font(.system(size: 17, weight: .semibold))
                    .foregroundStyle(isChecked ? BlackwoodPalette.success : BlackwoodPalette.mutedForeground)
                    .padding(.top, 8)
            }
            .buttonStyle(.plain)
            .accessibilityLabel(isChecked ? "Mark task incomplete" : "Mark task complete")
        case .quote:
            RoundedRectangle(cornerRadius: 1)
                .fill(BlackwoodPalette.accent)
                .frame(width: 3, height: 30)
                .padding(.top, 7)
        case .code:
            Image(systemName: "chevron.left.forwardslash.chevron.right")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.accent)
                .padding(.top, 11)
        case .divider:
            Image(systemName: "minus")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
                .padding(.top, 15)
        case .media:
            Image(systemName: "paperclip")
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.accent)
                .padding(.top, 10)
        }
    }

    private var indentationPadding: CGFloat {
        CGFloat(block.kind.indentation / 2) * 18
    }

    private var showsMarker: Bool {
        switch block.kind {
        case .paragraph, .heading, .divider:
            return false
        default:
            return true
        }
    }

    private var isCode: Bool {
        if case .code = block.kind { return true }
        return false
    }

    private var isTask: Bool {
        if case .task = block.kind { return true }
        return false
    }

    private var returnBehavior: MarkdownReturnBehavior {
        switch block.kind {
        case .paragraph, .quote:
            return .multiline
        case .code:
            return .newline
        default:
            return .createBlock
        }
    }

    private var minimumHeight: CGFloat {
        switch block.kind {
        case .heading(level: 1): return 48
        case .heading: return 42
        case .code: return 72
        default: return 42
        }
    }

    private var uiFont: UIFont {
        let font: UIFont
        switch block.kind {
        case .heading(level: 1):
            font = .systemFont(ofSize: 25, weight: .semibold)
        case .heading(level: 2):
            font = .systemFont(ofSize: 21, weight: .semibold)
        case .heading:
            font = .systemFont(ofSize: 18, weight: .semibold)
        case .code:
            font = .monospacedSystemFont(ofSize: 15, weight: .regular)
        case .quote:
            font = .italicSystemFont(ofSize: 17)
        default:
            font = .systemFont(ofSize: 17)
        }
        return UIFontMetrics.default.scaledFont(for: font)
    }

    private var swiftUIFont: Font {
        Font(uiFont)
    }

    private var accessibilityLabel: String {
        switch block.kind {
        case .paragraph: return "Text block"
        case .heading(let level): return "Heading \(level) block"
        case .bullet: return "Bulleted list block"
        case .numbered: return "Numbered list block"
        case .task: return "Checklist block"
        case .quote: return "Quote block"
        case .code: return "Code block"
        case .divider: return "Divider block"
        case .media: return "Attachment block"
        }
    }

    private var mediaTitle: String {
        block.text.range(of: #"^\s*<audio\b"#, options: [.regularExpression, .caseInsensitive]) == nil
            ? "Attachment"
            : "Voice recording"
    }

    private var mediaDetail: String {
        let patterns = [
            #"\bsrc=["']([^"']+)["']"#,
            #"\]\(([^)\s]+)"#,
        ]
        for pattern in patterns {
            guard let regex = try? NSRegularExpression(pattern: pattern, options: [.caseInsensitive]),
                  let match = regex.firstMatch(
                    in: block.text,
                    range: NSRange(block.text.startIndex..., in: block.text)
                  ),
                  let range = Range(match.range(at: 1), in: block.text) else { continue }
            let source = String(block.text[range])
            return URL(fileURLWithPath: source).lastPathComponent.removingPercentEncoding ?? source
        }
        return "Markdown attachment"
    }
}
