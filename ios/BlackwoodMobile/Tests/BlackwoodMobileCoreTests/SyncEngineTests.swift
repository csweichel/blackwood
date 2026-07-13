import Foundation
import Testing
@testable import BlackwoodMobileCore

private actor MockRemote: @preconcurrency BlackwoodRemote {
    var updatedDates: [String] = []
    var updatedSubpages: [String] = []
    var noteUpdateRequests: [(date: String, content: String, baseRevision: String)] = []
    var subpageUpdateRequests: [(date: String, name: String, content: String, baseRevision: String)] = []
    var createdUploadIDs: [String] = []
    var failFirstUpload = false
    var failNextUploadTerminal = false
    var failNextNoteUpdateWithPrecondition = false
    var failNextSubpageUpdateWithPrecondition = false
    var remoteNoteContent = "# Notes"
    var remoteNoteRevision = "rev-1"
    var remoteSubpageContent = ""
    var remoteSubpageRevision = "sub-rev-1"

    func setFailFirstUpload() {
        failFirstUpload = true
    }

    func setFailNextUploadTerminal() {
        failNextUploadTerminal = true
    }

    func setNextNotePrecondition(remoteContent: String, revision: String = "rev-remote") {
        failNextNoteUpdateWithPrecondition = true
        remoteNoteContent = remoteContent
        remoteNoteRevision = revision
    }

    func setNextSubpagePrecondition(remoteContent: String, revision: String = "sub-rev-remote") {
        failNextSubpageUpdateWithPrecondition = true
        remoteSubpageContent = remoteContent
        remoteSubpageRevision = revision
    }

    func fetchDailyNote(date: String) async throws -> APIDailyNote {
        APIDailyNote(
            id: "note",
            date: date,
            entries: [],
            createdAt: "",
            updatedAt: "",
            content: remoteNoteContent,
            revision: remoteNoteRevision
        )
    }

    func updateDailyNoteContent(date: String, content: String, baseRevision: String) async throws -> APIDailyNote {
        updatedDates.append(date)
        noteUpdateRequests.append((date: date, content: content, baseRevision: baseRevision))
        if failNextNoteUpdateWithPrecondition {
            failNextNoteUpdateWithPrecondition = false
            throw SyncFailure(message: "stale revision", disposition: .terminal, code: "failed_precondition")
        }
        remoteNoteContent = content
        remoteNoteRevision = "rev-\(noteUpdateRequests.count + 1)"
        return APIDailyNote(
            id: "note",
            date: date,
            entries: [],
            createdAt: "",
            updatedAt: "",
            content: content,
            revision: remoteNoteRevision
        )
    }

    func fetchSubpage(date: String, name: String) async throws -> APISubpage {
        APISubpage(name: name, content: remoteSubpageContent, date: date, revision: remoteSubpageRevision, updatedAt: "")
    }

    func updateSubpageContent(date: String, name: String, content: String, baseRevision: String) async throws -> APISubpage {
        updatedSubpages.append("\(date)|\(name)")
        subpageUpdateRequests.append((date: date, name: name, content: content, baseRevision: baseRevision))
        if failNextSubpageUpdateWithPrecondition {
            failNextSubpageUpdateWithPrecondition = false
            throw SyncFailure(message: "stale revision", disposition: .terminal, code: "failed_precondition")
        }
        remoteSubpageContent = content
        remoteSubpageRevision = "sub-rev-\(subpageUpdateRequests.count + 1)"
        return APISubpage(name: name, content: content, date: date, revision: remoteSubpageRevision, updatedAt: "")
    }

    func createAudioEntry(upload: PendingEntryUpload) async throws -> APIEntry {
        if failFirstUpload {
            failFirstUpload = false
            throw SyncFailure(message: "temporary outage", disposition: .retryable)
        }
        if failNextUploadTerminal {
            failNextUploadTerminal = false
            throw SyncFailure(message: "unsupported file", disposition: .terminal)
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
func staleDailyNoteUpdateMergesAndRetriesWithRemoteRevision() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let store = QueueStore(baseDirectory: base)
    let baseContent = "# Summary\n\nold\n\n# Notes\n\nbase"
    let localContent = "# Summary\n\nlocal\n\n# Notes\n\nbase"
    let remoteContent = "# Summary\n\nold\n\n# Notes\n\nremote"

    try await store.queueNoteUpdate(
        date: "2026-03-25",
        content: localContent,
        baseRevision: "rev-base",
        baseContent: baseContent
    )

    let remote = MockRemote()
    await remote.setNextNotePrecondition(remoteContent: remoteContent, revision: "rev-remote")
    let engine = SyncEngine(store: store, remote: remote)
    let report = try await engine.sync()

    let pendingNoteUpdates = try await store.pendingNoteUpdates()
    let cachedNote = try await store.cachedDailyNote(date: "2026-03-25")
    let requests = await remote.noteUpdateRequests

    #expect(report.syncedNoteUpdates == 1)
    #expect(pendingNoteUpdates.isEmpty)
    #expect(requests.count == 2)
    #expect(requests[0].baseRevision == "rev-base")
    #expect(requests[1].baseRevision == "rev-remote")
    #expect(requests[1].content.contains("# Summary\n\nlocal"))
    #expect(requests[1].content.contains("# Notes\n\nremote"))
    #expect(cachedNote?.content == requests[1].content)
}

@Test
func staleDailyNoteUpdateWithoutBasePreservesBothSides() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let store = QueueStore(baseDirectory: base)
    let localContent = "# Local\n\noffline"
    let remoteContent = "# Remote\n\nserver"

    try await store.queueNoteUpdate(date: "2026-03-25", content: localContent, baseRevision: "rev-old")

    let remote = MockRemote()
    await remote.setNextNotePrecondition(remoteContent: remoteContent, revision: "rev-remote")
    let engine = SyncEngine(store: store, remote: remote)
    let report = try await engine.sync()

    let pendingNoteUpdates = try await store.pendingNoteUpdates()
    let cachedNote = try await store.cachedDailyNote(date: "2026-03-25")
    let requests = await remote.noteUpdateRequests

    #expect(report.syncedNoteUpdates == 1)
    #expect(pendingNoteUpdates.isEmpty)
    #expect(requests.count == 2)
    #expect(requests[1].baseRevision == "rev-remote")
    #expect(requests[1].content.contains("# Remote\n\nserver"))
    #expect(requests[1].content.contains("# Offline edits from iOS"))
    #expect(requests[1].content.contains("# Local\n\noffline"))
    #expect(cachedNote?.content == requests[1].content)
}

@Test
func staleSubpageUpdateMergesAndRetriesWithRemoteRevision() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let store = QueueStore(baseDirectory: base)
    let baseContent = "# Plan\n\nold\n\n# Tasks\n\nbase"
    let localContent = "# Plan\n\nlocal\n\n# Tasks\n\nbase"
    let remoteContent = "# Plan\n\nold\n\n# Tasks\n\nremote"

    try await store.queueSubpageUpdate(
        date: "2026-03-25",
        name: "Project",
        content: localContent,
        baseRevision: "sub-rev-base",
        baseContent: baseContent
    )

    let remote = MockRemote()
    await remote.setNextSubpagePrecondition(remoteContent: remoteContent, revision: "sub-rev-remote")
    let engine = SyncEngine(store: store, remote: remote)
    let report = try await engine.sync()

    let pendingSubpageUpdates = try await store.pendingSubpageUpdates()
    let cachedSubpage = try await store.cachedSubpage(date: "2026-03-25", name: "Project")
    let requests = await remote.subpageUpdateRequests

    #expect(report.syncedSubpageUpdates == 1)
    #expect(pendingSubpageUpdates.isEmpty)
    #expect(requests.count == 2)
    #expect(requests[0].baseRevision == "sub-rev-base")
    #expect(requests[1].baseRevision == "sub-rev-remote")
    #expect(requests[1].content.contains("# Plan\n\nlocal"))
    #expect(requests[1].content.contains("# Tasks\n\nremote"))
    #expect(cachedSubpage?.content == requests[1].content)
}

