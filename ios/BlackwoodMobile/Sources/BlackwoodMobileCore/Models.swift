import Foundation

public struct APIAttachment: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let entryId: String
    public let filename: String
    public let contentType: String
    public let size: Int
    public let url: String

    enum CodingKeys: String, CodingKey {
        case id
        case entryId
        case filename
        case contentType
        case size
        case url
    }

    public init(
        id: String,
        entryId: String,
        filename: String,
        contentType: String,
        size: Int,
        url: String
    ) {
        self.id = id
        self.entryId = entryId
        self.filename = filename
        self.contentType = contentType
        self.size = size
        self.url = url
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        entryId = try container.decode(String.self, forKey: .entryId)
        filename = try container.decodeIfPresent(String.self, forKey: .filename) ?? ""
        contentType = try container.decodeIfPresent(String.self, forKey: .contentType) ?? ""
        if let intValue = try? container.decode(Int.self, forKey: .size) {
            size = intValue
        } else if let stringValue = try? container.decode(String.self, forKey: .size) {
            size = Int(stringValue) ?? 0
        } else {
            size = 0
        }
        url = try container.decodeIfPresent(String.self, forKey: .url) ?? ""
    }
}

public struct APIEntry: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let dailyNoteId: String
    public let type: Int
    public let content: String
    public let rawContent: String
    public let source: Int
    public let metadata: String
    public let attachments: [APIAttachment]
    public let createdAt: String
    public let updatedAt: String

    enum CodingKeys: String, CodingKey {
        case id
        case dailyNoteId
        case type
        case content
        case rawContent
        case source
        case metadata
        case attachments
        case createdAt
        case updatedAt
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        dailyNoteId = try container.decode(String.self, forKey: .dailyNoteId)
        type = try APIEntry.decodeEnumValue(from: container, forKey: .type, mapping: [
            "ENTRY_TYPE_UNSPECIFIED": 0,
            "ENTRY_TYPE_TEXT": 1,
            "ENTRY_TYPE_AUDIO": 2,
            "ENTRY_TYPE_PHOTO": 3,
            "ENTRY_TYPE_VIWOODS": 4,
            "ENTRY_TYPE_WEBCLIP": 5,
        ])
        content = try container.decodeIfPresent(String.self, forKey: .content) ?? ""
        rawContent = try container.decodeIfPresent(String.self, forKey: .rawContent) ?? content
        source = try APIEntry.decodeEnumValue(from: container, forKey: .source, mapping: [
            "ENTRY_SOURCE_UNSPECIFIED": 0,
            "ENTRY_SOURCE_WEB": 1,
            "ENTRY_SOURCE_TELEGRAM": 2,
            "ENTRY_SOURCE_WHATSAPP": 3,
            "ENTRY_SOURCE_API": 4,
            "ENTRY_SOURCE_IMPORT": 5,
        ])
        metadata = try container.decodeIfPresent(String.self, forKey: .metadata) ?? ""
        attachments = try container.decodeIfPresent([APIAttachment].self, forKey: .attachments) ?? []
        createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt) ?? ""
        updatedAt = try container.decodeIfPresent(String.self, forKey: .updatedAt) ?? ""
    }

    public init(
        id: String,
        dailyNoteId: String,
        type: Int,
        content: String,
        rawContent: String,
        source: Int,
        metadata: String,
        attachments: [APIAttachment],
        createdAt: String,
        updatedAt: String
    ) {
        self.id = id
        self.dailyNoteId = dailyNoteId
        self.type = type
        self.content = content
        self.rawContent = rawContent
        self.source = source
        self.metadata = metadata
        self.attachments = attachments
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    private static func decodeEnumValue(
        from container: KeyedDecodingContainer<CodingKeys>,
        forKey key: CodingKeys,
        mapping: [String: Int]
    ) throws -> Int {
        if let intValue = try? container.decode(Int.self, forKey: key) {
            return intValue
        }
        let stringValue = try container.decode(String.self, forKey: key)
        return mapping[stringValue] ?? 0
    }
}

public struct APIDailyNote: Codable, Equatable, Sendable {
    public let id: String
    public let date: String
    public let entries: [APIEntry]
    public let createdAt: String
    public let updatedAt: String
    public let content: String
    public let revision: String

    enum CodingKeys: String, CodingKey {
        case id
        case date
        case entries
        case createdAt
        case updatedAt
        case content
        case revision
    }

