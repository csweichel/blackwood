import SwiftUI

private enum BlackwoodPalette {
    static let background = Color(red: 250/255, green: 248/255, blue: 243/255)
    static let foreground = Color(red: 28/255, green: 36/255, blue: 51/255)
    static let card = Color(red: 250/255, green: 248/255, blue: 243/255)
    static let muted = Color(red: 239/255, green: 233/255, blue: 220/255)
    static let mutedForeground = Color(red: 106/255, green: 116/255, blue: 137/255)
    static let accent = Color(red: 74/255, green: 111/255, blue: 165/255)
    static let accentSubtle = Color(red: 226/255, green: 234/255, blue: 244/255)
    static let border = Color(red: 214/255, green: 206/255, blue: 188/255)
    static let destructive = Color(red: 184/255, green: 69/255, blue: 58/255)
    static let success = Color(red: 74/255, green: 139/255, blue: 92/255)
    static let warning = Color(red: 196/255, green: 136/255, blue: 45/255)
}

struct RootTabView: View {
    @ObservedObject var model: AppModel
    @State private var isSidebarPresented = false

    var body: some View {
        ZStack(alignment: .leading) {
            VStack(spacing: 0) {
                MinimalChromeHeader(
                    title: navigationTitle,
                    trailingContent: {
                        if model.selectedTab == .today {
                            todayHeaderActions
                        }
                    },
                    onToggleSidebar: {
                        withAnimation(.easeInOut(duration: 0.22)) {
                            isSidebarPresented.toggle()
                        }
                    }
                )

                Group {
                    switch model.selectedTab {
                    case .today:
                        NotesScreen(model: model)
                    case .search:
                        SearchScreen(model: model)
                    case .queue:
                        QueueScreen(model: model)
                    }
                }
            }
            .disabled(isSidebarPresented)

            if isSidebarPresented {
                Color.black.opacity(0.18)
                    .ignoresSafeArea()
                    .onTapGesture {
                        withAnimation(.easeInOut(duration: 0.22)) {
                            isSidebarPresented = false
                        }
                    }
                    .transition(.opacity)

                SidebarDrawer(
                    model: model,
                    onDismiss: {
                        withAnimation(.easeInOut(duration: 0.22)) {
                            isSidebarPresented = false
                        }
                    },
                    onOpenSettings: {
                        model.presentSettings()
                        withAnimation(.easeInOut(duration: 0.22)) {
                            isSidebarPresented = false
                        }
                    }
                )
                .transition(.move(edge: .leading))
            }
        }
        .background(BlackwoodPalette.background.ignoresSafeArea())
        .sheet(item: $model.presentedSheet) { sheet in
            switch sheet {
            case .recording:
                RecordingSheet(model: model)
            case .settings:
                SettingsScreen(model: model)
            }
        }
        .contentShape(Rectangle())
        .gesture(sidebarRevealGesture)
    }

    private var navigationTitle: String {
        switch model.selectedTab {
        case .today:
            return "Notes"
        case .search:
            return "Search"
        case .queue:
            return "Queue"
        }
    }

    private var sidebarRevealGesture: some Gesture {
        DragGesture(minimumDistance: 14)
            .onEnded { value in
                if !isSidebarPresented, value.startLocation.x < 24, value.translation.width > 70 {
                    withAnimation(.easeInOut(duration: 0.22)) {
                        isSidebarPresented = true
                    }
                } else if isSidebarPresented, value.translation.width < -60 {
                    withAnimation(.easeInOut(duration: 0.22)) {
                        isSidebarPresented = false
                    }
                }
            }
    }

    @ViewBuilder
    private var todayHeaderActions: some View {
        HStack(spacing: 10) {
            actionIconButton(systemImage: "mic.fill", filled: true) {
                model.presentRecorder()
            }

            if model.isEditing {
                actionIconButton(systemImage: "xmark", filled: false) {
                    model.cancelEditing()
                }
                actionIconButton(systemImage: "checkmark", filled: true) {
                    Task { await model.saveCurrentNote() }
                }
            } else {
                actionIconButton(systemImage: "square.and.pencil", filled: false) {
                    model.beginEditing()
                }
            }
        }
    }
}

struct NotesScreen: View {
    @ObservedObject var model: AppModel