@Test
func syncFlushesNoteUpdatesAndUploads() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    let audioURL = base.appendingPathComponent("clip.m4a")
    try Data("audio".utf8).write(to: audioURL)

    let store = QueueStore(baseDirectory: base)
    try await store.queueNoteUpdate(date: "2026-03-25", content: "updated", baseRevision: "rev-1")
    try await store.queueSubpageUpdate(date: "2026-03-25", name: "Project", content: "subpage", baseRevision: "sub-rev-1")
    try await store.queueAudioUpload(PendingEntryUpload(date: "2026-03-25", localFilePath: audioURL.path, duration: 3))

    let remote = MockRemote()
    let engine = SyncEngine(store: store, remote: remote)
    let report = try await engine.sync()
    let pendingNoteUpdates = try await store.pendingNoteUpdates()
    let pendingSubpageUpdates = try await store.pendingSubpageUpdates()
    let pendingUploads = try await store.pendingUploads()
    let cachedSubpage = try await store.cachedSubpage(date: "2026-03-25", name: "Project")

    #expect(report.syncedNoteUpdates == 1)
    #expect(report.syncedSubpageUpdates == 1)
    #expect(report.syncedUploads == 1)
    #expect(pendingNoteUpdates.isEmpty)
    #expect(pendingSubpageUpdates.isEmpty)
    #expect(pendingUploads.isEmpty)
    #expect(cachedSubpage?.content == "subpage")
    #expect(cachedSubpage?.revision == "sub-rev-2")
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
    let report = try await engine.sync(now: Date(timeIntervalSince1970: 1_000))

    let uploads = try await store.pendingUploads()
    #expect(uploads.count == 1)
    #expect(uploads[0].status == .failed)
    #expect(uploads[0].nextRetryAt != nil)
    #expect(report.nextUploadRetryAt == Date(timeIntervalSince1970: 1_005))
    #expect(report.retryableUploadFailureMessage == "temporary outage")
}