    public init(
        id: String,
        date: String,
        entries: [APIEntry],
        createdAt: String,
        updatedAt: String,
        content: String,
        revision: String
    ) {
        self.id = id
        self.date = date
        self.entries = entries
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.content = content
        self.revision = revision
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        date = try container.decode(String.self, forKey: .date)
        entries = try container.decodeIfPresent([APIEntry].self, forKey: .entries) ?? []
        createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt) ?? ""
        updatedAt = try container.decodeIfPresent(String.self, forKey: .updatedAt) ?? ""
        content = try container.decodeIfPresent(String.self, forKey: .content) ?? ""
        revision = try container.decodeIfPresent(String.self, forKey: .revision) ?? ""
    }
}

public struct APISubpage: Codable, Equatable, Sendable {
    public let name: String
    public let content: String
    public let date: String
    public let revision: String
    public let updatedAt: String

    enum CodingKeys: String, CodingKey {
        case name
        case content
        case date
        case revision
        case updatedAt
    }

    public init(name: String, content: String, date: String, revision: String, updatedAt: String) {
        self.name = name
        self.content = content
        self.date = date
        self.revision = revision
        self.updatedAt = updatedAt
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        name = try container.decode(String.self, forKey: .name)
        content = try container.decodeIfPresent(String.self, forKey: .content) ?? ""
        date = try container.decode(String.self, forKey: .date)
        revision = try container.decodeIfPresent(String.self, forKey: .revision) ?? ""
        updatedAt = try container.decodeIfPresent(String.self, forKey: .updatedAt) ?? ""
    }
}

public struct SearchResult: Codable, Equatable, Sendable, Identifiable {
    public let entryId: String
    public let date: String
    public let snippet: String
    public let score: Double

    public var id: String { entryId + "|" + date }
}

public struct SearchResponse: Codable, Equatable, Sendable {
    public let results: [SearchResult]
}

public struct CachedDailyNote: Codable, Equatable, Sendable {
    public let date: String
    public var content: String
    public var updatedAt: Date
    public var revision: String

    enum CodingKeys: String, CodingKey {
        case date
        case content
        case updatedAt
        case revision
    }

    public init(date: String, content: String, updatedAt: Date, revision: String) {
        self.date = date
        self.content = content
        self.updatedAt = updatedAt
        self.revision = revision
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        date = try container.decode(String.self, forKey: .date)
        content = try container.decode(String.self, forKey: .content)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        revision = try container.decodeIfPresent(String.self, forKey: .revision) ?? ""
    }
}

public struct CachedSubpage: Codable, Equatable, Sendable {
    public let date: String
    public let name: String
    public var content: String
    public var updatedAt: Date
    public var revision: String

    enum CodingKeys: String, CodingKey {
        case date
        case name
        case content
        case updatedAt
        case revision
    }

    public init(date: String, name: String, content: String, updatedAt: Date, revision: String) {
        self.date = date
        self.name = name
        self.content = content
        self.updatedAt = updatedAt
        self.revision = revision
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        date = try container.decode(String.self, forKey: .date)
        name = try container.decode(String.self, forKey: .name)
        content = try container.decode(String.self, forKey: .content)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        revision = try container.decodeIfPresent(String.self, forKey: .revision) ?? ""
    }
}

public struct PendingNoteUpdate: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let date: String
    public var content: String
    public var updatedAt: Date
    public var baseRevision: String
    public var baseContent: String

    public init(
        id: String = UUID().uuidString,
        date: String,
        content: String,
        updatedAt: Date = Date(),
        baseRevision: String,
        baseContent: String = ""
    ) {
        self.id = id
        self.date = date
        self.content = content
        self.updatedAt = updatedAt
        self.baseRevision = baseRevision
        self.baseContent = baseContent
    }

    enum CodingKeys: String, CodingKey {
        case id
        case date
        case content
        case updatedAt
        case baseRevision
        case baseContent
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        date = try container.decode(String.self, forKey: .date)
        content = try container.decode(String.self, forKey: .content)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        baseRevision = try container.decodeIfPresent(String.self, forKey: .baseRevision) ?? ""
        baseContent = try container.decodeIfPresent(String.self, forKey: .baseContent) ?? ""
    }
}