    var body: some View {
        AppContentScrollView {
            VStack(alignment: .leading, spacing: 14) {
                card(spacing: 14) {
                    DayCarousel(
                        selectedDate: model.selectedDate,
                        onSelectDate: { model.changeDate(to: $0) }
                    )
                }

                if let error = model.noteError, !error.isEmpty {
                    errorBanner(error)
                }

                card {
                    VStack(alignment: .leading, spacing: 18) {
                        CardHeader(
                            title: "Daily note",
                            detail: model.isEditing ? "Editing markdown" : "Ink and parchment view"
                        )

                        if model.isEditing {
                            TextEditor(text: $model.draftContent)
                                .font(.system(size: 17))
                                .foregroundStyle(BlackwoodPalette.foreground)
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
            }
        }
    }
}

struct SearchScreen: View {
    @ObservedObject var model: AppModel

    var body: some View {
        AppContentScrollView {
            VStack(alignment: .leading, spacing: 14) {
                SectionIntro(
                    eyebrow: "Search",
                    title: "Find moments and notes",
                    detail: "Semantic search across your archive."
                )

                card(spacing: 16) {
                    CardHeader(title: "Search", detail: "Jump straight back into a day note")
                    HStack(spacing: 10) {
                        Image(systemName: "magnifyingglass")
                            .foregroundStyle(BlackwoodPalette.mutedForeground)
                        TextField("Search your notes…", text: $model.searchQuery)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .submitLabel(.search)
                            .onSubmit {
                                Task { await model.runSearch() }
                            }
                    }

                    Button("Search") {
                        Task { await model.runSearch() }
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(BlackwoodPalette.accent)
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
                            .font(.system(size: 12, weight: .semibold))
                            .tracking(0.8)
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
        AppContentScrollView {
            VStack(alignment: .leading, spacing: 14) {
                SectionIntro(
                    eyebrow: "Queue",
                    title: "Sync and upload status",
                    detail: "Changes waiting to reach Blackwood."
                )

                card(spacing: 18) {
                    CardHeader(title: "Status", detail: queueStatusDetail)

                    LazyVGrid(columns: [GridItem(.flexible()), GridItem(.flexible())], spacing: 12) {
                        QueueMetricCard(title: "Connection", value: model.connectionStatusLabel)
                        QueueMetricCard(title: "Pending notes", value: "\(model.queueSnapshot.noteUpdateCount)")
                        QueueMetricCard(title: "Pending uploads", value: "\(model.queueSnapshot.uploadCount)")
                        QueueMetricCard(title: "Failed uploads", value: "\(model.queueSnapshot.failedUploadCount)")
                    }

                    Button("Sync Now") {
                        Task {
                            await model.syncNow(force: true)
                            uploads = await model.pendingUploads()
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(BlackwoodPalette.accent)
                }

                if uploads.isEmpty {
                    card {
                        Text("No queued recordings.")
                            .foregroundStyle(BlackwoodPalette.mutedForeground)
                    }
                } else {
                    ForEach(uploads) { upload in
                        card(spacing: 14) {
                            HStack(alignment: .top, spacing: 14) {
                                Image(systemName: "waveform")
                                    .font(.system(size: 16, weight: .semibold))
                                    .foregroundStyle(BlackwoodPalette.accent)
                                    .frame(width: 36, height: 36)
                                    .background(BlackwoodPalette.accentSubtle)
                                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))

                                VStack(alignment: .leading, spacing: 12) {
                                    Text("Voice memo")
                                        .font(.system(size: 18, weight: .semibold))
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
            }
        }
        .task {
            uploads = await model.pendingUploads()
        }
        .refreshable {
            await model.refreshQueueSnapshot()
            uploads = await model.pendingUploads()
        }
    }

    private var queueStatusDetail: String {
        if !model.isNetworkAvailable {
            return "No network connection"
        }
        switch model.serverReachability {
        case .reachable:
            return "Blackwood server reachable"
        case .checking, .unknown:
            return "Checking Blackwood server"
        case .unreachable:
            return "Queued changes stay local until the server returns"
        }
    }
}

private struct MinimalChromeHeader<TrailingContent: View>: View {
    let title: String
    @ViewBuilder let trailingContent: () -> TrailingContent
    let onToggleSidebar: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            AppIconButton(systemImage: "line.3.horizontal") {
                onToggleSidebar()
            }

            Text(title)
                .font(.system(size: 18, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.foreground)

            Spacer()

            trailingContent()
        }
        .padding(.horizontal, 20)
        .padding(.top, 8)
        .padding(.bottom, 8)
        .background(
            VStack(spacing: 0) {
                BlackwoodPalette.card
                DividerLine()
            }
            .ignoresSafeArea(edges: .top)
        )
    }
}

private struct SidebarDrawer: View {
    @ObservedObject var model: AppModel
    let onDismiss: () -> Void
    let onOpenSettings: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 20) {
            HStack {
                Spacer()
                AppIconButton(systemImage: "xmark") {
                    onDismiss()
                }
            }

            VStack(alignment: .leading, spacing: 8) {
                sidebarItem(.today, title: "Notes", icon: "doc.text")
                sidebarItem(.search, title: "Search", icon: "magnifyingglass")
                sidebarItem(.queue, title: "Queue", icon: "arrow.trianglehead.2.clockwise")
            }

            card(spacing: 10) {
                Text("STATUS")
                    .font(.system(size: 11, weight: .semibold))
                    .tracking(0.9)
                    .foregroundStyle(BlackwoodPalette.mutedForeground)

                HStack(spacing: 8) {
                    Circle()
                        .fill(model.connectionStatusTint)
                        .frame(width: 8, height: 8)
                    Text(model.connectionStatusLabel)
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundStyle(BlackwoodPalette.foreground)
                }

                Text(sidebarStatusDetail)
                    .font(.system(size: 13))
                    .foregroundStyle(BlackwoodPalette.mutedForeground)
            }

            Button {
                onOpenSettings()
            } label: {
                Label("Settings", systemImage: "gearshape")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(BlackwoodPalette.foreground)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(BlackwoodPalette.muted)
                    .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            }
            .buttonStyle(.plain)

            Spacer(minLength: 0)
        }
        .padding(.horizontal, 18)
        .padding(.top, 56)
        .padding(.bottom, 28)
        .frame(width: 286)
        .frame(maxHeight: .infinity, alignment: .topLeading)
        .background(BlackwoodPalette.card)
        .overlay(alignment: .trailing) {
            DividerLine()
                .frame(width: 1)
        }
        .ignoresSafeArea(edges: .bottom)
    }

    private func sidebarItem(_ tab: AppModel.Tab, title: String, icon: String) -> some View {
        Button {
            model.selectedTab = tab
            onDismiss()
        } label: {
            HStack(spacing: 10) {
                Image(systemName: icon)
                    .font(.system(size: 15, weight: .semibold))
                Text(title)
                    .font(.system(size: 15, weight: .semibold))
                Spacer()
            }
            .foregroundStyle(model.selectedTab == tab ? BlackwoodPalette.foreground : BlackwoodPalette.mutedForeground)
            .padding(.horizontal, 14)
            .padding(.vertical, 12)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(model.selectedTab == tab ? BlackwoodPalette.muted : Color.clear)
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        }
        .buttonStyle(.plain)
    }

    private var sidebarStatusDetail: String {
        if !model.isNetworkAvailable {
            return "No network connection."
        }
        switch model.serverReachability {
        case .reachable:
            return "Blackwood server reachable."
        case .checking, .unknown:
            return "Checking server reachability."
        case .unreachable(let message):
            return message
        }
    }
}

struct SettingsScreen: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: AppModel

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    SectionIntro(
                        eyebrow: "Settings",
                        title: "Connection and device setup",
                        detail: "Manage the Blackwood endpoint stored on this device."
                    )

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
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") {
                        dismiss()
                    }
                }
            }
        }
    }

    @ViewBuilder
    private var connectionStatusView: some View {
        switch model.connectionTestState {
        case .idle:
            VStack(alignment: .leading, spacing: 6) {
                Text("The server URL is stored locally on this device.")
                    .font(.caption)
                    .foregroundStyle(BlackwoodPalette.mutedForeground)
                Text(reachabilitySummary)
                    .font(.caption)
                    .foregroundStyle(reachabilityTint)
            }
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

    private var reachabilitySummary: String {
        if !model.isNetworkAvailable {
            return "No network connection."
        }
        switch model.serverReachability {
        case .unknown:
            return "Server reachability has not been checked yet."
        case .checking:
            return "Checking Blackwood server…"
        case .reachable(let version):
            return version.isEmpty ? "Blackwood is reachable." : "Blackwood is reachable • \(version)"
        case .unreachable(let message):
            return message
        }
    }

    private var reachabilityTint: Color {
        if !model.isNetworkAvailable {
            return BlackwoodPalette.warning
        }
        switch model.serverReachability {
        case .reachable:
            return BlackwoodPalette.success
        case .unknown, .checking:
            return BlackwoodPalette.mutedForeground
        case .unreachable:
            return BlackwoodPalette.destructive
        }
    }
}

struct RecordingSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: AppModel

    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                switch model.recorder.state {
                case .idle:
                    idleState
                case .preparing:
                    preparingState
                case .recording:
                    recordingState
                case .processing:
                    processingState
                case .completed(let duration):
                    completedState(duration: duration)
                case .failed(let message):
                    failedState(message: message)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .padding(24)
            .background(BlackwoodPalette.background.ignoresSafeArea())
            .navigationTitle("Record")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Close") {
                        if canDismissSheet {
                            model.recorder.reset()
                            dismiss()
                        }
                    }
                    .disabled(!canDismissSheet)
                }
            }
            .task {
                await model.recorder.prepareIfNeeded()
            }
        }
    }

