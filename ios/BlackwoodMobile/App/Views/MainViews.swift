import AVFoundation
import Combine
import SwiftUI
import UIKit

enum BlackwoodPalette {
    static let background = dynamicColor(
        light: UIColor(red: 250/255, green: 248/255, blue: 243/255, alpha: 1),
        dark: UIColor(red: 18/255, green: 22/255, blue: 30/255, alpha: 1)
    )
    static let foreground = dynamicColor(
        light: UIColor(red: 28/255, green: 36/255, blue: 51/255, alpha: 1),
        dark: UIColor(red: 235/255, green: 238/255, blue: 244/255, alpha: 1)
    )
    static let card = dynamicColor(
        light: UIColor(red: 250/255, green: 248/255, blue: 243/255, alpha: 1),
        dark: UIColor(red: 24/255, green: 29/255, blue: 39/255, alpha: 1)
    )
    static let muted = dynamicColor(
        light: UIColor(red: 239/255, green: 233/255, blue: 220/255, alpha: 1),
        dark: UIColor(red: 39/255, green: 46/255, blue: 60/255, alpha: 1)
    )
    static let mutedForeground = dynamicColor(
        light: UIColor(red: 106/255, green: 116/255, blue: 137/255, alpha: 1),
        dark: UIColor(red: 156/255, green: 167/255, blue: 187/255, alpha: 1)
    )
    static let accent = dynamicColor(
        light: UIColor(red: 74/255, green: 111/255, blue: 165/255, alpha: 1),
        dark: UIColor(red: 124/255, green: 166/255, blue: 224/255, alpha: 1)
    )
    static let accentSubtle = dynamicColor(
        light: UIColor(red: 226/255, green: 234/255, blue: 244/255, alpha: 1),
        dark: UIColor(red: 37/255, green: 52/255, blue: 74/255, alpha: 1)
    )
    static let border = dynamicColor(
        light: UIColor(red: 214/255, green: 206/255, blue: 188/255, alpha: 1),
        dark: UIColor(red: 59/255, green: 69/255, blue: 88/255, alpha: 1)
    )
    static let destructive = dynamicColor(
        light: UIColor(red: 184/255, green: 69/255, blue: 58/255, alpha: 1),
        dark: UIColor(red: 245/255, green: 118/255, blue: 105/255, alpha: 1)
    )
    static let success = dynamicColor(
        light: UIColor(red: 74/255, green: 139/255, blue: 92/255, alpha: 1),
        dark: UIColor(red: 110/255, green: 191/255, blue: 132/255, alpha: 1)
    )
    static let warning = dynamicColor(
        light: UIColor(red: 196/255, green: 136/255, blue: 45/255, alpha: 1),
        dark: UIColor(red: 240/255, green: 186/255, blue: 93/255, alpha: 1)
    )