public struct PendingSubpageUpdate: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let date: String
    public let name: String
    public var content: String
    public var updatedAt: Date
    public var baseRevision: String
    public var baseContent: String

    public init(
        id: String = UUID().uuidString,
        date: String,
        name: String,
        content: String,
        updatedAt: Date = Date(),
        baseRevision: String,
        baseContent: String = ""
    ) {
        self.id = id
        self.date = date
        self.name = name
        self.content = content
        self.updatedAt = updatedAt
        self.baseRevision = baseRevision
        self.baseContent = baseContent
    }

    enum CodingKeys: String, CodingKey {
        case id
        case date
        case name
        case content
        case updatedAt
        case baseRevision
        case baseContent
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        date = try container.decode(String.self, forKey: .date)
        name = try container.decode(String.self, forKey: .name)
        content = try container.decode(String.self, forKey: .content)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        baseRevision = try container.decodeIfPresent(String.self, forKey: .baseRevision) ?? ""
        baseContent = try container.decodeIfPresent(String.self, forKey: .baseContent) ?? ""
    }
}

public struct APIChangeEvent: Codable, Equatable, Sendable {
    public let kind: String
    public let date: String
    public let subpageName: String
    public let revision: String
    public let changedAt: String
}

public enum PendingUploadStatus: String, Codable, Equatable, Sendable {
    case pending
    case uploading
    case failed
}

public struct PendingEntryUpload: Codable, Equatable, Sendable, Identifiable {
    public let id: String
    public let clientRequestId: String
    public let date: String
    public let content: String
    public let source: Int
    public let type: Int
    public let localFilePath: String
    public let filename: String
    public let contentType: String
    public let createdAt: Date
    public let capturedAt: Date
    public let duration: TimeInterval
    public var status: PendingUploadStatus
    public var attemptCount: Int
    public var lastError: String?
    public var nextRetryAt: Date?

    public init(
        id: String = UUID().uuidString,
        clientRequestId: String = UUID().uuidString,
        date: String,
        content: String = "",
        source: Int = 4,
        type: Int = 2,
        localFilePath: String,
        filename: String = "recording.m4a",
        contentType: String = "audio/x-m4a",
        createdAt: Date = Date(),
        capturedAt: Date = Date(),
        duration: TimeInterval,
        status: PendingUploadStatus = .pending,
        attemptCount: Int = 0,
        lastError: String? = nil,
        nextRetryAt: Date? = nil
    ) {
        self.id = id
        self.clientRequestId = clientRequestId
        self.date = date
        self.content = content
        self.source = source
        self.type = type
        self.localFilePath = localFilePath
        self.filename = filename
        self.contentType = contentType
        self.createdAt = createdAt
        self.capturedAt = capturedAt
        self.duration = duration
        self.status = status
        self.attemptCount = attemptCount
        self.lastError = lastError
        self.nextRetryAt = nextRetryAt
    }
}

public struct QueueSnapshot: Equatable, Sendable {
    public let noteUpdateCount: Int
    public let subpageUpdateCount: Int
    public let uploadCount: Int
    public let failedUploadCount: Int

    public var totalNoteUpdateCount: Int {
        noteUpdateCount + subpageUpdateCount
    }
}

public enum SyncFailureDisposition: Equatable, Sendable {
    case retryable
    case terminal
}

public struct SyncFailure: Error, Equatable, Sendable {
    public let message: String
    public let disposition: SyncFailureDisposition
    public let code: String?

    public init(message: String, disposition: SyncFailureDisposition, code: String? = nil) {
        self.message = message
        self.disposition = disposition
        self.code = code
    }
}

public enum MarkdownStorage {
    private static let blockStateMarker = "<!-- blackwood:block-state:v1\n"
    private static let blockStateEnd = "\n-->"

