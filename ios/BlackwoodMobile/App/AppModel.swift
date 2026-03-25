import Foundation
import Network
import SwiftUI

@MainActor
final class AppModel: ObservableObject {
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
        case settings
    }

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
    @Published var isOnline = true
    @Published var queueSnapshot = QueueSnapshot(noteUpdateCount: 0, uploadCount: 0, failedUploadCount: 0)
    @Published var serverURLString = UserDefaults.standard.string(forKey: "blackwood.serverURL") ?? "http://127.0.0.1:8080"
    @Published var connectionTestState: ConnectionTestState = .idle
    @Published var isRecordingSheetPresented = false

    let recorder = AudioRecorderController()
    let store = QueueStore()

    private let monitor = NWPathMonitor()
    private let monitorQueue = DispatchQueue(label: "BlackwoodConnectivity")
    private var didStart = false

    func start() async {
        guard !didStart else { return }
        didStart = true
        recorder.onFinishedRecording = { [weak self] url, duration in
            Task { await self?.handleFinishedRecording(fileURL: url, duration: duration) }
        }
        monitor.pathUpdateHandler = { [weak self] path in
            Task { @MainActor in
                self?.isOnline = path.status == .satisfied
                if path.status == .satisfied {
                    await self?.syncNow()
                }
            }
        }
        monitor.start(queue: monitorQueue)
        await loadSelectedDate()
        await refreshQueueSnapshot()
        await handleShortcutIfNeeded()
        await syncNow()
    }

    func handleAppBecameActive() async {
        await handleShortcutIfNeeded()
        await refreshQueueSnapshot()
        if isOnline {
            await syncNow()
        }
    }

    func loadSelectedDate() async {
        let date = Self.dayString(from: selectedDate)
        isLoadingNote = true
        noteError = nil

        if let cached = try? await store.cachedDailyNote(date: date) {
            noteContent = cached.content
            draftContent = cached.content
        }

        guard let client = apiClient else {
            isLoadingNote = false
            return
        }

        do {
            let note = try await client.fetchDailyNote(date: date)
            noteContent = note.content
            draftContent = note.content
            try await store.cacheDailyNote(date: date, content: note.content)
        } catch {
            if noteContent.isEmpty {
                noteError = error.localizedDescription
            }
        }

        isLoadingNote = false
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
        } catch {
            searchError = error.localizedDescription
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
        await loadSelectedDate()
        if isOnline {
            await syncNow()
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
            connectionTestState = .success(version: response.version)
        } catch {
            connectionTestState = .failed(message: error.localizedDescription)
        }
    }

    func presentRecorder(autoStart: Bool = false) {
        isRecordingSheetPresented = true
        recorder.autoStartOnAppear = autoStart
    }

    func syncNow() async {
        guard isOnline, let client = apiClient else { return }
        do {
            let engine = SyncEngine(store: store, remote: client)
            _ = try await engine.sync()
            await refreshQueueSnapshot()
        } catch {
            noteError = error.localizedDescription
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
            await syncNow()
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
            contentType: "audio/mp4",
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

    private func handleShortcutIfNeeded() async {
        let shouldStart = UserDefaults.standard.bool(forKey: ShortcutKeys.startRecording)
        guard shouldStart else { return }
        UserDefaults.standard.set(false, forKey: ShortcutKeys.startRecording)
        selectedTab = .today
        presentRecorder(autoStart: true)
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
}
