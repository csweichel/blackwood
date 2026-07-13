import Foundation
import Network
import SwiftUI

@MainActor
final class AppModel: ObservableObject {
    enum AuthState: Equatable {
        case authenticated
        case needsLogin
        case needsSetup
    }

    enum ServerReachability: Equatable {
        case unknown
        case checking
        case reachable(version: String)
        case unreachable(message: String)
    }

    enum PresentedSheet: String, Identifiable {
        case recording
        case settings

        var id: String { rawValue }
    }

    struct SubpageRoute: Equatable, Identifiable {
        let date: String
        let name: String

        var id: String { "\(date)|\(name)" }
    }

    enum ConnectionTestState: Equatable {
        case idle
        case testing
        case success(version: String)
        case failed(message: String)
    }

    enum Tab: Hashable {
        case today
        case search
        case queue
    }

    private static let lastAuthenticatedKey = "blackwood.auth.lastAuthenticated"
    private static let lastSetupRequiredKey = "blackwood.auth.lastSetupRequired"
    private static let shortcutStore = ShortcutStore.defaults
    private static let defaultDailyNoteTemplate = "# Summary\n\n# Notes\n\n# Links\n"

    @Published var selectedTab: Tab = .today
    @Published var selectedDate = Date()
    @Published var noteContent = ""
    // Draft bindings intentionally do not publish every keystroke. The editor owns
    // its fine-grained rendering state and saves the latest value explicitly.
    var draftContent = ""
    @Published var noteRevision = ""
    @Published var activeSubpage: SubpageRoute?
    @Published var subpageContent = ""
    var subpageDraftContent = ""
    @Published var subpageRevision = ""
    @Published var isEditingSubpage = false
    @Published var isLoadingSubpage = false
    @Published var subpageError: String?
    @Published var isEditing = false
    @Published var isLoadingNote = false
    @Published var noteError: String?
    @Published var searchQuery = ""
    @Published var searchResults: [SearchResult] = []
    @Published var searchError: String?
    @Published var isSearching = false
    @Published var authState: AuthState
    @Published var authStatusMessage: String?
    @Published var authSetupInfo: AuthSetupInfo?
    @Published var isNetworkAvailable = true
    @Published var serverReachability: ServerReachability = .unknown
    @Published var queueSnapshot = QueueSnapshot(
        noteUpdateCount: 0,
        subpageUpdateCount: 0,
        uploadCount: 0,
        failedUploadCount: 0
    )
    @Published var serverURLString = UserDefaults.standard.string(forKey: "blackwood.serverURL") ?? "http://127.0.0.1:8080"
    @Published var connectionTestState: ConnectionTestState = .idle
    @Published var presentedSheet: PresentedSheet?

    let recorder = AudioRecorderController()
    let store = QueueStore()

    private let monitor = NWPathMonitor()
    private let monitorQueue = DispatchQueue(label: "BlackwoodConnectivity")
    private var didStart = false
    private var syncRetryAttempt = 0
    private var nextAutomaticSyncAllowedAt = Date.distantPast
    private var changeStreamTask: Task<Void, Never>?
    private var isSyncing = false
    private var noteSaveGeneration = 0
    private var subpageSaveGeneration = 0

    init() {
        authState = UserDefaults.standard.bool(forKey: Self.lastAuthenticatedKey)
            ? .authenticated
            : (UserDefaults.standard.bool(forKey: Self.lastSetupRequiredKey) ? .needsSetup : .needsLogin)
    }

    var isOnline: Bool {
        isNetworkAvailable && isServerReachable
    }

    var isAuthenticated: Bool {
        authState == .authenticated
    }

    var connectionStatusLabel: String {
        if !isNetworkAvailable {
            return "Offline"
        }
        switch serverReachability {
        case .unknown:
            return "Checking"
        case .checking:
            return "Checking"
        case .reachable:
            return "Online"
        case .unreachable:
            return "Server down"
        }
    }

