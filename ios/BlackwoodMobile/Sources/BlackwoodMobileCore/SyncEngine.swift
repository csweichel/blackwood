import Foundation

public struct SyncReport: Equatable, Sendable {
    public let syncedNoteUpdates: Int
    public let syncedUploads: Int
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
        var syncedUploads = 0

        let noteUpdates = try await store.pendingNoteUpdates()
        for update in noteUpdates {
            _ = try await remote.updateDailyNoteContent(date: update.date, content: update.content)
            try await store.removeNoteUpdate(id: update.id)
            syncedNoteUpdates += 1
        }

        let uploads = try await store.pendingUploads()
        for var upload in uploads {
            if let nextRetryAt = upload.nextRetryAt, nextRetryAt > now {
                continue
            }

            upload.status = .uploading
            upload.lastError = nil
            try await store.updateUpload(upload)

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
                    break
                }
            } catch {
                upload.attemptCount += 1
                upload.status = .failed
                upload.lastError = error.localizedDescription
                upload.nextRetryAt = now.addingTimeInterval(Self.retryDelay(forAttempt: upload.attemptCount))
                try await store.updateUpload(upload)
                break
            }
        }

        return SyncReport(syncedNoteUpdates: syncedNoteUpdates, syncedUploads: syncedUploads)
    }

    public static func retryDelay(forAttempt attempt: Int) -> TimeInterval {
        let capped = min(max(attempt, 1), 5)
        return pow(2, Double(capped - 1)) * 5
    }
}
