import Foundation

public actor QueueStore {
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()
    private let baseDirectory: URL
    private let cacheFile: URL
    private let noteUpdatesFile: URL
    private let uploadsFile: URL

    public init(baseDirectory: URL? = nil) {
        let root = baseDirectory ?? QueueStore.defaultBaseDirectory()
        self.baseDirectory = root
        self.cacheFile = root.appendingPathComponent("daily-note-cache.json")
        self.noteUpdatesFile = root.appendingPathComponent("pending-note-updates.json")
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

    public func queueNoteUpdate(date: String, content: String, updatedAt: Date = Date(), baseRevision: String) async throws {
        var updates = try loadNoteUpdates()
        if let existingIndex = updates.firstIndex(where: { $0.date == date }) {
            updates[existingIndex].content = content
            updates[existingIndex].updatedAt = updatedAt
            if updates[existingIndex].baseRevision.isEmpty {
                updates[existingIndex].baseRevision = baseRevision
            }
        } else {
            updates.append(PendingNoteUpdate(date: date, content: content, updatedAt: updatedAt, baseRevision: baseRevision))
        }
        updates.sort { $0.updatedAt < $1.updatedAt }
        try save(updates, to: noteUpdatesFile)
    }

    public func pendingNoteUpdates() async throws -> [PendingNoteUpdate] {
        try loadNoteUpdates()
    }

    public func removeNoteUpdate(id: String) async throws {
        var updates = try loadNoteUpdates()
        updates.removeAll { $0.id == id }
        try save(updates, to: noteUpdatesFile)
    }

    public func queueAudioUpload(_ upload: PendingEntryUpload) async throws {
        var uploads = try loadUploads()
        uploads.append(upload)
        uploads.sort { $0.createdAt < $1.createdAt }
        try save(uploads, to: uploadsFile)
    }

    public func pendingUploads() async throws -> [PendingEntryUpload] {
        try loadUploads()
    }

    public func updateUpload(_ upload: PendingEntryUpload) async throws {
        var uploads = try loadUploads()
        if let index = uploads.firstIndex(where: { $0.id == upload.id }) {
            uploads[index] = upload
            try save(uploads, to: uploadsFile)
        }
    }

    public func removeUpload(id: String, deleteLocalFile: Bool) async throws {
        var uploads = try loadUploads()
        guard let index = uploads.firstIndex(where: { $0.id == id }) else { return }
        let removed = uploads.remove(at: index)
        try save(uploads, to: uploadsFile)
        if deleteLocalFile {
            try? FileManager.default.removeItem(atPath: removed.localFilePath)
        }
    }

    public func snapshot() async throws -> QueueSnapshot {
        let updates = try loadNoteUpdates()
        let uploads = try loadUploads()
        let failed = uploads.filter { $0.status == .failed }.count
        return QueueSnapshot(noteUpdateCount: updates.count, uploadCount: uploads.count, failedUploadCount: failed)
    }

    private func loadCache() throws -> [String: CachedDailyNote] {
        try load([String: CachedDailyNote].self, from: cacheFile, defaultValue: [:])
    }

    private func loadNoteUpdates() throws -> [PendingNoteUpdate] {
        try load([PendingNoteUpdate].self, from: noteUpdatesFile, defaultValue: [])
    }

    private func loadUploads() throws -> [PendingEntryUpload] {
        try load([PendingEntryUpload].self, from: uploadsFile, defaultValue: [])
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
}