    public static func visibleMarkdown(from content: String) -> String {
        guard let markerRange = content.range(of: blockStateMarker, options: .backwards) else {
            return content.trimmingCharacters(in: .whitespacesAndNewlines)
        }

        let trailerStart = markerRange.upperBound
        guard let endRange = content[trailerStart...].range(of: blockStateEnd) else {
            return content.trimmingCharacters(in: .whitespacesAndNewlines)
        }

        let trailing = content[endRange.upperBound...].trimmingCharacters(in: .whitespacesAndNewlines)
        guard trailing.isEmpty else {
            return content.trimmingCharacters(in: .whitespacesAndNewlines)
        }

        return content[..<markerRange.lowerBound].trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

public struct MarkdownMergeResult: Equatable, Sendable {
    public let merged: String?
    public let ok: Bool
    public let conflicts: [String]
}

public enum MarkdownMerge {
    private struct Section {
        let heading: String
        let text: String
    }

    public static func merge(base: String, local: String, remote: String) -> MarkdownMergeResult {
        if base == local {
            return MarkdownMergeResult(merged: remote, ok: true, conflicts: [])
        }
        if base == remote || local == remote {
            return MarkdownMergeResult(merged: local, ok: true, conflicts: [])
        }

        guard hasTopLevelHeading(base) else {
            return mergeAppend(base: base, local: local, remote: remote)
        }

        let baseSections = splitSections(base)
        let localSections = splitSections(local)
        let remoteSections = splitSections(remote)
        let localMap = sectionMap(localSections)
        let remoteMap = sectionMap(remoteSections)
        let baseMap = sectionMap(baseSections)

        var headings: [String] = []
        var seen = Set<String>()
        func appendHeading(_ heading: String) {
            guard !seen.contains(heading) else { return }
            seen.insert(heading)
            headings.append(heading)
        }

        baseSections.forEach { appendHeading($0.heading) }
        remoteSections.forEach { appendHeading($0.heading) }
        localSections.forEach { appendHeading($0.heading) }

        var mergedSections: [String] = []
        var conflicts: [String] = []

        for heading in headings {
            let baseText = baseMap[heading]
            let localText = localMap[heading]
            let remoteText = remoteMap[heading]
            let localChanged = localText != baseText
            let remoteChanged = remoteText != baseText

            if !localChanged && !remoteChanged {
                mergedSections.append(baseText ?? "")
            } else if localChanged && !remoteChanged {
                if let localText {
                    mergedSections.append(localText)
                }
            } else if !localChanged && remoteChanged {
                if let remoteText {
                    mergedSections.append(remoteText)
                }
            } else if localText == remoteText {
                mergedSections.append(localText ?? "")
            } else {
                conflicts.append(heading.isEmpty ? "(preamble)" : heading)
                mergedSections.append(localText ?? remoteText ?? "")
            }
        }

        if !conflicts.isEmpty {
            return MarkdownMergeResult(merged: nil, ok: false, conflicts: conflicts)
        }

        return MarkdownMergeResult(merged: mergedSections.joined(separator: "\n"), ok: true, conflicts: [])
    }

    public static func preservingBothSides(local: String, remote: String) -> String {
        let trimmedRemote = remote.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedLocal = local.trimmingCharacters(in: .whitespacesAndNewlines)

        if trimmedRemote.isEmpty {
            return trimmedLocal
        }
        if trimmedLocal.isEmpty || trimmedLocal == trimmedRemote {
            return trimmedRemote
        }

        return "\(trimmedRemote)\n\n# Offline edits from iOS\n\n\(trimmedLocal)"
    }

    private static func hasTopLevelHeading(_ content: String) -> Bool {
        content
            .components(separatedBy: .newlines)
            .contains { $0.hasPrefix("# ") }
    }

    private static func splitSections(_ content: String) -> [Section] {
        let lines = content.components(separatedBy: .newlines)
        var sections: [Section] = []
        var currentHeading = ""
        var currentLines: [String] = []

        for line in lines {
            if line.hasPrefix("# ") {
                sections.append(Section(heading: currentHeading, text: currentLines.joined(separator: "\n")))
                currentHeading = line
                currentLines = [line]
            } else {
                currentLines.append(line)
            }
        }

        sections.append(Section(heading: currentHeading, text: currentLines.joined(separator: "\n")))
        return sections
    }

    private static func sectionMap(_ sections: [Section]) -> [String: String] {
        var map: [String: String] = [:]
        for section in sections {
            map[section.heading] = section.text
        }
        return map
    }

    private static func mergeAppend(base: String, local: String, remote: String) -> MarkdownMergeResult {
        if let remoteAppended = extractAppend(prefix: base, text: remote) {
            return MarkdownMergeResult(merged: local + remoteAppended, ok: true, conflicts: [])
        }
        if let localAppended = extractAppend(prefix: base, text: local) {
            return MarkdownMergeResult(merged: remote + localAppended, ok: true, conflicts: [])
        }
        return MarkdownMergeResult(merged: nil, ok: false, conflicts: ["(entire note)"])
    }

    private static func extractAppend(prefix: String, text: String) -> String? {
        guard text.hasPrefix(prefix) else {
            return nil
        }
        let rest = String(text.dropFirst(prefix.count))
        if rest.isEmpty || rest.hasPrefix("\n") {
            return rest
        }
        return nil
    }
}