    private var idleState: some View {
        VStack(spacing: 20) {
            recordingHero(symbol: "waveform.circle.fill", tint: BlackwoodPalette.accent)
            Text("Start a voice memo for \(AppModel.dayString(from: model.selectedDate))")
                .font(.system(size: 24, weight: .semibold))
                .multilineTextAlignment(.center)
            Text("Capture a thought quickly and let Blackwood queue it for your day.")
                .font(.system(size: 16))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
                .multilineTextAlignment(.center)
            Button("Start Recording") {
                Task { await model.recorder.startRecording() }
            }
            .buttonStyle(.borderedProminent)
            .tint(BlackwoodPalette.accent)
        }
    }

    private var preparingState: some View {
        VStack(spacing: 20) {
            recordingHero(symbol: "mic.fill", tint: BlackwoodPalette.accent)
            ProgressView()
                .controlSize(.large)
                .tint(BlackwoodPalette.accent)
            Text("Preparing microphone…")
                .font(.system(size: 18, weight: .semibold))
            Text("Getting your recorder ready.")
                .font(.system(size: 15))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
        }
    }

    private var recordingState: some View {
        VStack(spacing: 22) {
            Text("Recording")
                .font(.system(size: 13, weight: .semibold))
                .tracking(1.2)
                .foregroundStyle(BlackwoodPalette.destructive)

            Text(formattedDuration(model.recorder.duration))
                .font(.system(size: 52, weight: .semibold, design: .rounded))
                .foregroundStyle(BlackwoodPalette.foreground)

            RecordingLevelMeter(levels: model.recorder.levels)

            Text("Voice memo for \(AppModel.dayString(from: model.selectedDate))")
                .font(.system(size: 15))
                .foregroundStyle(BlackwoodPalette.mutedForeground)

            Button("Stop Recording") {
                model.recorder.stopRecording()
            }
            .buttonStyle(.borderedProminent)
            .tint(BlackwoodPalette.destructive)
        }
    }

