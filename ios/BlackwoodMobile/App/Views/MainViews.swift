import SwiftUI

private enum BlackwoodPalette {
    static let background = Color(red: 250/255, green: 248/255, blue: 243/255)
    static let foreground = Color(red: 28/255, green: 36/255, blue: 51/255)
    static let card = Color(red: 250/255, green: 248/255, blue: 243/255)
    static let muted = Color(red: 239/255, green: 233/255, blue: 220/255)
    static let mutedForeground = Color(red: 106/255, green: 116/255, blue: 137/255)
    static let accent = Color(red: 74/255, green: 111/255, blue: 165/255)
    static let border = Color(red: 214/255, green: 206/255, blue: 188/255)
    static let destructive = Color(red: 184/255, green: 69/255, blue: 58/255)
    static let success = Color(red: 74/255, green: 139/255, blue: 92/255)
}

struct RootTabView: View {
    @ObservedObject var model: AppModel

    var body: some View {
        TabView(selection: $model.selectedTab) {
            TodayScreen(model: model)
                .tabItem { Label("Today", systemImage: "doc.text") }
                .tag(AppModel.Tab.today)

            SearchScreen(model: model)
                .tabItem { Label("Search", systemImage: "magnifyingglass") }
                .tag(AppModel.Tab.search)

            QueueScreen(model: model)
                .tabItem { Label("Queue", systemImage: "arrow.trianglehead.2.clockwise") }
                .tag(AppModel.Tab.queue)

            SettingsScreen(model: model)
                .tabItem { Label("Settings", systemImage: "gearshape") }
                .tag(AppModel.Tab.settings)
        }
        .tint(BlackwoodPalette.accent)
        .sheet(isPresented: $model.isRecordingSheetPresented) {
            RecordingSheet(model: model)
        }
    }
}

struct TodayScreen: View {
    @ObservedObject var model: AppModel

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    header(title: "Blackwood", subtitle: "Day")
                    DayCarousel(
                        selectedDate: model.selectedDate,
                        onSelectDate: { model.changeDate(to: $0) }
                    )

                    HStack(spacing: 10) {
                        actionButton("Record", systemImage: "mic.fill", filled: true) {
                            model.presentRecorder()
                        }

                        if model.isEditing {
                            actionButton("Cancel", systemImage: "xmark", filled: false) {
                                model.cancelEditing()
                            }
                            actionButton("Save", systemImage: "checkmark", filled: true) {
                                Task { await model.saveCurrentNote() }
                            }
                        } else {
                            actionButton("Edit", systemImage: "square.and.pencil", filled: false) {
                                model.beginEditing()
                            }
                        }
                    }

                    if let error = model.noteError, !error.isEmpty {
                        errorBanner(error)
                    }

                    card {
                        if model.isEditing {
                            TextEditor(text: $model.draftContent)
                                .font(.system(size: 17))
                                .scrollContentBackground(.hidden)
                                .frame(minHeight: 360)
                        } else if model.isLoadingNote && model.noteContent.isEmpty {
                            ProgressView("Loading note…")
                                .frame(maxWidth: .infinity, minHeight: 220)
                        } else {
                            StructuredNoteView(
                                content: model.noteContent,
                                baseURL: model.normalizedServerURL,
                                date: AppModel.dayString(from: model.selectedDate)
                            )
                        }
                    }
                }
                .frame(maxWidth: 680)
                .padding(.horizontal, 20)
                .padding(.vertical, 24)
            }
            .background(BlackwoodPalette.background.ignoresSafeArea())
            .navigationBarHidden(true)
        }
    }
}

struct SearchScreen: View {
    @ObservedObject var model: AppModel

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    header(title: "Blackwood", subtitle: "Search")

