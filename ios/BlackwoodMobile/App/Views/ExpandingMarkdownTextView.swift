import SwiftUI
import UIKit

enum MarkdownReturnBehavior {
    case createBlock
    case multiline
    case newline
}

struct ExpandingMarkdownTextView: UIViewRepresentable {
    @Binding var text: String
    @Binding var isFocused: Bool
    @Binding var selectedRange: NSRange
    let baseFont: UIFont
    let isCode: Bool
    let returnBehavior: MarkdownReturnBehavior
    let accessibilityLabel: String
    let onTextChanged: () -> Void
    let onReturn: (NSRange) -> Void
    let onEndMultilineBlock: (NSRange) -> Void
    let onBackspaceAtStart: () -> Void

    func makeCoordinator() -> Coordinator {
        Coordinator(parent: self)
    }

    func makeUIView(context: Context) -> UITextView {
        let textView = UITextView()
        textView.delegate = context.coordinator
        textView.backgroundColor = .clear
        textView.isScrollEnabled = false
        textView.adjustsFontForContentSizeCategory = true
        textView.textContainerInset = UIEdgeInsets(top: 7, left: 0, bottom: 7, right: 0)
        textView.textContainer.lineFragmentPadding = 0
        textView.setContentCompressionResistancePriority(.defaultLow, for: .horizontal)
        textView.autocorrectionType = isCode ? .no : .default
        textView.autocapitalizationType = isCode ? .none : .sentences
        textView.smartDashesType = isCode ? .no : .default
        textView.smartQuotesType = isCode ? .no : .default
        textView.keyboardDismissMode = .interactive
        textView.tintColor = UIColor(BlackwoodPalette.accent)
        textView.accessibilityLabel = accessibilityLabel
        context.coordinator.applyStyle(to: textView)
        return textView
    }

    func updateUIView(_ textView: UITextView, context: Context) {
        context.coordinator.parent = self
        textView.accessibilityLabel = accessibilityLabel
        textView.autocorrectionType = isCode ? .no : .default
        textView.autocapitalizationType = isCode ? .none : .sentences
        textView.smartDashesType = isCode ? .no : .default
        textView.smartQuotesType = isCode ? .no : .default
        context.coordinator.applyStyle(to: textView)

        context.coordinator.reconcileFocus(in: textView, shouldFocus: isFocused)

        if isFocused, textView.selectedRange != selectedRange {
            let length = (textView.text as NSString).length
            let location = min(max(selectedRange.location, 0), length)
            let rangeLength = min(max(selectedRange.length, 0), length - location)
            textView.selectedRange = NSRange(location: location, length: rangeLength)
        }
    }

    func sizeThatFits(_ proposal: ProposedViewSize, uiView: UITextView, context: Context) -> CGSize? {
        guard let width = proposal.width else { return nil }
        let fittingSize = uiView.sizeThatFits(CGSize(width: width, height: .greatestFiniteMagnitude))
        return CGSize(width: width, height: max(fittingSize.height, 42))
    }

    static func dismantleUIView(_ uiView: UITextView, coordinator: Coordinator) {
        coordinator.cancelPendingFocusWork()
        uiView.delegate = nil
    }

    final class Coordinator: NSObject, UITextViewDelegate {
        var parent: ExpandingMarkdownTextView
        private var lastStyledText: String?
        private var lastStyleSignature: String?
        private var focusRequestGeneration = 0
        private var pendingFocusClear: DispatchWorkItem?

        init(parent: ExpandingMarkdownTextView) {
            self.parent = parent
        }

        func textViewDidBeginEditing(_ textView: UITextView) {
            focusRequestGeneration &+= 1
            pendingFocusClear?.cancel()
            pendingFocusClear = nil
            parent.isFocused = true
            parent.selectedRange = textView.selectedRange
        }

