import Foundation
import Testing
@testable import BlackwoodMobileCore

private actor MockRemote: BlackwoodRemote {
    var updatedDates: [String] = []
    var createdUploadIDs: [String] = []
    var failFirstUpload = false

    func setFailFirstUpload() {
        failFirstUpload = true
    }

    func fetchDailyNote(date: String) async throws -> APIDailyNote {
        APIDailyNote(id: "note", date: date, entries: [], createdAt: "", updatedAt: "", content: "# Notes", revision: "rev-1")
    }

    func updateDailyNoteContent(date: String, content: String, baseRevision: String) async throws -> APIDailyNote {
        updatedDates.append(date)
        return APIDailyNote(id: "note", date: date, entries: [], createdAt: "", updatedAt: "", content: content, revision: "rev-2")
    }

    func fetchSubpage(date: String, name: String) async throws -> APISubpage {
        APISubpage(name: name, content: "", date: date, revision: "sub-rev-1", updatedAt: "")
    }

    func updateSubpageContent(date: String, name: String, content: String, baseRevision: String) async throws -> APISubpage {
        APISubpage(name: name, content: content, date: date, revision: "sub-rev-2", updatedAt: "")
    }

    func createAudioEntry(upload: PendingEntryUpload) async throws -> APIEntry {
        if failFirstUpload {
            failFirstUpload = false
            throw SyncFailure(message: "temporary outage", disposition: .retryable)
        }
        createdUploadIDs.append(upload.id)
        return APIEntry(id: "entry", dailyNoteId: "note", type: 2, content: "", rawContent: "", source: 4, metadata: "", attachments: [], createdAt: "", updatedAt: "")
    }

    func search(query: String, limit: Int) async throws -> [SearchResult] { [] }

    func checkHealth() async throws -> HealthCheckResponse {
        HealthCheckResponse(status: "ok", version: "test")
    }

    func makeChangeStream() -> AsyncThrowingStream<APIChangeEvent, Error> {
        AsyncThrowingStream { continuation in
            continuation.finish()
        }
    }
}

@Test
func syncFlushesNoteUpdatesAndUploads() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    let audioURL = base.appendingPathComponent("clip.m4a")
    try Data("audio".utf8).write(to: audioURL)

    let store = QueueStore(baseDirectory: base)
    try await store.queueNoteUpdate(date: "2026-03-25", content: "updated", baseRevision: "rev-1")
    try await store.queueAudioUpload(PendingEntryUpload(date: "2026-03-25", localFilePath: audioURL.path, duration: 3))

    let remote = MockRemote()
    let engine = SyncEngine(store: store, remote: remote)
    let report = try await engine.sync()
    let pendingNoteUpdates = try await store.pendingNoteUpdates()
    let pendingUploads = try await store.pendingUploads()

    #expect(report.syncedNoteUpdates == 1)
    #expect(report.syncedUploads == 1)
    #expect(pendingNoteUpdates.isEmpty)
    #expect(pendingUploads.isEmpty)
}

@Test
func retryableUploadFailureLeavesUploadQueued() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    let audioURL = base.appendingPathComponent("clip.m4a")
    try Data("audio".utf8).write(to: audioURL)

    let store = QueueStore(baseDirectory: base)
    try await store.queueAudioUpload(PendingEntryUpload(date: "2026-03-25", localFilePath: audioURL.path, duration: 3))

    let remote = MockRemote()
    await remote.setFailFirstUpload()
    let engine = SyncEngine(store: store, remote: remote)
    _ = try await engine.sync(now: Date(timeIntervalSince1970: 1_000))

    let uploads = try await store.pendingUploads()
    #expect(uploads.count == 1)
    #expect(uploads[0].status == .failed)
    #expect(uploads[0].nextRetryAt != nil)
}