    private static func dynamicColor(light: UIColor, dark: UIColor) -> Color {
        Color(
            UIColor { traits in
                traits.userInterfaceStyle == .dark ? dark : light
            }
        )
    }
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
                RecordingSheet(model: model, recorder: model.recorder)
            case .settings:
                SettingsScreen(model: model)
            }
        }
        .sheet(item: $model.activeSubpage) { route in
            SubpageScreen(model: model, route: route)
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

struct AuthGateView: View {
    @ObservedObject var model: AppModel
    @State private var loginCode = ""
    @State private var setupCode = ""
    @State private var isSigningIn = false
    @State private var isConfirmingSetup = false

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                SectionIntro(
                    eyebrow: "Blackwood",
                    title: model.authState == .needsSetup ? "Set up TOTP" : "Sign in with TOTP",
                    detail: model.authState == .needsSetup
                        ? "Scan the QR code in your authenticator app, then confirm the current six-digit code."
                        : "Enter the current six-digit code from your authenticator app to unlock your notes."
                )

                if let message = model.authStatusMessage, !message.isEmpty {
                    card {
                        Text(message)
                            .font(.system(size: 14))
                            .foregroundStyle(BlackwoodPalette.destructive)
                    }
                }

                serverCard

                if model.authState == .needsSetup {
                    setupCard
                } else {
                    loginCard
                }
            }
            .frame(maxWidth: 680)
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
        .background(BlackwoodPalette.background.ignoresSafeArea())
        .task {
            await model.refreshAuthStatus()
            if model.authState == .needsSetup, model.authSetupInfo == nil {
                await model.loadAuthSetupInfo()
            }
        }
        .onChange(of: model.authState) { _, newValue in
            guard newValue == .needsSetup, model.authSetupInfo == nil else { return }
            Task { await model.loadAuthSetupInfo() }
        }
    }

    private var serverCard: some View {
        card(spacing: 14) {
            CardHeader(title: "Blackwood Server", detail: "This is the server your phone talks to.")

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

    private var loginCard: some View {
        card(spacing: 14) {
            CardHeader(title: "Sign in", detail: "Unlock Blackwood with your current TOTP code.")

            TextField("123456", text: $loginCode)
                .textInputAutocapitalization(.never)
                .keyboardType(.numberPad)
                .textContentType(.oneTimeCode)
                .padding(12)
                .background(BlackwoodPalette.muted.opacity(0.8))
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))

            Button {
                let code = sanitizedCode(loginCode)
                Task { @MainActor in
                    isSigningIn = true
                    defer { isSigningIn = false }
                    _ = await model.login(code: code)
                }
            } label: {
                HStack(spacing: 8) {
                    if isSigningIn {
                        ProgressView()
                            .controlSize(.small)
                    }
                    Text("Sign In")
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(BlackwoodPalette.accent)
            .disabled(isSigningIn || sanitizedCode(loginCode).count < 6)
        }
    }

    private var setupCard: some View {
        card(spacing: 14) {
            CardHeader(title: "TOTP Setup", detail: "Use the QR code to add Blackwood to your authenticator app.")

            if let setupInfo = model.authSetupInfo {
                VStack(alignment: .leading, spacing: 12) {
                    if let qrImage = qrCodeImage(from: setupInfo.qrCode) {
                        Image(uiImage: qrImage)
                            .resizable()
                            .interpolation(.none)
                            .scaledToFit()
                            .frame(maxWidth: 220)
                            .padding(14)
                            .background(.white)
                            .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
                    } else {
                        RoundedRectangle(cornerRadius: 18, style: .continuous)
                            .fill(BlackwoodPalette.muted)
                            .frame(width: 220, height: 220)
                            .overlay {
                                ProgressView()
                            }
                    }

                    VStack(alignment: .leading, spacing: 8) {
                        Text("Secret")
                            .font(.system(size: 12, weight: .semibold))
                            .tracking(0.9)
                            .foregroundStyle(BlackwoodPalette.mutedForeground)
                        Text(setupInfo.secret)
                            .font(.system(size: 14, design: .monospaced))
                            .textSelection(.enabled)
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(BlackwoodPalette.muted.opacity(0.8))
                            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    }
                }
            } else {
                HStack(spacing: 8) {
                    ProgressView()
                    Text("Generating setup details…")
                        .foregroundStyle(BlackwoodPalette.mutedForeground)
                }
            }

            TextField("123456", text: $setupCode)
                .textInputAutocapitalization(.never)
                .keyboardType(.numberPad)
                .textContentType(.oneTimeCode)
                .padding(12)
                .background(BlackwoodPalette.muted.opacity(0.8))
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))

            Button {
                let code = sanitizedCode(setupCode)
                Task { @MainActor in
                    isConfirmingSetup = true
                    defer { isConfirmingSetup = false }
                    _ = await model.confirmAuthSetup(code: code)
                }
            } label: {
                HStack(spacing: 8) {
                    if isConfirmingSetup {
                        ProgressView()
                            .controlSize(.small)
                    }
                    Text("Confirm & Sign In")
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(BlackwoodPalette.accent)
            .disabled(isConfirmingSetup || sanitizedCode(setupCode).count < 6 || model.authSetupInfo == nil)
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

    private func sanitizedCode(_ raw: String) -> String {
        String(raw.filter(\.isNumber).prefix(6))
    }

    private func qrCodeImage(from base64String: String) -> UIImage? {
        guard let data = Data(base64Encoded: base64String) else { return nil }
        return UIImage(data: data)
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

                if model.isEditing {
                    MarkdownCellEditor(
                        markdown: $model.draftContent,
                        placeholder: "Start writing your day…",
                        onSave: { await model.autoSaveCurrentNote($0) }
                    )
                    .id(AppModel.dayString(from: model.selectedDate))
                } else if model.isLoadingNote && model.noteContent.isEmpty {
                    ProgressView("Loading note…")
                        .frame(maxWidth: .infinity, minHeight: 220, alignment: .leading)
                } else {
                    StructuredNoteView(
                        content: model.noteContent,
                        baseURL: model.normalizedServerURL,
                        date: AppModel.dayString(from: model.selectedDate),
                        onOpenSubpage: { model.openSubpage(named: $0) }
                    )
                }
            }
        }
    }
}

struct SubpageScreen: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var model: AppModel
    let route: AppModel.SubpageRoute

    var body: some View {
        NavigationStack {
            AppContentScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    card(spacing: 8) {
                        Text(route.date.uppercased())
                            .font(.system(size: 11, weight: .semibold))
                            .tracking(1)
                            .foregroundStyle(BlackwoodPalette.mutedForeground)

                        Text(route.name)
                            .font(.system(size: 24, weight: .semibold))
                            .foregroundStyle(BlackwoodPalette.foreground)
                    }

                    if let error = model.subpageError, !error.isEmpty {
                        errorBanner(error)
                    }

                    if model.isEditingSubpage {
                        MarkdownCellEditor(
                            markdown: $model.subpageDraftContent,
                            placeholder: "Start writing this subpage…",
                            onSave: { await model.autoSaveCurrentSubpage($0) }
                        )
                        .id(route.id)
                    } else if model.isLoadingSubpage && model.subpageContent.isEmpty {
                        ProgressView("Loading subpage…")
                            .frame(maxWidth: .infinity, minHeight: 220, alignment: .leading)
                    } else {
                        StructuredNoteView(
                            content: model.subpageContent,
                            baseURL: model.normalizedServerURL,
                            date: route.date,
                            onOpenSubpage: { model.openSubpage(named: $0, for: route.date) }
                        )
                    }
                }
            }
            .background(BlackwoodPalette.background.ignoresSafeArea())
            .navigationTitle(route.name)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Close") {
                        closeSubpage()
                    }
                }

                ToolbarItemGroup(placement: .topBarTrailing) {
                    if model.isEditingSubpage {
                        Button("Done") {
                            Task { await model.saveCurrentSubpage() }
                        }
                    } else {
                        Button("Edit") {
                            model.beginEditingSubpage()
                        }
                    }
                }
            }
            .onDisappear {
                if model.activeSubpage?.id == route.id {
                    model.closeSubpage()
                }
            }
            .interactiveDismissDisabled(model.isEditingSubpage)
        }
    }

    private func closeSubpage() {
        Task {
            if model.isEditingSubpage {
                guard await model.saveCurrentSubpage() else { return }
            }
            model.closeSubpage()
            dismiss()
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
                        QueueMetricCard(title: "Pending notes", value: "\(model.queueSnapshot.totalNoteUpdateCount)")
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
            onDismiss()
            Task { await model.selectTab(tab) }
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

                    card(spacing: 12) {
                        CardHeader(title: "Session", detail: "Sign out of this device.")

                        Button("Sign Out") {
                            Task { @MainActor in
                                await model.logout()
                                dismiss()
                            }
                        }
                        .buttonStyle(.bordered)
                        .tint(BlackwoodPalette.destructive)
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
    @ObservedObject var recorder: AudioRecorderController

    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                switch recorder.state {
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
                            recorder.reset()
                            dismiss()
                        }
                    }
                    .disabled(!canDismissSheet)
                }
            }
            .task {
                await recorder.prepareIfNeeded()
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
                Task { await recorder.startRecording() }
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

            Text(formattedDuration(recorder.duration))
                .font(.system(size: 52, weight: .semibold, design: .rounded))
                .foregroundStyle(BlackwoodPalette.foreground)

            RecordingLevelMeter(levels: recorder.levels)

            Text("Voice memo for \(AppModel.dayString(from: model.selectedDate))")
                .font(.system(size: 15))
                .foregroundStyle(BlackwoodPalette.mutedForeground)

            Button("Stop Recording") {
                recorder.stopRecording()
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
                recorder.reset()
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
                recorder.dismissError()
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
        switch recorder.state {
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
    let onOpenSubpage: ((String) -> Void)?

    private var document: NoteDocument {
        NoteDocument(markdown: content)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if document.blocks.isEmpty {
                Text("No note content yet.")
                    .font(.system(size: 17))
                    .foregroundStyle(BlackwoodPalette.mutedForeground)
                    .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                ForEach(Array(document.blocks.enumerated()), id: \.offset) { _, block in
                    NoteBlockView(block: block, baseURL: baseURL, date: date, depth: 0)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .environment(\.openURL, OpenURLAction { url in
            guard url.scheme == "blackwood-subpage" else {
                return .systemAction(url)
            }
            let encodedName = url.host?.removingPercentEncoding
                ?? url.pathComponents.dropFirst().first?.removingPercentEncoding
                ?? ""
            guard !encodedName.isEmpty else {
                return .discarded
            }
            onOpenSubpage?(encodedName)
            return .handled
        })
    }
}

private struct NoteDocument {
    let blocks: [NoteBlock]

    init(markdown: String) {
        let visibleMarkdown = MarkdownStorage.visibleMarkdown(from: markdown)
        self.blocks = Self.parseBlocks(from: visibleMarkdown.components(separatedBy: .newlines))
    }

    private static func parseBlocks(from lines: [String]) -> [NoteBlock] {
        var blocks: [NoteBlock] = []
        var paragraphLines: [String] = []
        var index = 0

        func flushParagraph() {
            let text = paragraphLines.joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !text.isEmpty else {
                paragraphLines.removeAll()
                return
            }

            if let audioSource = Self.standaloneAudio(from: text) {
                blocks.append(.audio(source: audioSource))
            } else if let youtubeURL = Self.youtubeURL(from: text) {
                blocks.append(.youtube(youtubeURL))
            } else if let image = Self.standaloneImage(from: text) {
                blocks.append(.image(source: image.source, alt: image.alt))
            } else {
                blocks.append(.paragraph(text))
            }
            paragraphLines.removeAll()
        }

        while index < lines.count {
            let rawLine = lines[index]
            let line = rawLine.trimmingCharacters(in: .whitespaces)

            if line.isEmpty {
                flushParagraph()
                index += 1
                continue
            }

            if line == "---" || line == "***" {
                flushParagraph()
                blocks.append(.rule)
                index += 1
                continue
            }

            if let fence = codeFence(from: line) {
                flushParagraph()
                var codeLines: [String] = []
                index += 1
                while index < lines.count {
                    let closing = lines[index].trimmingCharacters(in: .whitespaces)
                    if closing.hasPrefix(fence.marker) {
                        index += 1
                        break
                    }
                    codeLines.append(lines[index])
                    index += 1
                }
                blocks.append(.codeBlock(language: fence.language, code: codeLines.joined(separator: "\n")))
                continue
            }

            if let heading = headingBlock(from: line) {
                flushParagraph()
                blocks.append(heading)
                index += 1
                continue
            }

            if let image = standaloneImage(from: line) {
                flushParagraph()
                blocks.append(.image(source: image.source, alt: image.alt))
                index += 1
                continue
            }

            if let audioSource = standaloneAudio(from: line) {
                flushParagraph()
                blocks.append(.audio(source: audioSource))
                index += 1
                continue
            }

            if listMatch(for: rawLine) != nil {
                flushParagraph()
                let regionStart = index
                index += 1
                while index < lines.count {
                    let nextLine = lines[index]
                    let trimmed = nextLine.trimmingCharacters(in: .whitespaces)
                    if trimmed.isEmpty {
                        break
                    }
                    guard listMatch(for: nextLine) != nil else {
                        break
                    }
                    index += 1
                }

                let region = Array(lines[regionStart..<index])
                if let first = region.first, let match = listMatch(for: first) {
                    let parsed = parseListItems(from: region, startingAt: 0, parentIndent: match.indent)
                    blocks.append(match.isOrdered ? .numberedList(parsed.items) : .bulletList(parsed.items))
                }
                continue
            }

            if line.hasPrefix(">") {
                flushParagraph()
                var quoteLines: [String] = []
                while index < lines.count {
                    let quoteLine = lines[index].trimmingCharacters(in: .whitespaces)
                    guard quoteLine.hasPrefix(">") else { break }
                    quoteLines.append(String(quoteLine.drop { $0 == ">" || $0 == " " }))
                    index += 1
                }
                blocks.append(.quote(quoteLines.joined(separator: "\n")))
                continue
            }

            paragraphLines.append(rawLine)
            index += 1
        }

        flushParagraph()
        return blocks
    }

    private static func headingBlock(from line: String) -> NoteBlock? {
        let prefixes = ["### ", "## ", "# "]
        for prefix in prefixes where line.hasPrefix(prefix) {
            return .heading(level: prefix.filter { $0 == "#" }.count, text: String(line.dropFirst(prefix.count)))
        }
        return nil
    }

    private static func codeFence(from line: String) -> (marker: String, language: String?)? {
        guard line.hasPrefix("```") || line.hasPrefix("~~~") else { return nil }
        let marker = String(line.prefix(3))
        let language = String(line.dropFirst(3)).trimmingCharacters(in: .whitespacesAndNewlines)
        return (marker, language.isEmpty ? nil : language)
    }

    private static func standaloneImage(from line: String) -> (source: String, alt: String?)? {
        let markdownPattern = #"^!\[([^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)$"#
        if let match = firstMatch(pattern: markdownPattern, in: line),
           let alt = capture(1, in: line, match: match),
           let source = capture(2, in: line, match: match) {
            return (source, alt.isEmpty ? nil : alt)
        }

        let linkPattern = #"^\[([^\]]+)\]\(([^)\s]+)(?:\s+"[^"]*")?\)$"#
        if let match = firstMatch(pattern: linkPattern, in: line),
           let label = capture(1, in: line, match: match),
           let source = capture(2, in: line, match: match),
           isImageURL(source) {
            return (source, label.isEmpty ? nil : label)
        }

        let htmlPattern = #"<img\b[^>]*src=["']([^"']+)["'][^>]*?(?:alt=["']([^"']*)["'])?[^>]*>"#
        if let match = firstMatch(pattern: htmlPattern, in: line, options: [.caseInsensitive]),
           let source = capture(1, in: line, match: match) {
            return (source, capture(2, in: line, match: match))
        }

        return nil
    }

    private static func standaloneAudio(from text: String) -> String? {
        let pattern = #"^<audio\b[^>]*\bsrc=["']([^"']+)["'][^>]*>(?:\s*</audio>)?$"#
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let match = firstMatch(pattern: pattern, in: trimmed, options: [.caseInsensitive]) else {
            return nil
        }
        return capture(1, in: trimmed, match: match)
    }

    private static func youtubeURL(from text: String) -> String? {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let candidate: String
        let linkPattern = #"^\[([^\]]+)\]\(([^)\s]+)(?:\s+"[^"]*")?\)$"#
        if let match = firstMatch(pattern: linkPattern, in: trimmed),
           let href = capture(2, in: trimmed, match: match) {
            candidate = href
        } else {
            candidate = trimmed
        }

        let patterns = [
            #"^(?:https?://)?(?:www\.)?youtube\.com/watch\?v=([\w-]+)(?:&[^\s)]*)?$"#,
            #"^(?:https?://)?youtu\.be/([\w-]+)(?:\?[^\s)]*)?$"#,
            #"^(?:https?://)?(?:www\.)?youtube-nocookie\.com/embed/([\w-]+)(?:\?[^\s)]*)?$"#,
        ]
        guard patterns.contains(where: { candidate.range(of: $0, options: .regularExpression) != nil }) else {
            return nil
        }
        return candidate.hasPrefix("http") ? candidate : "https://\(candidate)"
    }

    private static func listMatch(for rawLine: String) -> NoteListMatch? {
        var leadingWhitespace = 0
        for character in rawLine {
            if character == " " {
                leadingWhitespace += 1
            } else if character == "\t" {
                leadingWhitespace += 4
            } else {
                break
            }
        }

        let trimmed = rawLine.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else { return nil }

        for prefix in ["- ", "* ", "+ "] where trimmed.hasPrefix(prefix) {
            return NoteListMatch(indent: leadingWhitespace, text: String(trimmed.dropFirst(prefix.count)), isOrdered: false)
        }

        guard let dotIndex = trimmed.firstIndex(of: ".") else { return nil }
        let prefix = trimmed[..<dotIndex]
        guard !prefix.isEmpty, prefix.allSatisfy(\.isNumber) else { return nil }
        let afterDot = trimmed[trimmed.index(after: dotIndex)...]
        guard afterDot.first == " " else { return nil }
        return NoteListMatch(indent: leadingWhitespace, text: String(afterDot.dropFirst()), isOrdered: true)
    }

    private static func parseListItems(from lines: [String], startingAt startIndex: Int, parentIndent: Int) -> NoteParsedList {
        var items: [NoteListItem] = []
        var index = startIndex

        while index < lines.count {
            guard let match = listMatch(for: lines[index]), match.indent >= parentIndent else {
                break
            }
            if match.indent > parentIndent {
                break
            }

            var nextIndex = index + 1
            var children: [NoteBlock] = []
            if nextIndex < lines.count, let nextMatch = listMatch(for: lines[nextIndex]), nextMatch.indent > match.indent {
                let parsedChildren = parseListItems(from: lines, startingAt: nextIndex, parentIndent: nextMatch.indent)
                if !parsedChildren.items.isEmpty {
                    children = [nextMatch.isOrdered ? .numberedList(parsedChildren.items) : .bulletList(parsedChildren.items)]
                }
                nextIndex = parsedChildren.nextIndex
            }

            let task = taskState(from: match.text)
            items.append(NoteListItem(text: task.text, taskState: task.state, children: children))
            index = nextIndex
        }

        return NoteParsedList(items: items, nextIndex: index)
    }

    private static func taskState(from text: String) -> (text: String, state: Bool?) {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        if trimmed.hasPrefix("[ ] ") {
            return (String(trimmed.dropFirst(4)), false)
        }
        if trimmed.hasPrefix("[x] ") || trimmed.hasPrefix("[X] ") {
            return (String(trimmed.dropFirst(4)), true)
        }
        return (text, nil)
    }

    private static func isImageURL(_ value: String) -> Bool {
        let withoutQuery = value.split(whereSeparator: { $0 == "?" || $0 == "#" }).first.map(String.init) ?? value
        return [".apng", ".avif", ".bmp", ".gif", ".heic", ".heif", ".jpg", ".jpeg", ".png", ".svg", ".tif", ".tiff", ".webp"]
            .contains { withoutQuery.lowercased().hasSuffix($0) }
    }

    private static func firstMatch(
        pattern: String,
        in text: String,
        options: NSRegularExpression.Options = []
    ) -> NSTextCheckingResult? {
        guard let regex = try? NSRegularExpression(pattern: pattern, options: options) else { return nil }
        return regex.firstMatch(in: text, range: NSRange(text.startIndex..., in: text))
    }

    private static func capture(_ index: Int, in text: String, match: NSTextCheckingResult) -> String? {
        guard index < match.numberOfRanges, let range = Range(match.range(at: index), in: text) else {
            return nil
        }
        return String(text[range])
    }
}

private enum NoteBlock: Hashable {
    case heading(level: Int, text: String)
    case paragraph(String)
    case bulletList([NoteListItem])
    case numberedList([NoteListItem])
    case quote(String)
    case image(source: String, alt: String?)
    case audio(source: String)
    case youtube(String)
    case codeBlock(language: String?, code: String)
    case rule
}

private struct NoteListItem: Hashable {
    let text: String
    let taskState: Bool?
    let children: [NoteBlock]
}

private struct NoteListMatch {
    let indent: Int
    let text: String
    let isOrdered: Bool
}

private struct NoteParsedList {
    let items: [NoteListItem]
    let nextIndex: Int
}

private struct NoteBlockView: View {
    let block: NoteBlock
    let baseURL: URL?
    let date: String
    let depth: Int

    var body: some View {
        switch block {
        case .heading(let level, let text):
            markdownText(text, font: headingFont(level), color: BlackwoodPalette.foreground)
                .padding(.top, headingTopPadding(level))

        case .paragraph(let text):
            paragraphView(text, font: .system(size: 15), color: BlackwoodPalette.foreground)

        case .bulletList(let items):
            VStack(alignment: .leading, spacing: 4) {
                ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                    NoteListItemView(item: item, baseURL: baseURL, date: date, depth: depth, orderedIndex: nil)
                }
            }
            .padding(.leading, depth == 0 ? 2 : 0)

        case .numberedList(let items):
            VStack(alignment: .leading, spacing: 4) {
                ForEach(Array(items.enumerated()), id: \.offset) { index, item in
                    NoteListItemView(item: item, baseURL: baseURL, date: date, depth: depth, orderedIndex: index + 1)
                }
            }
            .padding(.leading, depth == 0 ? 2 : 0)

        case .quote(let text):
            HStack(alignment: .top, spacing: 12) {
                Rectangle()
                    .fill(BlackwoodPalette.accent)
                    .frame(width: 2)
                paragraphView(text, font: .system(size: 15), color: BlackwoodPalette.mutedForeground)
            }
            .padding(.vertical, 4)

        case .image(let source, let alt):
            NoteImageView(
                imageURL: resolvedURL(for: source),
                altText: alt
            )
            .padding(.vertical, 6)

        case .audio(let source):
            NoteAudioView(audioURL: resolvedURL(for: source))
                .padding(.vertical, 4)

        case .youtube(let url):
            Link(destination: URL(string: url) ?? URL(string: "https://youtube.com")!) {
                HStack(spacing: 10) {
                    Image(systemName: "play.rectangle.fill")
                        .font(.system(size: 22, weight: .semibold))
                    Text(youtubeLabel(for: url))
                        .font(.system(size: 15, weight: .medium))
                        .lineLimit(2)
                    Spacer(minLength: 0)
                    Image(systemName: "arrow.up.right")
                        .font(.system(size: 12, weight: .semibold))
                }
                .foregroundStyle(BlackwoodPalette.accent)
                .padding(12)
                .background(BlackwoodPalette.accentSubtle.opacity(0.55))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }

        case .codeBlock(let language, let code):
            VStack(alignment: .leading, spacing: 6) {
                if let language, !language.isEmpty {
                    Text(language.uppercased())
                        .font(.system(size: 10, weight: .semibold, design: .monospaced))
                        .foregroundStyle(BlackwoodPalette.mutedForeground)
                }
                ScrollView(.horizontal, showsIndicators: false) {
                    Text(code.isEmpty ? " " : code)
                        .font(.system(size: 13, design: .monospaced))
                        .foregroundStyle(BlackwoodPalette.foreground)
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .background(BlackwoodPalette.muted.opacity(0.45))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }

        case .rule:
            Rectangle()
                .fill(BlackwoodPalette.border)
                .frame(height: 1)
                .padding(.vertical, 8)
        }
    }

    private func headingFont(_ level: Int) -> Font {
        switch level {
        case 1:
            return .system(size: 24, weight: .bold)
        case 2:
            return .system(size: 20, weight: .semibold)
        default:
            return .system(size: 17, weight: .semibold)
        }
    }

    private func headingTopPadding(_ level: Int) -> CGFloat {
        switch level {
        case 1:
            return depth == 0 ? 10 : 4
        case 2:
            return 8
        default:
            return 5
        }
    }

    private func markdownText(_ markdown: String, font: Font, color: Color) -> some View {
        let renderedMarkdown = replacingWikilinks(in: markdown)
        return Group {
            if let rendered = try? AttributedString(
                markdown: renderedMarkdown,
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
    private func paragraphView(_ text: String, font: Font, color: Color) -> some View {
        let lines = text.components(separatedBy: .newlines)

        if lines.count <= 1 {
            markdownText(text, font: font, color: color)
                .lineSpacing(5)
        } else {
            VStack(alignment: .leading, spacing: 6) {
                ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                    if line.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        Color.clear
                            .frame(height: 6)
                    } else {
                        markdownText(line, font: font, color: color)
                            .lineSpacing(5)
                    }
                }
            }
        }
    }

    private func resolvedURL(for source: String) -> URL? {
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

    private func replacingWikilinks(in text: String) -> String {
        let pattern = #"\[\[([^\]]+)\]\]"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return text }
        let range = NSRange(text.startIndex..., in: text)
        let mutable = NSMutableString(string: text)

        let matches = regex.matches(in: text, range: range).reversed()
        for match in matches {
            guard let nameRange = Range(match.range(at: 1), in: text) else { continue }
            let name = String(text[nameRange])
            let encoded = name.addingPercentEncoding(withAllowedCharacters: .urlHostAllowed) ?? name
            mutable.replaceCharacters(in: match.range, with: "[\(name)](blackwood-subpage://\(encoded))")
        }

        return mutable as String
    }

    private func youtubeLabel(for url: String) -> String {
        guard let components = URLComponents(string: url) else { return "YouTube" }
        if let host = components.host, host.contains("youtu.be") {
            return "YouTube: \(components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/")))"
        }
        if let videoID = components.queryItems?.first(where: { $0.name == "v" })?.value {
            return "YouTube: \(videoID)"
        }
        return "YouTube"
    }
}

private struct NoteAudioView: View {
    let audioURL: URL?

    @State private var player: AVPlayer?
    @State private var isPlaying = false

    var body: some View {
        HStack(spacing: 12) {
            Button(action: togglePlayback) {
                Image(systemName: isPlaying ? "pause.fill" : "play.fill")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(Color.white)
                    .frame(width: 38, height: 38)
                    .background(BlackwoodPalette.accent)
                    .clipShape(Circle())
            }
            .buttonStyle(.plain)
            .disabled(audioURL == nil)
            .accessibilityLabel(isPlaying ? "Pause voice recording" : "Play voice recording")

            VStack(alignment: .leading, spacing: 2) {
                Text("Voice recording")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(BlackwoodPalette.foreground)
                Text(audioURL?.lastPathComponent.removingPercentEncoding ?? "Audio attachment")
                    .font(.system(size: 12))
                    .foregroundStyle(BlackwoodPalette.mutedForeground)
                    .lineLimit(1)
            }

            Spacer(minLength: 0)

            Image(systemName: "waveform")
                .font(.system(size: 17, weight: .medium))
                .foregroundStyle(BlackwoodPalette.accent)
                .accessibilityHidden(true)
        }
        .padding(11)
        .background(BlackwoodPalette.accentSubtle.opacity(0.48))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .stroke(BlackwoodPalette.border.opacity(0.7), lineWidth: 1)
        }
        .onReceive(NotificationCenter.default.publisher(for: .AVPlayerItemDidPlayToEndTime)) { notification in
            guard let finishedItem = notification.object as? AVPlayerItem,
                  finishedItem === player?.currentItem else { return }
            player?.seek(to: .zero)
            isPlaying = false
        }
        .onDisappear {
            player?.pause()
            isPlaying = false
        }
    }

    private func togglePlayback() {
        guard let audioURL else { return }
        let activePlayer: AVPlayer
        if let player {
            activePlayer = player
        } else {
            let newPlayer = AVPlayer(url: audioURL)
            player = newPlayer
            activePlayer = newPlayer
        }

        if isPlaying {
            activePlayer.pause()
        } else {
            activePlayer.play()
        }
        isPlaying.toggle()
    }
}

private struct NoteListItemView: View {
    let item: NoteListItem
    let baseURL: URL?
    let date: String
    let depth: Int
    let orderedIndex: Int?

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(alignment: .top, spacing: 8) {
                marker
                    .frame(width: orderedIndex == nil ? 18 : 28, alignment: .leading)
                NoteBlockView(block: .paragraph(item.text), baseURL: baseURL, date: date, depth: depth)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .frame(maxWidth: .infinity, alignment: .leading)

            ForEach(Array(item.children.enumerated()), id: \.offset) { _, child in
                NoteBlockView(block: child, baseURL: baseURL, date: date, depth: depth + 1)
                    .padding(.leading, 24)
            }
        }
    }

    @ViewBuilder
    private var marker: some View {
        if let taskState = item.taskState {
            Image(systemName: taskState ? "checkmark.square.fill" : "square")
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(taskState ? BlackwoodPalette.success : BlackwoodPalette.mutedForeground)
        } else {
            Text(listMarker)
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(BlackwoodPalette.foreground)
        }
    }

    private var listMarker: String {
        if let orderedIndex {
            if depth == 1 {
                return "\(alphaMarker(for: orderedIndex))."
            }
            return "\(orderedIndex)."
        }

        switch depth {
        case 0:
            return "•"
        case 1:
            return "◦"
        default:
            return "▪"
        }
    }

    private func alphaMarker(for index: Int) -> String {
        let letters = Array("abcdefghijklmnopqrstuvwxyz")
        let clamped = max(1, min(index, letters.count))
        return String(letters[clamped - 1])
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
        .scrollDismissesKeyboard(.interactively)
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