    var connectionStatusTint: Color {
        if !isNetworkAvailable {
            return Color(red: 196/255, green: 136/255, blue: 45/255)
        }
        switch serverReachability {
        case .reachable:
            return Color(red: 74/255, green: 139/255, blue: 92/255)
        case .unknown, .checking:
            return Color(red: 74/255, green: 111/255, blue: 165/255)
        case .unreachable:
            return Color(red: 184/255, green: 69/255, blue: 58/255)
        }
    }

    func start() async {
        guard !didStart else { return }
        didStart = true
        recorder.onFinishedRecording = { [weak self] url, duration in
            Task { await self?.handleFinishedRecording(fileURL: url, duration: duration) }
        }
        monitor.pathUpdateHandler = { [weak self] path in
            Task { @MainActor in
                self?.isNetworkAvailable = path.status == .satisfied
                if path.status == .satisfied {
                    await self?.refreshAuthStatus()
                    await self?.refreshServerReachability()
                    if self?.isAuthenticated == true {
                        await self?.syncNow()
                    }
                } else {
                    self?.serverReachability = .unknown
                }
            }
        }
        monitor.start(queue: monitorQueue)
        try? await store.resetInterruptedUploads()
        await refreshAuthStatus()
        if isAuthenticated {
            await enterAuthenticatedWorkspace()
        } else {
            await handlePendingLaunchActionIfNeeded()
        }
    }

    func handleAppBecameActive() async {
        await refreshAuthStatus()
        await handlePendingLaunchActionIfNeeded()
        if isAuthenticated {
            await refreshQueueSnapshot()
            await refreshServerReachability()
            await syncNow()
        }
    }

    func flushEditingDrafts() async {
        if isEditing {
            _ = await autoSaveCurrentNote(draftContent)
        }
        if isEditingSubpage {
            _ = await autoSaveCurrentSubpage(subpageDraftContent)
        }
    }

    func loadSelectedDate() async {
        guard isAuthenticated else { return }
        let date = Self.dayString(from: selectedDate)
        noteError = nil
        let hasCachedNote = await loadCachedNote(for: date)
        await refreshSelectedDateFromServer(date: date, showLoadingState: !hasCachedNote)
    }

    func changeDate(to date: Date) {
        if isEditing {
            Task {
                guard await saveCurrentNote() else { return }
                selectedDate = date
                await loadSelectedDate()
            }
            return
        }
        selectedDate = date
        Task { await loadSelectedDate() }
    }

    @discardableResult
    func selectTab(_ tab: Tab) async -> Bool {
        guard selectedTab != tab else { return true }
        if selectedTab == .today, isEditing {
            guard await saveCurrentNote() else { return false }
        }
        selectedTab = tab
        return true
    }

    func beginEditing() {
        let editableContent = MarkdownStorage.visibleMarkdown(from: noteContent)
        draftContent = editableContent.isEmpty ? Self.defaultDailyNoteTemplate : editableContent
        isEditing = true
    }

    func cancelEditing() {
        draftContent = MarkdownStorage.visibleMarkdown(from: noteContent)
        isEditing = false
    }

    func openSubpage(named name: String, for date: String? = nil) {
        let route = SubpageRoute(date: date ?? Self.dayString(from: selectedDate), name: name)
        activeSubpage = route
        subpageError = nil
        isEditingSubpage = false
        Task { await loadSubpage(route) }
    }

    func beginEditingSubpage() {
        subpageDraftContent = MarkdownStorage.visibleMarkdown(from: subpageContent)
        isEditingSubpage = true
    }

    func cancelEditingSubpage() {
        subpageDraftContent = MarkdownStorage.visibleMarkdown(from: subpageContent)
        isEditingSubpage = false
    }

    func closeSubpage() {
        activeSubpage = nil
        subpageContent = ""
        subpageDraftContent = ""
        subpageRevision = ""
        subpageError = nil
        isEditingSubpage = false
        isLoadingSubpage = false
    }

    @discardableResult
    func saveCurrentNote() async -> Bool {
        await persistCurrentNote(draftContent, endingEditing: true)
    }

    @discardableResult
    func autoSaveCurrentNote(_ content: String) async -> Bool {
        await persistCurrentNote(content, endingEditing: false)
    }