                    card {
                        HStack(spacing: 10) {
                            Image(systemName: "magnifyingglass")
                                .foregroundStyle(BlackwoodPalette.mutedForeground)
                            TextField("Search your notes…", text: $model.searchQuery)
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()
                        }

                        Button("Search") {
                            Task { await model.runSearch() }
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(BlackwoodPalette.accent)
                        .padding(.top, 12)
                    }

                    if model.isSearching {
                        card {
                            HStack(spacing: 10) {
                                ProgressView()
                                Text("Searching your notes…")
                                    .foregroundStyle(BlackwoodPalette.mutedForeground)
                            }
                        }
                    }

                    if let error = model.searchError, !error.isEmpty {
                        errorBanner(error)
                    }

                    if !model.isSearching && model.searchResults.isEmpty {
                        card {
                            Text("Search across all your notes using semantic search.")
                                .foregroundStyle(BlackwoodPalette.mutedForeground)
                        }
                    }

                    ForEach(groupedResults.keys.sorted().reversed(), id: \.self) { date in
                        VStack(alignment: .leading, spacing: 10) {
                            Text(formattedDate(date))
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundStyle(BlackwoodPalette.mutedForeground)

                            ForEach(groupedResults[date] ?? []) { result in
                                Button {
                                    model.openSearchResult(result)
                                } label: {
                                    card {
                                        Text(result.snippet)
                                            .font(.system(size: 16))
                                            .foregroundStyle(BlackwoodPalette.foreground)
                                    }
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                }
                .frame(maxWidth: 680)
                .padding(.horizontal, 20)
                .padding(.vertical, 24)
            }
            .background(BlackwoodPalette.background.ignoresSafeArea())
            .navigationBarHidden(true)
        }
    }

    private var groupedResults: [String: [SearchResult]] {
        Dictionary(grouping: model.searchResults, by: \.date)
    }

    private func formattedDate(_ date: String) -> String {
        guard let parsed = AppModel.date(from: date) else { return date }
        return parsed.formatted(.dateTime.weekday(.wide).month(.wide).day().year())
    }
}

struct QueueScreen: View {
    @ObservedObject var model: AppModel
    @State private var uploads: [PendingEntryUpload] = []

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    header(title: "Blackwood", subtitle: "Queue")

                    card {
                        VStack(alignment: .leading, spacing: 12) {
                            queueRow("Connection", model.isOnline ? "Online" : "Offline")
                            queueRow("Pending note saves", "\(model.queueSnapshot.noteUpdateCount)")
                            queueRow("Pending uploads", "\(model.queueSnapshot.uploadCount)")
                            queueRow("Failed uploads", "\(model.queueSnapshot.failedUploadCount)")

                            Button("Sync Now") {
                                Task {
                                    await model.syncNow()
                                    uploads = await model.pendingUploads()
                                }
                            }
                            .buttonStyle(.borderedProminent)
                            .tint(BlackwoodPalette.accent)
                        }
                    }

                    if uploads.isEmpty {
                        card {
                            Text("No queued recordings.")
                                .foregroundStyle(BlackwoodPalette.mutedForeground)
                        }
                    } else {
                        ForEach(uploads) { upload in
                            card {
                                VStack(alignment: .leading, spacing: 12) {
                                    Text("Voice memo")
                                        .font(.system(size: 20, weight: .semibold))
                                    Text("\(upload.date) • \(max(Int(upload.duration.rounded()), 1)) sec • \(upload.status.rawValue.capitalized)")
                                        .font(.system(size: 14))
                                        .foregroundStyle(BlackwoodPalette.mutedForeground)
                                    if let error = upload.lastError, !error.isEmpty {
                                        Text(error)
                                            .font(.system(size: 14))
                                            .foregroundStyle(BlackwoodPalette.destructive)
                                    }
                                    HStack(spacing: 10) {
                                        Button("Retry") {
                                            Task {
                                                await model.retryUpload(id: upload.id)
                                                uploads = await model.pendingUploads()
                                            }
                                        }
                                        .buttonStyle(.borderedProminent)
                                        .tint(BlackwoodPalette.accent)

                                        Button("Remove") {
                                            Task {
                                                await model.removeUpload(id: upload.id)
                                                uploads = await model.pendingUploads()
                                            }
                                        }
                                        .buttonStyle(.bordered)
                                        .tint(BlackwoodPalette.destructive)
                                    }
                                }
                            }
                        }
                    }
                }
                .frame(maxWidth: 680)
                .padding(.horizontal, 20)
                .padding(.vertical, 24)
            }
            .background(BlackwoodPalette.background.ignoresSafeArea())
            .navigationBarHidden(true)
            .task {
                uploads = await model.pendingUploads()
            }
            .refreshable {
                await model.refreshQueueSnapshot()
                uploads = await model.pendingUploads()
            }
        }
    }

    private func queueRow(_ title: String, _ value: String) -> some View {
        HStack {
            Text(title)
                .foregroundStyle(BlackwoodPalette.mutedForeground)
            Spacer()
            Text(value)
                .foregroundStyle(BlackwoodPalette.foreground)
        }
    }
}

