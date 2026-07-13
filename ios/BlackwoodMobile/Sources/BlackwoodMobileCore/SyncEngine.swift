import Foundation

public struct SyncReport: Equatable, Sendable {
    public let syncedNoteUpdates: Int
    public let syncedSubpageUpdates: Int
    public let syncedUploads: Int
    public let nextUploadRetryAt: Date?
    public let retryableUploadFailureMessage: String?
}

public final class SyncEngine: @unchecked Sendable {
    private let store: QueueStore
    private let remote: BlackwoodRemote

    public init(store: QueueStore, remote: BlackwoodRemote) {
        self.store = store
        self.remote = remote
    }

    public func sync(now: Date = Date()) async throws -> SyncReport {
        var syncedNoteUpdates = 0
        var syncedSubpageUpdates = 0
        var syncedUploads = 0
        var nextUploadRetryAt: Date?
        var retryableUploadFailureMessage: String?

        let noteUpdates = try await store.pendingNoteUpdates()
        for update in noteUpdates {
            do {
                let note = try await remote.updateDailyNoteContent(
                    date: update.date,
                    content: update.content,
                    baseRevision: update.baseRevision
                )
                try await store.cacheDailyNote(
                    date: update.date,
                    content: note.content,
                    updatedAt: ISO8601DateFormatter().date(from: note.updatedAt) ?? Date(),
                    revision: note.revision
                )
                try await store.removeNoteUpdate(id: update.id)
                syncedNoteUpdates += 1
            } catch let failure as SyncFailure {
                if failure.code == "failed_precondition" {
                    let remoteNote = try await remote.fetchDailyNote(date: update.date)
                    let mergedContent = Self.mergedQueuedContent(
                        base: update.baseContent,
                        local: update.content,
                        remote: remoteNote.content
                    )
                    let note = try await remote.updateDailyNoteContent(
                        date: update.date,
                        content: mergedContent,
                        baseRevision: remoteNote.revision
                    )
                    try await store.cacheDailyNote(
                        date: update.date,
                        content: note.content,
                        updatedAt: Self.date(from: note.updatedAt),
                        revision: note.revision
                    )
                    try await store.removeNoteUpdate(id: update.id)
                    syncedNoteUpdates += 1
                    continue
                }
                throw failure
            }
        }

        let subpageUpdates = try await store.pendingSubpageUpdates()
        for update in subpageUpdates {
            do {
                let subpage = try await remote.updateSubpageContent(
                    date: update.date,
                    name: update.name,
                    content: update.content,
                    baseRevision: update.baseRevision
                )
                try await store.cacheSubpage(
                    date: update.date,
                    name: update.name,
                    content: subpage.content,
                    updatedAt: ISO8601DateFormatter().date(from: subpage.updatedAt) ?? Date(),
                    revision: subpage.revision
                )
                try await store.removeSubpageUpdate(id: update.id)
                syncedSubpageUpdates += 1
            } catch let failure as SyncFailure {
                if failure.code == "failed_precondition" {
                    let remoteSubpage = try await remote.fetchSubpage(date: update.date, name: update.name)
                    let mergedContent = Self.mergedQueuedContent(
                        base: update.baseContent,
                        local: update.content,
                        remote: remoteSubpage.content
                    )
                    let subpage = try await remote.updateSubpageContent(
                        date: update.date,
                        name: update.name,
                        content: mergedContent,
                        baseRevision: remoteSubpage.revision
                    )
                    try await store.cacheSubpage(
                        date: update.date,
                        name: update.name,
                        content: subpage.content,
                        updatedAt: Self.date(from: subpage.updatedAt),
                        revision: subpage.revision
                    )
                    try await store.removeSubpageUpdate(id: update.id)
                    syncedSubpageUpdates += 1
                    continue
                }
                throw failure
            }
        }

        while var upload = try await store.claimNextUpload(now: now) {
            do {
                _ = try await remote.createAudioEntry(upload: upload)
                try await store.removeUpload(id: upload.id, deleteLocalFile: true)
                syncedUploads += 1
            } catch let failure as SyncFailure {
                upload.attemptCount += 1
                upload.status = .failed
                upload.lastError = failure.message
                upload.nextRetryAt = failure.disposition == .retryable
                    ? now.addingTimeInterval(Self.retryDelay(forAttempt: upload.attemptCount))
                    : nil
                try await store.updateUpload(upload)
                if failure.disposition == .retryable {
                    nextUploadRetryAt = upload.nextRetryAt
                    retryableUploadFailureMessage = failure.message
                    break
                }
            } catch {
                upload.attemptCount += 1
                upload.status = .failed
                upload.lastError = error.localizedDescription
                upload.nextRetryAt = now.addingTimeInterval(Self.retryDelay(forAttempt: upload.attemptCount))
                try await store.updateUpload(upload)
                nextUploadRetryAt = upload.nextRetryAt
                retryableUploadFailureMessage = error.localizedDescription
                break
            }
        }

        if let scheduledRetry = try await store.pendingUploads()
            .compactMap(\.nextRetryAt)
            .min(),
           nextUploadRetryAt.map({ scheduledRetry < $0 }) ?? true {
            nextUploadRetryAt = scheduledRetry
        }

        return SyncReport(
            syncedNoteUpdates: syncedNoteUpdates,
            syncedSubpageUpdates: syncedSubpageUpdates,
            syncedUploads: syncedUploads,
            nextUploadRetryAt: nextUploadRetryAt,
            retryableUploadFailureMessage: retryableUploadFailureMessage
        )
    }

    public static func retryDelay(forAttempt attempt: Int) -> TimeInterval {
        let capped = min(max(attempt, 1), 5)
        return pow(2, Double(capped - 1)) * 5
    }

    private static func mergedQueuedContent(base: String, local: String, remote: String) -> String {
        let visibleBase = MarkdownStorage.visibleMarkdown(from: base)
        let visibleLocal = MarkdownStorage.visibleMarkdown(from: local)
        let visibleRemote = MarkdownStorage.visibleMarkdown(from: remote)
        let result = MarkdownMerge.merge(base: visibleBase, local: visibleLocal, remote: visibleRemote)
        return result.merged ?? MarkdownMerge.preservingBothSides(local: visibleLocal, remote: visibleRemote)
    }

    private static func date(from value: String) -> Date {
        ISO8601DateFormatter().date(from: value) ?? Date()
    }
}