@Test
func futureUploadRetryIsReportedAfterEngineRestart() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    let audioURL = base.appendingPathComponent("clip.m4a")
    try Data("audio".utf8).write(to: audioURL)
    let retryAt = Date(timeIntervalSince1970: 1_010)

    let store = QueueStore(baseDirectory: base)
    try await store.queueAudioUpload(
        PendingEntryUpload(
            date: "2026-03-25",
            localFilePath: audioURL.path,
            duration: 3,
            status: .failed,
            attemptCount: 1,
            lastError: "temporary outage",
            nextRetryAt: retryAt
        )
    )

    let engine = SyncEngine(store: store, remote: MockRemote())
    let report = try await engine.sync(now: Date(timeIntervalSince1970: 1_000))

    #expect(report.syncedUploads == 0)
    #expect(report.nextUploadRetryAt == retryAt)
    #expect(report.retryableUploadFailureMessage == nil)
}

@Test
func terminalUploadFailureDoesNotRetryInSameSync() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    let audioURL = base.appendingPathComponent("clip.m4a")
    try Data("audio".utf8).write(to: audioURL)

    let store = QueueStore(baseDirectory: base)
    try await store.queueAudioUpload(PendingEntryUpload(date: "2026-03-25", localFilePath: audioURL.path, duration: 3))

    let remote = MockRemote()
    await remote.setFailNextUploadTerminal()
    let engine = SyncEngine(store: store, remote: remote)
    let report = try await engine.sync(now: Date(timeIntervalSince1970: 1_000))

    let uploads = try await store.pendingUploads()
    #expect(report.syncedUploads == 0)
    #expect(uploads.count == 1)
    #expect(uploads[0].status == .failed)
    #expect(uploads[0].attemptCount == 1)
    #expect(uploads[0].nextRetryAt == nil)
    #expect(report.nextUploadRetryAt == nil)
    #expect(report.retryableUploadFailureMessage == nil)
}