struct SettingsScreen: View {
    @ObservedObject var model: AppModel

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    header(title: "Blackwood", subtitle: "Settings")

                    card {
                        VStack(alignment: .leading, spacing: 12) {
                            Text("BLACKWOOD SERVER")
                                .font(.system(size: 12, weight: .semibold))
                                .tracking(1)
                                .foregroundStyle(BlackwoodPalette.mutedForeground)

                            TextField("Server URL", text: $model.serverURLString)
                                .textInputAutocapitalization(.never)
                                .keyboardType(.URL)
                                .textContentType(.URL)
                                .autocorrectionDisabled()
                                .padding(12)
                                .background(BlackwoodPalette.muted.opacity(0.8))
                                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))

                            HStack(spacing: 12) {
                                Button("Save Endpoint") {
                                    Task { await model.updateServerURL() }
                                }
                                .buttonStyle(.borderedProminent)
                                .tint(BlackwoodPalette.accent)

                                Button("Test Connection") {
                                    Task { await model.testServerConnection() }
                                }
                                .buttonStyle(.bordered)
                                .tint(BlackwoodPalette.accent)
                            }

                            connectionStatusView
                        }
                    }
                }
                .frame(maxWidth: 680)
                .padding(.horizontal, 20)
                .padding(.vertical, 24)
            }
            .background(BlackwoodPalette.background.ignoresSafeArea())
            .navigationBarHidden(true)
        }
    }

    @ViewBuilder
    private var connectionStatusView: some View {
        switch model.connectionTestState {
        case .idle:
            Text("The server URL is stored locally on this device.")
                .font(.caption)
                .foregroundStyle(BlackwoodPalette.mutedForeground)
        case .testing:
            HStack(spacing: 8) {
                ProgressView()
                    .controlSize(.small)
                Text("Testing connection…")
                    .font(.caption)
                    .foregroundStyle(BlackwoodPalette.mutedForeground)
            }
        case .success(let version):
            Text("Connected successfully\(version.isEmpty ? "" : " • \(version)")")
                .font(.caption)
                .foregroundStyle(BlackwoodPalette.success)
        case .failed(let message):
            Text(message)
                .font(.caption)
                .foregroundStyle(BlackwoodPalette.destructive)
        }
    }
}

struct RecordingSheet: View {
    @ObservedObject var model: AppModel

    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                switch model.recorder.state {
                case .idle, .preparing:
                    VStack(spacing: 16) {
                        Image(systemName: "waveform.circle.fill")
                            .font(.system(size: 60))
                            .foregroundStyle(BlackwoodPalette.accent)
                        Text("Start a voice memo for \(AppModel.dayString(from: model.selectedDate))")
                            .multilineTextAlignment(.center)
                        Button("Start Recording") {
                            Task { await model.recorder.startRecording() }
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(BlackwoodPalette.accent)
                    }
                case .recording:
                    VStack(spacing: 16) {
                        Text(model.recorder.duration, format: .number.precision(.fractionLength(0)))
                            .font(.system(size: 48, weight: .semibold, design: .rounded))
                        Text("Recording…")
                            .foregroundStyle(BlackwoodPalette.mutedForeground)
                        Button("Stop Recording") {
                            model.recorder.stopRecording()
                            model.isRecordingSheetPresented = false
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(BlackwoodPalette.destructive)
                    }
                case .failed(let message):
                    VStack(spacing: 16) {
                        Text(message)
                            .foregroundStyle(BlackwoodPalette.destructive)
                            .multilineTextAlignment(.center)
                        Button("Dismiss") {
                            model.recorder.dismissError()
                            model.isRecordingSheetPresented = false
                        }
                        .buttonStyle(.bordered)
                    }
                }
            }
            .padding(24)
            .background(BlackwoodPalette.background.ignoresSafeArea())
            .navigationTitle("Record")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Close") {
                        model.isRecordingSheetPresented = false
                    }
                }
            }
            .task {
                await model.recorder.prepareIfNeeded()
            }
        }
    }
}

private struct StructuredNoteView: View {
    let content: String
    let baseURL: URL?
    let date: String

    private var sections: [(title: String, body: String)] {
        let trimmed = content.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            return [("Summary", "No note content yet.")]
        }

        let lines = trimmed.components(separatedBy: .newlines)
        var sections: [(String, [String])] = []
        var currentTitle = "Summary"
        var currentBody: [String] = []

