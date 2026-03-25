import Foundation
import Testing
@testable import BlackwoodMobileCore

@Test
func noteUpdatesAreLastWriteWinsPerDate() async throws {
    let store = QueueStore(baseDirectory: FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString))
    try await store.queueNoteUpdate(date: "2026-03-25", content: "first")
    try await store.queueNoteUpdate(date: "2026-03-25", content: "second")

    let updates = try await store.pendingNoteUpdates()
    #expect(updates.count == 1)
    #expect(updates.first?.content == "second")
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
    #expect(uploads.count == 1)
    #expect(uploads[0].localFilePath == fileURL.path)
}