        func textViewDidEndEditing(_ textView: UITextView) {
            pendingFocusClear?.cancel()
            let expectedGeneration = focusRequestGeneration
            // SwiftUI can briefly end editing while reconciling a representable.
            // Give the same view a chance to reclaim first responder before
            // treating this as an intentional keyboard dismissal.
            let workItem = DispatchWorkItem { [weak self, weak textView] in
                guard let self, let textView else { return }
                guard !textView.isFirstResponder, textView.window != nil else { return }
                guard self.focusRequestGeneration == expectedGeneration else { return }
                guard self.parent.isFocused else { return }
                self.parent.isFocused = false
            }
            pendingFocusClear = workItem
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.08, execute: workItem)
        }

        func reconcileFocus(in textView: UITextView, shouldFocus: Bool) {
            if shouldFocus, !textView.isFirstResponder {
                focusRequestGeneration &+= 1
                let requestGeneration = focusRequestGeneration
                DispatchQueue.main.async { [weak self, weak textView] in
                    guard let self, let textView else { return }
                    guard self.focusRequestGeneration == requestGeneration else { return }
                    guard self.parent.isFocused, textView.window != nil else { return }
                    textView.becomeFirstResponder()
                }
            } else if !shouldFocus, textView.isFirstResponder {
                focusRequestGeneration &+= 1
                textView.resignFirstResponder()
            }
        }

        func cancelPendingFocusWork() {
            focusRequestGeneration &+= 1
            pendingFocusClear?.cancel()
            pendingFocusClear = nil
        }

        func textViewDidChange(_ textView: UITextView) {
            let updatedText = textView.text ?? ""
            parent.text = updatedText
            parent.selectedRange = textView.selectedRange
            applyStyle(to: textView, text: updatedText)
            textView.invalidateIntrinsicContentSize()
            if textView.markedTextRange == nil {
                parent.onTextChanged()
            }
        }

        func textViewDidChangeSelection(_ textView: UITextView) {
            guard textView.isFirstResponder else { return }
            parent.selectedRange = textView.selectedRange
        }

        func textView(
            _ textView: UITextView,
            shouldChangeTextIn range: NSRange,
            replacementText replacement: String
        ) -> Bool {
            if replacement == "\n" {
                switch parent.returnBehavior {
                case .createBlock:
                    parent.onReturn(range)
                    return false
                case .multiline:
                    let source = textView.text as NSString
                    if range.length == 0,
                       range.location == source.length,
                       source.hasSuffix("\n") {
                        parent.onEndMultilineBlock(range)
                        return false
                    }
                    return true
                case .newline:
                    return true
                }
            }
            if replacement.isEmpty, range.location == 0, range.length == 0 {
                parent.onBackspaceAtStart()
                return false
            }
            return true
        }

        func applyStyle(to textView: UITextView, text: String? = nil) {
            guard textView.markedTextRange == nil else { return }
            let selection = textView.selectedRange
            let content = text ?? parent.text
            let styleSignature = [
                parent.baseFont.fontDescriptor.postscriptName,
                String(describing: parent.baseFont.pointSize),
                String(parent.baseFont.fontDescriptor.symbolicTraits.rawValue),
                String(textView.traitCollection.userInterfaceStyle.rawValue),
                String(textView.traitCollection.accessibilityContrast.rawValue),
            ].joined(separator: "|")
            if textView.textStorage.string == content,
               lastStyledText == content,
               lastStyleSignature == styleSignature {
                return
            }
            let attributed = MarkdownInlineStyler.attributedString(
                content,
                baseFont: parent.baseFont,
                foreground: UIColor(BlackwoodPalette.foreground),
                muted: UIColor(BlackwoodPalette.mutedForeground),
                accent: UIColor(BlackwoodPalette.accent),
                codeBackground: UIColor(BlackwoodPalette.muted).withAlphaComponent(0.72)
            )
            let storage = textView.textStorage
            storage.beginEditing()
            if storage.string != content {
                storage.replaceCharacters(in: NSRange(location: 0, length: storage.length), with: content)
            }
            let fullRange = NSRange(location: 0, length: storage.length)
            storage.setAttributes([:], range: fullRange)
            attributed.enumerateAttributes(in: fullRange) { attributes, range, _ in
                storage.addAttributes(attributes, range: range)
            }
            storage.endEditing()
            let length = attributed.length
            let location = min(max(selection.location, 0), length)
            let rangeLength = min(max(selection.length, 0), length - location)
            textView.selectedRange = NSRange(location: location, length: rangeLength)
            textView.typingAttributes = [
                .font: parent.baseFont,
                .foregroundColor: UIColor(BlackwoodPalette.foreground),
            ]
            lastStyledText = content
            lastStyleSignature = styleSignature
        }
    }
}

