import Foundation
import Testing
@testable import BlackwoodMobileCore

@Test
func noteUpdatesAreLastWriteWinsPerDate() async throws {
    let store = QueueStore(baseDirectory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
    try await store.queueNoteUpdate(date: "2026-03-25", content: "first", baseRevision: "rev-1", baseContent: "original")
    try await store.queueNoteUpdate(date: "2026-03-25", content: "second", baseRevision: "rev-2", baseContent: "first")

    let updates = try await store.pendingNoteUpdates()
    #expect(updates.count == 1)
    #expect(updates.first?.content == "second")
    #expect(updates.first?.baseRevision == "rev-1")
    #expect(updates.first?.baseContent == "original")
}

@Test
func subpageUpdatesAreLastWriteWinsPerDateAndName() async throws {
    let store = QueueStore(baseDirectory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
    try await store.queueSubpageUpdate(
        date: "2026-03-25",
        name: "Project",
        content: "first",
        baseRevision: "sub-rev-1",
        baseContent: "original"
    )
    try await store.queueSubpageUpdate(
        date: "2026-03-25",
        name: "Project",
        content: "second",
        baseRevision: "sub-rev-2",
        baseContent: "first"
    )
    try await store.queueSubpageUpdate(date: "2026-03-25", name: "Other", content: "other", baseRevision: "sub-rev-3")

    let updates = try await store.pendingSubpageUpdates()
    #expect(updates.count == 2)
    #expect(updates.first(where: { $0.name == "Project" })?.content == "second")
    #expect(updates.first(where: { $0.name == "Project" })?.baseRevision == "sub-rev-1")
    #expect(updates.first(where: { $0.name == "Project" })?.baseContent == "original")
    #expect(updates.first(where: { $0.name == "Other" })?.content == "other")
}

@Test
func staleAutosaveCannotOverwriteNewerDailyNoteSave() async throws {
    let store = QueueStore(baseDirectory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
    let date = "2026-03-25"

    try await store.savePendingDailyNote(
        date: date,
        content: "newest",
        revision: "rev-1",
        baseContent: "original",
        saveSequence: 2
    )
    try await store.savePendingDailyNote(
        date: date,
        content: "stale",
        revision: "rev-1",
        baseContent: "original",
        saveSequence: 1
    )

    let cached = try await store.cachedDailyNote(date: date)
    let queued = try await store.pendingNoteUpdates()
    #expect(cached?.content == "newest")
    #expect(queued.first?.content == "newest")
}

@Test
func staleAutosaveCannotOverwriteNewerSubpageSave() async throws {
    let store = QueueStore(baseDirectory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
    let date = "2026-03-25"

    try await store.savePendingSubpage(
        date: date,
        name: "Project",
        content: "newest",
        revision: "rev-1",
        baseContent: "original",
        saveSequence: 2
    )
    try await store.savePendingSubpage(
        date: date,
        name: "Project",
        content: "stale",
        revision: "rev-1",
        baseContent: "original",
        saveSequence: 1
    )

    let cached = try await store.cachedSubpage(date: date, name: "Project")
    let queued = try await store.pendingSubpageUpdates()
    #expect(cached?.content == "newest")
    #expect(queued.first?.content == "newest")
}

@Test
func repeatedDailyNoteSavesPreserveAnInitiallyEmptyMergeBase() async throws {
    let store = QueueStore(baseDirectory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
    let date = "2026-03-25"

    try await store.savePendingDailyNote(
        date: date,
        content: "first local edit",
        revision: "",
        baseContent: "",
        saveSequence: 1
    )
    try await store.savePendingDailyNote(
        date: date,
        content: "second local edit",
        revision: "rev-that-arrived-later",
        baseContent: "first local edit",
        saveSequence: 2
    )

    let queued = try await store.pendingNoteUpdates()
    #expect(queued.first?.content == "second local edit")
    #expect(queued.first?.baseRevision == "")
    #expect(queued.first?.baseContent == "")
}

@Test
func repeatedSubpageSavesPreserveAnInitiallyEmptyMergeBase() async throws {
    let store = QueueStore(baseDirectory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
    let date = "2026-03-25"

    try await store.savePendingSubpage(
        date: date,
        name: "Project",
        content: "first local edit",
        revision: "",
        baseContent: "",
        saveSequence: 1
    )
    try await store.savePendingSubpage(
        date: date,
        name: "Project",
        content: "second local edit",
        revision: "rev-that-arrived-later",
        baseContent: "first local edit",
        saveSequence: 2
    )

    let queued = try await store.pendingSubpageUpdates()
    #expect(queued.first?.content == "second local edit")
    #expect(queued.first?.baseRevision == "")
    #expect(queued.first?.baseContent == "")
}

@Test
func cachedSubpagesPersistAcrossStoreInstances() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let store = QueueStore(baseDirectory: base)
    try await store.cacheSubpage(date: "2026-03-25", name: "Project", content: "offline note", revision: "sub-rev-1")

    let reloaded = QueueStore(baseDirectory: base)
    let cached = try await reloaded.cachedSubpage(date: "2026-03-25", name: "Project")

    #expect(cached?.content == "offline note")
    #expect(cached?.revision == "sub-rev-1")
}

@Test
func queuedUploadsPersistAcrossStoreInstances() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let fileURL = base.appendingPathComponent("recording.m4a")
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    try Data("audio".utf8).write(to: fileURL)

    let store = QueueStore(baseDirectory: base)
    try await store.queueAudioUpload(
        PendingEntryUpload(date: "2026-03-25", localFilePath: fileURL.path, duration: 12)
    )

    let reloaded = QueueStore(baseDirectory: base)
    let uploads = try await reloaded.pendingUploads()
    let decoder = JSONDecoder()
    decoder.dateDecodingStrategy = .iso8601
    let storedUploads = try decoder.decode(
        [PendingEntryUpload].self,
        from: Data(contentsOf: base.appendingPathComponent("pending-entry-uploads.json"))
    )
    #expect(uploads.count == 1)
    #expect(uploads[0].localFilePath == fileURL.path)
    #expect(storedUploads[0].localFilePath == "recording.m4a")
}

@Test
func queuedUploadRebasesLegacyAbsolutePathAfterContainerMoves() async throws {
    let parent = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let oldBase = parent.appendingPathComponent("old-container")
    let newBase = parent.appendingPathComponent("new-container")
    let filename = "recording-relocated.m4a"
    let oldFile = oldBase.appendingPathComponent("Recordings").appendingPathComponent(filename)
    let newFile = newBase.appendingPathComponent("Recordings").appendingPathComponent(filename)
    try FileManager.default.createDirectory(at: newFile.deletingLastPathComponent(), withIntermediateDirectories: true)
    try Data("audio".utf8).write(to: newFile)

    let legacyUpload = PendingEntryUpload(
        date: "2026-03-25",
        localFilePath: oldFile.path,
        filename: filename,
        duration: 12,
        status: .failed,
        attemptCount: 1,
        lastError: "This recording is no longer stored on the device. Remove it from the queue and record again."
    )
    let encoder = JSONEncoder()
    encoder.dateEncodingStrategy = .iso8601
    try encoder.encode([legacyUpload]).write(
        to: newBase.appendingPathComponent("pending-entry-uploads.json"),
        options: .atomic
    )

    let store = QueueStore(baseDirectory: newBase)
    let uploads = try await store.pendingUploads()
    let claimed = try await store.claimNextUpload()

    #expect(uploads.count == 1)
    #expect(uploads[0].localFilePath == newFile.path)
    #expect(uploads[0].status == .pending)
    #expect(uploads[0].lastError == nil)
    #expect(claimed?.localFilePath == newFile.path)
}

@Test
func audioUploadsAreDeduplicatedByFilePath() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let fileURL = base.appendingPathComponent("recording.m4a")
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    try Data("audio".utf8).write(to: fileURL)

    let store = QueueStore(baseDirectory: base)
    try await store.queueAudioUpload(
        PendingEntryUpload(clientRequestId: "first", date: "2026-03-25", localFilePath: fileURL.path, duration: 12)
    )
    try await store.queueAudioUpload(
        PendingEntryUpload(clientRequestId: "second", date: "2026-03-25", localFilePath: fileURL.path, duration: 12)
    )

    let uploads = try await store.pendingUploads()
    #expect(uploads.count == 1)
    #expect(uploads[0].clientRequestId == "first")
}

@Test
func claimNextUploadSkipsAlreadyUploadingItems() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let fileURL = base.appendingPathComponent("recording.m4a")
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    try Data("audio".utf8).write(to: fileURL)

    let store = QueueStore(baseDirectory: base)
    try await store.queueAudioUpload(
        PendingEntryUpload(date: "2026-03-25", localFilePath: fileURL.path, duration: 12)
    )

    let claimed = try await store.claimNextUpload()
    let secondClaim = try await store.claimNextUpload()

    #expect(claimed?.status == .uploading)
    #expect(secondClaim == nil)
}

@Test
func claimNextUploadSkipsTerminalFailedItems() async throws {
    let base = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let fileURL = base.appendingPathComponent("recording.m4a")
    try FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
    try Data("audio".utf8).write(to: fileURL)

    let store = QueueStore(baseDirectory: base)
    try await store.queueAudioUpload(
        PendingEntryUpload(
            date: "2026-03-25",
            localFilePath: fileURL.path,
            duration: 12,
            status: .failed,
            attemptCount: 1,
            lastError: "unsupported file",
            nextRetryAt: nil
        )
    )

    let claimed = try await store.claimNextUpload()

    #expect(claimed == nil)
}
