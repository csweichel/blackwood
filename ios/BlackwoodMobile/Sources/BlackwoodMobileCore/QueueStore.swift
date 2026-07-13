import Foundation

public actor QueueStore {
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()
    private let baseDirectory: URL
    private let cacheFile: URL
    private let noteUpdatesFile: URL
    private let subpageCacheFile: URL
    private let subpageUpdatesFile: URL
    private let uploadsFile: URL
    private var latestDailyNoteSaveSequence: [String: Int] = [:]
    private var latestSubpageSaveSequence: [String: Int] = [:]

    public init(baseDirectory: URL? = nil) {
        let root = baseDirectory ?? QueueStore.defaultBaseDirectory()
        self.baseDirectory = root
        self.cacheFile = root.appendingPathComponent("daily-note-cache.json")
        self.noteUpdatesFile = root.appendingPathComponent("pending-note-updates.json")
        self.subpageCacheFile = root.appendingPathComponent("subpage-cache.json")
        self.subpageUpdatesFile = root.appendingPathComponent("pending-subpage-updates.json")
        self.uploadsFile = root.appendingPathComponent("pending-entry-uploads.json")
        encoder.dateEncodingStrategy = .iso8601
        decoder.dateDecodingStrategy = .iso8601
    }

    public func cacheDailyNote(date: String, content: String, updatedAt: Date = Date(), revision: String = "") async throws {
        var cache = try loadCache()
        cache[date] = CachedDailyNote(date: date, content: content, updatedAt: updatedAt, revision: revision)
        try save(cache, to: cacheFile)
    }

    public func cachedDailyNote(date: String) async throws -> CachedDailyNote? {
        try loadCache()[date]
    }

    public func cacheSubpage(date: String, name: String, content: String, updatedAt: Date = Date(), revision: String = "") async throws {
        var cache = try loadSubpageCache()
        cache[Self.subpageKey(date: date, name: name)] = CachedSubpage(
            date: date,
            name: name,
            content: content,
            updatedAt: updatedAt,
            revision: revision
        )
        try save(cache, to: subpageCacheFile)
    }

    public func cachedSubpage(date: String, name: String) async throws -> CachedSubpage? {
        try loadSubpageCache()[Self.subpageKey(date: date, name: name)]
    }

    public func savePendingDailyNote(
        date: String,
        content: String,
        updatedAt: Date = Date(),
        revision: String,
        baseContent: String,
        saveSequence: Int
    ) async throws {
        guard saveSequence >= latestDailyNoteSaveSequence[date, default: 0] else { return }
        latestDailyNoteSaveSequence[date] = saveSequence

        var cache = try loadCache()
        cache[date] = CachedDailyNote(
            date: date,
            content: content,
            updatedAt: updatedAt,
            revision: revision
        )

        var updates = try loadNoteUpdates()
        if let existingIndex = updates.firstIndex(where: { $0.date == date }) {
            updates[existingIndex].content = content
            updates[existingIndex].updatedAt = updatedAt
        } else {
            updates.append(
                PendingNoteUpdate(
                    date: date,
                    content: content,
                    updatedAt: updatedAt,
                    baseRevision: revision,
                    baseContent: baseContent
                )
            )
        }
        updates.sort { $0.updatedAt < $1.updatedAt }

        try save(cache, to: cacheFile)
        try save(updates, to: noteUpdatesFile)
    }

    public func savePendingSubpage(
        date: String,
        name: String,
        content: String,
        updatedAt: Date = Date(),
        revision: String,
        baseContent: String,
        saveSequence: Int
    ) async throws {
        let key = Self.subpageKey(date: date, name: name)
        guard saveSequence >= latestSubpageSaveSequence[key, default: 0] else { return }
        latestSubpageSaveSequence[key] = saveSequence

        var cache = try loadSubpageCache()
        cache[key] = CachedSubpage(
            date: date,
            name: name,
            content: content,
            updatedAt: updatedAt,
            revision: revision
        )

        var updates = try loadSubpageUpdates()
        if let existingIndex = updates.firstIndex(where: { $0.date == date && $0.name == name }) {
            updates[existingIndex].content = content
            updates[existingIndex].updatedAt = updatedAt
        } else {
            updates.append(
                PendingSubpageUpdate(
                    date: date,
                    name: name,
                    content: content,
                    updatedAt: updatedAt,
                    baseRevision: revision,
                    baseContent: baseContent
                )
            )
        }
        updates.sort { $0.updatedAt < $1.updatedAt }

        try save(cache, to: subpageCacheFile)
        try save(updates, to: subpageUpdatesFile)
    }

    public func queueNoteUpdate(
        date: String,
        content: String,
        updatedAt: Date = Date(),
        baseRevision: String,
        baseContent: String = ""
    ) async throws {
        var updates = try loadNoteUpdates()
        if let existingIndex = updates.firstIndex(where: { $0.date == date }) {
            updates[existingIndex].content = content
            updates[existingIndex].updatedAt = updatedAt
        } else {
            updates.append(
                PendingNoteUpdate(
                    date: date,
                    content: content,
                    updatedAt: updatedAt,
                    baseRevision: baseRevision,
                    baseContent: baseContent
                )
            )
        }
        updates.sort { $0.updatedAt < $1.updatedAt }
        try save(updates, to: noteUpdatesFile)
    }

    public func queueSubpageUpdate(
        date: String,
        name: String,
        content: String,
        updatedAt: Date = Date(),
        baseRevision: String,
        baseContent: String = ""
    ) async throws {
        var updates = try loadSubpageUpdates()
        if let existingIndex = updates.firstIndex(where: { $0.date == date && $0.name == name }) {
            updates[existingIndex].content = content
            updates[existingIndex].updatedAt = updatedAt
        } else {
            updates.append(
                PendingSubpageUpdate(
                    date: date,
                    name: name,
                    content: content,
                    updatedAt: updatedAt,
                    baseRevision: baseRevision,
                    baseContent: baseContent
                )
            )
        }
        updates.sort { $0.updatedAt < $1.updatedAt }
        try save(updates, to: subpageUpdatesFile)
    }

    public func pendingNoteUpdates() async throws -> [PendingNoteUpdate] {
        try loadNoteUpdates()
    }

    public func pendingSubpageUpdates() async throws -> [PendingSubpageUpdate] {
        try loadSubpageUpdates()
    }

    public func removeNoteUpdate(id: String) async throws {
        var updates = try loadNoteUpdates()
        updates.removeAll { $0.id == id }
        try save(updates, to: noteUpdatesFile)
    }

    public func removeSubpageUpdate(id: String) async throws {
        var updates = try loadSubpageUpdates()
        updates.removeAll { $0.id == id }
        try save(updates, to: subpageUpdatesFile)
    }

    public func queueAudioUpload(_ upload: PendingEntryUpload) async throws {
        var uploads = try loadUploads()
        guard !uploads.contains(where: { existing in
            existing.clientRequestId == upload.clientRequestId || existing.localFilePath == upload.localFilePath
        }) else {
            return
        }
        uploads.append(upload)
        uploads.sort { $0.createdAt < $1.createdAt }
        try saveUploads(uploads)
    }

    public func pendingUploads() async throws -> [PendingEntryUpload] {
        try loadUploads()
    }

    public func updateUpload(_ upload: PendingEntryUpload) async throws {
        var uploads = try loadUploads()
        if let index = uploads.firstIndex(where: { $0.id == upload.id }) {
            uploads[index] = upload
            try saveUploads(uploads)
        }
    }

    public func claimNextUpload(now: Date = Date()) async throws -> PendingEntryUpload? {
        var uploads = try loadUploads()
        guard let index = uploads.firstIndex(where: { upload in
            switch upload.status {
            case .pending:
                return true
            case .failed:
                return upload.nextRetryAt.map { $0 <= now } ?? false
            case .uploading:
                return false
            }
        }) else {
            return nil
        }

        uploads[index].status = .uploading
        uploads[index].lastError = nil
        try saveUploads(uploads)
        return uploads[index]
    }

    public func resetInterruptedUploads() async throws {
        var uploads = try loadUploads()
        var changed = false
        for index in uploads.indices where uploads[index].status == .uploading {
            uploads[index].status = .pending
            uploads[index].lastError = nil
            uploads[index].nextRetryAt = nil
            changed = true
        }
        if changed {
            try saveUploads(uploads)
        }
    }

    public func removeUpload(id: String, deleteLocalFile: Bool) async throws {
        var uploads = try loadUploads()
        guard let index = uploads.firstIndex(where: { $0.id == id }) else { return }
        let removed = uploads.remove(at: index)
        try saveUploads(uploads)
        if deleteLocalFile {
            try? FileManager.default.removeItem(atPath: removed.localFilePath)
        }
    }

    public func snapshot() async throws -> QueueSnapshot {
        let updates = try loadNoteUpdates()
        let subpageUpdates = try loadSubpageUpdates()
        let uploads = try loadUploads()
        let failed = uploads.filter { $0.status == .failed }.count
        return QueueSnapshot(
            noteUpdateCount: updates.count,
            subpageUpdateCount: subpageUpdates.count,
            uploadCount: uploads.count,
            failedUploadCount: failed
        )
    }

    private func loadCache() throws -> [String: CachedDailyNote] {
        try load([String: CachedDailyNote].self, from: cacheFile, defaultValue: [:])
    }

    private func loadSubpageCache() throws -> [String: CachedSubpage] {
        try load([String: CachedSubpage].self, from: subpageCacheFile, defaultValue: [:])
    }

    private func loadNoteUpdates() throws -> [PendingNoteUpdate] {
        try load([PendingNoteUpdate].self, from: noteUpdatesFile, defaultValue: [])
    }

    private func loadSubpageUpdates() throws -> [PendingSubpageUpdate] {
        try load([PendingSubpageUpdate].self, from: subpageUpdatesFile, defaultValue: [])
    }

    private func loadUploads() throws -> [PendingEntryUpload] {
        var uploads = try load([PendingEntryUpload].self, from: uploadsFile, defaultValue: [])
        var migratedLegacyPath = false

        for index in uploads.indices {
            let storedPath = uploads[index].localFilePath
            let resolvedPath = resolveUploadPath(uploads[index])
            uploads[index].localFilePath = resolvedPath

            if (storedPath as NSString).isAbsolutePath,
               storedPath != resolvedPath,
               FileManager.default.fileExists(atPath: resolvedPath) {
                migratedLegacyPath = true
            }

            if storedPath != resolvedPath,
               FileManager.default.fileExists(atPath: resolvedPath),
               uploads[index].status == .failed,
               uploads[index].nextRetryAt == nil,
               uploads[index].lastError?.contains("no longer stored on the device") == true {
                uploads[index].status = .pending
                uploads[index].lastError = nil
                migratedLegacyPath = true
            }
        }

        if migratedLegacyPath {
            try saveUploads(uploads)
        }
        return uploads
    }

    private func saveUploads(_ uploads: [PendingEntryUpload]) throws {
        try save(uploads.map(portableUpload), to: uploadsFile)
    }

    private func portableUpload(_ upload: PendingEntryUpload) -> PendingEntryUpload {
        var portable = upload
        let rootPath = baseDirectory.standardizedFileURL.path
        let filePath = URL(fileURLWithPath: upload.localFilePath).standardizedFileURL.path
        let rootedPrefix = rootPath.hasSuffix("/") ? rootPath : "\(rootPath)/"
        if filePath.hasPrefix(rootedPrefix) {
            portable.localFilePath = String(filePath.dropFirst(rootedPrefix.count))
        }
        return portable
    }

    private func resolveUploadPath(_ upload: PendingEntryUpload) -> String {
        let storedPath = upload.localFilePath
        if !(storedPath as NSString).isAbsolutePath {
            let candidate = baseDirectory.appendingPathComponent(storedPath).standardizedFileURL
            let rootPath = baseDirectory.standardizedFileURL.path
            if candidate.path == rootPath || candidate.path.hasPrefix("\(rootPath)/") {
                return candidate.path
            }
        } else if FileManager.default.fileExists(atPath: storedPath) {
            return storedPath
        }

        let relocated = baseDirectory
            .appendingPathComponent("Recordings", isDirectory: true)
            .appendingPathComponent(upload.filename)
            .standardizedFileURL
        if FileManager.default.fileExists(atPath: relocated.path) {
            return relocated.path
        }
        return storedPath
    }

    private func save<T: Encodable>(_ value: T, to url: URL) throws {
        try FileManager.default.createDirectory(at: baseDirectory, withIntermediateDirectories: true, attributes: nil)
        let data = try encoder.encode(value)
        try data.write(to: url, options: .atomic)
    }

    private func load<T: Decodable>(_ type: T.Type, from url: URL, defaultValue: T) throws -> T {
        guard FileManager.default.fileExists(atPath: url.path) else {
            return defaultValue
        }
        let data = try Data(contentsOf: url)
        return try decoder.decode(T.self, from: data)
    }

    private static func defaultBaseDirectory() -> URL {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? URL(fileURLWithPath: NSTemporaryDirectory(), isDirectory: true)
        return base.appendingPathComponent("BlackwoodMobile", isDirectory: true)
    }

    private static func subpageKey(date: String, name: String) -> String {
        "\(date)|\(name)"
    }
}