private enum MarkdownInlineStyler {
    private static let boldExpression = try? NSRegularExpression(
        pattern: #"\*\*([^*\n]+)\*\*|__([^_\n]+)__"#
    )
    private static let italicExpression = try? NSRegularExpression(
        pattern: #"(?<!\*)\*([^*\n]+)\*(?!\*)|(?<!_)_([^_\n]+)_(?!_)"#
    )
    private static let codeExpression = try? NSRegularExpression(pattern: #"`([^`\n]+)`"#)
    private static let wikilinkExpression = try? NSRegularExpression(pattern: #"\[\[([^\]]+)\]\]"#)
    private static let linkExpression = try? NSRegularExpression(pattern: #"\[([^\]]+)\]\(([^)]+)\)"#)

    static func attributedString(
        _ text: String,
        baseFont: UIFont,
        foreground: UIColor,
        muted: UIColor,
        accent: UIColor,
        codeBackground: UIColor
    ) -> NSAttributedString {
        let fullRange = NSRange(location: 0, length: (text as NSString).length)
        let result = NSMutableAttributedString(
            string: text,
            attributes: [
                .font: baseFont,
                .foregroundColor: foreground,
            ]
        )

        style(
            expression: boldExpression,
            text: text,
            result: result,
            fullRange: fullRange,
            contentAttributes: [.font: font(baseFont, traits: .traitBold)],
            markerColor: muted
        )
        style(
            expression: italicExpression,
            text: text,
            result: result,
            fullRange: fullRange,
            contentAttributes: [.font: font(baseFont, traits: .traitItalic)],
            markerColor: muted
        )
        style(
            expression: codeExpression,
            text: text,
            result: result,
            fullRange: fullRange,
            contentAttributes: [
                .font: UIFont.monospacedSystemFont(ofSize: baseFont.pointSize * 0.94, weight: .regular),
                .backgroundColor: codeBackground,
            ],
            markerColor: muted
        )

        apply(expression: wikilinkExpression, text: text, result: result, fullRange: fullRange) { match in
            result.addAttributes([.foregroundColor: accent, .underlineStyle: NSUnderlineStyle.single.rawValue], range: match.range)
        }
        apply(expression: linkExpression, text: text, result: result, fullRange: fullRange) { match in
            result.addAttributes([.foregroundColor: accent, .underlineStyle: NSUnderlineStyle.single.rawValue], range: match.range)
        }

        return result
    }

    private static func style(
        expression: NSRegularExpression?,
        text: String,
        result: NSMutableAttributedString,
        fullRange: NSRange,
        contentAttributes: [NSAttributedString.Key: Any],
        markerColor: UIColor
    ) {
        apply(expression: expression, text: text, result: result, fullRange: fullRange) { match in
            result.addAttribute(.foregroundColor, value: markerColor, range: match.range)
            for captureIndex in 1..<match.numberOfRanges where match.range(at: captureIndex).location != NSNotFound {
                result.addAttributes(contentAttributes, range: match.range(at: captureIndex))
                result.addAttribute(
                    .foregroundColor,
                    value: UIColor(BlackwoodPalette.foreground),
                    range: match.range(at: captureIndex)
                )
            }
        }
    }

    private static func apply(
        expression: NSRegularExpression?,
        text: String,
        result: NSMutableAttributedString,
        fullRange: NSRange,
        action: (NSTextCheckingResult) -> Void
    ) {
        guard let expression else { return }
        expression.matches(in: text, range: fullRange).forEach(action)
    }

    private static func font(_ baseFont: UIFont, traits: UIFontDescriptor.SymbolicTraits) -> UIFont {
        guard let descriptor = baseFont.fontDescriptor.withSymbolicTraits(traits) else { return baseFont }
        return UIFont(descriptor: descriptor, size: baseFont.pointSize)
    }
}