        for line in lines {
            if line.hasPrefix("# ") {
                sections.append((currentTitle, currentBody))
                currentTitle = String(line.dropFirst(2)).trimmingCharacters(in: .whitespacesAndNewlines)
                currentBody = []
            } else {
                currentBody.append(line)
            }
        }
        sections.append((currentTitle, currentBody))

        return sections
            .map { ($0.0, $0.1.joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)) }
            .filter { !$0.0.isEmpty && !$0.1.isEmpty }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            ForEach(Array(sections.enumerated()), id: \.offset) { _, section in
                VStack(alignment: .leading, spacing: 10) {
                    HStack(spacing: 12) {
                        Text(section.title.uppercased())
                            .font(.system(size: 11, weight: .semibold))
                            .tracking(1)
                            .foregroundStyle(BlackwoodPalette.mutedForeground)
                        Rectangle()
                            .fill(BlackwoodPalette.border)
                            .frame(height: 1)
                    }

                    MarkdownBlockView(
                        markdown: section.body,
                        isSummary: section.title == "Summary",
                        baseURL: baseURL,
                        date: date
                    )
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

private struct MarkdownBlockView: View {
    let markdown: String
    let isSummary: Bool
    let baseURL: URL?
    let date: String

    private enum Block: Hashable {
        case heading(level: Int, text: String)
        case paragraph(String)
        case bulletList([String])
        case numberedList([String])
        case quote(String)
        case image(source: String, alt: String?)
        case rule
    }

    private var blocks: [Block] {
        var result: [Block] = []
        var paragraphLines: [String] = []
        var bullets: [String] = []
        var numbers: [String] = []

        func flushParagraph() {
            guard !paragraphLines.isEmpty else { return }
            result.append(.paragraph(paragraphLines.joined(separator: "\n")))
            paragraphLines.removeAll()
        }

        func flushBullets() {
            guard !bullets.isEmpty else { return }
            result.append(.bulletList(bullets))
            bullets.removeAll()
        }

        func flushNumbers() {
            guard !numbers.isEmpty else { return }
            result.append(.numberedList(numbers))
            numbers.removeAll()
        }

        for rawLine in markdown.components(separatedBy: .newlines) {
            let line = rawLine.trimmingCharacters(in: .whitespaces)

            if line.isEmpty {
                flushParagraph()
                flushBullets()
                flushNumbers()
                continue
            }

            if line == "---" {
                flushParagraph()
                flushBullets()
                flushNumbers()
                result.append(.rule)
                continue
            }

            if let image = imageBlock(from: line) {
                flushParagraph()
                flushBullets()
                flushNumbers()
                result.append(image)
                continue
            }

            if let heading = headingBlock(from: line) {
                flushParagraph()
                flushBullets()
                flushNumbers()
                result.append(heading)
                continue
            }

            if let bullet = bulletText(from: line) {
                flushParagraph()
                flushNumbers()
                bullets.append(bullet)
                continue
            }

            if let number = numberedText(from: line) {
                flushParagraph()
                flushBullets()
                numbers.append(number)
                continue
            }

            if line.hasPrefix(">") {
                flushParagraph()
                flushBullets()
                flushNumbers()
                result.append(.quote(String(line.drop { $0 == ">" || $0 == " " })))
                continue
            }

            flushBullets()
            flushNumbers()
            paragraphLines.append(line)
        }

        flushParagraph()
        flushBullets()
        flushNumbers()

        return result.isEmpty ? [.paragraph(markdown)] : result
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            ForEach(Array(blocks.enumerated()), id: \.offset) { _, block in
                switch block {
                case .heading(let level, let text):
                    markdownText(text, font: headingFont(level), color: BlackwoodPalette.foreground)
                        .padding(.top, level == 1 ? 4 : 2)

                case .paragraph(let text):
                    paragraphView(
                        text,
                        font: .system(size: 17),
                        color: isSummary ? BlackwoodPalette.mutedForeground : BlackwoodPalette.foreground,
                        italic: isSummary
                    )

                case .bulletList(let items):
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                            HStack(alignment: .top, spacing: 10) {
                                Text("•")
                                    .font(.system(size: 17, weight: .semibold))
                                    .foregroundStyle(BlackwoodPalette.foreground)
                                    .frame(width: 12, alignment: .leading)
                                paragraphView(item, font: .system(size: 17), color: BlackwoodPalette.foreground)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .padding(.leading, 4)

                case .numberedList(let items):
                    VStack(alignment: .leading, spacing: 8) {
                        ForEach(Array(items.enumerated()), id: \.offset) { index, item in
                            HStack(alignment: .top, spacing: 10) {
                                Text("\(index + 1).")
                                    .font(.system(size: 17, weight: .semibold))
                                    .foregroundStyle(BlackwoodPalette.foreground)
                                    .frame(width: 24, alignment: .leading)
                                paragraphView(item, font: .system(size: 17), color: BlackwoodPalette.foreground)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .padding(.leading, 4)

                case .quote(let text):
                    HStack(alignment: .top, spacing: 12) {
                        Rectangle()
                            .fill(BlackwoodPalette.accent)
                            .frame(width: 2)
                        paragraphView(text, font: .system(size: 16), color: BlackwoodPalette.mutedForeground)
                    }
                    .padding(.vertical, 6)

                case .image(let source, let alt):
                    NoteImageView(
                        imageURL: resolvedImageURL(for: source),
                        altText: alt
                    )
                    .padding(.vertical, 6)

                case .rule:
                    Rectangle()
                        .fill(BlackwoodPalette.border)
                        .frame(width: 40, height: 1)
                        .padding(.vertical, 4)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func markdownText(_ markdown: String, font: Font, color: Color) -> some View {
        Group {
            if let rendered = try? AttributedString(
                markdown: markdown,
                options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace)
            ) {
                Text(rendered)
            } else {
                Text(markdown)
            }
        }
        .font(font)
        .foregroundStyle(color)
        .fixedSize(horizontal: false, vertical: true)
    }

    @ViewBuilder
    private func paragraphView(_ text: String, font: Font, color: Color, italic: Bool = false) -> some View {
        let lines = text.components(separatedBy: .newlines)

        if lines.count <= 1 {
            markdownText(text, font: font, color: color)
                .italic(italic)
        } else {
            VStack(alignment: .leading, spacing: 6) {
                ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                    if line.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        Color.clear
                            .frame(height: 6)
                    } else {
                        markdownText(line, font: font, color: color)
                            .italic(italic)
                    }
                }
            }
        }
    }

    private func headingFont(_ level: Int) -> Font {
        switch level {
        case 1:
            return .system(size: 24, weight: .semibold)
        case 2:
            return .system(size: 21, weight: .semibold)
        default:
            return .system(size: 18, weight: .semibold)
        }
    }

    private func headingBlock(from line: String) -> Block? {
        let prefixes = ["### ", "## ", "# "]
        for prefix in prefixes {
            if line.hasPrefix(prefix) {
                return .heading(level: prefix.filter { $0 == "#" }.count, text: String(line.dropFirst(prefix.count)))
            }
        }
        return nil
    }

    private func imageBlock(from line: String) -> Block? {
        let markdownPattern = #"^!\[(.*?)\]\((.+?)\)$"#
        if let regex = try? NSRegularExpression(pattern: markdownPattern),
           let match = regex.firstMatch(in: line, range: NSRange(line.startIndex..., in: line)),
           let altRange = Range(match.range(at: 1), in: line),
           let sourceRange = Range(match.range(at: 2), in: line) {
            return .image(source: String(line[sourceRange]), alt: String(line[altRange]))
        }

        let htmlPattern = #"<img\b[^>]*src=["']([^"']+)["'][^>]*?(?:alt=["']([^"']*)["'])?[^>]*>"#
        if let regex = try? NSRegularExpression(pattern: htmlPattern, options: [.caseInsensitive]),
           let match = regex.firstMatch(in: line, range: NSRange(line.startIndex..., in: line)),
           let sourceRange = Range(match.range(at: 1), in: line) {
            let alt: String?
            if match.numberOfRanges > 2, let altRange = Range(match.range(at: 2), in: line) {
                alt = String(line[altRange])
            } else {
                alt = nil
            }
            return .image(source: String(line[sourceRange]), alt: alt)
        }

        return nil
    }

    private func bulletText(from line: String) -> String? {
        let prefixes = ["- ", "* ", "+ "]
        for prefix in prefixes where line.hasPrefix(prefix) {
            return String(line.dropFirst(prefix.count))
        }
        return nil
    }

    private func numberedText(from line: String) -> String? {
        guard let dotIndex = line.firstIndex(of: ".") else { return nil }
        let prefix = line[..<dotIndex]
        guard !prefix.isEmpty, prefix.allSatisfy(\.isNumber) else { return nil }
        let afterDot = line[line.index(after: dotIndex)...]
        guard afterDot.first == " " else { return nil }
        return String(afterDot.dropFirst())
    }

    private func resolvedImageURL(for source: String) -> URL? {
        if let absolute = URL(string: source), absolute.scheme != nil {
            return absolute
        }

        guard let baseURL else { return nil }

        if source.hasPrefix("/") {
            return URL(string: source, relativeTo: baseURL)?.absoluteURL
        }

        let encodedSource = source.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? source
        let path = "/api/daily-notes/\(date)/attachments/\(encodedSource)"
        return URL(string: path, relativeTo: baseURL)?.absoluteURL
    }
}

private struct NoteImageView: View {
    let imageURL: URL?
    let altText: String?

    var body: some View {
        Group {
            if let imageURL {
                AsyncImage(url: imageURL) { phase in
                    switch phase {
                    case .empty:
                        ZStack {
                            RoundedRectangle(cornerRadius: 14, style: .continuous)
                                .fill(BlackwoodPalette.muted.opacity(0.8))
                            ProgressView()
                                .tint(BlackwoodPalette.accent)
                        }
                        .frame(maxWidth: .infinity)
                        .frame(height: 220)

                    case .success(let image):
                        image
                            .resizable()
                            .aspectRatio(contentMode: .fit)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 14, style: .continuous)
                                    .stroke(BlackwoodPalette.border, lineWidth: 1)
                            )

                    case .failure:
                        fallback

                    @unknown default:
                        fallback
                    }
                }
            } else {
                fallback
            }
        }
    }

    private var fallback: some View {
        VStack(alignment: .leading, spacing: 8) {
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .fill(BlackwoodPalette.muted.opacity(0.8))
                .frame(height: 160)
                .overlay {
                    Image(systemName: "photo")
                        .font(.system(size: 28, weight: .medium))
                        .foregroundStyle(BlackwoodPalette.mutedForeground)
                }

            if let altText, !altText.isEmpty {
                Text(altText)
                    .font(.system(size: 14))
                    .foregroundStyle(BlackwoodPalette.mutedForeground)
            }
        }
    }
}

private struct DayCarousel: View {
    let selectedDate: Date
    let onSelectDate: (Date) -> Void

    @State private var displayedMonth: Date

    init(selectedDate: Date, onSelectDate: @escaping (Date) -> Void) {
        self.selectedDate = selectedDate
        self.onSelectDate = onSelectDate
        _displayedMonth = State(initialValue: Self.monthStart(for: selectedDate))
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                HStack(spacing: 10) {
                    monthButton("chevron.left") {
                        displayedMonth = Self.shiftMonth(displayedMonth, by: -1)
                    }
                    Text(monthTitle)
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundStyle(BlackwoodPalette.foreground)
                        .frame(minWidth: 144)
                    monthButton("chevron.right") {
                        displayedMonth = Self.shiftMonth(displayedMonth, by: 1)
                    }
                }
                Spacer()
                Button("Today") {
                    let today = Date()
                    displayedMonth = Self.monthStart(for: today)
                    onSelectDate(today)
                }
                .font(.system(size: 13, weight: .medium))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
            }

            ScrollViewReader { proxy in
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(alignment: .top, spacing: 8) {
                        ForEach(daysInMonth, id: \.self) { day in
                            let isSelected = Calendar.current.isDate(day, inSameDayAs: selectedDate)
                            let isToday = Calendar.current.isDateInToday(day)

                            Button {
                                onSelectDate(day)
                            } label: {
                                VStack(spacing: 4) {
                                    Text(Self.weekdayLetter(for: day))
                                        .font(.system(size: 10, weight: .semibold))
                                        .foregroundStyle(isSelected ? Color.white.opacity(0.9) : BlackwoodPalette.mutedForeground)
                                    Text(Self.dayNumber(for: day))
                                        .font(.system(size: 15, weight: .semibold))
                                        .foregroundStyle(isSelected ? .white : (isToday ? BlackwoodPalette.accent : BlackwoodPalette.foreground))
                                    Circle()
                                        .fill(isSelected ? Color.white.opacity(0.9) : (isToday ? BlackwoodPalette.accent : Color.clear))
                                        .frame(width: 4, height: 4)
                                }
                                .frame(width: 38, height: 52)
                                .background(isSelected ? BlackwoodPalette.accent : BlackwoodPalette.card)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                                        .stroke(isSelected ? BlackwoodPalette.accent : BlackwoodPalette.border, lineWidth: 1)
                                )
                                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                            }
                            .buttonStyle(.plain)
                            .id(Self.dayID(for: day))
                        }
                    }
                    .padding(2)
                }
                .onAppear {
                    proxy.scrollTo(Self.dayID(for: selectedDate), anchor: .center)
                }
                .onChange(of: selectedDate) { _, newDate in
                    displayedMonth = Self.monthStart(for: newDate)
                    withAnimation(.easeInOut(duration: 0.2)) {
                        proxy.scrollTo(Self.dayID(for: newDate), anchor: .center)
                    }
                }
                .onChange(of: displayedMonth) { _, newMonth in
                    withAnimation(.easeInOut(duration: 0.2)) {
                        proxy.scrollTo(Self.dayID(for: Self.scrollTarget(in: newMonth, selectedDate: selectedDate)), anchor: .center)
                    }
                }
            }
        }
    }

    private var monthTitle: String {
        displayedMonth.formatted(.dateTime.month(.wide).year())
    }

    private var daysInMonth: [Date] {
        let calendar = Calendar.current
        guard let range = calendar.range(of: .day, in: .month, for: displayedMonth) else { return [] }
        return range.compactMap { day in
            calendar.date(bySetting: .day, value: day, of: displayedMonth)
        }
    }

    private func monthButton(_ systemImage: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: systemImage)
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
                .frame(width: 28, height: 28)
                .background(BlackwoodPalette.muted.opacity(0.7))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        }
        .buttonStyle(.plain)
    }

    private static func monthStart(for date: Date) -> Date {
        let calendar = Calendar.current
        let components = calendar.dateComponents([.year, .month], from: date)
        return calendar.date(from: components) ?? date
    }

    private static func shiftMonth(_ date: Date, by offset: Int) -> Date {
        Calendar.current.date(byAdding: .month, value: offset, to: date) ?? date
    }

    private static func dayNumber(for date: Date) -> String {
        String(Calendar.current.component(.day, from: date))
    }

    private static func weekdayLetter(for date: Date) -> String {
        let index = Calendar.current.component(.weekday, from: date) - 1
        let letters = ["S", "M", "T", "W", "T", "F", "S"]
        return letters[max(0, min(index, letters.count - 1))]
    }

    private static func dayID(for date: Date) -> String {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.string(from: date)
    }

    private static func scrollTarget(in month: Date, selectedDate: Date) -> Date {
        let calendar = Calendar.current
        if calendar.isDate(selectedDate, equalTo: month, toGranularity: .month) {
            return selectedDate
        }
        return month
    }
}