    @discardableResult
    private func persistCurrentNote(_ content: String, endingEditing: Bool) async -> Bool {
        guard isAuthenticated else { return false }
        noteSaveGeneration += 1
        let saveGeneration = noteSaveGeneration

        if MarkdownStorage.visibleMarkdown(from: noteContent) == content {
            if endingEditing {
                isEditing = false
            }
            return true
        }

        let date = Self.dayString(from: selectedDate)
        let baseContent = noteContent

        do {
            try await store.savePendingDailyNote(
                date: date,
                content: content,
                revision: noteRevision,
                baseContent: baseContent,
                saveSequence: saveGeneration
            )
            guard saveGeneration == noteSaveGeneration else { return true }
            noteContent = content
            if endingEditing {
                isEditing = false
            }
            await refreshQueueSnapshot()
            Task { [weak self] in
                await self?.syncNow()
            }
            return true
        } catch {
            noteError = error.localizedDescription
            return false
        }
    }

    @discardableResult
    func saveCurrentSubpage() async -> Bool {
        await persistCurrentSubpage(subpageDraftContent, endingEditing: true)
    }

    @discardableResult
    func autoSaveCurrentSubpage(_ content: String) async -> Bool {
        await persistCurrentSubpage(content, endingEditing: false)
    }

    @discardableResult
    private func persistCurrentSubpage(_ content: String, endingEditing: Bool) async -> Bool {
        guard isAuthenticated, let route = activeSubpage else { return false }
        subpageSaveGeneration += 1
        let saveGeneration = subpageSaveGeneration

        if MarkdownStorage.visibleMarkdown(from: subpageContent) == content {
            if endingEditing {
                isEditingSubpage = false
            }
            return true
        }

        let baseContent = subpageContent
        subpageError = nil

        do {
            try await store.savePendingSubpage(
                date: route.date,
                name: route.name,
                content: content,
                revision: subpageRevision,
                baseContent: baseContent,
                saveSequence: saveGeneration
            )
            guard saveGeneration == subpageSaveGeneration else { return true }
            subpageContent = content
            if endingEditing {
                isEditingSubpage = false
            }
            await refreshQueueSnapshot()
            Task { [weak self] in
                await self?.syncNow()
            }
            return true
        } catch {
            subpageError = error.localizedDescription
            return false
        }
    }

    func runSearch() async {
        guard isAuthenticated else {
            searchResults = []
            searchError = "Sign in to search your notes."
            return
        }
        let query = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty else {
            searchResults = []
            searchError = nil
            return
        }
        guard let client = apiClient else {
            searchError = "Set a server URL to search."
            return
        }

        isSearching = true
        searchError = nil
        do {
            searchResults = try await client.search(query: query, limit: 20)
            markServerReachable()
        } catch {
            if await handleAuthFailure(error) {
                searchResults = []
                return
            }
            handleConnectionFailure(error)
            searchError = userFacingMessage(for: error, fallback: "Search needs a reachable Blackwood server.")
            searchResults = []
        }
        isSearching = false
    }

    func openSearchResult(_ result: SearchResult) {
        selectedDate = Self.date(from: result.date) ?? selectedDate
        selectedTab = .today
        Task { await loadSelectedDate() }
    }

    func updateServerURL() async {
        do {
            serverURLString = try normalizedServerURLString(from: serverURLString)
        } catch {
            connectionTestState = .failed(message: error.localizedDescription)
            return
        }
        UserDefaults.standard.set(serverURLString, forKey: "blackwood.serverURL")
        connectionTestState = .idle
        stopChangeStream()
        await refreshAuthStatus()
        if isAuthenticated {
            await loadSelectedDate()
            await refreshServerReachability()
            startChangeStream()
            if isNetworkAvailable {
                await syncNow()
            }
        }
    }

    func testServerConnection() async {
        connectionTestState = .testing
        do {
            let normalized = try normalizedServerURLString(from: serverURLString)
            serverURLString = normalized
            guard let client = BlackwoodAPIClient(baseURL: URL(string: normalized)!) as BlackwoodAPIClient? else {
                connectionTestState = .failed(message: "Invalid server URL")
                return
            }
            let response = try await client.checkHealth()
            markServerReachable(version: response.version)
            connectionTestState = .success(version: response.version)
        } catch {
            handleConnectionFailure(error)
            connectionTestState = .failed(message: userFacingMessage(for: error, fallback: "Blackwood is unreachable right now."))
        }
    }