    private var processingState: some View {
        VStack(spacing: 20) {
            recordingHero(symbol: "waveform.badge.magnifyingglass", tint: BlackwoodPalette.accent)
            ProgressView()
                .controlSize(.large)
                .tint(BlackwoodPalette.accent)
            Text("Finishing your voice memo…")
                .font(.system(size: 20, weight: .semibold))
            Text("Blackwood is saving the recording and adding it to the upload queue.")
                .font(.system(size: 15))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
                .multilineTextAlignment(.center)
        }
    }

    private func completedState(duration: TimeInterval) -> some View {
        VStack(spacing: 20) {
            recordingHero(symbol: "checkmark.circle.fill", tint: BlackwoodPalette.success)
            Text("Voice memo queued")
                .font(.system(size: 24, weight: .semibold))
            Text("\(formattedDuration(duration)) captured and ready to sync.")
                .font(.system(size: 16))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
            Button("Done") {
                model.recorder.reset()
                dismiss()
            }
            .buttonStyle(.borderedProminent)
            .tint(BlackwoodPalette.accent)
        }
    }

    private func failedState(message: String) -> some View {
        VStack(spacing: 18) {
            recordingHero(symbol: "exclamationmark.circle.fill", tint: BlackwoodPalette.destructive)
            Text(message)
                .foregroundStyle(BlackwoodPalette.destructive)
                .multilineTextAlignment(.center)
            Button("Dismiss") {
                model.recorder.dismissError()
                dismiss()
            }
            .buttonStyle(.bordered)
        }
    }