private func header(title: String, subtitle: String) -> some View {
    VStack(alignment: .leading, spacing: 8) {
        Text(title)
            .font(.system(size: 30, weight: .semibold))
            .foregroundStyle(BlackwoodPalette.foreground)
        Text(subtitle.uppercased())
            .font(.system(size: 12, weight: .semibold))
            .tracking(1.2)
            .foregroundStyle(BlackwoodPalette.mutedForeground)
    }
}

private func card<Content: View>(@ViewBuilder content: () -> Content) -> some View {
    VStack(alignment: .leading, spacing: 0, content: content)
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(BlackwoodPalette.card)
        .overlay(
            RoundedRectangle(cornerRadius: 24, style: .continuous)
                .stroke(BlackwoodPalette.border, lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
}

private func actionButton(_ title: String, systemImage: String, filled: Bool, action: @escaping () -> Void) -> some View {
    Button(action: action) {
        Label(title, systemImage: systemImage)
            .font(.system(size: 15, weight: .semibold))
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
    }
    .foregroundStyle(filled ? Color.white : BlackwoodPalette.foreground)
    .background(filled ? BlackwoodPalette.accent : BlackwoodPalette.muted)
    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
}

private func errorBanner(_ message: String) -> some View {
    Text(message)
        .font(.system(size: 14, weight: .medium))
        .foregroundStyle(BlackwoodPalette.destructive)
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(BlackwoodPalette.card)
        .overlay(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .stroke(BlackwoodPalette.destructive.opacity(0.25), lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
}