    func presentRecorder(autoStart: Bool = false) {
        guard isAuthenticated else { return }
        recorder.reset()
        recorder.autoStartOnAppear = autoStart
        if autoStart {
            recorder.state = .preparing
        }
        presentedSheet = .recording
    }

    func presentSettings() {
        presentedSheet = .settings
    }

    func syncNow(force: Bool = false) async {
        guard isAuthenticated else { return }
        guard isNetworkAvailable, let client = apiClient else { return }
        guard force || Date() >= nextAutomaticSyncAllowedAt else { return }
        guard !isSyncing else { return }
        isSyncing = true
        defer { isSyncing = false }

        do {
            let engine = SyncEngine(store: store, remote: client)
            _ = try await engine.sync()
            markServerReachable()
            syncRetryAttempt = 0
            nextAutomaticSyncAllowedAt = .distantPast
            await refreshQueueSnapshot()
            _ = await loadCachedNote(for: Self.dayString(from: selectedDate))
            if let activeSubpage {
                _ = await loadCachedSubpage(for: activeSubpage)
            }
        } catch {
            if await handleAuthFailure(error) {
                return
            }
            if let failure = error as? SyncFailure, failure.code == "failed_precondition" {
                noteError = failure.message
                await refreshSelectedDateFromServer(date: Self.dayString(from: selectedDate), showLoadingState: false)
                if let activeSubpage {
                    await loadSubpage(activeSubpage)
                }
                await refreshQueueSnapshot()
                return
            }
            if isConnectivityFailure(error) {
                handleConnectionFailure(error)
                scheduleAutomaticSyncRetry()
            } else {
                noteError = userFacingMessage(for: error, fallback: "Blackwood couldn’t sync your queued changes.")
            }
        }
    }

    func refreshQueueSnapshot() async {
        if let snapshot = try? await store.snapshot() {
            queueSnapshot = snapshot
        }
    }

    func pendingUploads() async -> [PendingEntryUpload] {
        (try? await store.pendingUploads()) ?? []
    }

    func retryUpload(id: String) async {
        guard var upload = (try? await store.pendingUploads())?.first(where: { $0.id == id }) else { return }
        upload.status = .pending
        upload.lastError = nil
        upload.nextRetryAt = nil
        do {
            try await store.updateUpload(upload)
            await refreshQueueSnapshot()
            await syncNow(force: true)
        } catch {
            noteError = error.localizedDescription
        }
    }

    func removeUpload(id: String) async {
        do {
            try await store.removeUpload(id: id, deleteLocalFile: true)
            await refreshQueueSnapshot()
        } catch {
            noteError = error.localizedDescription
        }
    }

    private var apiClient: BlackwoodAPIClient? {
        guard let normalized = try? normalizedServerURLString(from: serverURLString),
              let url = URL(string: normalized) else { return nil }
        return BlackwoodAPIClient(baseURL: url)
    }

    var normalizedServerURL: URL? {
        guard let normalized = try? normalizedServerURLString(from: serverURLString) else {
            return nil
        }
        return URL(string: normalized)
    }

