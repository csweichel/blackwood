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

    @Published var selectedTab: Tab = .today
    @Published var selectedDate = Date()
    @Published var noteContent = ""
    @Published var draftContent = ""
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
    @Published var queueSnapshot = QueueSnapshot(noteUpdateCount: 0, uploadCount: 0, failedUploadCount: 0)
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

    func loadSelectedDate() async {
        guard isAuthenticated else { return }
        let date = Self.dayString(from: selectedDate)
        noteError = nil
        let hasCachedNote = await loadCachedNote(for: date)
        await refreshSelectedDateFromServer(date: date, showLoadingState: !hasCachedNote)
    }

    func changeDate(to date: Date) {
        selectedDate = date
        Task { await loadSelectedDate() }
    }

    func beginEditing() {
        draftContent = noteContent.isEmpty ? "# Summary\n\n# Notes\n\n# Links\n" : noteContent
        isEditing = true
    }

    func cancelEditing() {
        draftContent = noteContent
        isEditing = false
    }

    func saveCurrentNote() async {
        guard isAuthenticated else { return }
        let date = Self.dayString(from: selectedDate)
        noteContent = draftContent
        isEditing = false

        do {
            try await store.cacheDailyNote(date: date, content: draftContent)
            try await store.queueNoteUpdate(date: date, content: draftContent)
            await refreshQueueSnapshot()
            await syncNow()
        } catch {
            noteError = error.localizedDescription
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
        await refreshAuthStatus()
        if isAuthenticated {
            await loadSelectedDate()
            await refreshServerReachability()
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
        do {
            let engine = SyncEngine(store: store, remote: client)
            _ = try await engine.sync()
            markServerReachable()
            syncRetryAttempt = 0
            nextAutomaticSyncAllowedAt = .distantPast
            await refreshQueueSnapshot()
        } catch {
            if await handleAuthFailure(error) {
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
        searchResults = []
        searchError = nil
        noteError = nil
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
        if isNetworkAvailable {
            await refreshServerReachability()
            await syncNow()
        }
    }

    @discardableResult
    private func loadCachedNote(for date: String) async -> Bool {
        guard let cached = try? await store.cachedDailyNote(date: date) else {
            return false
        }
        noteContent = cached.content
        if !isEditing {
            draftContent = cached.content
        }
        return true
    }

    private func refreshSelectedDateFromServer(date: String, showLoadingState: Bool) async {
        guard isAuthenticated else { return }
        if showLoadingState {
            isLoadingNote = true
        }
        defer { isLoadingNote = false }

        guard let client = apiClient else { return }

        do {
            let note = try await client.fetchDailyNote(date: date)
            noteContent = note.content
            if !isEditing {
                draftContent = note.content
            }
            try await store.cacheDailyNote(date: date, content: note.content)
            markServerReachable()
        } catch {
            if await handleAuthFailure(error) {
                return
            }
            handleConnectionFailure(error)
            if noteContent.isEmpty {
                noteError = userFacingMessage(for: error, fallback: "Blackwood is unreachable right now.")
            }
        }
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