    private func recordingHero(symbol: String, tint: Color) -> some View {
        Image(systemName: symbol)
            .font(.system(size: 64))
            .foregroundStyle(tint)
            .frame(width: 108, height: 108)
            .background(tint.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 30, style: .continuous))
    }

    private func formattedDuration(_ duration: TimeInterval) -> String {
        let wholeSeconds = max(Int(duration.rounded(.down)), 0)
        let minutes = wholeSeconds / 60
        let seconds = wholeSeconds % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }

    private var canDismissSheet: Bool {
        switch model.recorder.state {
        case .recording, .processing:
            return false
        default:
            return true
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
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                HStack(spacing: 8) {
                    monthButton("chevron.left") {
                        displayedMonth = Self.shiftMonth(displayedMonth, by: -1)
                    }
                    Text(monthTitle)
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundStyle(BlackwoodPalette.foreground)
                        .frame(minWidth: 124)
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
                .font(.system(size: 12, weight: .medium))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
            }

            ScrollViewReader { proxy in
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(alignment: .top, spacing: 6) {
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
                                .frame(width: 36, height: 48)
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
                    .padding(.vertical, 1)
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
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
                .frame(width: 26, height: 26)
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

private struct AppContentScrollView<Content: View>: View {
    let content: Content

    init(@ViewBuilder content: () -> Content) {
        self.content = content()
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                content
            }
            .frame(maxWidth: 680)
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
    }
}

private struct SectionIntro: View {
    let eyebrow: String
    let title: String
    let detail: String

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(eyebrow.uppercased())
                .font(.system(size: 11, weight: .semibold))
                .tracking(1)
                .foregroundStyle(BlackwoodPalette.mutedForeground)
            Text(title)
                .font(.system(size: 24, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.foreground)
            Text(detail)
                .font(.system(size: 13))
                .foregroundStyle(BlackwoodPalette.mutedForeground)
        }
    }
}

private struct CardHeader: View {
    let title: String
    let detail: String

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title.uppercased())
                .font(.system(size: 11, weight: .semibold))
                .tracking(0.9)
                .foregroundStyle(BlackwoodPalette.mutedForeground)
            Text(detail)
                .font(.system(size: 14))
                .foregroundStyle(BlackwoodPalette.foreground)
        }
    }
}

private struct AppIconButton: View {
    let systemImage: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: systemImage)
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.foreground)
                .frame(width: 36, height: 36)
                .background(BlackwoodPalette.muted)
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        }
        .buttonStyle(.plain)
    }
}

private struct QueueMetricCard: View {
    let title: String
    let value: String

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title.uppercased())
                .font(.system(size: 11, weight: .semibold))
                .tracking(0.9)
                .foregroundStyle(BlackwoodPalette.mutedForeground)
            Text(value)
                .font(.system(size: 20, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.foreground)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(BlackwoodPalette.muted.opacity(0.55))
        .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
    }
}

private struct RecordingLevelMeter: View {
    let levels: [CGFloat]

    var body: some View {
        HStack(alignment: .center, spacing: 6) {
            ForEach(Array(levels.enumerated()), id: \.offset) { _, level in
                Capsule()
                    .fill(BlackwoodPalette.accent)
                    .frame(width: 8, height: max(18, level * 92))
                    .animation(.easeOut(duration: 0.18), value: level)
            }
        }
        .frame(maxWidth: .infinity, minHeight: 100, alignment: .center)
        .padding(.horizontal, 12)
        .padding(.vertical, 16)
        .background(BlackwoodPalette.muted.opacity(0.45))
        .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
    }
}

private struct DividerLine: View {
    var body: some View {
        Rectangle()
            .fill(BlackwoodPalette.border)
            .frame(height: 1)
    }
}

private func card<Content: View>(spacing: CGFloat = 0, @ViewBuilder content: () -> Content) -> some View {
    VStack(alignment: .leading, spacing: spacing, content: content)
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(BlackwoodPalette.card)
        .overlay(
            RoundedRectangle(cornerRadius: 20, style: .continuous)
                .stroke(BlackwoodPalette.border, lineWidth: 1)
        )
        .clipShape(RoundedRectangle(cornerRadius: 20, style: .continuous))
}

private func actionIconButton(systemImage: String, filled: Bool, action: @escaping () -> Void) -> some View {
    Button(action: action) {
        Image(systemName: systemImage)
            .font(.system(size: 15, weight: .semibold))
            .frame(width: 36, height: 36)
    }
    .buttonStyle(.plain)
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