    private func normalizedServerURLString(from raw: String) throws -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            throw ValidationError("Enter a Blackwood server URL.")
        }

        let withScheme: String
        if trimmed.contains("://") {
            withScheme = trimmed
        } else {
            withScheme = "http://\(trimmed)"
        }

        guard var components = URLComponents(string: withScheme),
              let scheme = components.scheme?.lowercased(),
              ["http", "https"].contains(scheme),
              components.host != nil else {
            throw ValidationError("Use a full URL like http://192.168.1.10:8080 or https://notes.example.com.")
        }

        components.path = components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let normalizedPath = components.path.isEmpty ? "" : "/\(components.path)"
        components.path = normalizedPath
        components.query = nil
        components.fragment = nil

        guard let normalized = components.url?.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) else {
            throw ValidationError("Use a valid Blackwood server URL.")
        }
        return normalized
    }

    private struct ValidationError: LocalizedError {
        let message: String
        init(_ message: String) { self.message = message }
        var errorDescription: String? { message }
    }

    private func handleFinishedRecording(fileURL: URL, duration: TimeInterval) async {
        let date = Self.dayString(from: selectedDate)
        let upload = PendingEntryUpload(
            date: date,
            localFilePath: fileURL.path,
            filename: fileURL.lastPathComponent,
            contentType: audioContentType(for: fileURL),
            capturedAt: Date(),
            duration: duration
        )

        do {
            try await store.queueAudioUpload(upload)
            await refreshQueueSnapshot()
            await syncNow()
        } catch {
            noteError = error.localizedDescription
        }
    }

    private func handlePendingLaunchActionIfNeeded() async {
        guard let pendingAction = pendingLaunchAction else { return }
        guard isAuthenticated else { return }
        clearPendingLaunchAction()

        switch pendingAction {
        case .startRecording:
            selectedTab = .today
            presentRecorder(autoStart: true)
        }
    }

    private func enterAuthenticatedWorkspace() async {
        guard isAuthenticated else { return }
        await hydrateLaunchState()
        await handlePendingLaunchActionIfNeeded()
        Task { [weak self] in
            await self?.finishLaunchingInBackground()
        }
    }

    func refreshAuthStatus() async {
        guard let client = authClient else {
            authStatusMessage = "Set a valid Blackwood server URL."
            if authState != .authenticated {
                authState = UserDefaults.standard.bool(forKey: Self.lastSetupRequiredKey) ? .needsSetup : .needsLogin
            }
            return
        }

        do {
            let status = try await client.status()
            authStatusMessage = nil
            UserDefaults.standard.set(status.authenticated, forKey: Self.lastAuthenticatedKey)
            UserDefaults.standard.set(status.setupRequired, forKey: Self.lastSetupRequiredKey)

            if status.setupRequired {
                authState = .needsSetup
                if authSetupInfo == nil {
                    await loadAuthSetupInfo()
                }
            } else if status.authenticated {
                authState = .authenticated
                authSetupInfo = nil
            } else {
                authState = .needsLogin
                authSetupInfo = nil
            }
        } catch {
            if authState != .authenticated {
                authState = UserDefaults.standard.bool(forKey: Self.lastSetupRequiredKey) ? .needsSetup : .needsLogin
            }
            authStatusMessage = userFacingMessage(for: error, fallback: "Blackwood is unreachable right now.")
        }
    }

    func loadAuthSetupInfo() async {
        guard authState == .needsSetup, authSetupInfo == nil, let client = authClient else { return }
        do {
            authSetupInfo = try await client.getSetupInfo()
            authStatusMessage = nil
        } catch {
            authStatusMessage = userFacingMessage(for: error, fallback: "Couldn’t load TOTP setup details.")
        }
    }

    func login(code: String) async -> Bool {
        guard let client = authClient else {
            authStatusMessage = "Set a valid Blackwood server URL."
            return false
        }

        do {
            let response = try await client.login(code: code)
            guard response.ok else {
                authStatusMessage = response.error ?? "Invalid code. Please try again."
                return false
            }
            UserDefaults.standard.set(true, forKey: Self.lastAuthenticatedKey)
            UserDefaults.standard.set(false, forKey: Self.lastSetupRequiredKey)
            authState = .authenticated
            authSetupInfo = nil
            authStatusMessage = nil
            await enterAuthenticatedWorkspace()
            return true
        } catch {
            authStatusMessage = userFacingMessage(for: error, fallback: "Blackwood could not sign you in right now.")
            return false
        }
    }

    func confirmAuthSetup(code: String) async -> Bool {
        guard authState == .needsSetup else {
            return false
        }
        guard let setupInfo = await ensureAuthSetupInfo() else {
            authStatusMessage = "Couldn’t load TOTP setup details."
            return false
        }
        let secret = setupInfo.secret
        guard let client = authClient else {
            authStatusMessage = "Set a valid Blackwood server URL."
            return false
        }

        do {
            let response = try await client.confirmSetup(secret: secret, code: code)
            guard response.ok else {
                authStatusMessage = response.error ?? "Invalid code. Please try again."
                return false
            }
        } catch {
            authStatusMessage = userFacingMessage(for: error, fallback: "Blackwood could not save TOTP setup right now.")
            return false
        }

        UserDefaults.standard.set(false, forKey: Self.lastSetupRequiredKey)
        authState = .needsLogin
        authSetupInfo = nil
        return await login(code: code)
    }

    func logout() async {
        guard let client = authClient else {
            authState = .needsLogin
            return
        }

        do {
            try await client.logout()
        } catch {
            authStatusMessage = userFacingMessage(for: error, fallback: "Blackwood could not sign you out cleanly.")
        }

        UserDefaults.standard.set(false, forKey: Self.lastAuthenticatedKey)
        authState = UserDefaults.standard.bool(forKey: Self.lastSetupRequiredKey) ? .needsSetup : .needsLogin
        authSetupInfo = nil
        authStatusMessage = nil
        noteContent = ""
        draftContent = ""
        noteRevision = ""
        closeSubpage()
        searchResults = []
        searchError = nil
        noteError = nil
        stopChangeStream()
    }

    private func hydrateLaunchState() async {
        let date = Self.dayString(from: selectedDate)
        _ = await loadCachedNote(for: date)
        await refreshQueueSnapshot()
    }

    private func finishLaunchingInBackground() async {
        guard isAuthenticated else { return }
        let date = Self.dayString(from: selectedDate)
        await refreshSelectedDateFromServer(date: date, showLoadingState: noteContent.isEmpty)
        startChangeStream()
        if isNetworkAvailable {
            await refreshServerReachability()
            await syncNow()
        }
    }

    private func loadSubpage(_ route: SubpageRoute) async {
        guard isAuthenticated else { return }
        let hasCachedSubpage = await loadCachedSubpage(for: route)
        guard activeSubpage == route else { return }

        isLoadingSubpage = !hasCachedSubpage
        defer {
            if activeSubpage == route {
                isLoadingSubpage = false
            }
        }

        guard let client = apiClient else {
            if !hasCachedSubpage {
                subpageContent = ""
                subpageDraftContent = ""
                subpageRevision = ""
                isEditingSubpage = true
                subpageError = "Set a server URL to sync this subpage. Your edits will stay on this device."
            }
            return
        }

        do {
            let subpage = try await client.fetchSubpage(date: route.date, name: route.name)
            if activeSubpage != route || !isEditingSubpage {
                try? await store.cacheSubpage(
                    date: route.date,
                    name: route.name,
                    content: subpage.content,
                    revision: subpage.revision
                )
            }
            markServerReachable()
            guard activeSubpage == route else { return }
            guard !isEditingSubpage else { return }
            subpageContent = subpage.content
            subpageDraftContent = MarkdownStorage.visibleMarkdown(from: subpage.content)
            subpageRevision = subpage.revision
            subpageError = nil
        } catch {
            if await handleAuthFailure(error) {
                return
            }
            guard activeSubpage == route else { return }
            if let failure = error as? SyncFailure, failure.code == "not_found" {
                do {
                    let created = try await client.updateSubpageContent(
                        date: route.date,
                        name: route.name,
                        content: "",
                        baseRevision: ""
                    )
                    if activeSubpage != route || !isEditingSubpage {
                        try? await store.cacheSubpage(
                            date: route.date,
                            name: route.name,
                            content: created.content,
                            revision: created.revision
                        )
                    }
                    markServerReachable()
                    guard activeSubpage == route else { return }
                    guard !isEditingSubpage else { return }
                    subpageContent = created.content
                    subpageDraftContent = MarkdownStorage.visibleMarkdown(from: created.content)
                    subpageRevision = created.revision
                    subpageError = nil
                    isEditingSubpage = true
                    return
                } catch {
                    guard activeSubpage == route else { return }
                    if isConnectivityFailure(error) {
                        handleConnectionFailure(error)
                        if !hasCachedSubpage {
                            subpageContent = ""
                            subpageDraftContent = ""
                            subpageRevision = ""
                            isEditingSubpage = true
                            subpageError = "Blackwood is unreachable. Start writing here and the subpage will sync later."
                        }
                    } else {
                        subpageError = userFacingMessage(for: error, fallback: "Blackwood couldn’t create this subpage.")
                    }
                    return
                }
            }
            handleConnectionFailure(error)
            if hasCachedSubpage {
                subpageError = nil
            } else if isConnectivityFailure(error) {
                subpageContent = ""
                subpageDraftContent = ""
                subpageRevision = ""
                isEditingSubpage = true
                subpageError = "Blackwood is unreachable. Start writing here and the subpage will sync later."
            } else {
                subpageError = userFacingMessage(for: error, fallback: "Blackwood couldn’t load this subpage.")
            }
        }
    }

    @discardableResult
    private func loadCachedNote(for date: String) async -> Bool {
        guard let cached = try? await store.cachedDailyNote(date: date) else {
            return false
        }
        guard Self.dayString(from: selectedDate) == date else { return false }
        guard !isEditing else { return true }
        noteContent = cached.content
        noteRevision = cached.revision
        draftContent = MarkdownStorage.visibleMarkdown(from: cached.content)
        return true
    }

    @discardableResult
    private func loadCachedSubpage(for route: SubpageRoute) async -> Bool {
        guard let cached = try? await store.cachedSubpage(date: route.date, name: route.name) else {
            return false
        }
        guard activeSubpage == route else { return false }
        guard !isEditingSubpage else { return true }
        subpageContent = cached.content
        subpageRevision = cached.revision
        subpageDraftContent = MarkdownStorage.visibleMarkdown(from: cached.content)
        return true
    }

    private func refreshSelectedDateFromServer(date: String, showLoadingState: Bool) async {
        guard isAuthenticated else { return }
        let isSelectedDate = Self.dayString(from: selectedDate) == date
        if showLoadingState, isSelectedDate {
            isLoadingNote = true
        }
        defer {
            if Self.dayString(from: selectedDate) == date {
                isLoadingNote = false
            }
        }

        guard let client = apiClient else { return }

        do {
            let note = try await client.fetchDailyNote(date: date)
            if Self.dayString(from: selectedDate) != date || !isEditing {
                try await store.cacheDailyNote(
                    date: date,
                    content: note.content,
                    updatedAt: ISO8601DateFormatter().date(from: note.updatedAt) ?? Date(),
                    revision: note.revision
                )
            }
            markServerReachable()
            guard Self.dayString(from: selectedDate) == date else { return }
            guard !isEditing else { return }
            noteContent = note.content
            noteRevision = note.revision
            draftContent = MarkdownStorage.visibleMarkdown(from: note.content)
        } catch {
            if await handleAuthFailure(error) {
                return
            }
            guard Self.dayString(from: selectedDate) == date else { return }
            if let failure = error as? SyncFailure, failure.code == "failed_precondition" {
                noteError = "This note changed on another client. The latest version has been reloaded."
                await refreshSelectedDateFromServer(date: date, showLoadingState: false)
                return
            }
            handleConnectionFailure(error)
            if noteContent.isEmpty {
                noteError = userFacingMessage(for: error, fallback: "Blackwood is unreachable right now.")
            }
        }
    }

    private func startChangeStream() {
        guard changeStreamTask == nil, isAuthenticated, let client = apiClient else { return }
        changeStreamTask = Task { [weak self] in
            while let self, !Task.isCancelled {
                do {
                    for try await event in client.makeChangeStream() {
                        if Task.isCancelled { return }
                        switch event.kind {
                        case "CHANGE_EVENT_KIND_DAILY_NOTE_UPDATED":
                            let selected = Self.dayString(from: self.selectedDate)
                            guard event.date == selected else { continue }
                            guard !self.isEditing else { continue }
                            guard event.revision != self.noteRevision else { continue }
                            await self.refreshSelectedDateFromServer(date: selected, showLoadingState: false)
                        case "CHANGE_EVENT_KIND_SUBPAGE_UPDATED":
                            guard let route = self.activeSubpage else { continue }
                            guard route.date == event.date, route.name == event.subpageName else { continue }
                            guard !self.isEditingSubpage else { continue }
                            guard event.revision != self.subpageRevision else { continue }
                            await self.loadSubpage(route)
                        default:
                            continue
                        }
                    }
                    return
                } catch {
                    if Task.isCancelled { return }
                    try? await Task.sleep(for: .seconds(2))
                }
            }
        }
    }

    private func stopChangeStream() {
        changeStreamTask?.cancel()
        changeStreamTask = nil
    }

    private func audioContentType(for fileURL: URL) -> String {
        switch fileURL.pathExtension.lowercased() {
        case "m4a":
            return "audio/x-m4a"
        case "wav":
            return "audio/wav"
        case "mp3":
            return "audio/mpeg"
        default:
            return "application/octet-stream"
        }
    }

    static func dayString(from date: Date) -> String {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.string(from: date)
    }

    static func date(from dayString: String) -> Date? {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.dateFormat = "yyyy-MM-dd"
        return formatter.date(from: dayString)
    }

    private var isServerReachable: Bool {
        if case .reachable = serverReachability {
            return true
        }
        return false
    }

    private func refreshServerReachability() async {
        guard isNetworkAvailable, let client = apiClient else {
            serverReachability = .unknown
            return
        }

        serverReachability = .checking
        do {
            let response = try await client.checkHealth()
            markServerReachable(version: response.version)
        } catch {
            handleConnectionFailure(error)
        }
    }

    private func markServerReachable(version: String = "") {
        serverReachability = .reachable(version: version)
    }

    private func handleConnectionFailure(_ error: Error) {
        guard isConnectivityFailure(error) else { return }
        serverReachability = .unreachable(message: userFacingMessage(for: error, fallback: "Blackwood is unreachable right now."))
    }

    private func handleAuthFailure(_ error: Error) async -> Bool {
        guard let challenge = error as? AuthChallenge else {
            return false
        }

        authStatusMessage = challenge.message
        authSetupInfo = nil
        noteError = nil
        searchError = nil
        UserDefaults.standard.set(false, forKey: Self.lastAuthenticatedKey)

        switch challenge.kind {
        case .setupRequired:
            UserDefaults.standard.set(true, forKey: Self.lastSetupRequiredKey)
            authState = .needsSetup
            await loadAuthSetupInfo()
        case .unauthorized:
            UserDefaults.standard.set(false, forKey: Self.lastSetupRequiredKey)
            authState = .needsLogin
        }
        return true
    }

    private func scheduleAutomaticSyncRetry() {
        syncRetryAttempt += 1
        let cappedAttempt = min(syncRetryAttempt, 5)
        nextAutomaticSyncAllowedAt = Date().addingTimeInterval(pow(2, Double(cappedAttempt - 1)) * 5)
    }

    private func isConnectivityFailure(_ error: Error) -> Bool {
        if let failure = error as? SyncFailure {
            return failure.disposition == .retryable
        }
        if error is URLError {
            return true
        }
        let nsError = error as NSError
        return nsError.domain == NSURLErrorDomain
    }

    private func userFacingMessage(for error: Error, fallback: String) -> String {
        if let failure = error as? SyncFailure, !failure.message.isEmpty {
            return failure.message
        }
        if isConnectivityFailure(error) {
            return fallback
        }
        let description = error.localizedDescription.trimmingCharacters(in: .whitespacesAndNewlines)
        return description.isEmpty ? fallback : description
    }

    private func ensureAuthSetupInfo() async -> AuthSetupInfo? {
        if let info = authSetupInfo {
            return info
        }
        await loadAuthSetupInfo()
        return authSetupInfo
    }

    private var pendingLaunchAction: PendingLaunchAction? {
        guard let rawValue = Self.shortcutStore.string(forKey: ShortcutKeys.pendingLaunchAction) else {
            return nil
        }
        return PendingLaunchAction(rawValue: rawValue)
    }

    private func clearPendingLaunchAction() {
        Self.shortcutStore.removeObject(forKey: ShortcutKeys.pendingLaunchAction)
    }

    private var authClient: BlackwoodAuthClient? {
        guard let normalized = try? normalizedServerURLString(from: serverURLString),
              let url = URL(string: normalized) else { return nil }
        return BlackwoodAuthClient(baseURL: url)
    }
}
